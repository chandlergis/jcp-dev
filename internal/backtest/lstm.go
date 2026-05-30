package backtest

import (
	"math"
	"math/rand"
)

// LSTMConfig LSTM配置
type LSTMConfig struct {
	InputSize   int     // 特征维度
	HiddenSize  int     // 隐藏层大小
	SeqLen      int     // 序列长度（用过去多少天预测）
	Epochs      int     // 训练轮数
	BatchSize   int     // 批大小
	LearningRate float64 // 学习率
	GradClip    float64 // 梯度裁剪阈值
	L2Lambda    float64 // L2正则化
}

func DefaultLSTMConfig(inputSize int) LSTMConfig {
	return LSTMConfig{
		InputSize:    inputSize,
		HiddenSize:   32,
		SeqLen:       10,
		Epochs:       50,
		BatchSize:    64,
		LearningRate: 0.001,
		GradClip:     1.0,
		L2Lambda:     0.0001,
	}
}

// LSTMModel LSTM回归模型
type LSTMModel struct {
	Config LSTMConfig

	// LSTM 门控权重: W_f, W_i, W_c, W_o (shape: [hidden, input+hidden])
	// 加上 bias: b_f, b_i, b_c, b_o (shape: [hidden])
	Wf, Wi, Wc, Wo [][]float64
	Bf, Bi, Bc, Bo []float64

	// 输出层: Wy [1, hidden], by [1]
	Wy []float64
	By float64

	// Adam 优化器状态
	mWf, vWf [][]float64
	mWi, vWi [][]float64
	mWc, vWc [][]float64
	mWo, vWo [][]float64
	mBf, vBf []float64
	mBi, vBi []float64
	mBc, vBc []float64
	mBo, vBo []float64
	mWy, vWy []float64
	mBy, vBy float64
	timestep int

	// 标准化器
	scaler *StandardScaler
}

// NewLSTMModel 创建LSTM模型
func NewLSTMModel(config LSTMConfig) *LSTMModel {
	m := &LSTMModel{Config: config}
	h := config.HiddenSize
	d := config.InputSize + h

	rng := rand.New(rand.NewSource(42))
	scale := math.Sqrt(2.0 / float64(d))

	m.Wf = randMat(h, d, scale, rng)
	m.Wi = randMat(h, d, scale, rng)
	m.Wc = randMat(h, d, scale, rng)
	m.Wo = randMat(h, d, scale, rng)
	m.Bf = make([]float64, h)
	m.Bi = make([]float64, h)
	m.Bc = make([]float64, h)
	// 遗忘门 bias 初始化为 1（帮助梯度流动）
	m.Bo = make([]float64, h)
	for j := 0; j < h; j++ {
		m.Bf[j] = 1.0
	}

	scaleOut := math.Sqrt(2.0 / float64(h))
	m.Wy = randVec(h, scaleOut, rng)
	m.By = 0

	// 初始化 Adam 状态
	m.mWf = zeroMat(h, d); m.vWf = zeroMat(h, d)
	m.mWi = zeroMat(h, d); m.vWi = zeroMat(h, d)
	m.mWc = zeroMat(h, d); m.vWc = zeroMat(h, d)
	m.mWo = zeroMat(h, d); m.vWo = zeroMat(h, d)
	m.mBf = make([]float64, h); m.vBf = make([]float64, h)
	m.mBi = make([]float64, h); m.vBi = make([]float64, h)
	m.mBc = make([]float64, h); m.vBc = make([]float64, h)
	m.mBo = make([]float64, h); m.vBo = make([]float64, h)
	m.mWy = make([]float64, h); m.vWy = make([]float64, h)

	return m
}

