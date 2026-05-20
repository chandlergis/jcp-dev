package selector

import (
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/run-bigpig/jcp/internal/embed"
	"github.com/run-bigpig/jcp/internal/logger"
	"github.com/run-bigpig/jcp/internal/models"
)

var log = logger.New("selector")

// StockBasicInfo 股票基本信息
type StockBasicInfo struct {
	Symbol   string
	Name     string
	Industry string
	Market   string
}

// ProgressInfo 选股进度信息
type ProgressInfo struct {
	Total     int               `json:"total"`     // 总数
	Processed int               `json:"processed"` // 已处理数
	Found     int               `json:"found"`     // 已找到数
	Current   string            `json:"current"`   // 当前处理的股票
	Results   []models.SelectorStock `json:"results"` // 已找到的股票列表
	Done      bool              `json:"done"`      // 是否完成
	Cancelled bool              `json:"cancelled"` // 是否已取消
}

// ProgressCallback 进度回调函数
type ProgressCallback func(progress ProgressInfo)

// Manager 选股管理器
type Manager struct {
	strategies map[models.SelectorStrategy]Strategy
}

// NewManager 创建选股管理器
func NewManager() *Manager {
	m := &Manager{
		strategies: make(map[models.SelectorStrategy]Strategy),
	}
	m.registerStrategies()
	return m
}

// registerStrategies 注册所有策略
func (m *Manager) registerStrategies() {
	m.strategies[models.StrategyBBIKDJ] = NewBBIKDJSelector()
	m.strategies[models.StrategySuperB1] = NewSuperB1Selector()
	m.strategies[models.StrategyPullback] = NewBBIPullbackSelector()
}

// GetStrategy 获取策略
func (m *Manager) GetStrategy(strategy models.SelectorStrategy) Strategy {
	return m.strategies[strategy]
}

// GetAllStocks 获取所有股票基本信息
func (m *Manager) GetAllStocks() []StockBasicInfo {
	var basicData struct {
		Data struct {
			Fields []string        `json:"fields"`
			Items  [][]interface{} `json:"items"`
		} `json:"data"`
	}

	if err := json.Unmarshal(embed.StockBasicJSON, &basicData); err != nil {
		log.Error("解析股票基础数据失败: %v", err)
		return nil
	}

	var symbolIdx, nameIdx, industryIdx, tsCodeIdx int = -1, -1, -1, -1
	for i, field := range basicData.Data.Fields {
		switch field {
		case "symbol":
			symbolIdx = i
		case "name":
			nameIdx = i
		case "industry":
			industryIdx = i
		case "ts_code":
			tsCodeIdx = i
		}
	}

	if symbolIdx < 0 || nameIdx < 0 {
		return nil
	}

	stocks := make([]StockBasicInfo, 0, len(basicData.Data.Items))
	for _, item := range basicData.Data.Items {
		symbol, _ := item[symbolIdx].(string)
		name, _ := item[nameIdx].(string)

		industry := ""
		if industryIdx >= 0 && industryIdx < len(item) {
			industry, _ = item[industryIdx].(string)
		}

		market := ""
		fullSymbol := symbol
		if tsCodeIdx >= 0 && tsCodeIdx < len(item) {
			tsCode, _ := item[tsCodeIdx].(string)
			switch {
			case strings.HasSuffix(tsCode, ".SH"):
				market = "上海"
				fullSymbol = "sh" + symbol
			case strings.HasSuffix(tsCode, ".SZ"):
				market = "深圳"
				fullSymbol = "sz" + symbol
			case strings.HasSuffix(tsCode, ".BJ"):
				market = "北京"
				fullSymbol = "bj" + symbol
			}
		}

		stocks = append(stocks, StockBasicInfo{
			Symbol:   fullSymbol,
			Name:     name,
			Industry: industry,
			Market:   market,
		})
	}

	return stocks
}

// PreFilterStocks 预过滤股票
func (m *Manager) PreFilterStocks(stocks []StockBasicInfo) []StockBasicInfo {
	filtered := make([]StockBasicInfo, 0, len(stocks))
	for _, stock := range stocks {
		// 去掉ST股票
		if strings.Contains(stock.Name, "ST") {
			continue
		}
		// 去掉退市股
		if strings.Contains(stock.Name, "退") {
			continue
		}
		// 去掉北交所（流动性差）
		if strings.HasPrefix(stock.Symbol, "bj") {
			continue
		}
		filtered = append(filtered, stock)
	}
	return filtered
}

// RunStrategy 并发执行选股策略（带进度回调和取消支持）
type KLineDataFetcher func(symbol string) ([]models.KLineData, error)

// StockPredictor 股票预测接口
type StockPredictor interface {
	Predict(klines []models.KLineData) *models.PredictionResult
	IsTrained() bool
}

// PredictionResult 预测结果（从models层引用）
type PredictionResult = models.PredictionResult

