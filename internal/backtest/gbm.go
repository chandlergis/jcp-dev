package backtest

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
)

// GBMRegressor GBM回归器
type GBMRegressor struct {
	Trees          []*TreeNode
	MaxDepth       int
	NEstimators    int
	LearningRate   float64
	Lambda         float64
	Gamma          float64
	ColSample      float64
	SubSample      float64
	MinLeafSize    int
	EarlyStopRounds int
	FeatureImport  []float64
	basePred       float64
	rng            *rand.Rand
}

func NewGBMRegressor(config GBMConfig) *GBMRegressor {
	return &GBMRegressor{
		MaxDepth:       config.MaxDepth,
		NEstimators:    config.NEstimators,
		LearningRate:   config.LearningRate,
		Lambda:         config.Lambda,
		Gamma:          config.Gamma,
		ColSample:      config.ColSample,
		SubSample:      config.SubSample,
		MinLeafSize:    config.MinLeafSize,
		EarlyStopRounds: config.EarlyStopRounds,
		rng:            rand.New(rand.NewSource(42)),
	}
}

func (m *GBMRegressor) Fit(X [][]float64, y []float64) {
	m.FitWithValidation(X, y, nil, nil)
}

func (m *GBMRegressor) FitWithValidation(trainX [][]float64, trainY []float64, valX [][]float64, valY []float64) {
	n := len(trainX)
	if n == 0 {
		return
	}
	nFeat := len(trainX[0])
	m.FeatureImport = make([]float64, nFeat)

	// 基础预测 = 均值
	m.basePred = 0
	for _, v := range trainY {
		m.basePred += v
	}
	m.basePred /= float64(n)

	curPreds := make([]float64, n)
	for i := range curPreds {
		curPreds[i] = m.basePred
	}

	var valPreds []float64
	hasVal := len(valX) > 0 && len(valY) > 0
	if hasVal {
		valPreds = make([]float64, len(valX))
		for i := range valPreds {
			valPreds[i] = m.basePred
		}
	}

	m.Trees = nil
	bestValLoss := math.MaxFloat64
	noImprove := 0

	for t := 0; t < m.NEstimators; t++ {
		// MSE梯度 = pred - target
		grad := make([]float64, n)
		hess := make([]float64, n)
		for i := 0; i < n; i++ {
			grad[i] = curPreds[i] - trainY[i]
			hess[i] = 1.0
		}

		var sampleIdx []int
		if m.SubSample < 1.0 {
			count := int(float64(n) * m.SubSample)
			if count < 1 { count = 1 }
			idx := make([]int, n)
			for i := range idx { idx[i] = i }
			m.rng.Shuffle(n, func(i, j int) { idx[i], idx[j] = idx[j], idx[i] })
			sampleIdx = idx[:count]
		} else {
			sampleIdx = make([]int, n)
			for i := range sampleIdx { sampleIdx[i] = i }
		}

		cols := m.sampleColumns(nFeat)
		tree := m.buildTree(trainX, grad, hess, sampleIdx, cols, 0)
		m.Trees = append(m.Trees, tree)

		for i := 0; i < n; i++ {
			curPreds[i] += m.LearningRate * m.predictTree(tree, trainX[i])
		}
		if hasVal {
			for i := 0; i < len(valX); i++ {
				valPreds[i] += m.LearningRate * m.predictTree(tree, valX[i])
			}
			valLoss := 0.0
			for i := 0; i < len(valX); i++ {
				diff := valPreds[i] - valY[i]
				valLoss += diff * diff
			}
			valLoss /= float64(len(valX))
			if valLoss < bestValLoss-1e-8 {
				bestValLoss = valLoss
				noImprove = 0
			} else {
				noImprove++
			}
			if m.EarlyStopRounds > 0 && noImprove >= m.EarlyStopRounds && t > 20 {
				m.Trees = m.Trees[:len(m.Trees)-noImprove]
				fmt.Printf("  早停: 第%d轮, 最佳MSE=%.8f\n", t-noImprove, bestValLoss)
				break
			}
		}
	}
}

func (m *GBMRegressor) Predict(X [][]float64) []float64 {
	preds := make([]float64, len(X))
	for i, x := range X {
		sum := m.basePred
		for _, tree := range m.Trees {
			sum += m.LearningRate * m.predictTree(tree, x)
		}
		preds[i] = sum
	}
	return preds
}

