package main

import (
	"fmt"
	"math"
	"os"

	"github.com/run-bigpig/jcp/internal/backtest"
	"github.com/run-bigpig/jcp/internal/models"
	"github.com/run-bigpig/jcp/internal/services"
)

func main() {
	fmt.Println("============================================")
	fmt.Println("  股价预测回测系统 v3 (回归+信号)")
	fmt.Println("============================================")
	fmt.Println("策略: 预测3日收益率, 仅交易强信号(>0.5%)")
	fmt.Println()

	ms := services.NewMarketService()
	result, err := runBacktestV3(ms)
	if err != nil {
		fmt.Printf("失败: %v\n", err)
		os.Exit(1)
	}
	printV3Report(result)
}

type V3Result struct {
	StockCount    int
	Samples       int
	TrainSamples  int
	TestSamples   int
	FilteredOut   int
	TestMetrics   backtest.Metrics
	StrongMetrics backtest.Metrics
	StrongCount   int
	TopFeatures   []backtest.FeatureInfo
	StockMetrics  map[string]float64
	ReturnStats   ReturnStats
}

type ReturnStats struct {
	MeanPredReturn  float64
	MeanTrueReturn  float64
	Correlation     float64
	SignalAccuracy  float64 // 信号方向准确率
	ProfitFactor    float64 // 盈亏比
}

