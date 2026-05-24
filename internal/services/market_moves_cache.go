package services

import (
	"sync"
	"time"

	"github.com/run-bigpig/jcp/internal/models"
)

// movesCacheEntry 异动接口缓存条目
type movesCacheEntry struct {
	payload   interface{}
	timestamp time.Time
}

// movesCache 异动相关接口的统一缓存
type movesCache struct {
	mu    sync.RWMutex
	store map[string]*movesCacheEntry
}

var globalMovesCache = &movesCache{store: make(map[string]*movesCacheEntry)}

// getMovesCacheTTL 根据市场状态返回异动数据的缓存TTL
// 状态来源: MarketService.GetMarketStatus().Status
// 返回 0 表示不缓存（强制刷新），返回 24h+ 表示几乎永久缓存
func (ms *MarketService) getMovesCacheTTL(marketStatus string) time.Duration {
	switch marketStatus {
	case "trading":
		// 交易中：高频变化，10秒缓存（前端30秒刷新，命中率约2/3）
		return 10 * time.Second
	case "lunch_break":
		// 午休：行情冻结，缓存到13:00
		return ms.durationUntilTradingResume()
	case "pre_market":
		// 盘前：数据停留在前一交易日，缓存到9:30
		return ms.durationUntilTradingOpen()
	case "closed":
		// 已收盘：数据基本不变，缓存到次日9:30
		return ms.durationUntilNextTradingOpen()
	default:
		// 休市/周末/节假日：缓存到下一个交易日
		return ms.durationUntilNextTradingOpen()
	}
}

// durationUntilTradingResume 计算到13:00的剩余时间（午休结束）
func (ms *MarketService) durationUntilTradingResume() time.Duration {
	loc := time.FixedZone("CST", 8*60*60)
	now := time.Now().In(loc)
	resume := time.Date(now.Year(), now.Month(), now.Day(), 13, 0, 0, 0, loc)
	if d := resume.Sub(now); d > 0 {
		return d
	}
	return 10 * time.Second
}

// durationUntilTradingOpen 计算到当日9:30的剩余时间
func (ms *MarketService) durationUntilTradingOpen() time.Duration {
	loc := time.FixedZone("CST", 8*60*60)
	now := time.Now().In(loc)
	open := time.Date(now.Year(), now.Month(), now.Day(), 9, 30, 0, 0, loc)
	if d := open.Sub(now); d > 0 {
		return d
	}
	return 10 * time.Second
}

// durationUntilNextTradingOpen 计算到下一个交易日 9:30 的剩余时间
func (ms *MarketService) durationUntilNextTradingOpen() time.Duration {
	loc := time.FixedZone("CST", 8*60*60)
	now := time.Now().In(loc)

	// 向后扫描找下一个交易日
	for i := 1; i <= 10; i++ {
		next := now.AddDate(0, 0, i)
		if ok, _ := ms.isTradeDay(next); ok {
			open := time.Date(next.Year(), next.Month(), next.Day(), 9, 30, 0, 0, loc)
			if d := open.Sub(now); d > 0 {
				return d
			}
		}
	}
	// 兜底：12小时（避免极端情况下永久缓存）
	return 12 * time.Hour
}

// movesCacheGet 通用缓存读取
func movesCacheGet(key string, ttl time.Duration) (interface{}, bool) {
	if ttl <= 0 {
		return nil, false
	}
	globalMovesCache.mu.RLock()
	defer globalMovesCache.mu.RUnlock()
	entry, ok := globalMovesCache.store[key]
	if !ok {
		return nil, false
	}
	if time.Since(entry.timestamp) >= ttl {
		return nil, false
	}
	return entry.payload, true
}

// movesCacheSet 通用缓存写入
func movesCacheSet(key string, payload interface{}) {
	globalMovesCache.mu.Lock()
	defer globalMovesCache.mu.Unlock()
	globalMovesCache.store[key] = &movesCacheEntry{
		payload:   payload,
		timestamp: time.Now(),
	}
}

// 类型断言辅助：兼容三种结果类型

func castStockMoveList(v interface{}) (models.StockMoveList, bool) {
	if v == nil {
		return models.StockMoveList{}, false
	}
	r, ok := v.(models.StockMoveList)
	return r, ok
}

func castBoardFundFlowList(v interface{}) (models.BoardFundFlowList, bool) {
	if v == nil {
		return models.BoardFundFlowList{}, false
	}
	r, ok := v.(models.BoardFundFlowList)
	return r, ok
}

func castBoardLeaderList(v interface{}) (models.BoardLeaderList, bool) {
	if v == nil {
		return models.BoardLeaderList{}, false
	}
	r, ok := v.(models.BoardLeaderList)
	return r, ok
}