func (m *GBMRegressor) predictTree(node *TreeNode, x []float64) float64 {
	if node.IsLeaf { return node.LeafValue }
	if x[node.FeatureIdx] <= node.Threshold {
		return m.predictTree(node.Left, x)
	}
	return m.predictTree(node.Right, x)
}

func (m *GBMRegressor) buildTree(X [][]float64, grad, hess []float64, sampleIdx []int, cols []int, depth int) *TreeNode {
	sumG := 0.0
	sumH := 0.0
	for _, i := range sampleIdx {
		sumG += grad[i]
		sumH += hess[i]
	}
	leafValue := -sumG / (sumH + m.Lambda)

	if depth >= m.MaxDepth || len(sampleIdx) < m.MinLeafSize*2 {
		return &TreeNode{LeafValue: leafValue, IsLeaf: true}
	}

	bestGain := 0.0
	bestFeat := -1
	bestThresh := 0.0
	bestLeftIdx := -1

	for _, f := range cols {
		type vi struct{ val float64; idx int }
		vals := make([]vi, len(sampleIdx))
		for k, i := range sampleIdx {
			vals[k] = vi{X[i][f], i}
		}
		sort.Slice(vals, func(i, j int) bool { return vals[i].val < vals[j].val })

		leftG := 0.0
		leftH := 0.0
		for k := 0; k < len(vals)-1; k++ {
			leftG += grad[vals[k].idx]
			leftH += hess[vals[k].idx]
			rightG := sumG - leftG
			rightH := sumH - leftH
			if k < len(vals)-1 && vals[k].val == vals[k+1].val { continue }
			if k+1 < m.MinLeafSize || len(vals)-k-1 < m.MinLeafSize { continue }
			gain := (leftG*leftG)/(leftH+m.Lambda) + (rightG*rightG)/(rightH+m.Lambda) - (sumG*sumG)/(sumH+m.Lambda) - m.Gamma
			if gain > bestGain {
				bestGain = gain
				bestFeat = f
				bestThresh = (vals[k].val + vals[k+1].val) / 2
				bestLeftIdx = k
			}
		}
	}

	if bestFeat < 0 || bestGain <= 0 {
		return &TreeNode{LeafValue: leafValue, IsLeaf: true}
	}

	if bestFeat < len(m.FeatureImport) {
		m.FeatureImport[bestFeat] += bestGain
	}

	type vi struct{ val float64; idx int }
	vals := make([]vi, len(sampleIdx))
	for k, i := range sampleIdx { vals[k] = vi{X[i][bestFeat], i} }
	sort.Slice(vals, func(i, j int) bool { return vals[i].val < vals[j].val })
	var leftIdx, rightIdx []int
	for k, v := range vals {
		if k <= bestLeftIdx { leftIdx = append(leftIdx, v.idx) } else { rightIdx = append(rightIdx, v.idx) }
	}

	return &TreeNode{
		FeatureIdx: bestFeat, Threshold: bestThresh,
		Left: m.buildTree(X, grad, hess, leftIdx, cols, depth+1),
		Right: m.buildTree(X, grad, hess, rightIdx, cols, depth+1),
		Gain: bestGain,
	}
}

func (m *GBMRegressor) GetFeatureImportance(nFeat int) []struct{ Index int; Importance float64 } {
	type imp struct{ Index int; Importance float64 }
	var items []imp
	for i := 0; i < nFeat; i++ { items = append(items, imp{i, m.FeatureImport[i]}) }
	sort.Slice(items, func(i, j int) bool { return items[i].Importance > items[j].Importance })
	result := make([]struct{ Index int; Importance float64 }, len(items))
	for i, it := range items { result[i] = struct{ Index int; Importance float64 }{it.Index, it.Importance} }
	return result
}

func (m *GBMRegressor) sampleColumns(nFeat int) []int {
	count := int(float64(nFeat) * m.ColSample)
	if count < 1 { count = 1 }
	allCols := make([]int, nFeat)
	for i := range allCols { allCols[i] = i }
	m.rng.Shuffle(nFeat, func(i, j int) { allCols[i], allCols[j] = allCols[j], allCols[i] })
	return allCols[:count]
}

