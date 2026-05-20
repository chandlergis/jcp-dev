package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
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

	// 统计缓存命中
	cacheHit := 0
	cacheMiss := 0

	// 创建K线数据获取函数（带缓存）
	fetcher := func(symbol string) ([]models.KLineData, error) {
		// 先尝试从本地缓存获取
		if cached, ok := s.klineCache.Get(symbol, "1d"); ok && len(cached) > 0 {
			cacheHit++
			// 过滤掉价格为0的异常数据
			valid := make([]models.KLineData, 0, len(cached))
			for _, k := range cached {
				if k.Close > 0 && k.Open > 0 {
					valid = append(valid, k)
				}
			}
			return valid, nil
		}

		cacheMiss++
		// 从网络获取 - 获取250天数据以满足策略需求
		klines, err := s.marketSvc.GetKLineData(symbol, "1d", 250)
		if err != nil {
			return nil, err
		}

		// 过滤掉价格为0的异常数据
		valid := make([]models.KLineData, 0, len(klines))
		for _, k := range klines {
			if k.Close > 0 && k.Open > 0 {
				valid = append(valid, k)
			}
		}

		// 保存到本地缓存
		if len(valid) > 0 {
			s.klineCache.Set(symbol, "1d", valid)
		}

		return valid, nil
	}

	// 执行选股 - 并发数10
	start := time.Now()
	results := s.manager.RunStrategy(strategyID, filtered, fetcher, priceMin, priceMax, 10, progressCallback, s.cancelChan, s.predictor)
	elapsed := time.Since(start)

	// 保存缓存到文件
	if err := s.klineCache.SaveNow(); err != nil {
		selectorLog.Error("保存K线缓存失败: %v", err)
	}

	selectorLog.Info("选股完成: strategy=%s, 耗时=%v, 结果=%d只, 缓存命中=%d, 缓存未命中=%d", strategyID, elapsed, len(results), cacheHit, cacheMiss)

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