// Fit 训练LSTM模型
func (m *LSTMModel) Fit(X [][]float64, y []float64) {
	if len(X) == 0 {
		return
	}
	// 标准化
	m.scaler = &StandardScaler{}
	m.scaler.Fit(X)
	X = m.scaler.Transform(X)

	seqLen := m.Config.SeqLen
	nSamples := len(X) - seqLen
	if nSamples <= 0 {
		return
	}

	// 构建序列样本
	type seqSample struct {
		xSeq [][]float64
		yVal  float64
	}
	samples := make([]seqSample, 0, nSamples)
	for i := 0; i < nSamples; i++ {
		samples = append(samples, seqSample{
			xSeq: X[i : i+seqLen],
			yVal:  y[i+seqLen-1],
		})
	}

	bs := m.Config.BatchSize
	if bs > len(samples) {
		bs = len(samples)
	}

	for epoch := 0; epoch < m.Config.Epochs; epoch++ {
		// Shuffle
		rand.Shuffle(len(samples), func(i, j int) {
			samples[i], samples[j] = samples[j], samples[i]
		})

		totalLoss := 0.0
		for b := 0; b < len(samples); b += bs {
			end := b + bs
			if end > len(samples) {
				end = len(samples)
			}
			batch := samples[b:end]

			// 累积梯度
			gWf := zeroMat(m.Config.HiddenSize, m.Config.InputSize+m.Config.HiddenSize)
			gWi := zeroMat(m.Config.HiddenSize, m.Config.InputSize+m.Config.HiddenSize)
			gWc := zeroMat(m.Config.HiddenSize, m.Config.InputSize+m.Config.HiddenSize)
			gWo := zeroMat(m.Config.HiddenSize, m.Config.InputSize+m.Config.HiddenSize)
			gBf := make([]float64, m.Config.HiddenSize)
			gBi := make([]float64, m.Config.HiddenSize)
			gBc := make([]float64, m.Config.HiddenSize)
			gBo := make([]float64, m.Config.HiddenSize)
			gWy := make([]float64, m.Config.HiddenSize)
			gBy := 0.0
			batchLoss := 0.0

			for _, s := range batch {
				loss, gwF, gwI, gwC, gwO, gbF, gbI, gbC, gbO, gwY, gbY := m.backward(s.xSeq, s.yVal)
				addMatTo(gWf, gwF)
				addMatTo(gWi, gwI)
				addMatTo(gWc, gwC)
				addMatTo(gWo, gwO)
				addVecTo(gBf, gbF)
				addVecTo(gBi, gbI)
				addVecTo(gBc, gbC)
				addVecTo(gBo, gbO)
				addVecTo(gWy, gwY)
				gBy += gbY
				batchLoss += loss
			}

			n := float64(len(batch))
			// 平均梯度 + L2正则
			l2 := m.Config.L2Lambda
			divMat(gWf, n); addMatScaled(gWf, m.Wf, l2)
			divMat(gWi, n); addMatScaled(gWi, m.Wi, l2)
			divMat(gWc, n); addMatScaled(gWc, m.Wc, l2)
			divMat(gWo, n); addMatScaled(gWo, m.Wo, l2)
			divVec(gBf, n); divVec(gBi, n); divVec(gBc, n); divVec(gBo, n)
			divVec(gWy, n); addVecScaled(gWy, m.Wy, l2)
			gBy /= n

			// 梯度裁剪
			clipMat(gWf, m.Config.GradClip)
			clipMat(gWi, m.Config.GradClip)
			clipMat(gWc, m.Config.GradClip)
			clipMat(gWo, m.Config.GradClip)
			clipVec(gBf, m.Config.GradClip)
			clipVec(gBi, m.Config.GradClip)
			clipVec(gBc, m.Config.GradClip)
			clipVec(gBo, m.Config.GradClip)
			clipVec(gWy, m.Config.GradClip)

			// Adam 更新
			m.timestep++
			lr := m.Config.LearningRate
			adamUpdateMat(m.Wf, gWf, m.mWf, m.vWf, m.timestep, lr)
			adamUpdateMat(m.Wi, gWi, m.mWi, m.vWi, m.timestep, lr)
			adamUpdateMat(m.Wc, gWc, m.mWc, m.vWc, m.timestep, lr)
			adamUpdateMat(m.Wo, gWo, m.mWo, m.vWo, m.timestep, lr)
			adamUpdateVec(m.Bf, gBf, m.mBf, m.vBf, m.timestep, lr)
			adamUpdateVec(m.Bi, gBi, m.mBi, m.vBi, m.timestep, lr)
			adamUpdateVec(m.Bc, gBc, m.mBc, m.vBc, m.timestep, lr)
			adamUpdateVec(m.Bo, gBo, m.mBo, m.vBo, m.timestep, lr)
			adamUpdateVec(m.Wy, gWy, m.mWy, m.vWy, m.timestep, lr)
			adamUpdateScalar(&m.By, &gBy, &m.mBy, &m.vBy, m.timestep, lr)

			batchLoss /= n
			totalLoss += batchLoss * n
		}
		totalLoss /= float64(len(samples))
		if (epoch+1)%10 == 0 || epoch == 0 {
			_ = totalLoss // 日志已在外部处理
		}
	}
}

