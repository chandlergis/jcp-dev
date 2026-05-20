import { SelectorStrategy, SelectorResult, SelectorRecord, SelectorStrategyInfo } from '../types';
import { GetSelectorStrategies, RunSelector, SaveSelectorRecord, GetSelectorRecords, GetSelectorRecordsByDate, DeleteSelectorRecord, AddStocksToWatchlist, CancelSelector, IsSelectorRunning, GetSelectorCacheStats } from '../../wailsjs/go/main/App';

// 获取选股策略列表
export async function getSelectorStrategies(): Promise<SelectorStrategyInfo[]> {
  try {
    const result = await GetSelectorStrategies();
    return (result || []) as SelectorStrategyInfo[];
  } catch (err) {
    console.error('Failed to get selector strategies:', err);
    return [];
  }
}

// 执行选股
export async function runSelector(
  strategy: SelectorStrategy,
  priceMin: number = 3,
  priceMax: number = 9999
): Promise<SelectorResult | null> {
  try {
    const result = await RunSelector({ strategy, priceMin, priceMax });
    return result as SelectorResult;
  } catch (err) {
    console.error('Failed to run selector:', err);
    return null;
  }
}

// 取消选股
export async function cancelSelector(): Promise<void> {
  try {
    await CancelSelector();
  } catch (err) {
    console.error('Failed to cancel selector:', err);
  }
}

// 选股是否正在运行
export async function isSelectorRunning(): Promise<boolean> {
  try {
    return await IsSelectorRunning();
  } catch (err) {
    console.error('Failed to check selector status:', err);
    return false;
  }
}

// 保存选股记录
export async function saveSelectorRecord(result: SelectorResult): Promise<string> {
  try {
    const res = await SaveSelectorRecord(result as any);
    return res;
  } catch (err) {
    console.error('Failed to save selector record:', err);
    return 'error';
  }
}

// 获取所有选股记录
export async function getSelectorRecords(): Promise<SelectorRecord[]> {
  try {
    const result = await GetSelectorRecords();
    return (result || []) as SelectorRecord[];
  } catch (err) {
    console.error('Failed to get selector records:', err);
    return [];
  }
}

// 按日期获取选股记录
export async function getSelectorRecordsByDate(date: string): Promise<SelectorRecord[]> {
  try {
    const result = await GetSelectorRecordsByDate(date);
    return (result || []) as SelectorRecord[];
  } catch (err) {
    console.error('Failed to get selector records by date:', err);
    return [];
  }
}

// 删除选股记录
export async function deleteSelectorRecord(date: string, strategy: SelectorStrategy): Promise<string> {
  try {
    const result = await DeleteSelectorRecord(date, strategy);
    return result;
  } catch (err) {
    console.error('Failed to delete selector record:', err);
    return 'error';
  }
}

// 批量添加股票到自选股
export async function addStocksToWatchlist(stocks: { symbol: string; name: string }[]): Promise<string[]> {
  try {
    const stockList = stocks.map(s => ({
      symbol: s.symbol,
      name: s.name,
      price: 0,
      change: 0,
      changePercent: 0,
      volume: 0,
      amount: 0,
      marketCap: '',
      sector: '',
      open: 0,
      high: 0,
      low: 0,
      preClose: 0,
    }));
    const result = await AddStocksToWatchlist(stockList);
    return result || [];
  } catch (err) {
    console.error('Failed to add stocks to watchlist:', err);
    return [];
  }
}

// 获取缓存统计
export async function getSelectorCacheStats(): Promise<{ total: number; today: number }> {
  try {
    const result = await GetSelectorCacheStats();
    return { total: result.total || 0, today: result.today || 0 };
  } catch (err) {
    console.error('Failed to get cache stats:', err);
    return { total: 0, today: 0 };
  }
}
