package backtest

import (
	"math"
)

// LogisticRegression 逻辑回归模型
type LogisticRegression struct {
	Weights []float64
	Bias    float64
	LR      float64 // 学习率
	Lambda  float64 // L2正则化系数
	Iters   int     // 迭代次数
}

// NewLogisticRegression 创建逻辑回归模型
func NewLogisticRegression(lr, lambda float64, iters int) *LogisticRegression {
	return &LogisticRegression{
		LR:     lr,
		Lambda: lambda,
		Iters:  iters,
	}
}

// sigmoid 函数
func sigmoid(z float64) float64 {
	if z < -500 {
		return 0
	}
	if z > 500 {
		return 1
	}
	return 1.0 / (1.0 + math.Exp(-z))
}

// Fit 训练模型
func (m *LogisticRegression) Fit(X [][]float64, y []float64) {
	n := len(X)
	if n == 0 {
		return
	}
	nFeat := len(X[0])

	m.Weights = make([]float64, nFeat)
	m.Bias = 0.0

	for iter := 0; iter < m.Iters; iter++ {
		// 计算预测值
		preds := make([]float64, n)
		for i := 0; i < n; i++ {
			z := m.dot(X[i]) + m.Bias
			preds[i] = sigmoid(z)
		}

		// 计算梯度
		dw := make([]float64, nFeat)
		db := 0.0
		for i := 0; i < n; i++ {
			diff := preds[i] - y[i]
			for j := 0; j < nFeat; j++ {
				dw[j] += diff * X[i][j]
			}
			db += diff
		}

		// 平均梯度 + L2正则
		for j := 0; j < nFeat; j++ {
			dw[j] = dw[j]/float64(n) + m.Lambda*m.Weights[j]
		}
		db /= float64(n)

		// 更新权重
		for j := 0; j < nFeat; j++ {
			m.Weights[j] -= m.LR * dw[j]
		}
		m.Bias -= m.LR * db
	}
}

// PredictProba 预测概率
func (m *LogisticRegression) PredictProba(X [][]float64) []float64 {
	probs := make([]float64, len(X))
	for i, x := range X {
		z := m.dot(x) + m.Bias
		probs[i] = sigmoid(z)
	}
	return probs
}

// Predict 预测类别（0或1）
func (m *LogisticRegression) Predict(X [][]float64, threshold float64) []float64 {
	probs := m.PredictProba(X)
	preds := make([]float64, len(probs))
	for i, p := range probs {
		if p >= threshold {
			preds[i] = 1.0
		} else {
			preds[i] = 0.0
		}
	}
	return preds
}

// dot 计算权重与特征的点积
func (m *LogisticRegression) dot(x []float64) float64 {
	sum := 0.0
	for i, v := range x {
		if i < len(m.Weights) {
			sum += m.Weights[i] * v
		}
	}
	return sum
}

// Metrics 评估指标
type Metrics struct {
	Accuracy  float64
	Precision float64
	Recall    float64
	F1        float64
	TP        int
	FN        int
	FP        int
	TN        int
}

// Evaluate 计算分类指标
func Evaluate(yTrue, yPred []float64) Metrics {
	var tp, fn, fp, tn int
	for i := range yTrue {
		switch {
		case yTrue[i] == 1 && yPred[i] == 1:
			tp++
		case yTrue[i] == 1 && yPred[i] == 0:
			fn++
		case yTrue[i] == 0 && yPred[i] == 1:
			fp++
		case yTrue[i] == 0 && yPred[i] == 0:
			tn++
		}
	}

	total := float64(tp + fn + fp + tn)
	accuracy := 0.0
	if total > 0 {
		accuracy = float64(tp+tn) / total
	}

	precision := 0.0
	if tp+fp > 0 {
		precision = float64(tp) / float64(tp+fp)
	}

	recall := 0.0
	if tp+fn > 0 {
		recall = float64(tp) / float64(tp+fn)
	}

	f1 := 0.0
	if precision+recall > 0 {
		f1 = 2 * precision * recall / (precision + recall)
	}

	return Metrics{
		Accuracy:  accuracy,
		Precision: precision,
		Recall:    recall,
		F1:        f1,
		TP:        tp,
		FN:        fn,
		FP:        fp,
		TN:        tn,
	}
}