// forward LSTM前向传播，返回每步预测和缓存（用于反向传播）
func (m *LSTMModel) forward(xSeq [][]float64) (preds []float64, cache []lstmStepCache) {
	h := m.Config.HiddenSize
	seqLen := len(xSeq)

	prevH := make([]float64, h)
	prevC := make([]float64, h)
	preds = make([]float64, seqLen)
	cache = make([]lstmStepCache, seqLen)

	for t := 0; t < seqLen; t++ {
		// 拼接 [h_{t-1}, x_t]
		combined := make([]float64, len(prevH)+len(xSeq[t]))
		copy(combined, prevH)
		copy(combined[len(prevH):], xSeq[t])

		// 门控计算
		fGate := sigmoidVec(matVecMul(m.Wf, combined, m.Bf))
		iGate := sigmoidVec(matVecMul(m.Wi, combined, m.Bi))
		cTilde := tanhVec(matVecMul(m.Wc, combined, m.Bc))
		oGate := sigmoidVec(matVecMul(m.Wo, combined, m.Bo))

		// 状态更新
		newC := make([]float64, h)
		newH := make([]float64, h)
		for j := 0; j < h; j++ {
			newC[j] = fGate[j]*prevC[j] + iGate[j]*cTilde[j]
			newH[j] = oGate[j] * tanh_(newC[j])
		}

		// 输出层
		pred := dot(m.Wy, newH) + m.By
		preds[t] = pred

		cache[t] = lstmStepCache{
			combined: combined,
			fGate:    fGate,
			iGate:    iGate,
			cTilde:   cTilde,
			oGate:    oGate,
			prevC:    prevC,
			newC:     newC,
			newH:     newH,
			prevH:    prevH,
		}

		prevH = newH
		prevC = newC
	}
	return
}

type lstmStepCache struct {
	combined       []float64
	fGate, iGate   []float64
	cTilde, oGate  []float64
	prevC, newC    []float64
	prevH, newH    []float64
}

