package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/run-bigpig/jcp/internal/logger"
	"github.com/run-bigpig/jcp/internal/models"
	"github.com/run-bigpig/jcp/internal/selector"
)

var selectorLog = logger.New("selector_service")

// SelectorService 选股服务
type SelectorService struct {
	dataDir    string
	manager    *selector.Manager
	marketSvc  *MarketService
	predictor  *PredictionService
	klineCache *KLineCacheService
	mu         sync.RWMutex
	store      *models.SelectorRecordsStore
	cancelChan chan struct{} // 取消信号通道
	isRunning  bool          // 是否正在运行
}

// NewSelectorService 创建选股服务
func NewSelectorService(dataDir string, marketSvc *MarketService) *SelectorService {
	s := &SelectorService{
		dataDir:    dataDir,
		manager:    selector.NewManager(),
		marketSvc:  marketSvc,
		predictor:  NewPredictionService(dataDir),
		klineCache: NewKLineCacheService(dataDir),
		store:      &models.SelectorRecordsStore{Records: []models.SelectorRecord{}},
		cancelChan: make(chan struct{}),
	}
	s.load()

	// 初始化预测模型（优先从文件加载，否则后台训练）
	s.predictor.Init(marketSvc)

	return s
}

// load 加载选股记录
func (s *SelectorService) load() {
	filePath := filepath.Join(s.dataDir, "selector_records.json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			selectorLog.Error("加载选股记录失败: %v", err)
		}
		return
	}

	if err := json.Unmarshal(data, s.store); err != nil {
		selectorLog.Error("解析选股记录失败: %v", err)
	}
}

// save 保存选股记录
func (s *SelectorService) save() error {
	filePath := filepath.Join(s.dataDir, "selector_records.json")
	data, err := json.MarshalIndent(s.store, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, data, 0644)
}

// GetStrategyList 获取策略列表
func (s *SelectorService) GetStrategyList() []models.SelectorStrategyInfo {
	return models.GetStrategyList()
}

// CancelSelector 取消选股
func (s *SelectorService) CancelSelector() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isRunning {
		close(s.cancelChan)
		s.cancelChan = make(chan struct{})
		selectorLog.Info("已发送取消信号")
	}
}

// IsRunning 是否正在运行
func (s *SelectorService) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.isRunning
}

