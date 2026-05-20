package selector

import (
	"fmt"
	"math"
	"strings"
)

// Strategy 选股策略接口
type Strategy interface {
	// Name 策略名称
	Name() string
	// Select 执行选股，返回选中的股票代码列表
	Select(data []KLineData) bool
	// Score 计算得分，返回0-100的分数和得分详情
	Score(data []KLineData) (float64, string)
}

// BBIKDJSelector BBI+KDJ选股策略
type BBIKDJSelector struct {
	JThreshold     float64
	BBIMinWindow   int
	MaxWindow      int
	PriceRangePct  float64
	BBIQThreshold  float64
	JQThreshold    float64
}

func NewBBIKDJSelector() *BBIKDJSelector {
	return &BBIKDJSelector{
		JThreshold:    15,
		BBIMinWindow:  30,
		MaxWindow:     60,
		PriceRangePct: 150.0,
		BBIQThreshold: 0.10,
		JQThreshold:   0.25,
	}
}

func (s *BBIKDJSelector) Name() string {
	return "BBI+KDJ选股"
}

func (s *BBIKDJSelector) Select(data []KLineData) bool {
	if len(data) < s.MaxWindow+20 {
		return false
	}

	hist := data[len(data)-s.MaxWindow-20:]
	
	// 计算BBI
	bbi := ComputeBBI(hist)
	if bbi == nil {
		return false
	}

	// 0. 收盘价波动幅度约束
	win := hist[len(hist)-s.MaxWindow:]
	high, low := findMaxMin(win)
	if low <= 0 || (high/low-1) > s.PriceRangePct {
		return false
	}

	// 1. BBI上升（允许部分回撤）
	if !BBIDerivUptrend(bbi, s.BBIMinWindow, s.MaxWindow, s.BBIQThreshold) {
		return false
	}

	// 2. KDJ过滤
	_, _, j := ComputeKDJ(hist, 9)
	if len(j) == 0 {
		return false
	}
	jToday := j[len(j)-1]

	// 最近MaxWindow根K线的J分位
	jWindow := j[len(j)-s.MaxWindow:]
	jQuantile := Quantile(jWindow, s.JQThreshold)

	if !(jToday < s.JThreshold || jToday <= jQuantile) {
		return false
	}

	// 3. MACD：DIF > 0
	dif := ComputeDIF(hist, 12, 26)
	if len(dif) == 0 {
		return false
	}
	return dif[len(dif)-1] > 0
}

func (s *BBIKDJSelector) Score(data []KLineData) (float64, string) {
	if len(data) < s.MaxWindow+20 {
		return 0, "数据不足"
	}

	hist := data[len(data)-s.MaxWindow-20:]
	bbi := ComputeBBI(hist)
	_, _, j := ComputeKDJ(hist, 9)
	dif := ComputeDIF(hist, 12, 26)
	if bbi == nil || len(j) == 0 || len(dif) == 0 {
		return 0, "指标计算失败"
	}

	score := 0.0
	details := []string{}

	// 1. BBI趋势强度 (30分)
	bbiSlope := (bbi[len(bbi)-1] - bbi[len(bbi)-10]) / bbi[len(bbi)-10] * 100
	bbiScore := minFloat(30, maxFloat(0, bbiSlope*10))
	score += bbiScore
	details = append(details, fmt.Sprintf("BBI趋势:%.0f", bbiScore))

	// 2. KDJ超卖程度 (30分)
	jToday := j[len(j)-1]
	jScore := 0.0
	if jToday < 0 {
		jScore = 30
	} else if jToday < 10 {
		jScore = 25
	} else if jToday < 20 {
		jScore = 20
	} else if jToday < 30 {
		jScore = 15
	} else {
		jScore = 10
	}
	score += jScore
	details = append(details, fmt.Sprintf("KDJ超卖:%.0f", jScore))

	// 3. MACD强度 (20分)
	difToday := dif[len(dif)-1]
	difScore := minFloat(20, maxFloat(0, difToday*10))
	score += difScore
	details = append(details, fmt.Sprintf("MACD强度:%.0f", difScore))

	// 4. 量价配合 (20分)
	volumeScore := 0.0
	if len(hist) >= 5 {
		avgVol := 0.0
		for i := len(hist) - 5; i < len(hist); i++ {
			avgVol += float64(hist[i].Volume)
		}
		avgVol /= 5
		if avgVol > 0 {
			volRatio := float64(hist[len(hist)-1].Volume) / avgVol
			if volRatio > 1.5 {
				volumeScore = 20
			} else if volRatio > 1.2 {
				volumeScore = 15
			} else if volRatio > 1.0 {
				volumeScore = 10
			} else {
				volumeScore = 5
			}
		}
	}
	score += volumeScore
	details = append(details, fmt.Sprintf("量价配合:%.0f", volumeScore))

	return minFloat(100, score), strings.Join(details, ", ")
}