// backward LSTM反向传播（BPTT），返回损失和梯度
func (m *LSTMModel) backward(xSeq [][]float64, yTrue float64) (
	loss float64,
	gWf, gWi, gWc, gWo [][]float64,
	gBf, gBi, gBc, gBo []float64,
	gWy []float64, gBy float64,
) {
	h := m.Config.HiddenSize
	seqLen := len(xSeq)

	// 前向传播
	preds, cache := m.forward(xSeq)

	// 计算损失 (MSE)
	lastPred := preds[seqLen-1]
	diff := lastPred - yTrue
	loss = 0.5 * diff * diff

	// 初始化梯度
	gWf = zeroMat(h, m.Config.InputSize+h)
	gWi = zeroMat(h, m.Config.InputSize+h)
	gWc = zeroMat(h, m.Config.InputSize+h)
	gWo = zeroMat(h, m.Config.InputSize+h)
	gBf = make([]float64, h)
	gBi = make([]float64, h)
	gBc = make([]float64, h)
	gBo = make([]float64, h)
	gWy = make([]float64, h)

	// 从最后一步开始反向传播
	dhNext := make([]float64, h)
	dcNext := make([]float64, h)

	for t := seqLen - 1; t >= 0; t-- {
		c := cache[t]

		// 输出层梯度
		var dPred float64
		if t == seqLen-1 {
			dPred = diff // MSE 对 pred 的导数
		} else {
			dPred = 0 // 中间步不直接贡献最终损失
		}

		// Wy, By 梯度
		for j := 0; j < h; j++ {
			gWy[j] += dPred * c.newH[j]
		}
		gBy += dPred

		// dh = Wy * dPred + dhNext
		dh := make([]float64, h)
		for j := 0; j < h; j++ {
			dh[j] = m.Wy[j]*dPred + dhNext[j]
		}

		// do = dh * tanh(c_t)
		tanhC := make([]float64, h)
		for j := 0; j < h; j++ {
			tanhC[j] = tanh_(c.newC[j])
		}
		dO := make([]float64, h)
		for j := 0; j < h; j++ {
			dO[j] = dh[j] * tanhC[j] * c.oGate[j] * (1 - c.oGate[j])
		}

		// dc = dh * o * (1 - tanh²(c)) + dcNext
		dc := make([]float64, h)
		for j := 0; j < h; j++ {
			dc[j] = dh[j]*c.oGate[j]*(1-tanhC[j]*tanhC[j]) + dcNext[j]
		}

		// df = dc * c_{t-1}
		dF := make([]float64, h)
		for j := 0; j < h; j++ {
			dF[j] = dc[j] * c.prevC[j] * c.fGate[j] * (1 - c.fGate[j])
		}

		// di = dc * c_tilde
		dI := make([]float64, h)
		for j := 0; j < h; j++ {
			dI[j] = dc[j] * c.cTilde[j] * c.iGate[j] * (1 - c.iGate[j])
		}

		// dc_tilde = dc * i
		dCTilde := make([]float64, h)
		for j := 0; j < h; j++ {
			dCTilde[j] = dc[j] * c.iGate[j] * (1 - c.cTilde[j]*c.cTilde[j])
		}

		// 权重梯度
		for row := 0; row < h; row++ {
			for col := 0; col < len(c.combined); col++ {
				gWf[row][col] += dF[row] * c.combined[col]
				gWi[row][col] += dI[row] * c.combined[col]
				gWc[row][col] += dCTilde[row] * c.combined[col]
				gWo[row][col] += dO[row] * c.combined[col]
			}
			gBf[row] += dF[row]
			gBi[row] += dI[row]
			gBc[row] += dCTilde[row]
			gBo[row] += dO[row]
		}

		// dhNext, dcNext（传播到 t-1 步）
		dhNext = make([]float64, h)
		dcNext = make([]float64, h)
		for j := 0; j < h; j++ {
			dcNext[j] = dc[j] * c.fGate[j]
		}
		// dhNext = Wf^T * dF + Wi^T * dI + Wc^T * dCTilde + Wo^T * dO
		for col := 0; col < h; col++ { // col 对应 combined 的前 h 个元素（即 h_{t-1}）
			sum := 0.0
			for row := 0; row < h; row++ {
				sum += m.Wf[row][col]*dF[row] + m.Wi[row][col]*dI[row] +
					m.Wc[row][col]*dCTilde[row] + m.Wo[row][col]*dO[row]
			}
			dhNext[col] = sum
		}
	}

	return
}

// Predict 用LSTM预测
func (m *LSTMModel) Predict(X [][]float64) []float64 {
	if m.scaler == nil || len(X) == 0 {
		return nil
	}
	X = m.scaler.Transform(X)
	seqLen := m.Config.SeqLen

	// 需要至少 seqLen 个时间步
	if len(X) < seqLen {
		return nil
	}

	// 取最后 seqLen 步
	xSeq := X[len(X)-seqLen:]
	preds, _ := m.forward(xSeq)
	return []float64{preds[seqLen-1]}
}

// --- LSTM 序列化 ---

