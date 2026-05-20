package backtest

import (
	"math"

	"github.com/run-bigpig/jcp/internal/models"
)

// ComputeFeatures 计算扩展技术指标特征矩阵（40+个特征）
func ComputeFeatures(klines []models.KLineData, warmupDays int) [][]float64 {
	n := len(klines)
	if n <= warmupDays {
		return nil
	}

	closes := extractCloses(klines)
	opens := extractOpens(klines)
	highs := extractHighs(klines)
	lows := extractLows(klines)
	volumes := extractVolumes(klines)
	amounts := extractAmounts(klines)
	volF := float64Slice(volumes)

	// 均线
	ma5 := SMA(closes, 5)
	ma10 := SMA(closes, 10)
	ma20 := SMA(closes, 20)
	ma60 := SMA(closes, 60)

	// RSI
	rsi6 := RSI(closes, 6)
	rsi14 := RSI(closes, 14)

	// MACD
	dif, dea, macdHist := MACD(closes, 12, 26, 9)

	// 布林带
	upperBand, lowerBand := BollingerBands(closes, 20, 2.0)

	// ATR
	atr14 := ATR(highs, lows, closes, 14)

	// KDJ
	kVal, dVal, jVal := KDJ(highs, lows, closes, 9, 3, 3)

	// Williams %R
	wr14 := WilliamsR(highs, lows, closes, 14)

	// OBV
	obv := OBV(closes, volumes)
	obvMA5 := SMA(obv, 5)

	// 成交量均线
	volMA5 := SMA(volF, 5)
	volMA10 := SMA(volF, 10)
	_ = SMA(volF, 20) // volMA20 reserved

	// 多周期收益率
	ret3 := PeriodReturn(closes, 3)
	ret5 := PeriodReturn(closes, 5)
	ret10 := PeriodReturn(closes, 10)
	ret20 := PeriodReturn(closes, 20)

	// 历史波动率
	vol5 := HistoricalVolatility(closes, 5)
	vol10 := HistoricalVolatility(closes, 10)
	vol20 := HistoricalVolatility(closes, 20)

	var allFeatures [][]float64
	for i := warmupDays; i < n; i++ {
		var feat []float64

		// === 价格基础特征 ===
		// 日收益率
		ret := 0.0
		if closes[i-1] > 0 {
			ret = (closes[i] - closes[i-1]) / closes[i-1]
		}
		feat = append(feat, ret)

		// 振幅
		amp := 0.0
		if closes[i-1] > 0 {
			amp = (highs[i] - lows[i]) / closes[i-1]
		}
		feat = append(feat, amp)

		// 实体比
		body := math.Abs(closes[i] - opens[i])
		rng := highs[i] - lows[i]
		bodyRatio := 0.0
		if rng > 0 {
			bodyRatio = body / rng
		}
		feat = append(feat, bodyRatio)

		// 上影线比
		upperShadow := highs[i] - math.Max(opens[i], closes[i])
		upperShadowRatio := 0.0
		if rng > 0 {
			upperShadowRatio = upperShadow / rng
		}
		feat = append(feat, upperShadowRatio)

		// 下影线比
		lowerShadow := math.Min(opens[i], closes[i]) - lows[i]
		lowerShadowRatio := 0.0
		if rng > 0 {
			lowerShadowRatio = lowerShadow / rng
		}
		feat = append(feat, lowerShadowRatio)

		// 缺口
		gap := 0.0
		if i > 0 && closes[i-1] > 0 {
			gap = (opens[i] - closes[i-1]) / closes[i-1]
		}
		feat = append(feat, gap)

		// === 均线特征 ===
		feat = append(feat, safeDiv(closes[i]-ma5[i], ma5[i]))   // MA5偏离
		feat = append(feat, safeDiv(closes[i]-ma10[i], ma10[i])) // MA10偏离
		feat = append(feat, safeDiv(closes[i]-ma20[i], ma20[i])) // MA20偏离
		feat = append(feat, safeDiv(closes[i]-ma60[i], ma60[i])) // MA60偏离

		// 均线交叉信号
		ma5Cross10 := 0.0
		if i > 0 {
			prev := ma5[i-1] - ma10[i-1]
			curr := ma5[i] - ma10[i]
			if prev <= 0 && curr > 0 {
				ma5Cross10 = 1.0 // 金叉
			} else if prev >= 0 && curr < 0 {
				ma5Cross10 = -1.0 // 死叉
			}
		}
		feat = append(feat, ma5Cross10)

		ma5Cross20 := 0.0
		if i > 0 {
			prev := ma5[i-1] - ma20[i-1]
			curr := ma5[i] - ma20[i]
			if prev <= 0 && curr > 0 {
				ma5Cross20 = 1.0
			} else if prev >= 0 && curr < 0 {
				ma5Cross20 = -1.0
			}
		}
		feat = append(feat, ma5Cross20)

		// 均线斜率
		if i >= 5 {
			feat = append(feat, safeDiv(ma5[i]-ma5[i-5], ma5[i-5]))
		} else {
			feat = append(feat, 0)
		}

		// === 动量指标 ===
		feat = append(feat, rsi6[i]/100.0)  // RSI6归一化
		feat = append(feat, rsi14[i]/100.0) // RSI14归一化

		// RSI背离（价格新高但RSI未新高）
		rsiDiv := 0.0
		if i >= 10 {
			priceNewHigh := closes[i] >= maxInWindow(closes, i-10, i)
			rsiNewHigh := rsi14[i] >= maxInWindow(rsi14, i-10, i)
			if priceNewHigh && !rsiNewHigh {
				rsiDiv = -1.0 // 顶背离
			}
			priceNewLow := closes[i] <= minInWindow(closes, i-10, i)
			rsiNewLow := rsi14[i] <= minInWindow(rsi14, i-10, i)
			if priceNewLow && !rsiNewLow {
				rsiDiv = 1.0 // 底背离
			}
		}
		feat = append(feat, rsiDiv)

		// MACD
		if closes[i] > 0 {
			feat = append(feat, macdHist[i]/closes[i]*100) // MACD柱归一化
		} else {
			feat = append(feat, 0)
		}

		// MACD交叉
		macdCross := 0.0
		if i > 0 {
			prev := dif[i-1] - dea[i-1]
			curr := dif[i] - dea[i]
			if prev <= 0 && curr > 0 {
				macdCross = 1.0
			} else if prev >= 0 && curr < 0 {
				macdCross = -1.0
			}
		}
		feat = append(feat, macdCross)

		// MACD趋势（DIF方向）
		macdTrend := 0.0
		if i > 0 {
			macdTrend = math.Copysign(1, dif[i]-dif[i-1])
		}
		feat = append(feat, macdTrend)

		// === KDJ指标 ===
		feat = append(feat, kVal[i]/100.0)
		feat = append(feat, dVal[i]/100.0)
		feat = append(feat, jVal[i]/100.0)

		// KDJ交叉
		kdjCross := 0.0
		if i > 0 {
			prev := kVal[i-1] - dVal[i-1]
			curr := kVal[i] - dVal[i]
			if prev <= 0 && curr > 0 && kVal[i] < 30 {
				kdjCross = 1.0 // 低位金叉
			} else if prev >= 0 && curr < 0 && kVal[i] > 70 {
				kdjCross = -1.0 // 高位死叉
			}
		}
		feat = append(feat, kdjCross)

		// === Williams %R ===
		feat = append(feat, wr14[i]/100.0)

		// === 布林带 ===
		bandWidth := upperBand[i] - lowerBand[i]
		bbPos := 0.0
		if bandWidth > 0 {
			bbPos = (closes[i] - lowerBand[i]) / bandWidth
		}
		feat = append(feat, bbPos)

		bbWidth := 0.0
		if ma20[i] > 0 {
			bbWidth = bandWidth / ma20[i]
		}
		feat = append(feat, bbWidth)

		// === 波动率 ===
		feat = append(feat, atr14[i]/math.Max(closes[i], 1)) // ATR比值
		feat = append(feat, vol5[i])
		feat = append(feat, vol10[i])
		feat = append(feat, vol20[i])

		// 波动率变化
		volChange := 0.0
		if i >= 5 && vol5[i-5] > 0 {
			volChange = (vol5[i] - vol5[i-5]) / vol5[i-5]
		}
		feat = append(feat, volChange)

		// === 多周期收益率 ===
		feat = append(feat, ret3[i])
		feat = append(feat, ret5[i])
		feat = append(feat, ret10[i])
		feat = append(feat, ret20[i])

		// 收益率加速度
		retAccel := 0.0
		if i >= 5 {
			retAccel = ret5[i] - ret5[i-5]
		}
		feat = append(feat, retAccel)

		// === 成交量特征 ===
		volRatio5 := 0.0
		if volMA5[i] > 0 {
			volRatio5 = float64(volumes[i]) / volMA5[i]
		}
		feat = append(feat, volRatio5)

		volRatio10 := 0.0
		if volMA10[i] > 0 {
			volRatio10 = float64(volumes[i]) / volMA10[i]
		}
		feat = append(feat, volRatio10)

		// 成交额变化
		amtChange := 0.0
		if i > 0 && amounts[i-1] > 0 {
			amtChange = (amounts[i] - amounts[i-1]) / amounts[i-1]
		}
		feat = append(feat, amtChange)

		// 量价背离（价涨量缩 or 价跌量增）
		vpDiv := 0.0
		if i >= 5 {
			priceUp := ret5[i] > 0
			volDown := volRatio5 < 0.8
			if priceUp && volDown {
				vpDiv = -1.0 // 顶背离
			}
			priceDown := ret5[i] < 0
			volUp := volRatio5 > 1.2
			if priceDown && volUp {
				vpDiv = 1.0 // 底背离
			}
		}
		feat = append(feat, vpDiv)

		// OBV趋势
		obvTrend := 0.0
		if obvMA5[i] > 0 {
			obvTrend = (obv[i] - obvMA5[i]) / math.Abs(obvMA5[i])
		}
		feat = append(feat, clamp(obvTrend, -2, 2))

		// === 价格位置 ===
		// 20日价格位置
		pos20 := pricePosition(closes, i, 20)
		feat = append(feat, pos20)

		// 60日价格位置
		pos60 := pricePosition(closes, i, 60)
		feat = append(feat, pos60)

		// 连涨/连跌天数
		streak := 0
		for j := i; j > 0 && j > i-10; j-- {
			if closes[j] > closes[j-1] {
				if streak >= 0 {
					streak++
				} else {
					break
				}
			} else if closes[j] < closes[j-1] {
				if streak <= 0 {
					streak--
				} else {
					break
				}
			} else {
				break
			}
		}
		feat = append(feat, float64(streak)/10.0) // 归一化

		allFeatures = append(allFeatures, feat)
	}

	return allFeatures
}

