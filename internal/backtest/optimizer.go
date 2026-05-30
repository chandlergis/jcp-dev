package backtest

import (
	"fmt"
	"math"
)

// OptimizedModel 优化后的预测模型（超参搜索 + 集成投票 + 自适应阈值）
type OptimizedModel struct {
	models      []*GBMRegressor  // 多个不同参数的模型
	scaler      *StandardScaler
	weights     []float64        // 每个模型的权重（基于验证集表现）
	bestParams  []GBMConfig
	nFeatures   int
	topFeatures []int            // 保留的特征索引
}

// paramResult 超参搜索结果
type paramResult struct {
	config  GBMConfig
	valLoss float64
	model   *GBMRegressor
}

// OptimizerConfig 优化器配置
type OptimizerConfig struct {
	// 超参网格
	Depths       []int
	LearningRates []float64
	Lambdas      []float64
	// 集成
	NumModels    int
	// 特征筛选
	TopK         int // 保留前K个重要特征，0=全部
}

func DefaultOptimizerConfig() OptimizerConfig {
	return OptimizerConfig{
		Depths:        []int{3, 4, 5},
		LearningRates: []float64{0.03, 0.05, 0.08},
		Lambdas:       []float64{0.5, 1.0, 2.0},
		NumModels:     5,
		TopK:          35, // 从54个特征中选35个最重要的
	}
}

// OptimizeAndTrain 优化训练：超参搜索 + 特征筛选 + 集成
func OptimizeAndTrain(X [][]float64, y []float64, config OptimizerConfig) *OptimizedModel {
	if len(X) == 0 || len(y) == 0 {
		return nil
	}

	nFeat := len(X[0])

	// 划分训练集/验证集 (70/30)
	splitIdx := int(float64(len(X)) * 0.7)
	trainX, valX := X[:splitIdx], X[splitIdx:]
	trainY, valY := y[:splitIdx], y[splitIdx:]

	// 标准化
	scaler := &StandardScaler{}
	scaler.Fit(trainX)
	trainNorm := scaler.Transform(trainX)
	valNorm := scaler.Transform(valX)

	// 第1步：用默认参数训练一个初始模型，获取特征重要性
	fmt.Println("  [1/3] 特征筛选...")
	initModel := NewGBMRegressor(GBMConfig{
		MaxDepth: 4, NEstimators: 100, LearningRate: 0.05,
		Lambda: 1.0, Gamma: 0.0, ColSample: 0.8, SubSample: 0.8, MinLeafSize: 30,
	})
	initModel.Fit(trainNorm, trainY)

	// 选择 Top-K 特征
	topK := config.TopK
	if topK <= 0 || topK > nFeat {
		topK = nFeat
	}
	topFeatures := selectTopFeatures(initModel.FeatureImport, topK)
	fmt.Printf("  保留 %d/%d 个特征\n", topK, nFeat)

	// 筛选特征
	trainSelected := selectFeatures(trainNorm, topFeatures)
	valSelected := selectFeatures(valNorm, topFeatures)

	// 第2步：网格搜索
	fmt.Println("  [2/3] 超参网格搜索...")
	var results []paramResult

	total := len(config.Depths) * len(config.LearningRates) * len(config.Lambdas)
	count := 0
	for _, depth := range config.Depths {
		for _, lr := range config.LearningRates {
			for _, lambda := range config.Lambdas {
				count++
				cfg := GBMConfig{
					MaxDepth:     depth,
					NEstimators:  150,
					LearningRate: lr,
					Lambda:       lambda,
					Gamma:        0.0,
					ColSample:    0.8,
					SubSample:    0.8,
					MinLeafSize:  30,
				}
				model := NewGBMRegressor(cfg)
				model.Fit(trainSelected, trainY)

				// 验证集损失
				preds := model.Predict(valSelected)
				loss := mseLoss(preds, valY)
				acc := directionAccuracy(preds, valY)
				results = append(results, paramResult{cfg, loss, model})

				fmt.Printf("  [%d/%d] depth=%d lr=%.3f λ=%.1f val_loss=%.6f acc=%.1f%%\n",
					count, total, depth, lr, lambda, loss, acc*100)
			}
		}
	}

	// 排序选最优
	sortParamResults(results)
	bestCount := min(config.NumModels, len(results))
	fmt.Printf("\n  最优参数: depth=%d lr=%.3f λ=%.1f val_loss=%.6f\n",
		results[0].config.MaxDepth, results[0].config.LearningRate,
		results[0].config.Lambda, results[0].valLoss)

	// 第3步：集成投票
	fmt.Printf("  [3/3] 集成 %d 个模型...\n", bestCount)
	var models []*GBMRegressor
	var weights []float64
	var bestConfigs []GBMConfig

	for i := 0; i < bestCount; i++ {
		models = append(models, results[i].model)
		// 权重 = 1/loss（loss越小权重越大）
		w := 1.0 / (results[i].valLoss + 1e-10)
		weights = append(weights, w)
		bestConfigs = append(bestConfigs, results[i].config)
	}

	// 归一化权重
	totalW := 0.0
	for _, w := range weights {
		totalW += w
	}
	for i := range weights {
		weights[i] /= totalW
	}

	return &OptimizedModel{
		models:      models,
		scaler:      scaler,
		weights:     weights,
		bestParams:  bestConfigs,
		nFeatures:   nFeat,
		topFeatures: topFeatures,
	}
}