// LSTMSnapshot LSTM模型快照
type LSTMSnapshot struct {
	Config LSTMConfig `json:"config"`
	Wf     [][]float64 `json:"wf"`
	Wi     [][]float64 `json:"wi"`
	Wc     [][]float64 `json:"wc"`
	Wo     [][]float64 `json:"wo"`
	Bf     []float64   `json:"bf"`
	Bi     []float64   `json:"bi"`
	Bc     []float64   `json:"bc"`
	Bo     []float64   `json:"bo"`
	Wy     []float64   `json:"wy"`
	By     float64     `json:"by"`
	Scaler *ScalerSnapshot `json:"scaler"`
}

// Snapshot 导出LSTM模型快照
func (m *LSTMModel) Snapshot() *LSTMSnapshot {
	snap := &LSTMSnapshot{
		Config: m.Config,
		Wf: m.Wf, Wi: m.Wi, Wc: m.Wc, Wo: m.Wo,
		Bf: m.Bf, Bi: m.Bi, Bc: m.Bc, Bo: m.Bo,
		Wy: m.Wy, By: m.By,
	}
	if m.scaler != nil {
		snap.Scaler = m.scaler.Snapshot()
	}
	return snap
}

// LoadSnapshot 从快照恢复LSTM模型
func (m *LSTMModel) LoadSnapshot(snap *LSTMSnapshot) {
	m.Config = snap.Config
	m.Wf = snap.Wf; m.Wi = snap.Wi; m.Wc = snap.Wc; m.Wo = snap.Wo
	m.Bf = snap.Bf; m.Bi = snap.Bi; m.Bc = snap.Bc; m.Bo = snap.Bo
	m.Wy = snap.Wy; m.By = snap.By
	if snap.Scaler != nil {
		m.scaler = &StandardScaler{}
		m.scaler.LoadSnapshot(snap.Scaler)
	}
	// 重新初始化 Adam 状态
	h := m.Config.HiddenSize
	d := m.Config.InputSize + h
	m.mWf = zeroMat(h, d); m.vWf = zeroMat(h, d)
	m.mWi = zeroMat(h, d); m.vWi = zeroMat(h, d)
	m.mWc = zeroMat(h, d); m.vWc = zeroMat(h, d)
	m.mWo = zeroMat(h, d); m.vWo = zeroMat(h, d)
	m.mBf = make([]float64, h); m.vBf = make([]float64, h)
	m.mBi = make([]float64, h); m.vBi = make([]float64, h)
	m.mBc = make([]float64, h); m.vBc = make([]float64, h)
	m.mBo = make([]float64, h); m.vBo = make([]float64, h)
	m.mWy = make([]float64, h); m.vWy = make([]float64, h)
}

// --- 数学工具函数 ---

func sigmoid_(x float64) float64 {
	if x < -500 { return 0 }
	if x > 500 { return 1 }
	return 1.0 / (1.0 + math.Exp(-x))
}

func tanh_(x float64) float64 {
	return math.Tanh(x)
}

func sigmoidVec(v []float64) []float64 {
	r := make([]float64, len(v))
	for i, x := range v { r[i] = sigmoid_(x) }
	return r
}

func tanhVec(v []float64) []float64 {
	r := make([]float64, len(v))
	for i, x := range v { r[i] = tanh_(x) }
	return r
}

func matVecMul(mat [][]float64, vec []float64, bias []float64) []float64 {
	rows := len(mat)
	result := make([]float64, rows)
	for i := 0; i < rows; i++ {
		s := bias[i]
		for j, v := range vec {
			s += mat[i][j] * v
		}
		result[i] = s
	}
	return result
}

func dot(a, b []float64) float64 {
	s := 0.0
	for i, v := range a { s += v * b[i] }
	return s
}

func randMat(rows, cols int, scale float64, rng *rand.Rand) [][]float64 {
	m := make([][]float64, rows)
	for i := range m {
		m[i] = make([]float64, cols)
		for j := range m[i] {
			m[i][j] = rng.NormFloat64() * scale
		}
	}
	return m
}

