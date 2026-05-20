package services

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sync"

	"github.com/run-bigpig/jcp/internal/backtest"
	"github.com/run-bigpig/jcp/internal/logger"
	"github.com/run-bigpig/jcp/internal/models"
)

var predLog = logger.New("prediction")

// PredictionService 股价涨跌预测服务
type PredictionService struct {
	mu          sync.RWMutex
	model       *backtest.GBMRegressor
	scaler      *backtest.StandardScaler
	isTrained    bool
	trainStocks  int
	trainSamples int
	modelPath    string // 模型文件路径
}

// predictionFile 模型持久化文件结构
type predictionFile struct {
	Model  *backtest.GBMRegressorSnapshot `json:"model"`
	Scaler *backtest.ScalerSnapshot       `json:"scaler"`
	Stocks int                             `json:"stocks"`
	Samples int                            `json:"samples"`
}

// NewPredictionService 创建预测服务
func NewPredictionService(dataDir string) *PredictionService {
	return &PredictionService{
		modelPath: filepath.Join(dataDir, "prediction_model.json"),
	}
}

// Init 初始化：尝试从文件加载，失败则后台训练
func (ps *PredictionService) Init(marketSvc *MarketService) {
	if ps.LoadFromFile() {
		return
	}
	// 文件不存在或加载失败，后台训练
	go ps.trainInBackground(marketSvc)
}

// LoadFromFile 从文件加载模型
func (ps *PredictionService) LoadFromFile() bool {
	data, err := os.ReadFile(ps.modelPath)
	if err != nil {
		predLog.Info("模型文件不存在，需要训练: %v", err)
		return false
	}

	var pf predictionFile
	if err := json.Unmarshal(data, &pf); err != nil {
		predLog.Warn("模型文件解析失败: %v", err)
		return false
	}

	model := &backtest.GBMRegressor{}
	model.LoadSnapshot(pf.Model)

	scaler := &backtest.StandardScaler{}
	scaler.LoadSnapshot(pf.Scaler)

	ps.mu.Lock()
	ps.model = model
	ps.scaler = scaler
	ps.isTrained = true
	ps.trainStocks = pf.Stocks
	ps.trainSamples = pf.Samples
	ps.mu.Unlock()

	predLog.Info("从文件加载预测模型成功: %d支股票, %d样本", pf.Stocks, pf.Samples)
	return true
}

// SaveToFile 保存模型到文件
func (ps *PredictionService) SaveToFile() error {
	ps.mu.RLock()
	if !ps.isTrained {
		ps.mu.RUnlock()
		return nil
	}
	pf := predictionFile{
		Model:   ps.model.Snapshot(),
		Scaler:  ps.scaler.Snapshot(),
		Stocks:  ps.trainStocks,
		Samples: ps.trainSamples,
	}
	ps.mu.RUnlock()

	data, err := json.Marshal(pf)
	if err != nil {
		return err
	}

	// 确保目录存在
	dir := filepath.Dir(ps.modelPath)
	os.MkdirAll(dir, 0755)

	return os.WriteFile(ps.modelPath, data, 0644)
}

// trainInBackground 后台训练模型
func (ps *PredictionService) trainInBackground(marketSvc *MarketService) {
	predLog.Info("后台训练AI预测模型...")
	allStocks := loadAllStockSymbols()
	trainCodes := allStocks[:min(50, len(allStocks))]

	if err := ps.TrainOnFetcher(marketSvc, trainCodes, 300); err != nil {
		predLog.Warn("AI预测模型训练失败: %v", err)
		return
	}

	stocks, samples := ps.GetTrainInfo()
	predLog.Info("AI预测模型训练完成: %d支股票, %d样本", stocks, samples)

	// 保存到文件
	if err := ps.SaveToFile(); err != nil {
		predLog.Warn("保存模型文件失败: %v", err)
	} else {
		predLog.Info("模型已保存到: %s", ps.modelPath)
	}
}

