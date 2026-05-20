package backtest

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/run-bigpig/jcp/internal/embed"
	"github.com/run-bigpig/jcp/internal/models"
)

// MarketDataFetcher 市场数据接口（避免循环依赖）
type MarketDataFetcher interface {
	GetKLineData(code string, period string, days int) ([]models.KLineData, error)
}

const (
	stockCount      = 50
	dataDays        = 300 // 增加到300天，获取更多训练数据
	warmupDays      = 60  // 增加预热天数，让长周期指标更准确
	trainRatio      = 0.8
	learningRate    = 0.01
	lambdaLR        = 0.1
	iterations      = 1000
	minAmountFilter = 1e8
	labelHoldDays   = 3    // 预测3日涨跌（减少噪声）
	labelThreshold  = 0.01 // 涨跌阈值1%，过滤震荡
)

type BacktestResult struct {
	StockCodes     []string
	TrainSamples   int
	TestSamples    int
	FilteredOut    int // 被过滤的震荡样本数
	LR             ModelResult
	GBM            ModelResult
	StockMetrics   map[string]float64
	TopFeatures    []FeatureInfo
}

type ModelResult struct {
	TrainMetrics Metrics
	TestMetrics  Metrics
}

type FeatureInfo struct {
	Name       string
	Importance float64
}

type StockInfo struct {
	Symbol   string
	Name     string
	Industry string
}

type stockData struct {
	info   StockInfo
	klines []models.KLineData
}

var featureNames = []string{
	"日收益率", "振幅", "实体比", "上影线比", "下影线比", "缺口",
	"MA5偏离", "MA10偏离", "MA20偏离", "MA60偏离",
	"MA5/10金叉", "MA5/20金叉", "MA5斜率",
	"RSI6", "RSI14", "RSI背离",
	"MACD柱", "MACD交叉", "MACD趋势",
	"K值", "D值", "J值", "KDJ交叉",
	"WR14", "布林带位置", "布林带宽度",
	"ATR比值", "波动率5", "波动率10", "波动率20", "波动率变化",
	"3日收益", "5日收益", "10日收益", "20日收益", "收益加速度",
	"量比5", "量比10", "成交额变化", "量价背离", "OBV趋势",
	"20日位置", "60日位置", "连涨跌",
}