// ComputeLabels 计算标签：使用多日收益率 + 阈值过滤噪声
// threshold: 涨跌阈值，低于此值视为震荡（无效样本返回-1）
func ComputeLabels(klines []models.KLineData, warmupDays int, holdDays int, threshold float64) ([]float64, []bool) {
	n := len(klines)
	if n <= warmupDays+holdDays {
		return nil, nil
	}

	var labels []float64
	var valid []bool
	for i := warmupDays; i < n-holdDays; i++ {
		futureReturn := (klines[i+holdDays].Close - klines[i].Close) / klines[i].Close
		if math.Abs(futureReturn) < threshold {
			labels = append(labels, 0) // 标签值不重要
			valid = append(valid, false) // 无效样本，训练时跳过
		} else if futureReturn > 0 {
			labels = append(labels, 1.0)
			valid = append(valid, true)
		} else {
			labels = append(labels, 0.0)
			valid = append(valid, true)
		}
	}
	return labels, valid
}

// ComputeLabelsSimple 简单标签（次日涨跌，无过滤）
func ComputeLabelsSimple(klines []models.KLineData, warmupDays int) []float64 {
	n := len(klines)
	if n <= warmupDays+1 {
		return nil
	}
	labels := make([]float64, 0, n-warmupDays-1)
	for i := warmupDays; i < n-1; i++ {
		if klines[i+1].Close > klines[i].Close {
			labels = append(labels, 1.0)
		} else {
			labels = append(labels, 0.0)
		}
	}
	return labels
}