func runBacktestV3(ms *services.MarketService) (*V3Result, error) {
	candidates := backtest.SelectRandomStocks(150)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("无法获取股票列表")
	}

	type sd struct {
		info   backtest.StockInfo
		klines []models.KLineData
	}
	var allData []sd
	for i, stock := range candidates {
		if len(allData) >= 50 {
			break
		}
		fmt.Printf("[%d] %s %s ...", i+1, stock.Symbol, stock.Name)
		klines, err := ms.GetKLineData(stock.Symbol, "1d", 300)
		if err != nil {
			fmt.Printf(" 失败\n")
			continue
		}
		if len(klines) < 300 {
			fmt.Printf(" 不足\n")
			continue
		}
		avgAmt := calcAvg(klines, 20)
		if avgAmt < 1e8 {
			fmt.Printf(" 成交额低\n")
			continue
		}
		fmt.Printf(" OK\n")
		allData = append(allData, sd{stock, klines})
	}
	if len(allData) < 10 {
		return nil, fmt.Errorf("有效股票不足")
	}
	fmt.Printf("有效股票: %d 支\n", len(allData))

	warmup := 60
	holdDays := 3
	var allFeatures [][]float64
	var allReturns []float64 // 连续值标签（收益率）
	var stockNames []string
	filtered := 0

	for _, d := range allData {
		adjusted := backtest.FilterExRights(d.klines)
		if len(adjusted) < warmup+holdDays+10 {
			continue
		}
		features := backtest.ComputeFeatures(adjusted, warmup)
		if len(features) == 0 {
			continue
		}

		// 回归标签：3日收益率
		for j := 0; j < len(features) && j+holdDays < len(adjusted)-warmup; j++ {
			idx := warmup + j
			if idx+holdDays >= len(adjusted) {
				break
			}
			ret := (adjusted[idx+holdDays].Close - adjusted[idx].Close) / adjusted[idx].Close
			// 过滤极端值
			if math.Abs(ret) > 0.15 {
				filtered++
				continue
			}
			allFeatures = append(allFeatures, features[j])
			allReturns = append(allReturns, ret)
		}
		stockNames = append(stockNames, fmt.Sprintf("%s %s", d.info.Symbol, d.info.Name))
	}

	if len(allFeatures) == 0 {
		return nil, fmt.Errorf("特征为空")
	}

	fmt.Printf("样本: %d, 过滤极端: %d\n", len(allReturns), filtered)

	// 80/20 划分
	splitIdx := int(float64(len(allFeatures)) * 0.8)
	trainX := allFeatures[:splitIdx]
	trainY := allReturns[:splitIdx]
	testX := allFeatures[splitIdx:]
	testY := allReturns[splitIdx:]

	// 验证集（未使用）
	_ = int(float64(len(trainX)) * 0.85)
	trainXFit := trainX
	trainYFit := trainY

	fmt.Printf("训练: %d, 测试: %d\n", len(trainXFit), len(testX))

	// 标准化
	scaler := &backtest.StandardScaler{}
	scaler.Fit(trainX)
	trainFitN := scaler.Transform(trainXFit)
	testXN := scaler.Transform(testX)
	trainFullN := scaler.Transform(trainX)

	// GBM 回归模型
	fmt.Println("\n--- 训练 GBM 回归模型 ---")
	gbm := backtest.NewGBMRegressor(backtest.GBMConfig{
		MaxDepth:       4,
		NEstimators:    200,
		LearningRate:   0.05,
		Lambda:         0.5,
		Gamma:          0.0,
		ColSample:      0.8,
		SubSample:      0.8,
		MinLeafSize:    50,
		EarlyStopRounds: 0, // 不早停
	})
	gbm.Fit(trainFitN, trainYFit)
	bestRounds := len(gbm.Trees)
	fmt.Printf("训练轮数: %d\n", bestRounds)

	// 用全部训练数据重新训练
	gbmFinal := backtest.NewGBMRegressor(backtest.GBMConfig{
		MaxDepth:     4,
		NEstimators:  bestRounds,
		LearningRate: 0.05,
		Lambda:       0.5,
		Gamma:        0.0,
		ColSample:    1.0,
		SubSample:    0.8,
		MinLeafSize:  50,
	})
	gbmFinal.Fit(trainFullN, trainY)

	// 预测
	trainPreds := gbmFinal.Predict(trainFullN)
	testPreds := gbmFinal.Predict(testXN)

	// 将回归预测转为分类信号（涨/跌）
	trainPredClass := retToClass(trainPreds)
	testPredClass := retToClass(testPreds)
	trainTrueClass := retToClass(trainY)
	testTrueClass := retToClass(testY)

	_ = backtest.Evaluate(trainTrueClass, trainPredClass)
	testMetrics := backtest.Evaluate(testTrueClass, testPredClass)

	// 收益率统计
	stats := computeReturnStats(testPreds, testY)

	// 强信号过滤：只交易预测收益 > 0.5% 的样本
	strongPreds := make([]float64, 0)
	strongActuals := make([]float64, 0)
	strongCount := 0
	for i := range testPreds {
		if math.Abs(testPreds[i]) > 0.005 {
			strongCount++
			predClass := 0.0
			if testPreds[i] > 0 {
				predClass = 1.0
			}
			strongPreds = append(strongPreds, predClass)
			actualClass := 0.0
			if testY[i] > 0 {
				actualClass = 1.0
			}
			strongActuals = append(strongActuals, actualClass)
		}
	}
	strongMetrics := backtest.Evaluate(strongActuals, strongPreds)

	// 特征重要性
	nFeat := len(trainX[0])
	importances := gbmFinal.GetFeatureImportance(nFeat)
	var topFeats []backtest.FeatureInfo
	fNames := backtest.FeatureNames()
	for i, imp := range importances {
		if i >= 15 {
			break
		}
		name := fmt.Sprintf("特征%d", imp.Index)
		if imp.Index < len(fNames) {
			name = fNames[imp.Index]
		}
		topFeats = append(topFeats, backtest.FeatureInfo{name, imp.Importance})
	}

	// 每支股票准确率
	stockMetrics := make(map[string]float64)
	offset := 0
	for _, d := range allData {
		adjusted := backtest.FilterExRights(d.klines)
		if len(adjusted) < warmup+holdDays+10 {
			continue
		}
		features := backtest.ComputeFeatures(adjusted, warmup)
		if len(features) == 0 {
			continue
		}
		count := 0
		correct := 0
		for j := 0; j < len(features) && j+holdDays < len(adjusted)-warmup; j++ {
			idx := warmup + j
			if idx+holdDays >= len(adjusted) {
				break
			}
			ret := (adjusted[idx+holdDays].Close - adjusted[idx].Close) / adjusted[idx].Close
			if math.Abs(ret) > 0.15 {
				continue
			}
			testIdx := offset + count - splitIdx
			if testIdx >= 0 && testIdx < len(testPreds) {
				predClass := 0.0
				if testPreds[testIdx] > 0 {
					predClass = 1.0
				}
				trueClass := 0.0
				if ret > 0 {
					trueClass = 1.0
				}
				if predClass == trueClass {
					correct++
				}
			}
			count++
		}
		if count > 0 {
			key := fmt.Sprintf("%s %s", d.info.Symbol, d.info.Name)
			stockMetrics[key] = float64(correct) / float64(count)
		}
		offset += count
	}

	return &V3Result{
		StockCount:    len(allData),
		Samples:       len(allReturns),
		TrainSamples:  len(trainX),
		TestSamples:   len(testX),
		FilteredOut:   filtered,
		TestMetrics:   testMetrics,
		StrongMetrics: strongMetrics,
		StrongCount:   strongCount,
		TopFeatures:   topFeats,
		StockMetrics:  stockMetrics,
		ReturnStats:   stats,
	}, nil
}

func retToClass(rets []float64) []float64 {
	classes := make([]float64, len(rets))
	for i, r := range rets {
		if r > 0 {
			classes[i] = 1.0
		}
	}
	return classes
}

