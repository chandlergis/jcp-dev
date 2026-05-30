package services

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sync"

	"github.com/run-bigpig/jcp/internal/logger"
	"github.com/run-bigpig/jcp/internal/models"
	"github.com/run-bigpig/jcp/internal/selector"
	"github.com/run-bigpig/jcp/internal/training"
)

var trainingLog = logger.New("training_service")

// TrainingService 训练服务
type TrainingService struct {
	dataDir      string
	manager      *training.Manager
	selectorMgr  *selector.Manager
	marketSvc    *MarketService
	predictionSvc *PredictionService
	mu           sync.RWMutex
	store        *models.TrainingStore
	config       models.TrainingConfig
}

// NewTrainingService 创建训练服务
func NewTrainingService(dataDir string, marketSvc *MarketService) *TrainingService {
	s := &TrainingService{
		dataDir:     dataDir,
		manager:     training.NewManager(),
		selectorMgr: selector.NewManager(),
		marketSvc:   marketSvc,
		store:       &models.TrainingStore{Records: []models.TrainingRecord{}},
		config:      models.DefaultTrainingConfig(),
	}
	s.load()
	return s
}

// load 加载训练记录
func (s *TrainingService) load() {
	filePath := filepath.Join(s.dataDir, "training_records.json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			trainingLog.Error("加载训练记录失败: %v", err)
		}
		return
	}

	if err := json.Unmarshal(data, s.store); err != nil {
		trainingLog.Error("解析训练记录失败: %v", err)
	}
}

// save 保存训练记录
func (s *TrainingService) save() error {
	filePath := filepath.Join(s.dataDir, "training_records.json")
	data, err := json.MarshalIndent(s.store, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, data, 0644)
}

// CreateSession 创建训练会话
func (s *TrainingService) CreateSession() (*models.TrainingSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 获取所有股票
	allStocks := s.selectorMgr.GetAllStocks()
	if len(allStocks) == 0 {
		return nil, fmt.Errorf("没有可用的股票")
	}

	// 随机选择一只股票
	stock := allStocks[rand.Intn(len(allStocks))]

	// 获取K线数据
	klines, err := s.marketSvc.GetKLineData(stock.Symbol, "1d", 250)
	if err != nil || len(klines) < s.config.MinDays {
		// 重试其他股票
		for i := 0; i < 10; i++ {
			stock = allStocks[rand.Intn(len(allStocks))]
			klines, err = s.marketSvc.GetKLineData(stock.Symbol, "1d", 250)
			if err == nil && len(klines) >= s.config.MinDays {
				break
			}
		}
		if err != nil || len(klines) < s.config.MinDays {
			return nil, fmt.Errorf("无法获取足够的K线数据")
		}
	}

	// 创建会话
	session, err := s.manager.CreateSession(stock.Symbol, stock.Name, klines, s.config)
	if err != nil {
		return nil, err
	}

	trainingLog.Info("创建训练会话: %s (%s), 日期范围: %s ~ %s, 共%d天",
		stock.Name, stock.Symbol, session.StartDate, session.EndDate, session.TotalDays)

	return session, nil
}

// GetSession 获取训练会话
func (s *TrainingService) GetSession(sessionID string) *models.TrainingSession {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.manager.GetSession(sessionID)
}

// GetVisibleKlines 获取可见的K线数据
func (s *TrainingService) GetVisibleKlines(sessionID string) []models.KLineData {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.manager.GetVisibleKlines(sessionID)
}

// ExecuteTrade 执行交易
func (s *TrainingService) ExecuteTrade(sessionID string, action models.TradeAction, positionLevel models.PositionLevel) (*models.TradeRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	trade, err := s.manager.ExecuteTrade(sessionID, action, positionLevel)
	if err != nil {
		return nil, err
	}

	// 检查是否有新里程碑
	session := s.manager.GetSession(sessionID)
	if session != nil && len(session.Milestones) > 0 {
		// 里程碑会在前端处理
	}

	return trade, nil
}

// NextDay 推进到下一天
func (s *TrainingService) NextDay(sessionID string) (*models.KLineData, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	kline, finished, err := s.manager.NextDay(sessionID)
	if err != nil {
		return nil, false, err
	}

	if finished {
		// 保存训练记录
		s.saveTrainingRecord(sessionID)
	}

	return kline, finished, nil
}

// AbortSession 中止训练
func (s *TrainingService) AbortSession(sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.manager.AbortSession(sessionID); err != nil {
		return err
	}

	// 保存训练记录
	s.saveTrainingRecord(sessionID)

	return nil
}

// GetTrades 获取交易记录
func (s *TrainingService) GetTrades(sessionID string) []models.TradeRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.manager.GetTrades(sessionID)
}

// GetCapitalCurve 获取资金曲线
func (s *TrainingService) GetCapitalCurve(sessionID string) []models.CapitalSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.manager.GetCapitalCurve(sessionID)
}

// GetStats 获取统计数据
func (s *TrainingService) GetStats(sessionID string) map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.manager.CalculateStats(sessionID)
}

// GetMilestoneInfo 获取里程碑信息
func (s *TrainingService) GetMilestoneInfo(milestoneType models.MilestoneType) *models.MilestoneInfo {
	milestones := models.GetMilestones()
	for _, m := range milestones {
		if m.Type == milestoneType {
			return &m
		}
	}
	return nil
}

// saveTrainingRecord 保存训练记录
func (s *TrainingService) saveTrainingRecord(sessionID string) {
	session := s.manager.GetSession(sessionID)
	if session == nil {
		return
	}

	record := models.TrainingRecord{
		Session:      *session,
		Trades:       s.manager.GetTrades(sessionID),
		CapitalCurve: s.manager.GetCapitalCurve(sessionID),
	}

	s.store.Records = append(s.store.Records, record)

	if err := s.save(); err != nil {
		trainingLog.Error("保存训练记录失败: %v", err)
	} else {
		trainingLog.Info("保存训练记录成功: %s", sessionID)
	}
}

// GetTrainingRecords 获取所有训练记录
func (s *TrainingService) GetTrainingRecords() []models.TrainingRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.store.Records
}

// GetBestRecord 获取最佳记录
func (s *TrainingService) GetBestRecord() *models.TrainingRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.store.Records) == 0 {
		return nil
	}

	best := &s.store.Records[0]
	for i := 1; i < len(s.store.Records); i++ {
		if s.store.Records[i].Session.TotalReturn > best.Session.TotalReturn {
			best = &s.store.Records[i]
		}
	}

	return best
}

// GetTrainingPrediction 获取当前训练会话的AI预测
func (s *TrainingService) GetTrainingPrediction(sessionID string) *models.PredictionResult {
	if s.predictionSvc == nil || !s.predictionSvc.IsTrained() {
		return nil
	}

	// 获取当前可见的K线数据（包含历史，不含未来）
	klines := s.manager.GetVisibleKlines(sessionID)
	if len(klines) < 60 {
		return nil
	}

	return s.predictionSvc.Predict(klines)
}

// SetPredictionService 设置预测服务
func (s *TrainingService) SetPredictionService(ps *PredictionService) {
	s.predictionSvc = ps
}
