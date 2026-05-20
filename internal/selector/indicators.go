package selector

import (
	"math"

	"github.com/run-bigpig/jcp/internal/models"
)

// KLineData K线数据别名
type KLineData = models.KLineData

// ComputeKDJ 计算KDJ指标
func ComputeKDJ(data []KLineData, n int) (K, D, J []float64) {
	length := len(data)
	if length == 0 {
		return nil, nil, nil
	}

	K = make([]float64, length)
	D = make([]float64, length)
	J = make([]float64, length)

	for i := 0; i < length; i++ {
		// 计算N日内的最低价和最高价
		start := i - n + 1
		if start < 0 {
			start = 0
		}

		lowN := math.MaxFloat64
		highN := -math.MaxFloat64
		for j := start; j <= i; j++ {
			if data[j].Low < lowN {
				lowN = data[j].Low
			}
			if data[j].High > highN {
				highN = data[j].High
			}
		}

		// RSV = (C - LLV(L,N)) / (HHV(H,N) - LLV(L,N)) * 100
		rsv := 50.0
		if highN-lowN > 1e-9 {
			rsv = (data[i].Close - lowN) / (highN - lowN) * 100
		}

		if i == 0 {
			K[i] = 50.0
			D[i] = 50.0
		} else {
			K[i] = 2.0/3.0*K[i-1] + 1.0/3.0*rsv
			D[i] = 2.0/3.0*D[i-1] + 1.0/3.0*K[i]
		}
		J[i] = 3*K[i] - 2*D[i]
	}

	return K, D, J
}

// ComputeBBI 计算BBI指标
func ComputeBBI(data []KLineData) []float64 {
	length := len(data)
	if length < 24 {
		return nil
	}

	closes := make([]float64, length)
	for i, d := range data {
		closes[i] = d.Close
	}

	ma3 := computeMA(closes, 3)
	ma6 := computeMA(closes, 6)
	ma12 := computeMA(closes, 12)
	ma24 := computeMA(closes, 24)

	bbi := make([]float64, length)
	for i := 0; i < length; i++ {
		if ma3[i] != 0 && ma6[i] != 0 && ma12[i] != 0 && ma24[i] != 0 {
			bbi[i] = (ma3[i] + ma6[i] + ma12[i] + ma24[i]) / 4
		}
	}

	return bbi
}

// ComputeRSV 计算RSV指标
func ComputeRSV(data []KLineData, n int) []float64 {
	length := len(data)
	if length == 0 {
		return nil
	}

	rsv := make([]float64, length)
	for i := 0; i < length; i++ {
		start := i - n + 1
		if start < 0 {
			start = 0
		}

		lowN := math.MaxFloat64
		highCloseN := -math.MaxFloat64
		for j := start; j <= i; j++ {
			if data[j].Low < lowN {
				lowN = data[j].Low
			}
			if data[j].Close > highCloseN {
				highCloseN = data[j].Close
			}
		}

		if highCloseN-lowN > 1e-9 {
			rsv[i] = (data[i].Close - lowN) / (highCloseN - lowN) * 100
		} else {
			rsv[i] = 50.0
		}
	}

	return rsv
}

// ComputeDIF 计算MACD的DIF值
func ComputeDIF(data []KLineData, fast, slow int) []float64 {
	length := len(data)
	if length == 0 {
		return nil
	}

	closes := make([]float64, length)
	for i, d := range data {
		closes[i] = d.Close
	}

	emaFast := computeEMA(closes, fast)
	emaSlow := computeEMA(closes, slow)

	dif := make([]float64, length)
	for i := 0; i < length; i++ {
		dif[i] = emaFast[i] - emaSlow[i]
	}

	return dif
}

// ComputeRSI 计算RSI指标
func ComputeRSI(data []KLineData, n int) []float64 {
	length := len(data)
	if length < 2 {
		return nil
	}

	rsi := make([]float64, length)
	up := make([]float64, length)
	down := make([]float64, length)

	for i := 1; i < length; i++ {
		diff := data[i].Close - data[i-1].Close
		if diff > 0 {
			up[i] = diff
		} else {
			down[i] = -diff
		}
	}

	// 使用Wilder's Smoothing (EMA with com=n-1)
	maUp := computeEMA(up, n)
	maDown := computeEMA(down, n)

	for i := 0; i < length; i++ {
		if maDown[i] > 1e-9 {
			rs := maUp[i] / maDown[i]
			rsi[i] = 100 - 100/(1+rs)
		} else {
			rsi[i] = 100
		}
	}

	return rsi
}

