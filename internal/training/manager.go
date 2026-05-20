package training

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/run-bigpig/jcp/internal/logger"
	"github.com/run-bigpig/jcp/internal/models"
)

var log = logger.New("training")

// Manager 训练管理器
type Manager struct {
	sessions map[string]*models.TrainingSession
	trades   map[string][]models.TradeRecord
	curves   map[string][]models.CapitalSnapshot
	klines   map[string][]models.KLineData // sessionID -> klines
}

// NewManager 创建训练管理器
func NewManager() *Manager {
	return &Manager{
		sessions: make(map[string]*models.TrainingSession),
		trades:   make(map[string][]models.TradeRecord),
		curves:   make(map[string][]models.CapitalSnapshot),
		klines:   make(map[string][]models.KLineData),
	}
}

// CreateSession 创建训练会话
func (m *Manager) CreateSession(
	stockCode string,
	stockName string,
	klines []models.KLineData,
	config models.TrainingConfig,
) (*models.TrainingSession, error) {
	if len(klines) < config.MinDays {
		return nil, fmt.Errorf("K线数据不足，需要至少%d天", config.MinDays)
	}

	// 前置天数（用户可见的历史K线）
	warmupDays := 60
	if warmupDays > len(klines)-config.MinDays {
		warmupDays = len(klines) - config.MinDays
	}

	// 训练天数（用户可以交易的天数）
	trainingDays := config.MinDays + rand.Intn(config.MaxDays-config.MinDays+1)
	if warmupDays+trainingDays > len(klines) {
		trainingDays = len(klines) - warmupDays
	}

	// 总天数 = 前置天数 + 训练天数
	totalDays := warmupDays + trainingDays

	// 随机选择起始位置
	startIdx := rand.Intn(len(klines) - totalDays + 1)
	selectedKlines := klines[startIdx : startIdx+totalDays]

	// 生成会话ID
	sessionID := fmt.Sprintf("training_%d", time.Now().UnixNano())

	session := &models.TrainingSession{
		ID:             sessionID,
		StockCode:      stockCode,
		StockName:      stockName,
		StartDate:      selectedKlines[0].Time,
		EndDate:        selectedKlines[len(selectedKlines)-1].Time,
		CurrentDate:    selectedKlines[warmupDays].Time, // 当前日期 = warmup最后一天
		CurrentIndex:   warmupDays,                       // 从warmup之后开始
		TotalDays:      trainingDays,                     // 训练天数（不含warmup）
		InitialCapital: config.InitialCapital,
		Cash:           config.InitialCapital,
		TotalAsset:     config.InitialCapital,
		Position:       0,
		AvgCost:        0,
		PositionValue:  0,
		TotalReturn:    0,
		TotalProfit:    0,
		Status:         models.TrainingStatusRunning,
		IsTradingDay:   true,
		Milestones:     []models.MilestoneType{},
		TradeCount:     0,
		WinCount:       0,
		MaxDrawdown:    0,
		WinStreak:      0,
		MaxWinStreak:   0,
		CreatedAt:      time.Now().Format(time.RFC3339),
	}

	// 保存会话和K线数据
	m.sessions[sessionID] = session
	m.klines[sessionID] = selectedKlines
	m.trades[sessionID] = []models.TradeRecord{}
	m.curves[sessionID] = []models.CapitalSnapshot{
		{
			Date:       selectedKlines[warmupDays].Time,
			TotalAsset: config.InitialCapital,
			Cash:       config.InitialCapital,
			Position:   0,
			Price:      selectedKlines[warmupDays].Close,
		},
	}

	return session, nil
}

// GetSession 获取训练会话
func (m *Manager) GetSession(sessionID string) *models.TrainingSession {
	return m.sessions[sessionID]
}

// GetKlines 获取训练K线数据
func (m *Manager) GetKlines(sessionID string) []models.KLineData {
	return m.klines[sessionID]
}

// GetCurrentKline 获取当前K线
func (m *Manager) GetCurrentKline(sessionID string) *models.KLineData {
	session := m.sessions[sessionID]
	if session == nil {
		return nil
	}
	klines := m.klines[sessionID]
	if session.CurrentIndex >= len(klines) {
		return nil
	}
	return &klines[session.CurrentIndex]
}

// GetVisibleKlines 获取可见的K线数据（不包含未来数据）
func (m *Manager) GetVisibleKlines(sessionID string) []models.KLineData {
	session := m.sessions[sessionID]
	if session == nil {
		return nil
	}
	klines := m.klines[sessionID]
	if session.CurrentIndex >= len(klines) {
		return klines
	}
	return klines[:session.CurrentIndex+1]
}