// SuperB1Selector SuperB1选股策略
type SuperB1Selector struct {
	LookbackN      int
	CloseVolPct    float64
	PriceDropPct   float64
	JThreshold     float64
	JQThreshold    float64
	bbiSelector    *BBIKDJSelector
}

func NewSuperB1Selector() *SuperB1Selector {
	return &SuperB1Selector{
		LookbackN:    30,
		CloseVolPct:  0.10,
		PriceDropPct: 0.02,
		JThreshold:   20,
		JQThreshold:  0.30,
		bbiSelector:  NewBBIKDJSelector(),
	}
}

func (s *SuperB1Selector) Name() string {
	return "SuperB1选股"
}

func (s *SuperB1Selector) Select(data []KLineData) bool {
	minLen := s.LookbackN + s.bbiSelector.MaxWindow + 20
	if len(data) < minLen {
		return false
	}

	hist := data[len(data)-minLen:]

	// Step-1: 搜索满足BBIKDJ的t_m
	lbHist := hist[len(hist)-s.LookbackN-1 : len(hist)-1]
	var tmIdx int = -1
	for i := len(lbHist) - 1; i >= 0; i-- {
		testData := hist[:len(hist)-s.LookbackN+i]
		if s.bbiSelector.Select(testData) {
			// 检查盘整区间
			stableSeg := hist[len(hist)-s.LookbackN+i : len(hist)-1]
			if len(stableSeg) < 3 {
				continue
			}
			high, low := findMaxMin(stableSeg)
			if low > 0 && (high/low-1) <= s.CloseVolPct {
				tmIdx = i
				break
			}
		}
	}

	if tmIdx < 0 {
		return false
	}

	// Step-3: 当日相对前一日跌幅
	closeToday := hist[len(hist)-1].Close
	closePrev := hist[len(hist)-2].Close
	if closePrev <= 0 || (closePrev-closeToday)/closePrev < s.PriceDropPct {
		return false
	}

	// Step-4: J值极低
	_, _, j := ComputeKDJ(hist, 9)
	if len(j) == 0 {
		return false
	}
	jToday := j[len(j)-1]
	jWindow := j[len(j)-s.LookbackN:]
	jQVal := Quantile(jWindow, s.JQThreshold)

	return jToday < s.JThreshold || jToday <= jQVal
}