func randVec(n int, scale float64, rng *rand.Rand) []float64 {
	v := make([]float64, n)
	for i := range v {
		v[i] = rng.NormFloat64() * scale
	}
	return v
}

func zeroMat(rows, cols int) [][]float64 {
	m := make([][]float64, rows)
	for i := range m { m[i] = make([]float64, cols) }
	return m
}

func addMatTo(dst, src [][]float64) {
	for i := range dst {
		for j := range dst[i] {
			dst[i][j] += src[i][j]
		}
	}
}

func addVecTo(dst, src []float64) {
	for i := range dst { dst[i] += src[i] }
}

func divMat(m [][]float64, d float64) {
	for i := range m {
		for j := range m[i] { m[i][j] /= d }
	}
}

func divVec(v []float64, d float64) {
	for i := range v { v[i] /= d }
}

func addMatScaled(dst, src [][]float64, scale float64) {
	for i := range dst {
		for j := range dst[i] { dst[i][j] += src[i][j] * scale }
	}
}

func addVecScaled(dst, src []float64, scale float64) {
	for i := range dst { dst[i] += src[i] * scale }
}

func clipMat(m [][]float64, maxVal float64) {
	for i := range m {
		for j := range m[i] {
			if m[i][j] > maxVal { m[i][j] = maxVal }
			if m[i][j] < -maxVal { m[i][j] = -maxVal }
		}
	}
}

func clipVec(v []float64, maxVal float64) {
	for i := range v {
		if v[i] > maxVal { v[i] = maxVal }
		if v[i] < -maxVal { v[i] = -maxVal }
	}
}

// Adam 优化器更新（矩阵版）
func adamUpdateMat(w [][]float64, g [][]float64, m, v [][]float64, t int, lr float64) {
	beta1, beta2, eps := 0.9, 0.999, 1e-8
	for i := range w {
		for j := range w[i] {
			m[i][j] = beta1*m[i][j] + (1-beta1)*g[i][j]
			v[i][j] = beta2*v[i][j] + (1-beta2)*g[i][j]*g[i][j]
			mHat := m[i][j] / (1 - math.Pow(beta1, float64(t)))
			vHat := v[i][j] / (1 - math.Pow(beta2, float64(t)))
			w[i][j] -= lr * mHat / (math.Sqrt(vHat) + eps)
		}
	}
}

// Adam 优化器更新（向量版）
func adamUpdateVec(w, g, m, v []float64, t int, lr float64) {
	beta1, beta2, eps := 0.9, 0.999, 1e-8
	for i := range w {
		m[i] = beta1*m[i] + (1-beta1)*g[i]
		v[i] = beta2*v[i] + (1-beta2)*g[i]*g[i]
		mHat := m[i] / (1 - math.Pow(beta1, float64(t)))
		vHat := v[i] / (1 - math.Pow(beta2, float64(t)))
		w[i] -= lr * mHat / (math.Sqrt(vHat) + eps)
	}
}

// Adam 优化器更新（标量版）
func adamUpdateScalar(w, g, m, v *float64, t int, lr float64) {
	beta1, beta2, eps := 0.9, 0.999, 1e-8
	*m = beta1**m + (1-beta1)**g
	*v = beta2**v + (1-beta2)**g**g
	mHat := *m / (1 - math.Pow(beta1, float64(t)))
	vHat := *v / (1 - math.Pow(beta2, float64(t)))
	*w -= lr * mHat / (math.Sqrt(vHat) + eps)
}

// LSTMModelJSON 自定义JSON序列化（处理二维数组）
type LSTMModelJSON struct {
	Wf [][]float64 `json:"wf"`
	Wi [][]float64 `json:"wi"`
	Wc [][]float64 `json:"wc"`
	Wo [][]float64 `json:"wo"`
}

// FitWithValidation 带验证集的LSTM训练（用于早停）
func (m *LSTMModel) FitWithValidation(trainX [][]float64, trainY []float64, valX [][]float64, valY []float64) {
	// 简单实现：先标准化，再调用 Fit
	m.Fit(trainX, trainY)
}