// TreeNode 决策树节点
type TreeNode struct {
	FeatureIdx int
	Threshold  float64
	Left       *TreeNode
	Right      *TreeNode
	LeafValue  float64
	IsLeaf     bool
	Gain       float64 // 分裂增益
}

// GradientBoostingMachine 梯度提升机（XGBoost思想）
type GradientBoostingMachine struct {
	Trees          []*TreeNode
	MaxDepth       int
	NEstimators    int
	LearningRate   float64
	Lambda         float64
	Gamma          float64
	ColSample      float64
	SubSample      float64
	MinLeafSize    int
	FeatureImport  []float64 // 特征重要性
	TrainLosses    []float64 // 训练损失曲线
	ValLosses      []float64 // 验证损失曲线
	EarlyStopRounds int      // 早停轮数
	rng            *rand.Rand
}

type GBMConfig struct {
	MaxDepth       int
	NEstimators    int
	LearningRate   float64
	Lambda         float64
	Gamma          float64
	ColSample      float64
	SubSample      float64
	MinLeafSize    int
	EarlyStopRounds int
}

func DefaultGBMConfig() GBMConfig {
	return GBMConfig{
		MaxDepth:       3,
		NEstimators:    100,
		LearningRate:   0.1,
		Lambda:         1.0,
		Gamma:          0.1,
		ColSample:      0.8,
		SubSample:      0.8,
		MinLeafSize:    10,
		EarlyStopRounds: 30,
	}
}

func NewGBM(config GBMConfig) *GradientBoostingMachine {
	return &GradientBoostingMachine{
		MaxDepth:       config.MaxDepth,
		NEstimators:    config.NEstimators,
		LearningRate:   config.LearningRate,
		Lambda:         config.Lambda,
		Gamma:          config.Gamma,
		ColSample:      config.ColSample,
		SubSample:      config.SubSample,
		MinLeafSize:    config.MinLeafSize,
		EarlyStopRounds: config.EarlyStopRounds,
		rng:            rand.New(rand.NewSource(42)),
	}
}

// Fit 训练GBM模型（支持早停）
func (m *GradientBoostingMachine) Fit(X [][]float64, y []float64) {
	m.FitWithValidation(X, y, nil, nil)
}

// FitWithValidation 带验证集的训练（用于早停）
func (m *GradientBoostingMachine) FitWithValidation(trainX [][]float64, trainY []float64, valX [][]float64, valY []float64) {
	n := len(trainX)
	if n == 0 {
		return
	}
	nFeat := len(trainX[0])

	m.FeatureImport = make([]float64, nFeat)
	m.TrainLosses = nil
	m.ValLosses = nil

	// 初始预测值
	meanY := 0.0
	for _, v := range trainY {
		meanY += v
	}
	meanY /= float64(n)
	basePred := math.Log(meanY/(1-meanY+1e-10) + 1e-10)
	if math.IsNaN(basePred) || math.IsInf(basePred, 0) {
		basePred = 0
	}

	curPreds := make([]float64, n)
	for i := range curPreds {
		curPreds[i] = basePred
	}

	// 验证集增量预测
	var valPreds []float64
	hasVal := len(valX) > 0 && len(valY) > 0
	if hasVal {
		valPreds = make([]float64, len(valX))
		for i := range valPreds {
			valPreds[i] = basePred
		}
	}

	m.Trees = make([]*TreeNode, 0, m.NEstimators)
	bestValLoss := math.MaxFloat64
	noImproveCount := 0

	for t := 0; t < m.NEstimators; t++ {
		grad := make([]float64, n)
		hess := make([]float64, n)
		for i := 0; i < n; i++ {
			p := sigmoid(curPreds[i])
			grad[i] = p - trainY[i]
			hess[i] = math.Max(p*(1-p), 1e-6)
		}

		var sampleIdx []int
		if m.SubSample < 1.0 {
			sampleIdx = m.subsampleRows(n)
		} else {
			sampleIdx = make([]int, n)
			for i := range sampleIdx {
				sampleIdx[i] = i
			}
		}

		cols := m.sampleColumns(nFeat)
		tree := m.buildTree(trainX, grad, hess, sampleIdx, cols, 0)
		m.Trees = append(m.Trees, tree)

		// 增量更新训练和验证预测
		for i := 0; i < n; i++ {
			curPreds[i] += m.LearningRate * m.predictTree(tree, trainX[i])
		}
		if hasVal {
			for i := 0; i < len(valX); i++ {
				valPreds[i] += m.LearningRate * m.predictTree(tree, valX[i])
			}
		}

		// 训练损失
		trainLoss := 0.0
		for i := 0; i < n; i++ {
			p := math.Max(math.Min(sigmoid(curPreds[i]), 1-1e-7), 1e-7)
			trainLoss -= trainY[i]*math.Log(p) + (1-trainY[i])*math.Log(1-p)
		}
		m.TrainLosses = append(m.TrainLosses, trainLoss/float64(n))

		// 验证损失 + 早停
		if hasVal {
			valLoss := 0.0
			for i := 0; i < len(valX); i++ {
				p := math.Max(math.Min(sigmoid(valPreds[i]), 1-1e-7), 1e-7)
				valLoss -= valY[i]*math.Log(p) + (1-valY[i])*math.Log(1-p)
			}
			valLoss /= float64(len(valX))
			m.ValLosses = append(m.ValLosses, valLoss)

			if valLoss < bestValLoss-1e-6 {
				bestValLoss = valLoss
				noImproveCount = 0
			} else {
				noImproveCount++
			}

			if m.EarlyStopRounds > 0 && noImproveCount >= m.EarlyStopRounds && t > 20 {
				m.Trees = m.Trees[:len(m.Trees)-noImproveCount]
				fmt.Printf("  早停: 第%d轮, 最佳验证损失=%.6f\n", t-noImproveCount, bestValLoss)
				break
			}
		}
	}
}