func (s *SuperB1Selector) Score(data []KLineData) (float64, string) {
	minLen := s.LookbackN + s.bbiSelector.MaxWindow + 20
	if len(data) < minLen {
		return 0, "数据不足"
	}

	hist := data[len(data)-minLen:]
	score := 0.0
	details := []string{}

	// 1. 盘整区间稳定性 (30分)
	lbHist := hist[len(hist)-s.LookbackN-1 : len(hist)-1]
	if len(lbHist) >= 3 {
		high, low := findMaxMin(lbHist)
		if low > 0 {
			volatility := (high/low - 1)
			stabilityScore := 30 * (1 - volatility/s.CloseVolPct)
			score += maxFloat(0, minFloat(30, stabilityScore))
			details = append(details, fmt.Sprintf("盘整稳定:%.0f", stabilityScore))
		}
	}

	// 2. 当日跌幅适度性 (25分)
	closeToday := hist[len(hist)-1].Close
	closePrev := hist[len(hist)-2].Close
	if closePrev > 0 {
		dropPct := (closePrev - closeToday) / closePrev
		dropScore := 25.0
		if dropPct > 0.05 {
			dropScore = 15 // 跌太多扣分
		} else if dropPct > 0.03 {
			dropScore = 20
		}
		score += dropScore
		details = append(details, fmt.Sprintf("跌幅适度:%.0f", dropScore))
	}

	// 3. J值超卖程度 (25分)
	_, _, j := ComputeKDJ(hist, 9)
	if len(j) > 0 {
		jToday := j[len(j)-1]
		jScore := 0.0
		if jToday < 0 {
			jScore = 25
		} else if jToday < 10 {
			jScore = 20
		} else if jToday < 20 {
			jScore = 15
		} else {
			jScore = 10
		}
		score += jScore
		details = append(details, fmt.Sprintf("J值超卖:%.0f", jScore))
	}

	// 4. 量价配合 (20分)
	volumeScore := 0.0
	if len(hist) >= 5 {
		avgVol := 0.0
		for i := len(hist) - 5; i < len(hist); i++ {
			avgVol += float64(hist[i].Volume)
		}
		avgVol /= 5
		if avgVol > 0 {
			volRatio := float64(hist[len(hist)-1].Volume) / avgVol
			if volRatio > 1.5 {
				volumeScore = 20
			} else if volRatio > 1.2 {
				volumeScore = 15
			} else if volRatio > 1.0 {
				volumeScore = 10
			} else {
				volumeScore = 5
			}
		}
	}
	score += volumeScore
	details = append(details, fmt.Sprintf("量价配合:%.0f", volumeScore))

	return minFloat(100, score), strings.Join(details, ", ")
}

// BBIPullbackSelector BBI回踩选股策略
// 条件：
// 1. BBI趋势上升
// 2. 价格回踩BBI附近
// 3. 短期RSV <= 30
// 4. 长期RSV >= 85
// 5. J值低位
type BBIPullbackSelector struct {
	BBIMinWindow   int
	MaxWindow      int
	Tolerance      float64
	JThreshold     float64
	BBIQThreshold  float64
	NShort         int
	NLong          int
	RSVShortMax    float64  // 短期RSV上限
	RSVLongMin     float64  // 长期RSV下限
}

func NewBBIPullbackSelector() *BBIPullbackSelector {
	return &BBIPullbackSelector{
		BBIMinWindow:  20,
		MaxWindow:     60,
		Tolerance:     0.03,
		JThreshold:    50,
		BBIQThreshold: 0.15,
		NShort:        3,
		NLong:         14,
		RSVShortMax:   40,   // 放宽短期RSV
		RSVLongMin:    70,   // 放宽长期RSV
	}
}

func (s *BBIPullbackSelector) Name() string {
	return "BBI回踩选股"
}

func (s *BBIPullbackSelector) Select(data []KLineData) bool {
	if len(data) < s.MaxWindow+s.NLong {
		return false
	}

	hist := data[len(data)-max(s.MaxWindow, s.NLong):]

	// 检查数据长度
	if len(hist) < 24 {
		return false
	}

	// 计算BBI
	bbi := ComputeBBI(hist)
	if bbi == nil || len(bbi) != len(hist) {
		return false
	}

	// 1. 检查BBI趋势
	if !BBIDerivUptrend(bbi, s.BBIMinWindow, s.MaxWindow, s.BBIQThreshold) {
		return false
	}

	// 2. 计算KDJ
	_, _, j := ComputeKDJ(hist, 9)
	if len(j) == 0 || len(j) != len(hist) {
		return false
	}

	// 3. 检查回踩
	current := hist[len(hist)-1]
	bbiVal := bbi[len(bbi)-1]
	lowVal := current.Low
	closeVal := current.Close
	jVal := j[len(j)-1]

	// 检查BBI值是否有效
	if bbiVal <= 0 {
		return false
	}

	// 条件A: 最低价触达BBI下方或附近
	if lowVal > bbiVal*(1+s.Tolerance) {
		return false
	}

	// 条件B: 收盘价在BBI附近（可以微破）
	if closeVal < bbiVal*(1-s.Tolerance*2) {
		return false
	}

	// 条件C: J值低位确认
	if jVal > s.JThreshold {
		return false
	}

	// 条件D: 短期RSV
	rsvShort := ComputeRSV(hist, s.NShort)
	if rsvShort == nil || len(rsvShort) != len(hist) {
		return false
	}
	rsvShortToday := rsvShort[len(rsvShort)-1]
	if rsvShortToday > s.RSVShortMax {
		return false
	}

	// 条件E: 长期RSV
	rsvLong := ComputeRSV(hist, s.NLong)
	if rsvLong == nil || len(rsvLong) != len(hist) {
		return false
	}
	rsvLongToday := rsvLong[len(rsvLong)-1]
	if rsvLongToday < s.RSVLongMin {
		return false
	}

	return true
}