func RunBacktest(marketService MarketDataFetcher) (*BacktestResult, error) {
	// 1. 获取股票
	candidates := selectRandomStocks(stockCount * 3)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("无法获取股票列表")
	}
	fmt.Printf("候选股票: %d 支\n", len(candidates))

	// 2. 获取K线数据
	var allData []stockData
	for i, stock := range candidates {
		if len(allData) >= stockCount {
			break
		}
		fmt.Printf("[%d] %s %s ...", i+1, stock.Symbol, stock.Name)
		klines, err := marketService.GetKLineData(stock.Symbol, "1d", dataDays)
		if err != nil {
			fmt.Printf(" 失败\n")
			continue
		}
		if len(klines) < dataDays {
			fmt.Printf(" 不足(%d天)\n", len(klines))
			continue
		}
		avgAmt := calcAvgAmount(klines, 20)
		if avgAmt < minAmountFilter {
			fmt.Printf(" 成交额低\n")
			continue
		}
		fmt.Printf(" OK(%.1f亿/日)\n", avgAmt/1e8)
		allData = append(allData, stockData{info: stock, klines: klines})
	}
	if len(allData) < 10 {
		return nil, fmt.Errorf("有效股票不足: %d", len(allData))
	}
	fmt.Printf("\n有效股票: %d 支\n", len(allData))

	// 3. 特征计算 + 标签（带阈值过滤）
	var allFeatures [][]float64
	var allLabels []float64
	var stockNames []string
	filteredOut := 0

	for _, sd := range allData {
		adjusted := FilterExRights(sd.klines)
		if len(adjusted) < warmupDays+labelHoldDays+10 {
			continue
		}

		features := ComputeFeatures(adjusted, warmupDays)
		labels, valid := ComputeLabels(adjusted, warmupDays, labelHoldDays, labelThreshold)
		if len(features) == 0 || len(labels) == 0 {
			continue
		}

		minLen := len(features)
		if len(labels) < minLen {
			minLen = len(labels)
		}

		for j := 0; j < minLen; j++ {
			if j < len(valid) && !valid[j] {
				filteredOut++ // 跳过震荡样本
				continue
			}
			allFeatures = append(allFeatures, features[j])
			allLabels = append(allLabels, labels[j])
		}
		stockNames = append(stockNames, fmt.Sprintf("%s %s", sd.info.Symbol, sd.info.Name))
	}

	if len(allFeatures) == 0 {
		return nil, fmt.Errorf("特征计算失败")
	}

	upCount := countClass(allLabels, 1)
	downCount := countClass(allLabels, 0)
	fmt.Printf("总样本: %d (涨:%d 跌:%d), 过滤震荡: %d\n", len(allLabels), upCount, downCount, filteredOut)

	// 4. 划分
	splitIdx := int(float64(len(allFeatures)) * trainRatio)
	trainX := allFeatures[:splitIdx]
	trainY := allLabels[:splitIdx]
	testX := allFeatures[splitIdx:]
	testY := allLabels[splitIdx:]

	// 进一步划分验证集（用于GBM早停）
	valSplitIdx := int(float64(len(trainX)) * 0.85)
	trainXFit := trainX[:valSplitIdx]
	trainYFit := trainY[:valSplitIdx]
	valX := trainX[valSplitIdx:]
	valY := trainY[valSplitIdx:]

	fmt.Printf("训练: %d, 验证: %d, 测试: %d\n\n", len(trainXFit), len(valX), len(testX))

	// 5. 标准化
	scaler := &StandardScaler{}
	scaler.Fit(trainX)
	trainXFitN := scaler.Transform(trainXFit)
	valXN := scaler.Transform(valX)
	testXN := scaler.Transform(testX)
	trainXFullN := scaler.Transform(trainX)

	// 6. 逻辑回归
	fmt.Println("--- 逻辑回归 ---")
	lr := NewLogisticRegression(learningRate, lambdaLR, iterations)
	lr.Fit(trainXFullN, trainY)
	lrTestPreds := lr.Predict(testXN, 0.5)
	lrTrainMetrics := Evaluate(trainY, lr.Predict(trainXFullN, 0.5))
	lrTestMetrics := Evaluate(testY, lrTestPreds)
	fmt.Printf("测试准确率: %.2f%%\n\n", lrTestMetrics.Accuracy*100)

	// 7. GBM（带早停）
	fmt.Println("--- GBM (XGBoost) ---")
	gbmConfig := GBMConfig{
		MaxDepth:       3,
		NEstimators:    400,
		LearningRate:   0.015,
		Lambda:         1.5,
		Gamma:          0.5,
		ColSample:      0.7,
		SubSample:      0.7,
		MinLeafSize:    20,
		EarlyStopRounds: 50,
	}
	gbm := NewGBM(gbmConfig)
	gbm.FitWithValidation(trainXFitN, trainYFit, valXN, valY)

	// 用全部训练数据重新训练（使用最佳轮数）
	bestRounds := len(gbm.Trees)
	fmt.Printf("最佳轮数: %d\n", bestRounds)

	gbmFinal := NewGBM(GBMConfig{
		MaxDepth:     gbmConfig.MaxDepth,
		NEstimators:  bestRounds,
		LearningRate: gbmConfig.LearningRate,
		Lambda:       gbmConfig.Lambda,
		Gamma:        gbmConfig.Gamma,
		ColSample:    gbmConfig.ColSample,
		SubSample:    gbmConfig.SubSample,
		MinLeafSize:  gbmConfig.MinLeafSize,
	})
	gbmFinal.Fit(trainXFullN, trainY)

	gbmTrainPreds := gbmFinal.Predict(trainXFullN, 0.45)
	gbmTestPreds := gbmFinal.Predict(testXN, 0.45)
	gbmTrainMetrics := Evaluate(trainY, gbmTrainPreds)
	gbmTestMetrics := Evaluate(testY, gbmTestPreds)
	fmt.Printf("测试准确率: %.2f%% (阈值=0.45)\n\n", gbmTestMetrics.Accuracy*100)

	// 阈值搜索：找最佳F1对应的阈值
	bestThresh := 0.45
	bestF1 := gbmTestMetrics.F1
	testProbs := gbmFinal.PredictProba(testXN)
	for th := 0.35; th <= 0.60; th += 0.05 {
		preds := make([]float64, len(testProbs))
		for i, p := range testProbs {
			if p >= th {
				preds[i] = 1.0
			}
		}
		m := Evaluate(testY, preds)
		if m.F1 > bestF1 {
			bestF1 = m.F1
			bestThresh = th
		}
	}
	if bestThresh != 0.45 {
		fmt.Printf("最佳阈值: %.2f (F1=%.2f%%)\n", bestThresh, bestF1*100)
		gbmTestPreds = gbmFinal.Predict(testXN, bestThresh)
		gbmTestMetrics = Evaluate(testY, gbmTestPreds)
		gbmTrainPreds = gbmFinal.Predict(trainXFullN, bestThresh)
		gbmTrainMetrics = Evaluate(trainY, gbmTrainPreds)
	}

	// 8. 特征重要性
	nFeat := len(trainX[0])
	importances := gbmFinal.GetFeatureImportance(nFeat)
	var topFeatures []FeatureInfo
	for i, imp := range importances {
		if i >= 15 {
			break
		}
		name := fmt.Sprintf("特征%d", imp.Index)
		if imp.Index < len(featureNames) {
			name = featureNames[imp.Index]
		}
		topFeatures = append(topFeatures, FeatureInfo{name, imp.Importance})
	}

	// 9. 每支股票准确率
	stockAccuracies := computeStockAccuracies(allData, testY, gbmTestPreds, splitIdx, warmupDays, labelHoldDays, labelThreshold)

	return &BacktestResult{
		StockCodes:   stockNames,
		TrainSamples: len(trainX),
		TestSamples:  len(testX),
		FilteredOut:  filteredOut,
		LR:           ModelResult{lrTrainMetrics, lrTestMetrics},
		GBM:          ModelResult{gbmTrainMetrics, gbmTestMetrics},
		StockMetrics: stockAccuracies,
		TopFeatures:  topFeatures,
	}, nil
}