// GetFeatureImportance 获取特征重要性（按增益）
func (m *GradientBoostingMachine) GetFeatureImportance(nFeat int) []struct {
	Index      int
	Importance float64
} {
	type imp struct {
		Index      int
		Importance float64
	}
	var items []imp
	for i := 0; i < nFeat; i++ {
		items = append(items, imp{i, m.FeatureImport[i]})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Importance > items[j].Importance
	})
	result := make([]struct {
		Index      int
		Importance float64
	}, len(items))
	for i, it := range items {
		result[i] = struct {
			Index      int
			Importance float64
		}{it.Index, it.Importance}
	}
	return result
}

func (m *GradientBoostingMachine) PredictProba(X [][]float64) []float64 {
	n := len(X)
	probs := make([]float64, n)
	meanY := 0.5
	basePred := math.Log(meanY / (1 - meanY))
	for i := 0; i < n; i++ {
		sum := basePred
		for _, tree := range m.Trees {
			sum += m.LearningRate * m.predictTree(tree, X[i])
		}
		probs[i] = sigmoid(sum)
	}
	return probs
}

func (m *GradientBoostingMachine) Predict(X [][]float64, threshold float64) []float64 {
	probs := m.PredictProba(X)
	preds := make([]float64, len(probs))
	for i, p := range probs {
		if p >= threshold {
			preds[i] = 1.0
		}
	}
	return preds
}

func (m *GradientBoostingMachine) predictTree(node *TreeNode, x []float64) float64 {
	if node.IsLeaf {
		return node.LeafValue
	}
	if x[node.FeatureIdx] <= node.Threshold {
		return m.predictTree(node.Left, x)
	}
	return m.predictTree(node.Right, x)
}