func (m *Manager) RunStrategy(
	strategyID models.SelectorStrategy,
	stocks []StockBasicInfo,
	fetcher KLineDataFetcher,
	priceMin, priceMax float64,
	maxConcurrency int,
	progressCallback ProgressCallback,
	cancelChan <-chan struct{},
	predictor StockPredictor,
) []models.SelectorStock {
	strategy := m.strategies[strategyID]
	if strategy == nil {
		log.Error("未知策略: %s", strategyID)
		return nil
	}

	if maxConcurrency <= 0 {
		maxConcurrency = 10 // 降低默认并发数
	}

	total := len(stocks)
	var processed int64
	var cancelled int32
	var mu sync.Mutex
	var wg sync.WaitGroup
	results := make([]models.SelectorStock, 0)
	sem := make(chan struct{}, maxConcurrency)

	// 监听取消信号
	go func() {
		<-cancelChan
		atomic.StoreInt32(&cancelled, 1)
	}()

	for _, stock := range stocks {
		// 检查是否已取消
		if atomic.LoadInt32(&cancelled) > 0 {
			log.Info("选股已被取消")
			break
		}

		wg.Add(1)
		go func(s StockBasicInfo) {
			defer wg.Done()

			// 检查取消
			if atomic.LoadInt32(&cancelled) > 0 {
				return
			}

			sem <- struct{}{}
			defer func() { <-sem }()

			// 再次检查取消
			if atomic.LoadInt32(&cancelled) > 0 {
				return
			}

			// 更新进度
			currentProcessed := int(atomic.AddInt64(&processed, 1))
			if progressCallback != nil && currentProcessed%10 == 0 {
				mu.Lock()
				progressCallback(ProgressInfo{
					Total:     total,
					Processed: currentProcessed,
					Found:     len(results),
					Current:   s.Name,
					Results:   results,
				})
				mu.Unlock()
			}

			// 获取K线数据
			klines, err := fetcher(s.Symbol)
			if err != nil {
				if currentProcessed%100 == 0 {
					log.Debug("获取K线数据失败: %s, err=%v", s.Symbol, err)
				}
				return
			}
			if len(klines) == 0 {
				return
			}

			// 过滤掉价格为0的K线（盘前或异常数据）
			validKlines := make([]models.KLineData, 0, len(klines))
			for _, k := range klines {
				if k.Close > 0 && k.Open > 0 {
					validKlines = append(validKlines, k)
				}
			}
			if len(validKlines) == 0 {
				return
			}
			klines = validKlines

			// 检查数据量是否足够
			if len(klines) < 30 {
				if currentProcessed%500 == 0 {
					log.Debug("K线数据不足: %s, len=%d", s.Symbol, len(klines))
				}
				return
			}

			// 股价过滤
			lastPrice := klines[len(klines)-1].Close
			if lastPrice < priceMin || lastPrice > priceMax {
				return
			}

			// 停牌检测（最近3天有成交量即可）
			isHalted := true
			for i := len(klines) - 1; i >= 0 && i >= len(klines)-3; i-- {
				if klines[i].Volume > 0 {
					isHalted = false
					break
				}
			}
			if isHalted {
				return
			}

			// 过滤掉除权除息导致的价格跳变（单日涨跌超过20%的K线）
			cleanedKlines := make([]models.KLineData, 0, len(klines))
			cleanedKlines = append(cleanedKlines, klines[0])
			for i := 1; i < len(klines); i++ {
				prevClose := klines[i-1].Close
				currClose := klines[i].Close
				if prevClose > 0 {
					changePct := (currClose - prevClose) / prevClose * 100
					// 如果涨跌幅超过20%，可能是除权除息，跳过前一根K线
					if changePct > 20 || changePct < -20 {
						cleanedKlines = []models.KLineData{klines[i]}
						continue
					}
				}
				cleanedKlines = append(cleanedKlines, klines[i])
			}
			klines = cleanedKlines

			// 检查过滤后数据量
			if len(klines) < 30 {
				return
			}

			// 执行策略
			if strategy.Select(klines) {
				last := klines[len(klines)-1]
				// 计算涨跌幅
				change := 0.0
				changePercent := 0.0
				if len(klines) >= 2 {
					prevClose := klines[len(klines)-2].Close
					if prevClose > 0 {
						change = last.Close - prevClose
						changePercent = change / prevClose * 100
					}
				}

				// 计算得分
				score, scoreDetail := strategy.Score(klines)

				stockResult := models.SelectorStock{
					Symbol:        s.Symbol,
					Name:          s.Name,
					Industry:      s.Industry,
					Price:         last.Close,
					Change:        change,
					ChangePercent: changePercent,
					Volume:        last.Volume,
					Amount:        last.Amount,
					Score:         score,
					ScoreDetail:   scoreDetail,
				}

				// AI涨跌预测
				if predictor != nil && predictor.IsTrained() {
					if pred := predictor.Predict(klines); pred != nil {
						stockResult.PredDirection = pred.Direction
						stockResult.PredReturn = pred.Return
						stockResult.PredConfidence = pred.Confidence
						stockResult.PredSignal = pred.Signal
					}
				}

				mu.Lock()
				results = append(results, stockResult)
				log.Info("选股命中: %s (%s), 价格=%.2f, 涨跌=%.2f%%, 得分=%.1f", s.Name, s.Symbol, last.Close, changePercent, score)
				// 实时回调，通知有新结果
				if progressCallback != nil {
					progressCallback(ProgressInfo{
						Total:     total,
						Processed: int(atomic.LoadInt64(&processed)),
						Found:     len(results),
						Current:   s.Name,
						Results:   results,
					})
				}
				mu.Unlock()
			}
		}(stock)
	}

	wg.Wait()

	isCancelled := atomic.LoadInt32(&cancelled) > 0

	// 最终回调
	if progressCallback != nil {
		mu.Lock()
		progressCallback(ProgressInfo{
			Total:     total,
			Processed: int(atomic.LoadInt64(&processed)),
			Found:     len(results),
			Current:   "",
			Results:   results,
			Done:      true,
			Cancelled: isCancelled,
		})
		mu.Unlock()
	}

	log.Info("选股完成: 策略=%s, 总数=%d, 已处理=%d, 命中=%d, 取消=%v", strategyID, total, atomic.LoadInt64(&processed), len(results), isCancelled)
	return results
}