// ExecuteTrade 执行交易
func (m *Manager) ExecuteTrade(sessionID string, action models.TradeAction, positionLevel models.PositionLevel) (*models.TradeRecord, error) {
	session := m.sessions[sessionID]
	if session == nil {
		return nil, fmt.Errorf("会话不存在")
	}
	if session.Status != models.TrainingStatusRunning {
		return nil, fmt.Errorf("训练已结束")
	}

	kline := m.GetCurrentKline(sessionID)
	if kline == nil {
		return nil, fmt.Errorf("无法获取当前K线")
	}

	price := kline.Close

	switch action {
	case models.TradeActionBuy:
		return m.executeBuy(session, price, positionLevel)
	case models.TradeActionSell:
		return m.executeSell(session, price, positionLevel)
	default:
		return nil, fmt.Errorf("无效的交易动作")
	}
}

// executeBuy 执行买入（支持加仓）
func (m *Manager) executeBuy(session *models.TrainingSession, price float64, level models.PositionLevel) (*models.TradeRecord, error) {
	// 计算买入数量（按手=100股）
	ratio := models.GetPositionRatio(level)
	availableAmount := session.Cash * ratio
	quantity := int(availableAmount/price/100) * 100

	if quantity <= 0 {
		return nil, fmt.Errorf("资金不足")
	}

	amount := float64(quantity) * price
	if amount > session.Cash {
		return nil, fmt.Errorf("资金不足")
	}

	// 更新持仓成本（加仓平均）
	totalCost := session.AvgCost*float64(session.Position) + price*float64(quantity)
	session.Cash -= amount
	session.Position += quantity
	session.AvgCost = totalCost / float64(session.Position)
	session.PositionValue = float64(session.Position) * price
	session.TotalAsset = session.Cash + session.PositionValue
	session.TradeCount++

	// 创建交易记录
	trade := models.TradeRecord{
		ID:             fmt.Sprintf("trade_%d", time.Now().UnixNano()),
		SessionID:      session.ID,
		Action:         models.TradeActionBuy,
		Date:           session.CurrentDate,
		Price:          price,
		Quantity:       quantity,
		Amount:         amount,
		PositionLevel:  level,
		CashAfter:      session.Cash,
		PositionAfter:  session.Position,
		AssetAfter:     session.TotalAsset,
		CreatedAt:      time.Now().Format(time.RFC3339),
	}

	m.trades[session.ID] = append(m.trades[session.ID], trade)

	// 更新资金曲线
	m.updateCapitalCurve(session, price)

	log.Info("买入: %s, 价格=%.2f, 数量=%d, 金额=%.2f", session.StockCode, price, quantity, amount)

	return &trade, nil
}

// executeSell 执行卖出
func (m *Manager) executeSell(session *models.TrainingSession, price float64, level models.PositionLevel) (*models.TradeRecord, error) {
	if session.Position <= 0 {
		return nil, fmt.Errorf("没有持仓")
	}

	// 计算卖出数量
	ratio := models.GetPositionRatio(level)
	quantity := int(float64(session.Position) * ratio / 100) * 100
	if quantity <= 0 {
		quantity = session.Position // 全部卖出
	}
	if quantity > session.Position {
		quantity = session.Position
	}

	amount := float64(quantity) * price

	// 计算盈亏
	profit := (price - session.AvgCost) * float64(quantity)
	profitPercent := (price/session.AvgCost - 1) * 100

	// 更新会话
	session.Cash += amount
	session.Position -= quantity
	if session.Position == 0 {
		session.AvgCost = 0
	}
	session.PositionValue = float64(session.Position) * price
	session.TotalAsset = session.Cash + session.PositionValue
	session.TotalProfit = session.TotalAsset - session.InitialCapital
	session.TotalReturn = (session.TotalAsset/session.InitialCapital - 1) * 100
	session.TradeCount++

	// 更新胜率和连胜
	if profit > 0 {
		session.WinCount++
		session.WinStreak++
		if session.WinStreak > session.MaxWinStreak {
			session.MaxWinStreak = session.WinStreak
		}
	} else {
		session.WinStreak = 0
	}

	// 更新最大回撤
	peakAsset := session.InitialCapital
	for _, snapshot := range m.curves[session.ID] {
		if snapshot.TotalAsset > peakAsset {
			peakAsset = snapshot.TotalAsset
		}
	}
	drawdown := (peakAsset - session.TotalAsset) / peakAsset * 100
	if drawdown > session.MaxDrawdown {
		session.MaxDrawdown = drawdown
	}

	// 创建交易记录
	trade := models.TradeRecord{
		ID:             fmt.Sprintf("trade_%d", time.Now().UnixNano()),
		SessionID:      session.ID,
		Action:         models.TradeActionSell,
		Date:           session.CurrentDate,
		Price:          price,
		Quantity:       quantity,
		Amount:         amount,
		PositionLevel:  level,
		CashAfter:      session.Cash,
		PositionAfter:  session.Position,
		AssetAfter:     session.TotalAsset,
		Profit:         profit,
		ProfitPercent:  profitPercent,
		CreatedAt:      time.Now().Format(time.RFC3339),
	}

	m.trades[session.ID] = append(m.trades[session.ID], trade)

	// 更新资金曲线
	m.updateCapitalCurve(session, price)

	// 检查里程碑
	m.checkMilestones(session, profit, profitPercent)

	log.Info("卖出: %s, 价格=%.2f, 数量=%d, 盈亏=%.2f (%.2f%%)", session.StockCode, price, quantity, profit, profitPercent)

	return &trade, nil
}

