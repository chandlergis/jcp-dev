package models

// TradeAction 交易动作
type TradeAction string

const (
	TradeActionBuy  TradeAction = "buy"  // 买入
	TradeActionSell TradeAction = "sell" // 卖出
)

// PositionLevel 仓位级别
type PositionLevel string

const (
	PositionFull   PositionLevel = "full"   // 全仓
	PositionHalf   PositionLevel = "half"   // 半仓
	PositionQuarter PositionLevel = "quarter" // 1/4仓
	PositionTenth  PositionLevel = "tenth"  // 1/10仓
)

// MilestoneType 里程碑类型
type MilestoneType string

const (
	MilestoneReturn10    MilestoneType = "return_10"    // 收益10%
	MilestoneReturn30    MilestoneType = "return_30"    // 收益30%
	MilestoneReturn50    MilestoneType = "return_50"    // 收益50%
	MilestoneReturn100   MilestoneType = "return_100"   // 收益100%
	MilestoneWinStreak3  MilestoneType = "win_streak_3"  // 连续3次盈利
	MilestoneSingleProfit20 MilestoneType = "single_profit_20" // 单笔收益20%
)

// MilestoneInfo 里程碑信息
type MilestoneInfo struct {
	Type        MilestoneType `json:"type"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Icon        string        `json:"icon"`
}

// GetMilestones 获取所有里程碑定义
func GetMilestones() []MilestoneInfo {
	return []MilestoneInfo{
		{MilestoneReturn10, "初级交易员", "累计收益达到10%", "🌱"},
		{MilestoneReturn30, "中级交易员", "累计收益达到30%", "📈"},
		{MilestoneReturn50, "高级交易员", "累计收益达到50%", "🏆"},
		{MilestoneReturn100, "交易大师", "累计收益达到100%", "👑"},
		{MilestoneWinStreak3, "稳定盈利者", "连续3次盈利交易", "🎯"},
		{MilestoneSingleProfit20, "精准抄底", "单笔收益超过20%", "💎"},
	}
}

// TrainingStatus 训练状态
type TrainingStatus string

const (
	TrainingStatusRunning  TrainingStatus = "running"  // 进行中
	TrainingStatusFinished TrainingStatus = "finished" // 已结束
	TrainingStatusAborted  TrainingStatus = "aborted"  // 已中止
)

// TrainingSession 训练会话
type TrainingSession struct {
	ID           string          `json:"id"`
	StockCode    string          `json:"stockCode"`
	StockName    string          `json:"stockName"`
	StartDate    string          `json:"startDate"`    // 训练开始日期
	EndDate      string          `json:"endDate"`      // 训练结束日期
	CurrentDate  string          `json:"currentDate"`  // 当前显示日期
	CurrentIndex int             `json:"currentIndex"` // 当前K线索引
	TotalDays    int             `json:"totalDays"`    // 总交易日数
	
	// 资金相关
	InitialCapital float64 `json:"initialCapital"` // 初始资金
	Cash           float64 `json:"cash"`           // 当前现金
	TotalAsset     float64 `json:"totalAsset"`     // 总资产（现金+持仓市值）
	
	// 持仓相关
	Position       int     `json:"position"`       // 持仓数量
	AvgCost        float64 `json:"avgCost"`        // 持仓成本
	PositionValue  float64 `json:"positionValue"`  // 持仓市值
	
	// 收益相关
	TotalReturn    float64 `json:"totalReturn"`    // 总收益率(%)
	TotalProfit    float64 `json:"totalProfit"`    // 总盈亏金额
	
	// 状态
	Status         TrainingStatus `json:"status"`
	IsTradingDay   bool           `json:"isTradingDay"` // 当前是否可交易
	
	// 里程碑
	Milestones     []MilestoneType `json:"milestones"` // 已达成的里程碑
	
	// 统计
	TradeCount     int     `json:"tradeCount"`     // 交易次数
	WinCount       int     `json:"winCount"`       // 盈利次数
	MaxDrawdown    float64 `json:"maxDrawdown"`    // 最大回撤(%)
	WinStreak      int     `json:"winStreak"`      // 当前连胜次数
	MaxWinStreak   int     `json:"maxWinStreak"`   // 最大连胜次数
	
	CreatedAt      string  `json:"createdAt"`
	FinishedAt     string  `json:"finishedAt,omitempty"`
}

// TradeRecord 交易记录
type TradeRecord struct {
	ID          string      `json:"id"`
	SessionID   string      `json:"sessionId"`
	Action      TradeAction `json:"action"`
	Date        string      `json:"date"`
	Price       float64     `json:"price"`
	Quantity    int         `json:"quantity"`
	Amount      float64     `json:"amount"` // 交易金额
	PositionLevel PositionLevel `json:"positionLevel"`
	
	// 交易后状态
	CashAfter      float64 `json:"cashAfter"`
	PositionAfter  int     `json:"positionAfter"`
	AssetAfter     float64 `json:"assetAfter"`
	
	// 盈亏（仅卖出时有值）
	Profit         float64 `json:"profit,omitempty"`
	ProfitPercent  float64 `json:"profitPercent,omitempty"`
	
	CreatedAt      string  `json:"createdAt"`
}

// CapitalSnapshot 资金快照（用于绘制资金曲线）
type CapitalSnapshot struct {
	Date       string  `json:"date"`
	TotalAsset float64 `json:"totalAsset"`
	Cash       float64 `json:"cash"`
	Position   int     `json:"position"`
	Price      float64 `json:"price"`
}

// TrainingRecord 训练记录（保存到文件）
type TrainingRecord struct {
	Session  TrainingSession  `json:"session"`
	Trades   []TradeRecord    `json:"trades"`
	CapitalCurve []CapitalSnapshot `json:"capitalCurve"`
}

// TrainingStore 训练记录存储
type TrainingStore struct {
	Records []TrainingRecord `json:"records"`
}

// TrainingConfig 训练配置
type TrainingConfig struct {
	InitialCapital float64 `json:"initialCapital"` // 初始资金，默认100万
	MinDays        int     `json:"minDays"`        // 最少交易日
	MaxDays        int     `json:"maxDays"`        // 最多交易日
}

// DefaultTrainingConfig 默认训练配置
func DefaultTrainingConfig() TrainingConfig {
	return TrainingConfig{
		InitialCapital: 1000000, // 100万
		MinDays:        60,
		MaxDays:        120,
	}
}

// GetPositionRatio 获取仓位比例
func GetPositionRatio(level PositionLevel) float64 {
	switch level {
	case PositionFull:
		return 1.0
	case PositionHalf:
		return 0.5
	case PositionQuarter:
		return 0.25
	case PositionTenth:
		return 0.1
	default:
		return 0.5
	}
}
