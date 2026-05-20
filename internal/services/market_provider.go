package services

import (
	"time"

	"github.com/run-bigpig/jcp/internal/models"
	"github.com/run-bigpig/jcp/internal/pkg/proxy"
)

type marketProvider interface {
	FetchStockDataWithOrderBook(codes ...string) ([]StockWithOrderBook, error)
	FetchKLineData(code string, period string, days int) ([]models.KLineData, error)
	FetchMarketIndices() ([]models.MarketIndex, error)
	SearchStocks(keyword string, limit int) ([]StockSearchResult, error)
}

func NewMarketServiceWithProviders(primary marketProvider, fallback marketProvider) *MarketService {
	ms := newMarketService()
	ms.primaryProvider = primary
	ms.fallbackProvider = fallback
	return ms
}

func newMarketService() *MarketService {
	ms := &MarketService{
		client:        proxy.GetManager().GetClientWithTimeout(10 * time.Second),
		cache:         make(map[string]*stockCache),
		cacheTTL:      2 * time.Second,
		klineCache:    make(map[string]*klineCache),
		klineCacheTTL: klineCacheTTLDefault,
	}
	go ms.cleanCacheLoop()
	return ms
}

func (ms *MarketService) fetchStockDataWithFallback(codes ...string) ([]StockWithOrderBook, error) {
	if len(codes) == 0 {
		return nil, nil
	}

	if ms.primaryProvider != nil {
		data, err := ms.primaryProvider.FetchStockDataWithOrderBook(codes...)
		if err == nil && len(data) > 0 {
			return data, nil
		}
		log.Warn("主行情源获取实时数据失败，切换兜底源: %v", err)
	}

	if ms.fallbackProvider != nil {
		return ms.fallbackProvider.FetchStockDataWithOrderBook(codes...)
	}

	return nil, nil
}

func (ms *MarketService) fetchKLineDataWithFallback(code string, period string, days int) ([]models.KLineData, error) {
	if ms.primaryProvider != nil {
		data, err := ms.primaryProvider.FetchKLineData(code, period, days)
		if err == nil && len(data) > 0 {
			return data, nil
		}
		log.Warn("主行情源获取K线失败，切换兜底源: %v", err)
	}

	if ms.fallbackProvider != nil {
		return ms.fallbackProvider.FetchKLineData(code, period, days)
	}

	return nil, nil
}

func (ms *MarketService) fetchMarketIndicesWithFallback() ([]models.MarketIndex, error) {
	if ms.primaryProvider != nil {
		data, err := ms.primaryProvider.FetchMarketIndices()
		if err == nil && len(data) > 0 {
			return data, nil
		}
		log.Warn("主行情源获取指数失败，切换兜底源: %v", err)
	}

	if ms.fallbackProvider != nil {
		return ms.fallbackProvider.FetchMarketIndices()
	}

	return nil, nil
}

func (ms *MarketService) searchStocksWithFallback(keyword string, limit int) []StockSearchResult {
	if ms.primaryProvider != nil {
		results, err := ms.primaryProvider.SearchStocks(keyword, limit)
		if err == nil {
			return results
		}
		log.Warn("主行情源搜索股票失败，切换兜底源: %v", err)
	}

	if ms.fallbackProvider != nil {
		results, err := ms.fallbackProvider.SearchStocks(keyword, limit)
		if err == nil {
			return results
		}
	}

	return nil
}

type sinaMarketProvider struct {
	service *MarketService
}

func newSinaMarketProvider(service *MarketService) *sinaMarketProvider {
	return &sinaMarketProvider{service: service}
}

func (p *sinaMarketProvider) FetchStockDataWithOrderBook(codes ...string) ([]StockWithOrderBook, error) {
	return p.service.fetchStockDataWithOrderBookFromSina(codes...)
}

func (p *sinaMarketProvider) FetchKLineData(code string, period string, days int) ([]models.KLineData, error) {
	return p.service.fetchKLineDataFromSina(code, period, days)
}

func (p *sinaMarketProvider) FetchMarketIndices() ([]models.MarketIndex, error) {
	return p.service.fetchMarketIndicesFromSina()
}

func (p *sinaMarketProvider) SearchStocks(keyword string, limit int) ([]StockSearchResult, error) {
	return searchEmbeddedStocks(keyword, limit), nil
}
