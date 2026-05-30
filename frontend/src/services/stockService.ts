// 市场数据服务 - 调用后端API
import { GetStockRealTimeData, GetKLineData, GetOrderBook, SearchStocks, GetStockPrediction } from '@wailsjs/go/main/App';
import type { Stock, KLineData, OrderBook, PredictionResult } from '../types';

// 股票搜索结果类型
export interface StockSearchResult {
  symbol: string;
  name: string;
  industry: string;
  market: string;
}

export const getStockRealTimeData = async (codes: string[]): Promise<Stock[]> => {
  return await GetStockRealTimeData(codes);
};

export const getKLineData = async (code: string, period: string, days: number): Promise<KLineData[]> => {
  return await GetKLineData(code, period, days);
};

// 获取真实五档盘口数据
export const getOrderBook = async (code: string): Promise<OrderBook> => {
  return await GetOrderBook(code);
};

// 搜索股票
export const searchStocks = async (keyword: string): Promise<StockSearchResult[]> => {
  if (!keyword.trim()) return [];
  return await SearchStocks(keyword) as StockSearchResult[];
};

// 获取单支股票AI预测
export const getStockPrediction = async (symbol: string): Promise<PredictionResult | null> => {
  try {
    const result = await GetStockPrediction(symbol);
    return result as PredictionResult | null;
  } catch (err) {
    return null;
  }
};
