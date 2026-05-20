package models

// SelectorStrategy 选股策略类型
type SelectorStrategy string

const (
	StrategyBBIKDJ       SelectorStrategy = "bbi_kdj"
	StrategySuperB1      SelectorStrategy = "super_b1"
	StrategyPeakKDJ      SelectorStrategy = "peak_kdj"
	StrategyBBIShortLong SelectorStrategy = "bbi_short_long"
	StrategyBreakout     SelectorStrategy = "breakout_volume_kdj"
	StrategyDivergence   SelectorStrategy = "kdj_divergence"
	StrategyPullback     SelectorStrategy = "bbi_pullback"
)

// SelectorStrategyInfo 策略信息
type SelectorStrategyInfo struct {
	ID          SelectorStrategy `json:"id"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
}

// GetStrategyList 获取所有策略信息
func GetStrategyList() []SelectorStrategyInfo {
	return []SelectorStrategyInfo{
		{StrategyBBIKDJ, "BBI+KDJ选股", "BBI趋势上升+KDJ超卖+MACD金叉"},
		{StrategySuperB1, "SuperB1选股", "历史匹配BBIKDJ+盘整区间+当日下跌+J值极低"},
		{StrategyPullback, "BBI回踩选股", "BBI趋势回踩+短期RSV≤30+长期RSV≥85"},
	}
}

// SelectorResult 选股结果
type SelectorResult struct {
	Strategy     SelectorStrategy     `json:"strategy"`
	StrategyName string               `json:"strategyName"`
	Date         string               `json:"date"`
	Stocks       []SelectorStock      `json:"stocks"`
	Total        int                  `json:"total"`
	Params       SelectorFilterParams `json:"params"`
}

// SelectorStock 选中的股票
type SelectorStock struct {
	Symbol        string  `json:"symbol"`
	Name          string  `json:"name"`
	Industry      string  `json:"industry"`
	Price         float64 `json:"price"`
	Change        float64 `json:"change"`
	ChangePercent float64 `json:"changePercent"`
	Volume        int64   `json:"volume"`
	Amount        float64 `json:"amount"`
	Score         float64 `json:"score"`          // 综合得分 0-100
	ScoreDetail   string  `json:"scoreDetail"`    // 得分详情
	// AI预测字段
	PredDirection  string  `json:"predDirection,omitempty"`  // "涨" / "跌"
	PredReturn     float64 `json:"predReturn,omitempty"`     // 预测收益率(%)
	PredConfidence float64 `json:"predConfidence,omitempty"` // 预测置信度 0-1
	PredSignal     string  `json:"predSignal,omitempty"`     // "强买入"/"买入"/"观望"/"卖出"/"强卖出"
}

// SelectorFilterParams 选股过滤参数
type SelectorFilterParams struct {
	PriceMin float64 `json:"priceMin"` // 最低股价
	PriceMax float64 `json:"priceMax"` // 最高股价
}

// SelectorRecord 选股记录（持久化）
type SelectorRecord struct {
	Date            string           `json:"date"`
	Strategy        SelectorStrategy `json:"strategy"`
	StrategyName    string           `json:"strategyName"`
	Stocks          []SelectorStock  `json:"stocks"`
	Params          SelectorFilterParams `json:"params"`
	ExecutedAt      string           `json:"executedAt"`
	AddedToWatchlist []string         `json:"addedToWatchlist,omitempty"`
}

// SelectorRecordsStore 选股记录存储结构
type SelectorRecordsStore struct {
	Records []SelectorRecord `json:"records"`
}

// PredictionResult 预测结果
type PredictionResult struct {
	Direction  string  `json:"direction"`  // "涨" / "跌"
	Return     float64 `json:"return"`     // 预测收益率(%)
	Confidence float64 `json:"confidence"` // 置信度 0-1
	Signal     string  `json:"signal"`     // 信号等级
}