func computeReturnStats(preds, actuals []float64) ReturnStats {
	if len(preds) == 0 {
		return ReturnStats{}
	}

	sumPred := 0.0
	sumTrue := 0.0
	correct := 0
	totalProfit := 0.0
	totalLoss := 0.0

	for i := range preds {
		sumPred += preds[i]
		sumTrue += actuals[i]
		// 信号方向准确率
		if (preds[i] > 0 && actuals[i] > 0) || (preds[i] <= 0 && actuals[i] <= 0) {
			correct++
		}
		// 盈亏比
		if preds[i] > 0 {
			if actuals[i] > 0 {
				totalProfit += actuals[i]
			} else {
				totalLoss += math.Abs(actuals[i])
			}
		}
	}

	n := float64(len(preds))
	profitFactor := 0.0
	if totalLoss > 0 {
		profitFactor = totalProfit / totalLoss
	} else if totalProfit > 0 {
		profitFactor = 999
	}

	// 相关系数
	meanP := sumPred / n
	meanA := sumTrue / n
	cov := 0.0
	varP := 0.0
	varA := 0.0
	for i := range preds {
		dp := preds[i] - meanP
		da := actuals[i] - meanA
		cov += dp * da
		varP += dp * dp
		varA += da * da
	}
	corr := 0.0
	if varP > 0 && varA > 0 {
		corr = cov / math.Sqrt(varP*varA)
	}

	return ReturnStats{
		MeanPredReturn: meanP * 100,
		MeanTrueReturn: meanA * 100,
		Correlation:    corr,
		SignalAccuracy: float64(correct) / n,
		ProfitFactor:   profitFactor,
	}
}

func calcAvg(klines []models.KLineData, n int) float64 {
	start := len(klines) - n
	if start < 0 {
		start = 0
	}
	sum := 0.0
	cnt := 0
	for i := start; i < len(klines); i++ {
		if klines[i].Amount > 0 {
			sum += klines[i].Amount
			cnt++
		}
	}
	if cnt == 0 {
		return 0
	}
	return sum / float64(cnt)
}

func printV3Report(r *V3Result) {
	fmt.Println()
	fmt.Println("================================================")
	fmt.Println("       股价预测回测报告 (GBM回归+信号)")
	fmt.Println("================================================")
	fmt.Printf("股票: %d 支, 样本: %d (训练:%d 测试:%d)\n", r.StockCount, r.Samples, r.TrainSamples, r.TestSamples)
	fmt.Printf("过滤极端值: %d\n", r.FilteredOut)
	fmt.Println()

	fmt.Println("--- 分类指标 (全部测试样本) ---")
	fmt.Printf("  准确率:   %.2f%%\n", r.TestMetrics.Accuracy*100)
	fmt.Printf("  精确率:   %.2f%%\n", r.TestMetrics.Precision*100)
	fmt.Printf("  召回率:   %.2f%%\n", r.TestMetrics.Recall*100)
	fmt.Printf("  F1分数:   %.2f%%\n", r.TestMetrics.F1*100)
	fmt.Println()

	if r.StrongCount > 0 {
		fmt.Println("--- 强信号指标 (|预测收益|>0.5%) ---")
		fmt.Printf("  信号样本: %d / %d (%.1f%%)\n", r.StrongCount, r.TestSamples, float64(r.StrongCount)/float64(r.TestSamples)*100)
		fmt.Printf("  准确率:   %.2f%%\n", r.StrongMetrics.Accuracy*100)
		fmt.Printf("  精确率:   %.2f%%\n", r.StrongMetrics.Precision*100)
		fmt.Printf("  召回率:   %.2f%%\n", r.StrongMetrics.Recall*100)
		fmt.Printf("  F1分数:   %.2f%%\n", r.StrongMetrics.F1*100)
		fmt.Println()
	}

	fmt.Println("--- 收益率预测质量 ---")
	fmt.Printf("  预测均值:   %.3f%%\n", r.ReturnStats.MeanPredReturn)
	fmt.Printf("  实际均值:   %.3f%%\n", r.ReturnStats.MeanTrueReturn)
	fmt.Printf("  相关系数:   %.4f\n", r.ReturnStats.Correlation)
	fmt.Printf("  信号准确率: %.2f%%\n", r.ReturnStats.SignalAccuracy*100)
	fmt.Printf("  盈亏比:     %.2f\n", r.ReturnStats.ProfitFactor)
	fmt.Println()

	fmt.Println("--- 混淆矩阵 ---")
	fmt.Println("              预测涨    预测跌")
	fmt.Printf("  实际涨       %d        %d\n", r.TestMetrics.TP, r.TestMetrics.FN)
	fmt.Printf("  实际跌       %d        %d\n", r.TestMetrics.FP, r.TestMetrics.TN)
	fmt.Println()

	if len(r.TopFeatures) > 0 {
		fmt.Println("--- Top 15 特征重要性 ---")
		for i, f := range r.TopFeatures {
			fmt.Printf("  %2d. %-12s  %.1f\n", i+1, f.Name, f.Importance)
		}
		fmt.Println()
	}

	if len(r.StockMetrics) > 0 {
		top := backtest.TopStocksByAccuracy(r.StockMetrics, 10)
		fmt.Println("--- 个股信号准确率 Top 10 ---")
		for i, s := range top {
			fmt.Printf("  %2d. %-20s  %.1f%%\n", i+1, s.Stock, s.Accuracy*100)
		}
		fmt.Println()
		above55, above60 := 0, 0
		for _, acc := range r.StockMetrics {
			if acc > 0.55 {
				above55++
			}
			if acc > 0.60 {
				above60++
			}
		}
		fmt.Printf("信号准确率 > 55%%: %d 支 | > 60%%: %d 支\n", above55, above60)
	}
}