// GetBestModel 获取最优单模型
func (om *OptimizedModel) GetBestModel() *GBMRegressor {
	if len(om.models) > 0 {
		return om.models[0]
	}
	return nil
}

// GetScaler 获取标准化器
func (om *OptimizedModel) GetScaler() *StandardScaler {
	return om.scaler
}

// GetTopFeatures 获取选中的特征索引
func (om *OptimizedModel) GetTopFeatures() []int {
	return om.topFeatures
}

// Predict 集成预测
func (om *OptimizedModel) Predict(X [][]float64) []float64 {
	if len(X) == 0 || len(om.models) == 0 {
		return nil
	}
	norm := om.scaler.Transform(X)
	selected := selectFeatures(norm, om.topFeatures)

	// 加权平均
	preds := make([]float64, len(X))
	for m, model := range om.models {
		modelPreds := model.Predict(selected)
		for i, p := range modelPreds {
			preds[i] += p * om.weights[m]
		}
	}
	return preds
}

// PredictSingle 单样本预测
func (om *OptimizedModel) PredictSingle(features []float64) float64 {
	preds := om.Predict([][]float64{features})
	if len(preds) > 0 {
		return preds[0]
	}
	return 0
}

// GetAdaptiveThreshold 自适应阈值：根据近期波动率调整
// 波动率高 → 阈值高（只交易强信号）
// 波动率低 → 阈值低（小信号也有价值）
func GetAdaptiveThreshold(closes []float64, baseThreshold float64) float64 {
	if len(closes) < 20 {
		return baseThreshold
	}
	// 计算20日波动率
	vol := HistoricalVolatility(closes, 20)
	currentVol := vol[len(vol)-1]

	// 用10日波动率做基线
	vol10 := HistoricalVolatility(closes, 10)
	baselineVol := vol10[len(vol10)-1]
	if baselineVol <= 0 {
		return baseThreshold
	}

	// 波动率比率
	volRatio := currentVol / baselineVol
	// 高波动时提高阈值，低波动时降低阈值
	adjustment := 0.5 + 0.5*volRatio // 0.5~1.5
	threshold := baseThreshold * adjustment

	// 限制范围
	if threshold < 0.001 {
		threshold = 0.001
	}
	if threshold > 0.01 {
		threshold = 0.01
	}
	return threshold
}

// GetEnsembleSignal 获取集成信号（带自适应阈值）
func (om *OptimizedModel) GetEnsembleSignal(predReturn float64, closes []float64) (string, float64) {
	threshold := GetAdaptiveThreshold(closes, 0.002) // 基础阈值 0.2%
	confidence := math.Tanh(math.Abs(predReturn) / 0.005)

	pct := predReturn * 100
	switch {
	case pct > 0.5 && confidence > 0.5:
		return "强买入", confidence
	case pct > threshold*100:
		return "买入", confidence
	case pct < -0.5 && confidence > 0.5:
		return "强卖出", confidence
	case pct < -threshold*100:
		return "卖出", confidence
	default:
		return "观望", confidence
	}
}

// --- 辅助函数 ---

func selectTopFeatures(importances []float64, k int) []int {
	type fi struct {
		idx  int
		imp  float64
	}
	var items []fi
	for i, v := range importances {
		items = append(items, fi{i, v})
	}
	// 按重要性降序排序
	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			if items[j].imp > items[i].imp {
				items[i], items[j] = items[j], items[i]
			}
		}
	}
	result := make([]int, k)
	for i := 0; i < k && i < len(items); i++ {
		result[i] = items[i].idx
	}
	return result
}

func selectFeatures(X [][]float64, indices []int) [][]float64 {
	result := make([][]float64, len(X))
	for i, row := range X {
		newRow := make([]float64, len(indices))
		for j, idx := range indices {
			if idx < len(row) {
				newRow[j] = row[idx]
			}
		}
		result[i] = newRow
	}
	return result
}

func mseLoss(preds, actuals []float64) float64 {
	sum := 0.0
	for i := range preds {
		d := preds[i] - actuals[i]
		sum += d * d
	}
	return sum / float64(len(preds))
}

func directionAccuracy(preds, actuals []float64) float64 {
	correct := 0
	for i := range preds {
		if (preds[i] > 0 && actuals[i] > 0) || (preds[i] <= 0 && actuals[i] <= 0) {
			correct++
		}
	}
	return float64(correct) / float64(len(preds))
}

func sortParamResults(results []paramResult) {
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].valLoss < results[i].valLoss {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
}