// loadAllStockSymbols 加载所有股票代码
func loadAllStockSymbols() []string {
	// 这里复用 embed 包的股票列表
	// 简单实现：返回一些常见的大市值股票代码
	return []string{
		"sh600519", "sh601318", "sz000858", "sh600036", "sz000001",
		"sh600276", "sh601012", "sz002714", "sh600887", "sz000333",
		"sh601888", "sz002475", "sh600900", "sh601398", "sh601939",
		"sh600030", "sz000002", "sh600000", "sh601166", "sz002304",
		"sh600809", "sz000568", "sh601668", "sh600690", "sz002415",
		"sh601899", "sh600585", "sz000725", "sh601601", "sh600050",
		"sz002352", "sh600016", "sh601288", "sz000063", "sh600309",
		"sz002230", "sh601628", "sh600009", "sz000100", "sh600048",
		"sz002049", "sh601688", "sh600570", "sz000651", "sh600436",
		"sz002142", "sh601225", "sz000338", "sh600010", "sz002601",
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// TrainOnStocks 用多支股票的历史数据训练模型
func (ps *PredictionService) TrainOnStocks(marketSvc *MarketService, stockCodes []string, days int) error {
	return ps.TrainOnFetcher(marketSvc, stockCodes, days)
}

// TrainOnFetcher 用数据获取接口训练模型
func (ps *PredictionService) TrainOnFetcher(fetcher interface {
	GetKLineData(code string, period string, days int) ([]models.KLineData, error)
}, stockCodes []string, days int) error {
	predLog.Info("开始训练预测模型, 股票数=%d, 天数=%d", len(stockCodes), days)

	warmup := 60
	holdDays := 3

	var allFeatures [][]float64
	var allReturns []float64
	trainedStocks := 0

	for _, code := range stockCodes {
		klines, err := fetcher.GetKLineData(code, "1d", days)
		if err != nil || len(klines) < days {
			continue
		}

		adjusted := backtest.FilterExRights(klines)
		if len(adjusted) < warmup+holdDays+10 {
			continue
		}

		features := backtest.ComputeFeatures(adjusted, warmup)
		if len(features) == 0 {
			continue
		}

		for j := 0; j < len(features) && j+holdDays < len(adjusted)-warmup; j++ {
			idx := warmup + j
			if idx+holdDays >= len(adjusted) {
				break
			}
			ret := (adjusted[idx+holdDays].Close - adjusted[idx].Close) / adjusted[idx].Close
			if math.Abs(ret) > 0.15 {
				continue
			}
			allFeatures = append(allFeatures, features[j])
			allReturns = append(allReturns, ret)
		}
		trainedStocks++
	}

	if len(allFeatures) < 100 {
		predLog.Error("训练数据不足: %d 样本", len(allFeatures))
		return nil
	}

	scaler := &backtest.StandardScaler{}
	scaler.Fit(allFeatures)
	normFeatures := scaler.Transform(allFeatures)

	model := backtest.NewGBMRegressor(backtest.GBMConfig{
		MaxDepth:     4,
		NEstimators:  200,
		LearningRate: 0.05,
		Lambda:       0.5,
		Gamma:        0.0,
		ColSample:    0.8,
		SubSample:    0.8,
		MinLeafSize:  50,
	})
	model.Fit(normFeatures, allReturns)

	ps.mu.Lock()
	ps.model = model
	ps.scaler = scaler
	ps.isTrained = true
	ps.trainStocks = trainedStocks
	ps.trainSamples = len(allFeatures)
	ps.mu.Unlock()

	predLog.Info("预测模型训练完成: %d支股票, %d样本", trainedStocks, len(allFeatures))
	return nil
}

// Predict 对单支股票的K线数据进行预测
func (ps *PredictionService) Predict(klines []models.KLineData) *models.PredictionResult {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	if !ps.isTrained || ps.model == nil {
		return nil
	}

	warmup := 60
	if len(klines) < warmup+5 {
		return nil
	}

	adjusted := backtest.FilterExRights(klines)
	if len(adjusted) < warmup+5 {
		return nil
	}

	features := backtest.ComputeFeatures(adjusted, warmup)
	if len(features) == 0 {
		return nil
	}

	lastFeat := features[len(features)-1:]
	normFeat := ps.scaler.Transform(lastFeat)
	predReturn := ps.model.Predict(normFeat)[0]
	confidence := math.Min(math.Abs(predReturn)/0.03, 1.0)

	predLog.Debug("预测 %s: 原始值=%.6f, 收益=%.3f%%", "stock", predReturn, predReturn*100)

	direction := "跌"
	if predReturn > 0 {
		direction = "涨"
	}

	return &models.PredictionResult{
		Direction:  direction,
		Return:     predReturn * 100,
		Confidence: confidence,
		Signal:     getSignal(predReturn, confidence),
	}
}

// IsTrained 模型是否已训练
func (ps *PredictionService) IsTrained() bool {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return ps.isTrained
}

// GetTrainInfo 获取训练信息
func (ps *PredictionService) GetTrainInfo() (stocks int, samples int) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return ps.trainStocks, ps.trainSamples
}

func getSignal(predReturn float64, confidence float64) string {
	// predReturn 是小数形式 (0.01 = 1%), 转为百分比值
	pct := predReturn * 100
	absPct := math.Abs(pct)
	switch {
	case pct > 0 && absPct > 1.5 && confidence > 0.3:
		return "强买入"
	case pct > 0 && absPct > 0.5:
		return "买入"
	case pct < 0 && absPct > 1.5 && confidence > 0.3:
		return "强卖出"
	case pct < 0 && absPct > 0.5:
		return "卖出"
	default:
		return "观望"
	}
}