// RunSelector 执行选股（带进度回调）
func (s *SelectorService) RunSelector(strategyID models.SelectorStrategy, priceMin, priceMax float64, progressCallback selector.ProgressCallback) *models.SelectorResult {
	// 检查是否已在运行
	s.mu.Lock()
	if s.isRunning {
		s.mu.Unlock()
		selectorLog.Warn("选股已在运行中")
		return nil
	}
	s.isRunning = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.isRunning = false
		s.mu.Unlock()
	}()

	selectorLog.Info("开始执行选股: strategy=%s, priceMin=%.2f, priceMax=%.2f", strategyID, priceMin, priceMax)

	// 获取所有股票
	allStocks := s.manager.GetAllStocks()
	selectorLog.Info("获取到 %d 只股票", len(allStocks))

	// 预过滤
	filtered := s.manager.PreFilterStocks(allStocks)
	selectorLog.Info("预过滤后剩余 %d 只股票", len(filtered))

	// 统计缓存使用情况
	var (
		cacheFresh     int64 // 完全命中（最快路径）
		cacheNeedQuote int64 // 历史命中、需要补当日行情
		cacheStale     int64 // 完全未命中、需要拉取历史
	)

	// 提前获取市场状态和最近交易日（避免每次都计算）
	marketStatus := s.marketSvc.GetMarketStatus().Status
	lastTradeDate := s.marketSvc.GetLastTradeDate()
	selectorLog.Info("市场状态=%s, 最近交易日=%s", marketStatus, lastTradeDate)

	// 创建K线数据获取函数（智能缓存策略）
	fetcher := func(symbol string) ([]models.KLineData, error) {
		freshness, cached := s.klineCache.CheckFreshness(symbol, "1d", lastTradeDate, marketStatus)

		var klines []models.KLineData
		needSaveCache := false

		switch freshness {
		case CacheFresh:
			// 最快路径：历史数据完整且当日不需要更新（休市/盘前/已收盘且当日K线已存在）
			atomic.AddInt64(&cacheFresh, 1)
			klines = cached

		case CacheNeedQuote:
			// 历史数据可用，补充/校准当日行情（不重拉历史）
			atomic.AddInt64(&cacheNeedQuote, 1)
			klines = cached
			klines = s.mergeRealtimeQuote(symbol, klines)
			needSaveCache = true // mergeRealtimeQuote 会追加或更新收盘价，总是需要写回

		case CacheStale:
			// 完全未命中：拉取完整历史
			atomic.AddInt64(&cacheStale, 1)
			fresh, err := s.marketSvc.GetKLineData(symbol, "1d", 250)
			if err != nil {
				if len(cached) > 0 {
					klines = cached
				} else {
					return nil, err
				}
			} else {
				klines = fresh
			}
			// 校准当日收盘价
			klines = s.mergeRealtimeQuote(symbol, klines)
			needSaveCache = true
		}

		// 过滤掉价格为0的异常数据
		valid := make([]models.KLineData, 0, len(klines))
		for _, k := range klines {
			if k.Close > 0 && k.Open > 0 {
				valid = append(valid, k)
			}
		}
		klines = valid

		// 写回缓存（只在真正有变化时写）
		if needSaveCache && len(klines) > 0 {
			s.klineCache.Set(symbol, "1d", klines)
		}

		return klines, nil
	}

	// 执行选股 - 并发数10
	start := time.Now()
	results := s.manager.RunStrategy(strategyID, filtered, fetcher, priceMin, priceMax, 10, progressCallback, s.cancelChan, s.predictor)
	elapsed := time.Since(start)

	// 保存缓存到文件
	if err := s.klineCache.SaveNow(); err != nil {
		selectorLog.Error("保存K线缓存失败: %v", err)
	}

	selectorLog.Info("选股完成: strategy=%s, 耗时=%v, 结果=%d只 | 缓存:完全命中=%d, 补当日行情=%d, 完整拉取=%d",
		strategyID, elapsed, len(results),
		atomic.LoadInt64(&cacheFresh),
		atomic.LoadInt64(&cacheNeedQuote),
		atomic.LoadInt64(&cacheStale))

	// 获取策略名称
	strategyName := string(strategyID)
	strategy := s.manager.GetStrategy(strategyID)
	if strategy != nil {
		strategyName = strategy.Name()
	}

	return &models.SelectorResult{
		Strategy:     strategyID,
		StrategyName: strategyName,
		Date:         time.Now().Format("2006-01-02"),
		Stocks:       results,
		Total:        len(results),
		Params: models.SelectorFilterParams{
			PriceMin: priceMin,
			PriceMax: priceMax,
		},
	}
}

// SaveSelectorRecord 保存选股记录
func (s *SelectorService) SaveSelectorRecord(result *models.SelectorResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	record := models.SelectorRecord{
		Date:         result.Date,
		Strategy:     result.Strategy,
		StrategyName: result.StrategyName,
		Stocks:       result.Stocks,
		Params:       result.Params,
		ExecutedAt:   time.Now().Format(time.RFC3339),
	}

	s.store.Records = append(s.store.Records, record)

	if err := s.save(); err != nil {
		selectorLog.Error("保存选股记录失败: %v", err)
		return err
	}

	selectorLog.Info("保存选股记录成功: date=%s, strategy=%s, stocks=%d", record.Date, record.Strategy, len(record.Stocks))
	return nil
}

// GetSelectorRecords 获取所有选股记录
func (s *SelectorService) GetSelectorRecords() []models.SelectorRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 返回副本，按时间倒序
	records := make([]models.SelectorRecord, len(s.store.Records))
	copy(records, s.store.Records)

	// 按时间倒序排列
	for i := 0; i < len(records)/2; i++ {
		j := len(records) - 1 - i
		records[i], records[j] = records[j], records[i]
	}

	return records
}

