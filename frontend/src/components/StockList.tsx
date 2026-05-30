import React, { useState, useEffect, useRef } from 'react';
import { Stock, MarketIndex, PredictionResult } from '../types';
import { searchStocks, StockSearchResult, getStockPrediction } from '../services/stockService';
import { TrendingUp, TrendingDown, Search, X, List, Filter, History, Sparkles } from 'lucide-react';
import { MarketIndices } from './MarketIndices';
import { SelectorPanel } from './SelectorPanel';
import { SelectorRecordDialog } from './SelectorRecordDialog';
import { useTheme } from '../contexts/ThemeContext';
import { useCandleColor } from '../contexts/CandleColorContext';

type TabType = 'watchlist' | 'selector' | 'records';

interface StockListProps {
  stocks: Stock[]; // The current watchlist
  selectedSymbol: string;
  onSelect: (symbol: string) => void;
  onAddStock: (stock: Stock) => void;
  onRemoveStock?: (symbol: string) => void;
  onWatchlistChange?: () => void;
  marketIndices?: MarketIndex[];
}

export const StockList: React.FC<StockListProps> = ({
  stocks,
  selectedSymbol,
  onSelect,
  onAddStock,
  onRemoveStock,
  onWatchlistChange,
  marketIndices
}) => {
  const { colors } = useTheme();
  const cc = useCandleColor();
  const [activeTab, setActiveTab] = useState<TabType>('watchlist');
  const [searchTerm, setSearchTerm] = useState('');
  const [searchResults, setSearchResults] = useState<StockSearchResult[]>([]);
  const [showDropdown, setShowDropdown] = useState(false);
  const [isSearching, setIsSearching] = useState(false);
  const [showRecordDialog, setShowRecordDialog] = useState(false);
  const [predSymbol, setPredSymbol] = useState<string | null>(null);
  const [predResult, setPredResult] = useState<PredictionResult | null>(null);
  const [predLoading, setPredLoading] = useState(false);
  const searchRef = useRef<HTMLDivElement>(null);
  const debounceRef = useRef<ReturnType<typeof setTimeout>>();

  // 点击外部关闭下拉
  useEffect(() => {
    const handleClickOutside = (e: MouseEvent) => {
      if (searchRef.current && !searchRef.current.contains(e.target as Node)) {
        setShowDropdown(false);
      }
    };
    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, []);

  // 搜索防抖
  useEffect(() => {
    if (debounceRef.current) {
      clearTimeout(debounceRef.current);
    }

    if (!searchTerm.trim()) {
      setSearchResults([]);
      setShowDropdown(false);
      return;
    }

    setIsSearching(true);
    debounceRef.current = setTimeout(async () => {
      const results = await searchStocks(searchTerm);
      // 确保 results 是数组，并过滤掉已在自选股中的股票
      const safeResults = Array.isArray(results) ? results : [];
      const filteredResults = safeResults.filter(
        r => !stocks.some(s => s.symbol === r.symbol)
      );
      setSearchResults(filteredResults);
      setShowDropdown(filteredResults.length > 0);
      setIsSearching(false);
    }, 300);

    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current);
    };
  }, [searchTerm]);

  // 选择搜索结果添加股票
  const handleSelectResult = (result: StockSearchResult) => {
    const newStock: Stock = {
      symbol: result.symbol,
      name: result.name,
      price: 0,
      change: 0,
      changePercent: 0,
      volume: 0,
      amount: 0,
      marketCap: '',
      sector: result.industry,
      open: 0,
      high: 0,
      low: 0,
      preClose: 0,
    };
    onAddStock(newStock);
    setSearchTerm('');
    setShowDropdown(false);
  };

  // AI 预测
  const handlePredict = async (e: React.MouseEvent, symbol: string) => {
    e.stopPropagation();
    if (predSymbol === symbol) {
      // 点击同一支股票，关闭预测
      setPredSymbol(null);
      setPredResult(null);
      return;
    }
    setPredSymbol(symbol);
    setPredLoading(true);
    setPredResult(null);
    const result = await getStockPrediction(symbol);
    setPredResult(result);
    setPredLoading(false);
  };

  const tabs = [
    { id: 'watchlist' as TabType, label: '自选股', icon: List },
    { id: 'selector' as TabType, label: '选股', icon: Filter },
    { id: 'records' as TabType, label: '记录', icon: History },
  ];

  return (
    <div className="flex flex-col h-full relative">
      {/* Tab Header */}
      <div className={`flex border-b ${colors.isDark ? 'border-slate-700' : 'border-slate-200'}`}>
        {tabs.map(tab => {
          const Icon = tab.icon;
          const isActive = activeTab === tab.id;
          return (
            <button
              key={tab.id}
              onClick={() => setActiveTab(tab.id)}
              className={`flex-1 flex items-center justify-center gap-1.5 py-2.5 text-xs font-medium transition-colors ${
                isActive
                  ? 'text-accent-2 border-b-2 border-accent'
                  : colors.isDark
                    ? 'text-slate-400 hover:text-slate-200 hover:bg-slate-800/50'
                    : 'text-slate-500 hover:text-slate-700 hover:bg-slate-100'
              }`}
            >
              <Icon size={14} />
              <span>{tab.label}</span>
            </button>
          );
        })}
      </div>

      {/* Tab Content */}
      {activeTab === 'watchlist' && (
        <>
          <div className="p-4 border-b fin-divider-soft">
            {/* 大盘指数 */}
            <div className="mb-4 pb-3 border-b fin-divider-soft flex justify-center">
              <MarketIndices indices={marketIndices || []} />
            </div>
            <div ref={searchRef} className="relative z-50">
              <div className="relative">
                <Search className={`absolute left-3 top-2.5 h-4 w-4 ${colors.isDark ? 'text-slate-400' : 'text-slate-500'}`} />
                <input
                  type="text"
                  value={searchTerm}
                  onChange={(e) => setSearchTerm(e.target.value)}
                  onFocus={() => searchResults.length > 0 && setShowDropdown(true)}
                  placeholder="搜索股票代码或名称..."
                  className={`w-full fin-input rounded-lg pl-9 pr-4 py-2 text-sm ${colors.isDark ? 'placeholder-slate-500' : 'placeholder-slate-400'}`}
                />
                {isSearching && (
                  <div className="absolute right-3 top-2.5 h-4 w-4 border-2 border-accent border-t-transparent rounded-full animate-spin" />
                )}
              </div>

              {/* 搜索下拉结果 */}
              {showDropdown && (
                <div className={`absolute top-full left-0 right-0 mt-1 max-h-64 overflow-y-auto rounded-lg shadow-xl text-left ${colors.isDark ? 'bg-slate-800 border border-slate-600' : 'bg-white border border-slate-300'}`}>
                  {searchResults.map((result) => (
                    <div
                      key={result.symbol}
                      onClick={() => handleSelectResult(result)}
                      className={`px-3 py-2 cursor-pointer border-b last:border-b-0 ${colors.isDark ? 'hover:bg-slate-700 border-slate-700' : 'hover:bg-slate-100 border-slate-200'}`}
                    >
                      <div className="flex justify-between items-center">
                        <div>
                          <span className={colors.isDark ? 'text-slate-200' : 'text-slate-700'}>{result.name}</span>
                          <span className="ml-2 font-mono text-accent-2 text-sm">{result.symbol}</span>
                        </div>
                        <span className={`text-xs ${colors.isDark ? 'text-slate-500' : 'text-slate-400'}`}>{result.market}</span>
                      </div>
                      {result.industry && (
                        <div className={`text-xs mt-0.5 ${colors.isDark ? 'text-slate-500' : 'text-slate-400'}`}>{result.industry}</div>
                      )}
                    </div>
                  ))}
                </div>
              )}
            </div>
          </div>

          <div className="flex-1 overflow-y-auto fin-scrollbar">
            {stocks.map((stock) => {
              const isSelected = stock.symbol === selectedSymbol;
              const isPositive = stock.change >= 0;

              return (
                <div
                  key={stock.symbol}
                  onClick={() => onSelect(stock.symbol)}
                  className={`group p-4 border-b fin-divider-soft cursor-pointer transition-colors ${colors.isDark ? 'hover:bg-slate-800/40' : 'hover:bg-slate-100/60'} ${isSelected ? (colors.isDark ? 'bg-slate-800/40' : 'bg-slate-100/60') + ' border-l-4 border-l-accent' : 'border-l-4 border-l-transparent'}`}
                >
                  <div className="flex justify-between items-start mb-1">
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-2">
                        <span className={`font-bold ${colors.isDark ? 'text-slate-100' : 'text-slate-800'}`}>{stock.name}</span>
                        <button
                          onClick={(e) => handlePredict(e, stock.symbol)}
                          className={`p-0.5 rounded transition-all ${
                            predSymbol === stock.symbol
                              ? 'text-amber-400 bg-amber-500/20'
                              : `opacity-0 group-hover:opacity-100 ${colors.isDark ? 'text-slate-500 hover:text-amber-400 hover:bg-amber-500/10' : 'text-slate-400 hover:text-amber-500 hover:bg-amber-100'}`
                          }`}
                          title="AI 预测"
                        >
                          <Sparkles size={14} />
                        </button>
                        {onRemoveStock && (
                          <button
                            onClick={(e) => {
                              e.stopPropagation();
                              onRemoveStock(stock.symbol);
                            }}
                            className={`opacity-0 group-hover:opacity-100 p-0.5 rounded hover:bg-red-500/20 hover:text-red-400 transition-all ${colors.isDark ? 'text-slate-500' : 'text-slate-400'}`}
                          >
                            <X size={14} />
                          </button>
                        )}
                      </div>
                      <div className={`text-xs font-mono truncate text-left ${colors.isDark ? 'text-slate-400' : 'text-slate-500'}`}>{stock.symbol}</div>
                    </div>
                    <div className="text-right">
                      <div className={`font-mono ${cc.getColorClass(isPositive)}`}>
                        {stock.price.toFixed(2)}
                      </div>
                      <div className={`text-xs font-mono flex items-center justify-end ${cc.getColorClass(isPositive)}`}>
                        {isPositive ? <TrendingUp size={12} className="mr-1"/> : <TrendingDown size={12} className="mr-1"/>}
                        {isPositive ? '+' : ''}{stock.changePercent.toFixed(2)}%
                      </div>
                    </div>
                  </div>
                  <div className={`flex justify-between items-center text-xs mt-2 ${colors.isDark ? 'text-slate-500' : 'text-slate-400'}`}>
                    <span>量: {formatVolume(stock.volume)}</span>
                    {stock.sector && (
                      <span className={`fin-chip px-1.5 py-0.5 rounded ${colors.isDark ? 'text-slate-300' : 'text-slate-600'}`}>{stock.sector}</span>
                    )}
                  </div>

                  {/* AI 预测结果 */}
                  {predSymbol === stock.symbol && (
                    <div className="mt-2" onClick={e => e.stopPropagation()}>
                      {predLoading ? (
                        <div className={`flex items-center gap-2 text-xs ${colors.isDark ? 'text-slate-500' : 'text-slate-400'}`}>
                          <div className="h-3 w-3 border-2 border-amber-400 border-t-transparent rounded-full animate-spin" />
                          AI 分析中...
                        </div>
                      ) : predResult ? (
                        <div className={`p-2 rounded-lg border ${
                          predResult.signal === '强买入' || predResult.signal === '买入'
                            ? 'bg-green-500/10 border-green-500/30'
                            : predResult.signal === '强卖出' || predResult.signal === '卖出'
                              ? 'bg-red-500/10 border-red-500/30'
                              : 'bg-slate-500/10 border-slate-500/30'
                        }`}>
                          <div className="flex items-center justify-between">
                            <span className={`text-xs font-bold ${
                              predResult.signal === '强买入' || predResult.signal === '买入'
                                ? 'text-green-500'
                                : predResult.signal === '强卖出' || predResult.signal === '卖出'
                                  ? 'text-red-500'
                                  : 'text-slate-400'
                            }`}>
                              {predResult.signal}
                            </span>
                            <span className={`text-xs font-mono ${predResult.direction === '涨' ? 'text-green-500' : 'text-red-500'}`}>
                              {predResult.direction} {Math.abs(predResult.return).toFixed(2)}%
                            </span>
                          </div>
                          <div className="flex items-center gap-1.5 mt-1">
                            <div className={`flex-1 h-1 rounded-full overflow-hidden ${colors.isDark ? 'bg-slate-700' : 'bg-slate-200'}`}>
                              <div
                                className={`h-full rounded-full ${
                                  predResult.confidence > 0.5 ? 'bg-green-500' : predResult.confidence > 0.3 ? 'bg-yellow-500' : 'bg-slate-400'
                                }`}
                                style={{ width: `${Math.min(predResult.confidence * 100, 100)}%` }}
                              />
                            </div>
                            <span className={`text-[10px] ${colors.isDark ? 'text-slate-500' : 'text-slate-400'}`}>
                              {(predResult.confidence * 100).toFixed(0)}%
                            </span>
                          </div>
                        </div>
                      ) : (
                        <div className={`text-xs ${colors.isDark ? 'text-slate-500' : 'text-slate-400'}`}>
                          预测不可用
                        </div>
                      )}
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        </>
      )}

      {activeTab === 'selector' && (
        <SelectorPanel onStockSelect={onSelect} onStockAdded={onWatchlistChange} />
      )}

      {activeTab === 'records' && (
        <div className="flex-1 overflow-y-auto p-4">
          <button
            onClick={() => setShowRecordDialog(true)}
            className={`w-full flex items-center justify-center gap-2 px-4 py-3 rounded-lg text-sm font-medium transition-colors ${
              colors.isDark
                ? 'bg-slate-800 text-slate-200 hover:bg-slate-700'
                : 'bg-slate-100 text-slate-700 hover:bg-slate-200'
            }`}
          >
            <History size={16} />
            <span>查看选股记录</span>
          </button>
        </div>
      )}

      {/* Selector Record Dialog */}
      <SelectorRecordDialog
        isOpen={showRecordDialog}
        onClose={() => setShowRecordDialog(false)}
        onStockSelect={onSelect}
      />
    </div>
  );
};

// 格式化成交量
const formatVolume = (vol: number): string => {
  if (vol >= 100000000) return (vol / 100000000).toFixed(2) + '亿';
  if (vol >= 10000) return (vol / 10000).toFixed(0) + '万';
  return vol.toString();
};