// BBIDerivUptrend 判断BBI是否整体上升
func BBIDerivUptrend(bbi []float64, minWindow, maxWindow int, qThreshold float64) bool {
	if len(bbi) < minWindow {
		return false
	}

	// 找到第一个非NaN值
	startIdx := 0
	for i, v := range bbi {
		if !math.IsNaN(v) && v != 0 {
			startIdx = i
			break
		}
	}

	validBBI := bbi[startIdx:]
	if len(validBBI) < minWindow {
		return false
	}

	longest := len(validBBI)
	if maxWindow > 0 && maxWindow < longest {
		longest = maxWindow
	}

	// 从最长窗口向下搜索
	for w := longest; w >= minWindow; w-- {
		seg := validBBI[len(validBBI)-w:]
		if len(seg) < 2 {
			continue
		}

		// 归一化
		norm := make([]float64, len(seg))
		base := seg[0]
		if base == 0 {
			continue
		}
		for i, v := range seg {
			norm[i] = v / base
		}

		// 计算一阶差分
		diffs := make([]float64, len(norm)-1)
		for i := 1; i < len(norm); i++ {
			diffs[i-1] = norm[i] - norm[i-1]
		}

		// 计算分位数
		if len(diffs) > 0 {
			qIdx := int(float64(len(diffs)) * qThreshold)
			if qIdx >= len(diffs) {
				qIdx = len(diffs) - 1
			}
			sortedDiffs := make([]float64, len(diffs))
			copy(sortedDiffs, diffs)
			quickSort(sortedDiffs, 0, len(sortedDiffs)-1)
			if sortedDiffs[qIdx] >= 0 {
				return true
			}
		}
	}

	return false
}

// FindPeaks 寻找波峰
func FindPeaks(data []float64, distance int) []int {
	if len(data) < 3 {
		return nil
	}

	var peaks []int
	for i := 1; i < len(data)-1; i++ {
		if data[i] > data[i-1] && data[i] > data[i+1] {
			// 检查距离约束
			if len(peaks) > 0 {
				lastPeak := peaks[len(peaks)-1]
				if i-lastPeak < distance {
					// 如果新峰更高，替换旧峰
					if data[i] > data[lastPeak] {
						peaks[len(peaks)-1] = i
					}
					continue
				}
			}
			peaks = append(peaks, i)
		}
	}

	return peaks
}

// computeMA 计算移动平均
func computeMA(data []float64, n int) []float64 {
	length := len(data)
	if length < n {
		return make([]float64, length)
	}

	ma := make([]float64, length)
	sum := 0.0
	for i := 0; i < n-1; i++ {
		sum += data[i]
		ma[i] = 0
	}

	for i := n - 1; i < length; i++ {
		sum += data[i]
		if i >= n {
			sum -= data[i-n]
		}
		ma[i] = sum / float64(n)
	}

	return ma
}

// computeEMA 计算指数移动平均
func computeEMA(data []float64, n int) []float64 {
	length := len(data)
	if length == 0 {
		return nil
	}

	ema := make([]float64, length)
	multiplier := 2.0 / float64(n+1)

	ema[0] = data[0]
	for i := 1; i < length; i++ {
		ema[i] = (data[i]-ema[i-1])*multiplier + ema[i-1]
	}

	return ema
}

// quickSort 快速排序
func quickSort(arr []float64, low, high int) {
	if low < high {
		pi := partition(arr, low, high)
		quickSort(arr, low, pi-1)
		quickSort(arr, pi+1, high)
	}
}

func partition(arr []float64, low, high int) int {
	pivot := arr[high]
	i := low - 1
	for j := low; j < high; j++ {
		if arr[j] < pivot {
			i++
			arr[i], arr[j] = arr[j], arr[i]
		}
	}
	arr[i+1], arr[high] = arr[high], arr[i+1]
	return i + 1
}

// Quantile 计算分位数
func Quantile(data []float64, q float64) float64 {
	if len(data) == 0 {
		return 0
	}

	sorted := make([]float64, len(data))
	copy(sorted, data)
	quickSort(sorted, 0, len(sorted)-1)

	idx := q * float64(len(sorted)-1)
	lower := int(idx)
	upper := lower + 1
	if upper >= len(sorted) {
		return sorted[len(sorted)-1]
	}

	frac := idx - float64(lower)
	return sorted[lower]*(1-frac) + sorted[upper]*frac
}