// NextDay 推进到下一天
func (m *Manager) NextDay(sessionID string) (*models.KLineData, bool, error) {
	session := m.sessions[sessionID]
	if session == nil {
		return nil, false, fmt.Errorf("会话不存在")
	}
	if session.Status != models.TrainingStatusRunning {
		return nil, false, fmt.Errorf("训练已结束")
	}

	klines := m.klines[sessionID]
	if session.CurrentIndex >= len(klines)-1 {
		// 训练结束
		session.Status = models.TrainingStatusFinished
		session.FinishedAt = time.Now().Format(time.RFC3339)
		
		// 如果还有持仓，强制平仓
		if session.Position > 0 {
			price := klines[len(klines)-1].Close
			m.ForceClose(sessionID, price)
		}
		
		return nil, true, nil
	}

	session.CurrentIndex++
	currentKline := klines[session.CurrentIndex]
	session.CurrentDate = currentKline.Time
	session.IsTradingDay = true

	// 更新持仓市值
	if session.Position > 0 {
		session.PositionValue = float64(session.Position) * currentKline.Close
		session.TotalAsset = session.Cash + session.PositionValue
		session.TotalProfit = session.TotalAsset - session.InitialCapital
		session.TotalReturn = (session.TotalAsset/session.InitialCapital - 1) * 100
	}

	// 更新资金曲线
	m.updateCapitalCurve(session, currentKline.Close)

	return &currentKline, false, nil
}

// ForceClose 强制平仓
func (m *Manager) ForceClose(sessionID string, price float64) {
	session := m.sessions[sessionID]
	if session == nil || session.Position <= 0 {
		return
	}

	quantity := session.Position
	amount := float64(quantity) * price
	profit := (price - session.AvgCost) * float64(quantity)
	profitPercent := (price/session.AvgCost - 1) * 100

	session.Cash += amount
	session.Position = 0
	session.AvgCost = 0
	session.PositionValue = 0
	session.TotalAsset = session.Cash
	session.TotalProfit = session.TotalAsset - session.InitialCapital
	session.TotalReturn = (session.TotalAsset/session.InitialCapital - 1) * 100

	if profit > 0 {
		session.WinCount++
	}

	// 创建交易记录
	trade := models.TradeRecord{
		ID:             fmt.Sprintf("trade_%d", time.Now().UnixNano()),
		SessionID:      session.ID,
		Action:         models.TradeActionSell,
		Date:           session.CurrentDate,
		Price:          price,
		Quantity:       quantity,
		Amount:         amount,
		PositionLevel:  models.PositionFull,
		CashAfter:      session.Cash,
		PositionAfter:  0,
		AssetAfter:     session.TotalAsset,
		Profit:         profit,
		ProfitPercent:  profitPercent,
		CreatedAt:      time.Now().Format(time.RFC3339),
	}

	m.trades[session.ID] = append(m.trades[session.ID], trade)
}

// checkMilestones 检查里程碑
func (m *Manager) checkMilestones(session *models.TrainingSession, profit float64, profitPercent float64) {
	achieved := make([]models.MilestoneType, 0)

	// 检查收益率里程碑
	returnMilestones := []struct {
		threshold float64
		milestone models.MilestoneType
	}{
		{10, models.MilestoneReturn10},
		{30, models.MilestoneReturn30},
		{50, models.MilestoneReturn50},
		{100, models.MilestoneReturn100},
	}

	for _, rm := range returnMilestones {
		if session.TotalReturn >= rm.threshold {
			if !m.hasMilestone(session, rm.milestone) {
				session.Milestones = append(session.Milestones, rm.milestone)
				achieved = append(achieved, rm.milestone)
			}
		}
	}

	// 检查连胜里程碑
	if session.WinStreak >= 3 {
		if !m.hasMilestone(session, models.MilestoneWinStreak3) {
			session.Milestones = append(session.Milestones, models.MilestoneWinStreak3)
			achieved = append(achieved, models.MilestoneWinStreak3)
		}
	}

	// 检查单笔收益里程碑
	if profitPercent >= 20 {
		if !m.hasMilestone(session, models.MilestoneSingleProfit20) {
			session.Milestones = append(session.Milestones, models.MilestoneSingleProfit20)
			achieved = append(achieved, models.MilestoneSingleProfit20)
		}
	}

	if len(achieved) > 0 {
		log.Info("达成里程碑: %v", achieved)
	}
}