// StandardScaler 标准化器
type StandardScaler struct {
	Means []float64
	Stds  []float64
}

func (s *StandardScaler) Fit(data [][]float64) {
	if len(data) == 0 {
		return
	}
	nFeat := len(data[0])
	s.Means = make([]float64, nFeat)
	s.Stds = make([]float64, nFeat)

	for j := 0; j < nFeat; j++ {
		sum := 0.0
		for _, row := range data {
			sum += row[j]
		}
		s.Means[j] = sum / float64(len(data))

		sumSq := 0.0
		for _, row := range data {
			diff := row[j] - s.Means[j]
			sumSq += diff * diff
		}
		s.Stds[j] = math.Sqrt(sumSq / float64(len(data)))
		if s.Stds[j] < 1e-10 {
			s.Stds[j] = 1.0
		}
	}
}

func (s *StandardScaler) Transform(data [][]float64) [][]float64 {
	result := make([][]float64, len(data))
	for i, row := range data {
		newRow := make([]float64, len(row))
		for j, val := range row {
			newRow[j] = (val - s.Means[j]) / s.Stds[j]
		}
		result[i] = newRow
	}
	return result
}

// --- 技术指标计算函数 ---

func SMA(data []float64, period int) []float64 {
	n := len(data)
	result := make([]float64, n)
	sum := 0.0
	for i := 0; i < n; i++ {
		sum += data[i]
		if i >= period {
			sum -= data[i-period]
		}
		if i >= period-1 {
			result[i] = sum / float64(period)
		}
	}
	return result
}