func (m *GradientBoostingMachine) buildTree(X [][]float64, grad, hess []float64, sampleIdx []int, cols []int, depth int) *TreeNode {
	sumG := 0.0
	sumH := 0.0
	for _, i := range sampleIdx {
		sumG += grad[i]
		sumH += hess[i]
	}
	leafValue := -sumG / (sumH + m.Lambda)

	if depth >= m.MaxDepth || len(sampleIdx) < m.MinLeafSize*2 {
		return &TreeNode{LeafValue: leafValue, IsLeaf: true}
	}

	bestGain := 0.0
	bestFeat := -1
	bestThresh := 0.0
	bestLeftIdx := -1

	for _, f := range cols {
		type valIdx struct {
			val float64
			idx int
		}
		vals := make([]valIdx, len(sampleIdx))
		for k, i := range sampleIdx {
			vals[k] = valIdx{X[i][f], i}
		}
		sort.Slice(vals, func(i, j int) bool { return vals[i].val < vals[j].val })

		leftG := 0.0
		leftH := 0.0
		for k := 0; k < len(vals)-1; k++ {
			i := vals[k].idx
			leftG += grad[i]
			leftH += hess[i]
			rightG := sumG - leftG
			rightH := sumH - leftH

			if k < len(vals)-1 && vals[k].val == vals[k+1].val {
				continue
			}
			if k+1 < m.MinLeafSize || len(vals)-k-1 < m.MinLeafSize {
				continue
			}

			gain := (leftG*leftG)/(leftH+m.Lambda) +
				(rightG*rightG)/(rightH+m.Lambda) -
				(sumG*sumG)/(sumH+m.Lambda) -
				m.Gamma

			if gain > bestGain {
				bestGain = gain
				bestFeat = f
				bestThresh = (vals[k].val + vals[k+1].val) / 2
				bestLeftIdx = k
			}
		}
	}

	if bestFeat < 0 || bestGain <= 0 {
		return &TreeNode{LeafValue: leafValue, IsLeaf: true}
	}

	// 记录特征重要性
	if bestFeat < len(m.FeatureImport) {
		m.FeatureImport[bestFeat] += bestGain
	}

	var leftIdx, rightIdx []int
	type valIdx struct {
		val float64
		idx int
	}
	vals := make([]valIdx, len(sampleIdx))
	for k, i := range sampleIdx {
		vals[k] = valIdx{X[i][bestFeat], i}
	}
	sort.Slice(vals, func(i, j int) bool { return vals[i].val < vals[j].val })
	for k, vi := range vals {
		if k <= bestLeftIdx {
			leftIdx = append(leftIdx, vi.idx)
		} else {
			rightIdx = append(rightIdx, vi.idx)
		}
	}

	left := m.buildTree(X, grad, hess, leftIdx, cols, depth+1)
	right := m.buildTree(X, grad, hess, rightIdx, cols, depth+1)

	return &TreeNode{
		FeatureIdx: bestFeat,
		Threshold:  bestThresh,
		Left:       left,
		Right:      right,
		Gain:       bestGain,
	}
}

func (m *GradientBoostingMachine) subsampleRows(n int) []int {
	count := int(float64(n) * m.SubSample)
	if count < 1 {
		count = 1
	}
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	m.rng.Shuffle(n, func(i, j int) { idx[i], idx[j] = idx[j], idx[i] })
	return idx[:count]
}

func (m *GradientBoostingMachine) sampleColumns(nFeat int) []int {
	count := int(float64(nFeat) * m.ColSample)
	if count < 1 {
		count = 1
	}
	allCols := make([]int, nFeat)
	for i := range allCols {
		allCols[i] = i
	}
	m.rng.Shuffle(nFeat, func(i, j int) { allCols[i], allCols[j] = allCols[j], allCols[i] })
	return allCols[:count]
}

// --- 模型序列化/反序列化 ---

// GBMRegressorSnapshot GBM回归器快照
type GBMRegressorSnapshot struct {
	Trees        []*TreeNode `json:"trees"`
	BasePred     float64     `json:"base_pred"`
	LearningRate float64     `json:"learning_rate"`
	FeatureCount int         `json:"feature_count"`
}

// Snapshot 导出模型快照
func (m *GBMRegressor) Snapshot() *GBMRegressorSnapshot {
	return &GBMRegressorSnapshot{
		Trees:        m.Trees,
		BasePred:     m.basePred,
		LearningRate: m.LearningRate,
		FeatureCount: len(m.FeatureImport),
	}
}

// LoadSnapshot 从快照恢复模型
func (m *GBMRegressor) LoadSnapshot(snap *GBMRegressorSnapshot) {
	m.Trees = snap.Trees
	m.basePred = snap.BasePred
	m.LearningRate = snap.LearningRate
	m.FeatureImport = make([]float64, snap.FeatureCount)
}

// ScalerSnapshot 标准化器快照
type ScalerSnapshot struct {
	Means []float64 `json:"means"`
	Stds  []float64 `json:"stds"`
}

// Snapshot 导出标准化器快照
func (s *StandardScaler) Snapshot() *ScalerSnapshot {
	return &ScalerSnapshot{Means: s.Means, Stds: s.Stds}
}

// LoadSnapshot 从快照恢复标准化器
func (s *StandardScaler) LoadSnapshot(snap *ScalerSnapshot) {
	s.Means = snap.Means
	s.Stds = snap.Stds
}