// GetSelectorRecordsByDate 按日期获取选股记录
func (s *SelectorService) GetSelectorRecordsByDate(date string) []models.SelectorRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var records []models.SelectorRecord
	for _, r := range s.store.Records {
		if r.Date == date {
			records = append(records, r)
		}
	}
	return records
}

// DeleteSelectorRecord 删除选股记录
func (s *SelectorService) DeleteSelectorRecord(date string, strategy models.SelectorStrategy) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var newRecords []models.SelectorRecord
	for _, r := range s.store.Records {
		if r.Date != date || r.Strategy != strategy {
			newRecords = append(newRecords, r)
		}
	}
	s.store.Records = newRecords

	return s.save()
}

// MarkStocksAddedToWatchlist 标记股票已添加到自选股
func (s *SelectorService) MarkStocksAddedToWatchlist(date string, strategy models.SelectorStrategy, symbols []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.store.Records {
		if s.store.Records[i].Date == date && s.store.Records[i].Strategy == strategy {
			s.store.Records[i].AddedToWatchlist = symbols
			break
		}
	}

	return s.save()
}

// GetCacheStats 获取缓存统计
func (s *SelectorService) GetCacheStats() (total int, todayCount int) {
	return s.klineCache.GetCacheStats()
}

// ClearExpiredCache 清理过期缓存
func (s *SelectorService) ClearExpiredCache(days int) int {
	return s.klineCache.ClearExpired(days)
}

// GetPredictionService 获取预测服务（供训练营使用）
func (s *SelectorService) GetPredictionService() *PredictionService {
	return s.predictor
}

// getTrainStockCodes 从候选股票中选取训练用的股票代码
func (s *SelectorService) getTrainStockCodes(stocks []selector.StockBasicInfo, maxCount int) []string {
	if len(stocks) <= maxCount {
		codes := make([]string, len(stocks))
		for i, st := range stocks {
			codes[i] = st.Symbol
		}
		return codes
	}
	codes := make([]string, 0, maxCount)
	step := len(stocks) / maxCount
	for i := 0; i < len(stocks) && len(codes) < maxCount; i += step {
		codes = append(codes, stocks[i].Symbol)
	}
	return codes
}

// mergeRealtimeQuote 用实时行情补充/更新当日K线
// 场景1: 缓存里没有今天K线 → 追加
// 场景2: 缓存里有今天K线但价格是盘中的 → 用收盘价更新
func (s *SelectorService) mergeRealtimeQuote(symbol string, klines []models.KLineData) []models.KLineData {
	if len(klines) == 0 {
		return klines
	}

	lastTradeDate := s.marketSvc.GetLastTradeDate()
	lastKline := klines[len(klines)-1]

	// 场景1: 最后一根K线不是最近交易日 → 需要追加
	if lastKline.Time != lastTradeDate {
		// 获取实时行情
		quotes, err := s.marketSvc.GetStockDataWithOrderBook(symbol)
		if err != nil || len(quotes) == 0 {
			return klines
		}
		q := quotes[0]
		if q.Price <= 0 || q.Open <= 0 {
			return klines
		}

		klines = append(klines, models.KLineData{
			Time:   lastTradeDate,
			Open:   q.Open,
			High:   q.High,
			Low:    q.Low,
			Close:  q.Price,
			Volume: q.Volume,
			Amount: q.Amount,
		})
		return klines
	}

	// 场景2: 最后一根K线是最近交易日 → 用实时行情校准收盘价
	// (TDX日K在盘中可能是中间价，收盘后应更新为最终收盘价)
	quotes, err := s.marketSvc.GetStockDataWithOrderBook(symbol)
	if err != nil || len(quotes) == 0 {
		return klines
	}
	q := quotes[0]
	if q.Price <= 0 {
		return klines
	}

	// 只在收盘价有差异时更新（避免无意义的写入）
	idx := len(klines) - 1
	old := klines[idx]
	priceDiff := q.Price - old.Close
	if priceDiff > 0.001 || priceDiff < -0.001 {
		klines[idx] = models.KLineData{
			Time:   lastTradeDate,
			Open:   q.Open,
			High:   q.High,
			Low:    q.Low,
			Close:  q.Price,
			Volume: q.Volume,
			Amount: q.Amount,
		}
	}

	return klines
}