func EMA(data []float64, period int) []float64 {
	n := len(data)
	result := make([]float64, n)
	k := 2.0 / float64(period+1)
	sum := 0.0
	for i := 0; i < period && i < n; i++ {
		sum += data[i]
	}
	if period <= n {
		result[period-1] = sum / float64(period)
	}
	for i := period; i < n; i++ {
		result[i] = data[i]*k + result[i-1]*(1-k)
	}
	return result
}

func RSI(closes []float64, period int) []float64 {
	n := len(closes)
	result := make([]float64, n)
	gains := make([]float64, n)
	losses := make([]float64, n)
	for i := 1; i < n; i++ {
		diff := closes[i] - closes[i-1]
		if diff > 0 {
			gains[i] = diff
		} else {
			losses[i] = -diff
		}
	}
	avgGain := SMA(gains, period)
	avgLoss := SMA(losses, period)
	for i := 0; i < n; i++ {
		if avgLoss[i] > 0 {
			rs := avgGain[i] / avgLoss[i]
			result[i] = 100 - 100/(1+rs)
		} else {
			result[i] = 100
		}
	}
	return result
}

func MACD(closes []float64, fast, slow, signal int) ([]float64, []float64, []float64) {
	emaFast := EMA(closes, fast)
	emaSlow := EMA(closes, slow)
	n := len(closes)
	dif := make([]float64, n)
	for i := 0; i < n; i++ {
		dif[i] = emaFast[i] - emaSlow[i]
	}
	dea := EMA(dif, signal)
	hist := make([]float64, n)
	for i := 0; i < n; i++ {
		hist[i] = dif[i] - dea[i]
	}
	return dif, dea, hist
}

func BollingerBands(closes []float64, period int, mult float64) ([]float64, []float64) {
	n := len(closes)
	upper := make([]float64, n)
	lower := make([]float64, n)
	mid := SMA(closes, period)
	for i := period - 1; i < n; i++ {
		sumSq := 0.0
		for j := i - period + 1; j <= i; j++ {
			diff := closes[j] - mid[i]
			sumSq += diff * diff
		}
		std := math.Sqrt(sumSq / float64(period))
		upper[i] = mid[i] + mult*std
		lower[i] = mid[i] - mult*std
	}
	return upper, lower
}

func ATR(highs, lows, closes []float64, period int) []float64 {
	n := len(closes)
	tr := make([]float64, n)
	for i := 0; i < n; i++ {
		if i == 0 {
			tr[i] = highs[i] - lows[i]
		} else {
			hl := highs[i] - lows[i]
			hc := math.Abs(highs[i] - closes[i-1])
			lc := math.Abs(lows[i] - closes[i-1])
			tr[i] = math.Max(hl, math.Max(hc, lc))
		}
	}
	return SMA(tr, period)
}

// KDJ 随机指标
func KDJ(highs, lows, closes []float64, n, m1, m2 int) ([]float64, []float64, []float64) {
	length := len(closes)
	rsv := make([]float64, length)
	k := make([]float64, length)
	d := make([]float64, length)
	j := make([]float64, length)

	for i := 0; i < length; i++ {
		start := i - n + 1
		if start < 0 {
			start = 0
		}
		highN := maxInWindow(highs, start, i)
		lowN := minInWindow(lows, start, i)
		if highN > lowN {
			rsv[i] = (closes[i] - lowN) / (highN - lowN) * 100
		}
	}

	// K = SMA(RSV, m1), D = SMA(K, m2)
	k[0] = 50
	d[0] = 50
	for i := 1; i < length; i++ {
		k[i] = (rsv[i] + float64(m1-1)*k[i-1]) / float64(m1)
		d[i] = (k[i] + float64(m2-1)*d[i-1]) / float64(m2)
		j[i] = 3*k[i] - 2*d[i]
	}
	return k, d, j
}

