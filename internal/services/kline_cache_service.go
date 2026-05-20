package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/run-bigpig/jcp/internal/logger"
	"github.com/run-bigpig/jcp/internal/models"
)

var klineCacheLog = logger.New("kline_cache")

// KLineCacheEntry K线缓存条目
type KLineCacheEntry struct {
	Symbol    string             `json:"symbol"`
	Period    string             `json:"period"`
	Data      []models.KLineData `json:"data"`
	UpdatedAt time.Time          `json:"updated_at"`
}

// KLineCacheStore K线缓存存储
type KLineCacheStore struct {
	Entries map[string]*KLineCacheEntry `json:"entries"` // key: "symbol:period"
}

// KLineCacheService K线缓存服务
type KLineCacheService struct {
	dataDir string
	mu      sync.RWMutex
	store   *KLineCacheStore
	changed bool
}

// NewKLineCacheService 创建K线缓存服务
func NewKLineCacheService(dataDir string) *KLineCacheService {
	s := &KLineCacheService{
		dataDir: dataDir,
		store: &KLineCacheStore{
			Entries: make(map[string]*KLineCacheEntry),
		},
	}
	s.load()
	// 启动定时保存
	go s.autoSave()
	return s
}

// load 加载缓存
func (s *KLineCacheService) load() {
	filePath := filepath.Join(s.dataDir, "kline_cache.json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			klineCacheLog.Error("加载K线缓存失败: %v", err)
		}
		return
	}

	if err := json.Unmarshal(data, s.store); err != nil {
		klineCacheLog.Error("解析K线缓存失败: %v", err)
		return
	}

	klineCacheLog.Info("加载K线缓存成功，共 %d 条记录", len(s.store.Entries))
}

// save 保存缓存
func (s *KLineCacheService) save() error {
	filePath := filepath.Join(s.dataDir, "kline_cache.json")
	data, err := json.MarshalIndent(s.store, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, data, 0644)
}

// autoSave 定时保存
func (s *KLineCacheService) autoSave() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		s.mu.Lock()
		if s.changed {
			if err := s.save(); err != nil {
				klineCacheLog.Error("自动保存K线缓存失败: %v", err)
			} else {
				s.changed = false
				klineCacheLog.Debug("自动保存K线缓存成功")
			}
		}
		s.mu.Unlock()
	}
}

// Get 获取缓存的K线数据
func (s *KLineCacheService) Get(symbol, period string) ([]models.KLineData, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := symbol + ":" + period
	entry, ok := s.store.Entries[key]
	if !ok {
		return nil, false
	}

	// 检查缓存是否过期（日线数据当天有效）
	if period == "1d" {
		now := time.Now()
		entryDate := entry.UpdatedAt.Format("2006-01-02")
		todayDate := now.Format("2006-01-02")
		if entryDate != todayDate {
			// 非今天的数据，返回false要求更新
			return entry.Data, false
		}
	}

	return entry.Data, true
}

// Set 设置缓存
func (s *KLineCacheService) Set(symbol, period string, data []models.KLineData) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := symbol + ":" + period
	s.store.Entries[key] = &KLineCacheEntry{
		Symbol:    symbol,
		Period:    period,
		Data:      data,
		UpdatedAt: time.Now(),
	}
	s.changed = true
}

// GetBatch 批量获取缓存
func (s *KLineCacheService) GetBatch(symbols []string, period string) map[string][]models.KLineData {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string][]models.KLineData)
	for _, symbol := range symbols {
		key := symbol + ":" + period
		if entry, ok := s.store.Entries[key]; ok {
			result[symbol] = entry.Data
		}
	}
	return result
}

// SetBatch 批量设置缓存
func (s *KLineCacheService) SetBatch(data map[string][]models.KLineData, period string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for symbol, klines := range data {
		key := symbol + ":" + period
		s.store.Entries[key] = &KLineCacheEntry{
			Symbol:    symbol,
			Period:    period,
			Data:      klines,
			UpdatedAt: time.Now(),
		}
	}
	s.changed = true
}

// GetCacheStats 获取缓存统计
func (s *KLineCacheService) GetCacheStats() (total int, todayCount int) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	today := time.Now().Format("2006-01-02")
	total = len(s.store.Entries)
	for _, entry := range s.store.Entries {
		if entry.UpdatedAt.Format("2006-01-02") == today {
			todayCount++
		}
	}
	return
}

// ClearExpired 清理过期缓存
func (s *KLineCacheService) ClearExpired(days int) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().AddDate(0, 0, -days)
	removed := 0

	for key, entry := range s.store.Entries {
		if entry.UpdatedAt.Before(cutoff) {
			delete(s.store.Entries, key)
			removed++
		}
	}

	if removed > 0 {
		s.changed = true
		klineCacheLog.Info("清理过期缓存 %d 条", removed)
	}

	return removed
}

// SaveNow 立即保存
func (s *KLineCacheService) SaveNow() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.save(); err != nil {
		return err
	}
	s.changed = false
	return nil
}