// hasMilestone 检查是否已有里程碑
func (m *Manager) hasMilestone(session *models.TrainingSession, milestone models.MilestoneType) bool {
	for _, ms := range session.Milestones {
		if ms == milestone {
			return true
		}
	}
	return false
}

// updateCapitalCurve 更新资金曲线
func (m *Manager) updateCapitalCurve(session *models.TrainingSession, price float64) {
	snapshot := models.CapitalSnapshot{
		Date:       session.CurrentDate,
		TotalAsset: session.TotalAsset,
		Cash:       session.Cash,
		Position:   session.Position,
		Price:      price,
	}
	m.curves[session.ID] = append(m.curves[session.ID], snapshot)
}

// GetTrades 获取交易记录
func (m *Manager) GetTrades(sessionID string) []models.TradeRecord {
	return m.trades[sessionID]
}

// GetCapitalCurve 获取资金曲线
func (m *Manager) GetCapitalCurve(sessionID string) []models.CapitalSnapshot {
	return m.curves[sessionID]
}

// GetNewMilestones 获取新达成的里程碑（用于前端显示弹窗）
func (m *Manager) GetNewMilestones(sessionID string) []models.MilestoneType {
	session := m.sessions[sessionID]
	if session == nil {
		return nil
	}
	// 返回最近一次交易后新达成的里程碑
	// 这里简化处理，返回所有已达成的里程碑
	return session.Milestones
}

// AbortSession 中止训练
func (m *Manager) AbortSession(sessionID string) error {
	session := m.sessions[sessionID]
	if session == nil {
		return fmt.Errorf("会话不存在")
	}
	if session.Status != models.TrainingStatusRunning {
		return fmt.Errorf("训练已结束")
	}

	session.Status = models.TrainingStatusAborted
	session.FinishedAt = time.Now().Format(time.RFC3339)

	// 如果还有持仓，强制平仓
	if session.Position > 0 {
		kline := m.GetCurrentKline(sessionID)
		if kline != nil {
			m.ForceClose(sessionID, kline.Close)
		}
	}

	return nil
}

// CalculateStats 计算统计数据
func (m *Manager) CalculateStats(sessionID string) map[string]interface{} {
	session := m.sessions[sessionID]
	if session == nil {
		return nil
	}

	trades := m.trades[sessionID]
	curves := m.curves[sessionID]

	// 计算平均持仓天数
	totalHoldingDays := 0
	holdingCount := 0
	var buyDate string
	for _, trade := range trades {
		if trade.Action == models.TradeActionBuy {
			buyDate = trade.Date
		} else if trade.Action == models.TradeActionSell && buyDate != "" {
			// 简化计算，这里需要实际计算天数差异
			totalHoldingDays++
			holdingCount++
			buyDate = ""
		}
	}

	avgHoldingDays := 0.0
	if holdingCount > 0 {
		avgHoldingDays = float64(totalHoldingDays) / float64(holdingCount)
	}

	// 计算夏普比率（简化计算）
	returns := make([]float64, 0)
	for i := 1; i < len(curves); i++ {
		if curves[i-1].TotalAsset > 0 {
			r := (curves[i].TotalAsset - curves[i-1].TotalAsset) / curves[i-1].TotalAsset
			returns = append(returns, r)
		}
	}

	meanReturn := 0.0
	for _, r := range returns {
		meanReturn += r
	}
	if len(returns) > 0 {
		meanReturn /= float64(len(returns))
	}

	variance := 0.0
	for _, r := range returns {
		variance += math.Pow(r-meanReturn, 2)
	}
	if len(returns) > 1 {
		variance /= float64(len(returns) - 1)
	}

	sharpeRatio := 0.0
	if variance > 0 {
		sharpeRatio = meanReturn / math.Sqrt(variance) * math.Sqrt(252) // 年化
	}

	winRate := 0.0
	if session.TradeCount > 0 {
		winRate = float64(session.WinCount) / float64(session.TradeCount) * 100
	}

	return map[string]interface{}{
		"totalReturn":    session.TotalReturn,
		"totalProfit":    session.TotalProfit,
		"tradeCount":     session.TradeCount,
		"winCount":       session.WinCount,
		"winRate":        winRate,
		"maxDrawdown":    session.MaxDrawdown,
		"avgHoldingDays": avgHoldingDays,
		"sharpeRatio":    sharpeRatio,
		"maxWinStreak":   session.MaxWinStreak,
	}
}