// WilliamsR 威廉指标
func WilliamsR(highs, lows, closes []float64, period int) []float64 {
	n := len(closes)
	result := make([]float64, n)
	for i := period - 1; i < n; i++ {
		highN := maxInWindow(highs, i-period+1, i)
		lowN := minInWindow(lows, i-period+1, i)
		if highN > lowN {
			result[i] = (highN - closes[i]) / (highN - lowN) * -100
		}
	}
	return result
}

// OBV 能量潮
func OBV(closes []float64, volumes []int64) []float64 {
	n := len(closes)
	obv := make([]float64, n)
	for i := 1; i < n; i++ {
		if closes[i] > closes[i-1] {
			obv[i] = obv[i-1] + float64(volumes[i])
		} else if closes[i] < closes[i-1] {
			obv[i] = obv[i-1] - float64(volumes[i])
		} else {
			obv[i] = obv[i-1]
		}
	}
	return obv
}

// PeriodReturn 多周期收益率
func PeriodReturn(closes []float64, period int) []float64 {
	n := len(closes)
	result := make([]float64, n)
	for i := period; i < n; i++ {
		if closes[i-period] > 0 {
			result[i] = (closes[i] - closes[i-period]) / closes[i-period]
		}
	}
	return result
}

// HistoricalVolatility 历史波动率（年化）
func HistoricalVolatility(closes []float64, period int) []float64 {
	n := len(closes)
	result := make([]float64, n)
	for i := period; i < n; i++ {
		sum := 0.0
		mean := 0.0
		for j := i - period + 1; j <= i; j++ {
			if closes[j-1] > 0 {
				r := math.Log(closes[j] / closes[j-1])
				mean += r
			}
		}
		mean /= float64(period)
		for j := i - period + 1; j <= i; j++ {
			if closes[j-1] > 0 {
				r := math.Log(closes[j] / closes[j-1])
				sum += (r - mean) * (r - mean)
			}
		}
		result[i] = math.Sqrt(sum / float64(period)) * math.Sqrt(252)
	}
	return result
}

// --- 辅助函数 ---

func extractCloses(klines []models.KLineData) []float64 {
	r := make([]float64, len(klines))
	for i, k := range klines {
		r[i] = k.Close
	}
	return r
}
func extractOpens(klines []models.KLineData) []float64 {
	r := make([]float64, len(klines))
	for i, k := range klines {
		r[i] = k.Open
	}
	return r
}
func extractHighs(klines []models.KLineData) []float64 {
	r := make([]float64, len(klines))
	for i, k := range klines {
		r[i] = k.High
	}
	return r
}
func extractLows(klines []models.KLineData) []float64 {
	r := make([]float64, len(klines))
	for i, k := range klines {
		r[i] = k.Low
	}
	return r
}
func extractVolumes(klines []models.KLineData) []int64 {
	r := make([]int64, len(klines))
	for i, k := range klines {
		r[i] = k.Volume
	}
	return r
}
func extractAmounts(klines []models.KLineData) []float64 {
	r := make([]float64, len(klines))
	for i, k := range klines {
		r[i] = k.Amount
	}
	return r
}
func float64Slice(ints []int64) []float64 {
	r := make([]float64, len(ints))
	for i, v := range ints {
		r[i] = float64(v)
	}
	return r
}

func safeDiv(a, b float64) float64 {
	if math.Abs(b) < 1e-10 {
		return 0
	}
	return a / b
}

func maxInWindow(data []float64, start, end int) float64 {
	if start < 0 {
		start = 0
	}
	if end >= len(data) {
		end = len(data) - 1
	}
	m := data[start]
	for i := start + 1; i <= end; i++ {
		if data[i] > m {
			m = data[i]
		}
	}
	return m
}

func minInWindow(data []float64, start, end int) float64 {
	if start < 0 {
		start = 0
	}
	if end >= len(data) {
		end = len(data) - 1
	}
	m := data[start]
	for i := start + 1; i <= end; i++ {
		if data[i] < m {
			m = data[i]
		}
	}
	return m
}

func pricePosition(closes []float64, i, period int) float64 {
	start := i - period + 1
	if start < 0 {
		start = 0
	}
	high := maxInWindow(closes, start, i)
	low := minInWindow(closes, start, i)
	if high > low {
		return (closes[i] - low) / (high - low)
	}
	return 0.5
}

func clamp(val, minVal, maxVal float64) float64 {
	if val < minVal {
		return minVal
	}
	if val > maxVal {
		return maxVal
	}
	return val
}
