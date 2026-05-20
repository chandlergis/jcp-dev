package backtest

import (
	"math"

	"github.com/run-bigpig/jcp/internal/models"
)

const exRightThreshold = 0.20

// FilterExRights 过滤除权除息导致的价格跳变，并做前复权调整
// 检测单日涨跌幅超过20%的异常点，在该点做前复权使价格序列连续
func FilterExRights(klines []models.KLineData) []models.KLineData {
	if len(klines) < 2 {
		return klines
	}

	// 从后往前扫描，找到第一个除权点
	// 然后从该点往前做前复权
	adjusted := make([]models.KLineData, len(klines))
	copy(adjusted, klines)

	// 多轮处理，可能有多次除权
	for {
		exIdx := findExRightIndex(adjusted)
		if exIdx < 0 {
			break
		}
		adjusted = forwardAdjust(adjusted, exIdx)
	}

	return adjusted
}

// findExRightIndex 找到第一个除权除息点的索引
func findExRightIndex(klines []models.KLineData) int {
	for i := 1; i < len(klines); i++ {
		preClose := klines[i-1].Close
		if preClose <= 0 {
			continue
		}
		changePct := math.Abs(klines[i].Close-preClose) / preClose
		if changePct > exRightThreshold {
			return i
		}
	}
	return -1
}

// forwardAdjust 对除权点之前的数据做前复权
// 使除权点前一天的收盘价 = 除权点当天的开盘价（或收盘价的合理衔接）
func forwardAdjust(klines []models.KLineData, exIdx int) []models.KLineData {
	if exIdx <= 0 || exIdx >= len(klines) {
		return klines
	}

	prevClose := klines[exIdx-1].Close
	currClose := klines[exIdx].Close
	if prevClose <= 0 {
		return klines
	}

	// 计算调整因子：使前复权后价格连续
	factor := currClose / prevClose

	// 对除权点之前（含前一天）的所有K线做前复权
	for i := 0; i < exIdx; i++ {
		klines[i].Open *= factor
		klines[i].High *= factor
		klines[i].Low *= factor
		klines[i].Close *= factor
		klines[i].Avg *= factor
		// 成交量反向调整（价格变低，成交量等比变大）
		if factor > 0 {
			klines[i].Volume = int64(float64(klines[i].Volume) / factor)
		}
	}

	return klines
}

// ComputeDailyReturns 计算日收益率序列
func ComputeDailyReturns(klines []models.KLineData) []float64 {
	if len(klines) < 2 {
		return nil
	}
	returns := make([]float64, len(klines)-1)
	for i := 1; i < len(klines); i++ {
		preClose := klines[i-1].Close
		if preClose > 0 {
			returns[i-1] = (klines[i].Close - preClose) / preClose
		}
	}
	return returns
}