func (s *BBIPullbackSelector) Score(data []KLineData) (float64, string) {
	if len(data) < s.MaxWindow+s.NLong {
		return 0, "数据不足"
	}

	hist := data[len(data)-max(s.MaxWindow, s.NLong):]
	bbi := ComputeBBI(hist)
	_, _, j := ComputeKDJ(hist, 9)
	rsvShort := ComputeRSV(hist, s.NShort)
	rsvLong := ComputeRSV(hist, s.NLong)

	if bbi == nil || len(j) == 0 || rsvShort == nil || rsvLong == nil {
		return 0, "指标计算失败"
	}

	score := 0.0
	details := []string{}

	// 1. BBI趋势强度 (25分)
	bbiVal := bbi[len(bbi)-1]
	bbiPrev := bbi[len(bbi)-10]
	if bbiPrev > 0 {
		bbiSlope := (bbiVal - bbiPrev) / bbiPrev * 100
		bbiScore := minFloat(25, maxFloat(0, bbiSlope*8))
		score += bbiScore
		details = append(details, fmt.Sprintf("BBI趋势:%.0f", bbiScore))
	}

	// 2. 回踩精准度 (25分)
	current := hist[len(hist)-1]
	lowVal := current.Low
	closeVal := current.Close
	// 计算最低价与BBI的距离
	distToBBI := (bbiVal - lowVal) / bbiVal
	precisionScore := 0.0
	if distToBBI >= 0 && distToBBI <= 0.01 {
		precisionScore = 25 // 完美回踩
	} else if distToBBI <= 0.02 {
		precisionScore = 20
	} else if distToBBI <= 0.03 {
		precisionScore = 15
	} else {
		precisionScore = 10
	}
	score += precisionScore
	details = append(details, fmt.Sprintf("回踩精准:%.0f", precisionScore))

	// 3. 收盘价位置 (20分)
	closeScore := 0.0
	if closeVal > bbiVal {
		closeScore = 20 // 收盘在BBI之上
	} else if closeVal > bbiVal*0.98 {
		closeScore = 15
	} else {
		closeScore = 10
	}
	score += closeScore
	details = append(details, fmt.Sprintf("收盘位置:%.0f", closeScore))

	// 4. RSV配合度 (15分)
	rsvShortVal := rsvShort[len(rsvShort)-1]
	rsvLongVal := rsvLong[len(rsvLong)-1]
	rsvScore := 0.0
	if rsvShortVal <= 30 && rsvLongVal >= 85 {
		rsvScore = 15
	} else if rsvShortVal <= 40 && rsvLongVal >= 75 {
		rsvScore = 12
	} else {
		rsvScore = 8
	}
	score += rsvScore
	details = append(details, fmt.Sprintf("RSV配合:%.0f", rsvScore))

	// 5. J值低位 (15分)
	jVal := j[len(j)-1]
	jScore := 0.0
	if jVal < 10 {
		jScore = 15
	} else if jVal < 20 {
		jScore = 12
	} else if jVal < 30 {
		jScore = 10
	} else {
		jScore = 8
	}
	score += jScore
	details = append(details, fmt.Sprintf("J值低位:%.0f", jScore))

	return minFloat(100, score), strings.Join(details, ", ")
}

// 辅助函数
func findMaxMin(data []KLineData) (high, low float64) {
	high = -math.MaxFloat64
	low = math.MaxFloat64
	for _, d := range data {
		if d.High > high {
			high = d.High
		}
		if d.Low < low {
			low = d.Low
		}
	}
	return
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
