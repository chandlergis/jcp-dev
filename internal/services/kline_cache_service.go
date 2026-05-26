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

// Get 获取缓存的K线数据（仅返回数据，不判断有效性）
func (s *KLineCacheService) Get(symbol, period string) ([]models.KLineData, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := symbol + ":" + period
	entry, ok := s.store.Entries[key]
	if !ok {
		return nil, false
	}
	return entry.Data, true
}

// CacheFreshness 缓存新鲜度
type CacheFreshness int

const (
	CacheStale     CacheFreshness = iota // 完全过期，需要重新拉取历史
	CacheNeedQuote                       // 历史数据可用，但需要补充实时行情
	CacheFresh                           // 完全新鲜，直接使用
)

// CheckFreshness 检查缓存新鲜度
// 根据市场状态和缓存中最后一根K线的日期来判断
// lastTradeDate: 最近一个交易日（YYYY-MM-DD），由调用方传入
// marketStatus: 当前市场状态 "trading" / "closed" / "pre_market" / "lunch_break"
func (s *KLineCacheService) CheckFreshness(symbol, period, lastTradeDate, marketStatus string) (CacheFreshness, []models.KLineData) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := symbol + ":" + period
	entry, ok := s.store.Entries[key]
	if !ok || len(entry.Data) == 0 {
		return CacheStale, nil
	}

	if period != "1d" {
		// 非日K的缓存：按30秒TTL处理
		if time.Since(entry.UpdatedAt) < 30*time.Second {
			return CacheFresh, entry.Data
		}
		return CacheStale, entry.Data
	}

	// 日K的缓存新鲜度判断
	lastKlineDate := entry.Data[len(entry.Data)-1].Time
	entryToday := entry.UpdatedAt.Format("2006-01-02") == time.Now().Format("2006-01-02")
	entryAfterClose := entryToday && entry.UpdatedAt.Hour() >= 15

	// 检查倒数第二根K线是否是上一个交易日（排除数据缺口）
	// 如果缓存有缺口（比如缺了周五的数据），即使最后一根是今天，涨跌幅也会算错
	hasDataGap := false
	if len(entry.Data) >= 2 {
		prevKlineDate := entry.Data[len(entry.Data)-2].Time
		// prevKlineDate 应该是 lastTradeDate 的前一个交易日
		// 简单检查：如果间隔超过4天（跨了周末+节假日），肯定有缺口
		// 更精确的检查：两个日期之间应该只差1-3个交易日
		prevParsed, err1 := time.Parse("2006-01-02", prevKlineDate)
		lastParsed, err2 := time.Parse("2006-01-02", lastTradeDate)
		if err1 == nil && err2 == nil {
			diffDays := lastParsed.Sub(prevParsed).Hours() / 24
			// 正常情况：1-3天（连续交易日，如周一到周三=2天，周五到周一=3天）
			// 如果间隔>=4天，说明中间有交易日数据缺失
			// 唯一例外：长假（如春节7天），但此时重拉数据也无害
			if diffDays >= 4 {
				hasDataGap = true
				klineCacheLog.Info("[CheckFreshness %s] 数据缺口: prev=%s, last=%s, 间隔%.0f天",
					symbol, prevKlineDate, lastTradeDate, diffDays)
			}
		}
	}

	switch marketStatus {
	case "closed":
		// 已收盘（交易日15:00后 或 休市日）
		if hasDataGap {
			// 有数据缺口 → 强制重新拉取完整历史（缓存数据不完整，涨跌幅会算错）
			return CacheStale, entry.Data
		}
		if lastKlineDate == lastTradeDate && entryAfterClose {
			// K线日期对得上 + 缓存是在收盘后更新的 + 没有数据缺口 → 完全新鲜
			return CacheFresh, entry.Data
		}
		// 其他情况：需要从API补充当日行情
		return CacheNeedQuote, entry.Data

	case "lunch_break":
		// 午休（11:30-13:00）：上午收盘价已定，TDX日K通常已更新
		if lastKlineDate == lastTradeDate {
			return CacheFresh, entry.Data
		}
		return CacheNeedQuote, entry.Data

	case "trading":
		// 交易中：永远需要补实时行情（价格在变）
		return CacheNeedQuote, entry.Data

	case "pre_market":
		// 盘前：最近交易日数据已存在 → 完全新鲜（盘前数据不会变）
		if lastKlineDate == lastTradeDate {
			return CacheFresh, entry.Data
		}
		return CacheNeedQuote, entry.Data

	default:
		// 休市日（周末/节假日）
		if lastKlineDate == lastTradeDate && !hasDataGap {
			return CacheFresh, entry.Data
		}
		// 有数据缺口或K线日期不对 → 需要重新拉取
		return CacheStale, entry.Data
	}
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