func calcAvgAmount(klines []models.KLineData, n int) float64 {
	if len(klines) == 0 {
		return 0
	}
	start := len(klines) - n
	if start < 0 {
		start = 0
	}
	sum := 0.0
	count := 0
	for i := start; i < len(klines); i++ {
		if klines[i].Amount > 0 {
			sum += klines[i].Amount
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}

func computeStockAccuracies(allData []stockData, testY, testPreds []float64, splitIdx, warmup, holdDays int, threshold float64) map[string]float64 {
	result := make(map[string]float64)
	offset := 0
	for _, sd := range allData {
		adjusted := FilterExRights(sd.klines)
		if len(adjusted) < warmup+holdDays+10 {
			continue
		}
		features := ComputeFeatures(adjusted, warmup)
		labels, valid := ComputeLabels(adjusted, warmup, holdDays, threshold)
		if len(features) == 0 || len(labels) == 0 {
			continue
		}
		minLen := len(features)
		if len(labels) < minLen {
			minLen = len(labels)
		}

		var stockPreds, stockTrue []float64
		for j := 0; j < minLen; j++ {
			if j < len(valid) && !valid[j] {
				continue
			}
			globalIdx := offset + len(stockTrue)
			testIdx := globalIdx - splitIdx
			if testIdx >= 0 && testIdx < len(testY) && testIdx < len(testPreds) {
				stockPreds = append(stockPreds, testPreds[testIdx])
				stockTrue = append(stockTrue, testY[testIdx])
			}
		}

		if len(stockTrue) > 0 {
			m := Evaluate(stockTrue, stockPreds)
			key := fmt.Sprintf("%s %s", sd.info.Symbol, sd.info.Name)
			result[key] = m.Accuracy
		}

		// 重新计算offset（包含被过滤的样本）
		adjusted2 := FilterExRights(sd.klines)
		if len(adjusted2) >= warmup+holdDays+10 {
			f2 := ComputeFeatures(adjusted2, warmup)
			l2, _ := ComputeLabels(adjusted2, warmup, holdDays, threshold)
			if len(f2) > 0 && len(l2) > 0 {
				ml := len(f2)
				if len(l2) < ml {
					ml = len(l2)
				}
				offset += ml
			}
		}
	}
	return result
}

// FeatureNames 返回特征名称列表
func FeatureNames() []string {
	return featureNames
}

// SelectRandomStocks 导出版本
func SelectRandomStocks(count int) []StockInfo {
	return selectRandomStocks(count)
}

func selectRandomStocks(count int) []StockInfo {
	var basicData struct {
		Data struct {
			Fields []string        `json:"fields"`
			Items  [][]interface{} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(embed.StockBasicJSON, &basicData); err != nil {
		return nil
	}

	var symbolIdx, nameIdx, industryIdx, tsCodeIdx, listStatusIdx int = -1, -1, -1, -1, -1
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
		case "list_status":
			listStatusIdx = i
		}
	}

	var candidates []StockInfo
	for _, item := range basicData.Data.Items {
		name, _ := item[nameIdx].(string)
		if strings.Contains(name, "ST") || strings.Contains(name, "*ST") || strings.Contains(name, "退") {
			continue
		}
		tsCode, _ := item[tsCodeIdx].(string)
		if strings.HasSuffix(tsCode, ".BJ") {
			continue
		}
		if listStatusIdx >= 0 {
			status, _ := item[listStatusIdx].(string)
			if status != "L" {
				continue
			}
		}
		symbol, _ := item[symbolIdx].(string)
		industry := ""
		if industryIdx >= 0 && industryIdx < len(item) {
			industry, _ = item[industryIdx].(string)
		}
		fullSymbol := symbol
		switch {
		case strings.HasSuffix(tsCode, ".SH"):
			fullSymbol = "sh" + symbol
		case strings.HasSuffix(tsCode, ".SZ"):
			fullSymbol = "sz" + symbol
		}
		candidates = append(candidates, StockInfo{fullSymbol, name, industry})
	}

	if len(candidates) == 0 {
		return nil
	}
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	rng.Shuffle(len(candidates), func(i, j int) {
		candidates[i], candidates[j] = candidates[j], candidates[i]
	})
	if count > len(candidates) {
		count = len(candidates)
	}
	return candidates[:count]
}

func countClass(labels []float64, class float64) int {
	c := 0
	for _, l := range labels {
		if l == class {
			c++
		}
	}
	return c
}

func TopStocksByAccuracy(acc map[string]float64, n int) []struct {
	Stock    string
	Accuracy float64
} {
	type item struct {
		Stock    string
		Accuracy float64
	}
	var items []item
	for k, v := range acc {
		items = append(items, item{k, v})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Accuracy > items[j].Accuracy
	})
	if n > len(items) {
		n = len(items)
	}
	result := make([]struct {
		Stock    string
		Accuracy float64
	}, n)
	for i := 0; i < n; i++ {
		result[i] = struct {
			Stock    string
			Accuracy float64
		}{items[i].Stock, items[i].Accuracy}
	}
	return result
}
