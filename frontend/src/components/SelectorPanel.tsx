import React, { useState, useEffect, useCallback } from 'react';
import { SelectorStrategy, SelectorStrategyInfo, SelectorResult, SelectorStock } from '../types';
import { getSelectorStrategies, runSelector, saveSelectorRecord, addStocksToWatchlist, cancelSelector, getSelectorCacheStats } from '../services/selectorService';
import { useTheme } from '../contexts/ThemeContext';
import { useCandleColor } from '../contexts/CandleColorContext';
import { Search, Play, Save, Plus, TrendingUp, TrendingDown, ChevronDown, ChevronUp, XCircle, Database } from 'lucide-react';
import { EventsOn } from '../../wailsjs/runtime/runtime';

interface SelectorPanelProps {
  onStockSelect: (symbol: string) => void;
  onStockAdded?: () => void;
}

interface ProgressInfo {
  total: number;
  processed: number;
  found: number;
  current: string;
  results: SelectorStock[];
  done: boolean;
  cancelled: boolean;
}

export const SelectorPanel: React.FC<SelectorPanelProps> = ({ onStockSelect, onStockAdded }) => {
  const { colors } = useTheme();
  const cc = useCandleColor();
  const [strategies, setStrategies] = useState<SelectorStrategyInfo[]>([]);
  const [selectedStrategy, setSelectedStrategy] = useState<SelectorStrategy>('bbi_kdj');
  const [priceMin, setPriceMin] = useState<number>(3);
  const [priceMax, setPriceMax] = useState<number>(100);
  const [loading, setLoading] = useState<boolean>(false);
  const [progress, setProgress] = useState<ProgressInfo | null>(null);
  const [result, setResult] = useState<SelectorResult | null>(null);
  const [selectedStocks, setSelectedStocks] = useState<Set<string>>(new Set());
  const [saving, setSaving] = useState<boolean>(false);
  const [showStrategies, setShowStrategies] = useState<boolean>(false);
  const [cacheStats, setCacheStats] = useState<{ total: number; today: number }>({ total: 0, today: 0 });

  useEffect(() => {
    loadStrategies();
    loadCacheStats();
  }, []);

  // 监听进度事件
  useEffect(() => {
    const unsubscribe = EventsOn('selector:progress', (data: ProgressInfo) => {
      setProgress(data);
      
      // 如果完成，更新结果
      if (data.done) {
        setLoading(false);
        if (data.results && data.results.length > 0) {
          setResult({
            strategy: selectedStrategy,
            strategyName: strategies.find(s => s.id === selectedStrategy)?.name || selectedStrategy,
            date: new Date().toISOString().split('T')[0],
            stocks: data.results,
            total: data.results.length,
            params: { priceMin, priceMax }
          });
          setSelectedStocks(new Set(data.results.map(s => s.symbol)));
        }
      }
    });

    return () => {
      unsubscribe();
    };
  }, [selectedStrategy, strategies, priceMin, priceMax]);

  const loadStrategies = async () => {
    const list = await getSelectorStrategies();
    setStrategies(list);
  };

  const loadCacheStats = async () => {
    const stats = await getSelectorCacheStats();
    setCacheStats(stats);
  };

  const handleRunSelector = useCallback(async () => {
    setLoading(true);
    setProgress(null);
    setResult(null);
    setSelectedStocks(new Set());

    try {
      const res = await runSelector(selectedStrategy, priceMin, priceMax);
      if (res) {
        setResult(res);
        // 默认全选
        setSelectedStocks(new Set(res.stocks.map(s => s.symbol)));
      }
      // 刷新缓存统计
      await loadCacheStats();
    } catch (err) {
      console.error('Selector failed:', err);
    } finally {
      setLoading(false);
    }
  }, [selectedStrategy, priceMin, priceMax]);

  const handleCancelSelector = useCallback(async () => {
    await cancelSelector();
  }, []);

  const handleSaveRecord = useCallback(async () => {
    if (!result) return;
    setSaving(true);
    try {
      await saveSelectorRecord(result);
    } catch (err) {
      console.error('Save record failed:', err);
    } finally {
      setSaving(false);
    }
  }, [result]);

  const handleAddToWatchlist = useCallback(async () => {
    if (!result) return;
    const stocksToAdd = result.stocks.filter(s => selectedStocks.has(s.symbol));
    if (stocksToAdd.length === 0) return;

    setSaving(true);
    try {
      const added = await addStocksToWatchlist(stocksToAdd);
      if (added.length > 0) {
        // 更新记录中的已添加标记
        await saveSelectorRecord(result);
        if (onStockAdded) {
          onStockAdded();
        }
      }
    } catch (err) {
      console.error('Add to watchlist failed:', err);
    } finally {
      setSaving(false);
    }
  }, [result, selectedStocks, onStockAdded]);

  const toggleStock = (symbol: string) => {
    setSelectedStocks(prev => {
      const next = new Set(prev);
      if (next.has(symbol)) {
        next.delete(symbol);
      } else {
        next.add(symbol);
      }
      return next;
    });
  };

  const toggleAll = () => {
    if (!result) return;
    if (selectedStocks.size === result.stocks.length) {
      setSelectedStocks(new Set());
    } else {
      setSelectedStocks(new Set(result.stocks.map(s => s.symbol)));
    }
  };

  const currentStrategy = strategies.find(s => s.id === selectedStrategy);

  // 计算进度百分比
  const progressPercent = progress ? Math.round((progress.processed / progress.total) * 100) : 0;

  return (
    <div className="flex flex-col h-full">
      {/* 策略选择区域 */}
      <div className={`p-3 border-b ${colors.isDark ? 'border-slate-700' : 'border-slate-200'}`}>
        <div className="mb-2">
          <button
            onClick={() => setShowStrategies(!showStrategies)}
            className={`w-full flex items-center justify-between px-3 py-2 rounded-lg text-sm ${
              colors.isDark 
                ? 'bg-slate-800 text-slate-200 hover:bg-slate-700' 
                : 'bg-slate-100 text-slate-700 hover:bg-slate-200'
            }`}
          >
            <span>{currentStrategy?.name || '选择策略'}</span>
            {showStrategies ? <ChevronUp size={16} /> : <ChevronDown size={16} />}
          </button>
        </div>

        {showStrategies && (
          <div className={`mb-2 rounded-lg overflow-hidden ${
            colors.isDark ? 'bg-slate-800' : 'bg-slate-100'
          }`}>
            {strategies.map(strategy => (
              <button
                key={strategy.id}
                onClick={() => {
                  setSelectedStrategy(strategy.id);
                  setShowStrategies(false);
                }}
                className={`w-full text-left px-3 py-2 text-sm transition-colors ${
                  selectedStrategy === strategy.id
                    ? 'bg-accent/20 text-accent-2'
                    : colors.isDark
                      ? 'hover:bg-slate-700 text-slate-300'
                      : 'hover:bg-slate-200 text-slate-600'
                }`}
              >
                <div className="font-medium">{strategy.name}</div>
                <div className={`text-xs mt-0.5 ${colors.isDark ? 'text-slate-500' : 'text-slate-400'}`}>
                  {strategy.description}
                </div>
              </button>
            ))}
          </div>
        )}

        {/* 价格范围 */}
        <div className="flex items-center gap-2 mb-2">
          <span className={`text-xs ${colors.isDark ? 'text-slate-400' : 'text-slate-500'}`}>股价:</span>
          <input
            type="number"
            value={priceMin}
            onChange={(e) => setPriceMin(Number(e.target.value))}
            className={`w-16 px-2 py-1 text-xs rounded ${
              colors.isDark 
                ? 'bg-slate-800 text-slate-200 border-slate-600' 
                : 'bg-white text-slate-700 border-slate-300'
            } border`}
            min={0}
            step={1}
          />
          <span className={`text-xs ${colors.isDark ? 'text-slate-500' : 'text-slate-400'}`}>~</span>
          <input
            type="number"
            value={priceMax}
            onChange={(e) => setPriceMax(Number(e.target.value))}
            className={`w-16 px-2 py-1 text-xs rounded ${
              colors.isDark 
                ? 'bg-slate-800 text-slate-200 border-slate-600' 
                : 'bg-white text-slate-700 border-slate-300'
            } border`}
            min={0}
            step={1}
          />
          <span className={`text-xs ${colors.isDark ? 'text-slate-500' : 'text-slate-400'}`}>元</span>
        </div>

        {/* 缓存统计 */}
        {cacheStats.total > 0 && (
          <div className={`flex items-center gap-1 mb-2 text-xs ${colors.isDark ? 'text-slate-500' : 'text-slate-400'}`}>
            <Database size={12} />
            <span>已缓存 {cacheStats.total} 只股票数据</span>
            {cacheStats.today > 0 && (
              <span className={colors.isDark ? 'text-green-400' : 'text-green-600'}>
                ({new Date().toLocaleDateString('zh-CN')} 已更新 {cacheStats.today} 只)
              </span>
            )}
          </div>
        )}

        {/* 执行/取消按钮 */}
        {loading ? (
          <button
            onClick={handleCancelSelector}
            className="w-full flex items-center justify-center gap-2 px-4 py-2 rounded-lg text-sm font-medium bg-red-500 text-white hover:bg-red-600 transition-colors"
          >
            <XCircle size={16} />
            <span>取消选股</span>
          </button>
        ) : (
          <button
            onClick={handleRunSelector}
            className="w-full flex items-center justify-center gap-2 px-4 py-2 rounded-lg text-sm font-medium bg-accent text-white hover:bg-accent/80 transition-colors"
          >
            <Play size={16} />
            <span>开始选股</span>
          </button>
        )}
      </div>

      {/* 进度区域 */}
      {loading && progress && (
        <div className={`px-3 py-2 border-b ${colors.isDark ? 'border-slate-700 bg-slate-800/50' : 'border-slate-200 bg-slate-50'}`}>
          {/* 进度条 */}
          <div className="mb-2">
            <div className="flex items-center justify-between mb-1">
              <span className={`text-xs ${colors.isDark ? 'text-slate-400' : 'text-slate-500'}`}>
                扫描进度
              </span>
              <span className={`text-xs font-medium ${colors.isDark ? 'text-slate-300' : 'text-slate-600'}`}>
                {progress.processed}/{progress.total} ({progressPercent}%)
              </span>
            </div>
            <div className={`h-2 rounded-full overflow-hidden ${colors.isDark ? 'bg-slate-700' : 'bg-slate-200'}`}>
              <div 
                className="h-full bg-accent rounded-full transition-all duration-300"
                style={{ width: `${progressPercent}%` }}
              />
            </div>
          </div>

          {/* 当前状态 */}
          <div className="flex items-center justify-between">
            <span className={`text-xs truncate ${colors.isDark ? 'text-slate-500' : 'text-slate-400'}`}>
              {progress.current ? `正在分析: ${progress.current}` : '准备中...'}
            </span>
            <span className={`text-xs font-medium ${colors.isDark ? 'text-accent-2' : 'text-accent'}`}>
              已找到 {progress.found} 只
            </span>
          </div>

          {/* 实时结果预览 */}
          {progress.results && progress.results.length > 0 && (
            <div className="mt-2 space-y-1 max-h-32 overflow-y-auto">
              {progress.results.slice(-5).map(stock => (
                <div
                  key={stock.symbol}
                  className={`flex items-center justify-between py-1 px-2 rounded text-xs ${
                    colors.isDark ? 'bg-slate-700/50' : 'bg-white'
                  }`}
                >
                  <span className={colors.isDark ? 'text-slate-300' : 'text-slate-600'}>
                    {stock.name}
                  </span>
                  <span className={`font-mono ${cc.getColorClass(stock.change >= 0)}`}>
                    {stock.change >= 0 ? '+' : ''}{stock.changePercent.toFixed(2)}%
                  </span>
                </div>
              ))}
              {progress.results.length > 5 && (
                <div className={`text-xs text-center ${colors.isDark ? 'text-slate-500' : 'text-slate-400'}`}>
                  ...还有 {progress.results.length - 5} 只
                </div>
              )}
            </div>
          )}
        </div>
      )}

      {/* 结果区域 */}
      <div className="flex-1 overflow-y-auto">
        {result && !loading && (
          <div className="p-3">
            {/* 结果统计 */}
            <div className={`flex items-center justify-between mb-3 px-2 py-1.5 rounded ${
              colors.isDark ? 'bg-slate-800' : 'bg-slate-100'
            }`}>
              <span className={`text-xs ${colors.isDark ? 'text-slate-400' : 'text-slate-500'}`}>
                共 {result.total} 只股票（按得分排序）
              </span>
              <div className="flex items-center gap-2">
                <button
                  onClick={toggleAll}
                  className={`text-xs px-2 py-0.5 rounded ${
                    colors.isDark 
                      ? 'text-slate-400 hover:text-slate-200 hover:bg-slate-700' 
                      : 'text-slate-500 hover:text-slate-700 hover:bg-slate-200'
                  }`}
                >
                  {selectedStocks.size === result.stocks.length ? '取消全选' : '全选'}
                </button>
              </div>
            </div>

            {/* 行业分布 */}
            {result.stocks.length > 0 && (() => {
              const industryMap = new Map<string, number>();
              result.stocks.forEach(s => {
                const ind = s.industry || '未知';
                industryMap.set(ind, (industryMap.get(ind) || 0) + 1);
              });
              const sorted = Array.from(industryMap.entries()).sort((a, b) => b[1] - a[1]);
              const maxCount = sorted[0]?.[1] || 1;
              const topN = sorted.slice(0, 10);
              
              return (
                <div className={`mb-3 p-2 rounded ${colors.isDark ? 'bg-slate-800/50' : 'bg-slate-50'}`}>
                  <div className={`text-xs font-bold mb-2 ${colors.isDark ? 'text-slate-400' : 'text-slate-500'}`}>
                    行业分布 (TOP 10)
                  </div>
                  <div className="space-y-1">
                    {topN.map(([industry, count]) => (
                      <div key={industry} className="flex items-center gap-2">
                        <span className={`text-xs w-20 truncate ${colors.isDark ? 'text-slate-400' : 'text-slate-500'}`}>
                          {industry}
                        </span>
                        <div className={`flex-1 h-3 rounded-full overflow-hidden ${colors.isDark ? 'bg-slate-700' : 'bg-slate-200'}`}>
                          <div 
                            className="h-full bg-accent rounded-full transition-all"
                            style={{ width: `${(count / maxCount) * 100}%` }}
                          />
                        </div>
                        <span className={`text-xs w-6 text-right ${colors.isDark ? 'text-slate-400' : 'text-slate-500'}`}>
                          {count}
                        </span>
                      </div>
                    ))}
                  </div>
                </div>
              );
            })()}

            {/* 股票列表 */}
            <div className="space-y-1">
              {[...result.stocks]
                .sort((a, b) => b.score - a.score)
                .map(stock => (
                <div
                  key={stock.symbol}
                  className={`flex items-center gap-2 p-2 rounded-lg cursor-pointer transition-colors ${
                    colors.isDark 
                      ? 'hover:bg-slate-800/60' 
                      : 'hover:bg-slate-100'
                  }`}
                  onClick={() => onStockSelect(stock.symbol)}
                >
                  <input
                    type="checkbox"
                    checked={selectedStocks.has(stock.symbol)}
                    onChange={(e) => {
                      e.stopPropagation();
                      toggleStock(stock.symbol);
                    }}
                    className="w-4 h-4 accent-accent"
                  />
                  {/* 得分 */}
                  <div 
                    className={`w-10 h-10 rounded-lg flex items-center justify-center text-xs font-bold cursor-help ${
                      stock.score >= 80 
                        ? 'bg-green-500/20 text-green-500' 
                        : stock.score >= 60 
                          ? 'bg-yellow-500/20 text-yellow-500'
                          : 'bg-slate-500/20 text-slate-400'
                    }`}
                    title={stock.scoreDetail || `得分: ${stock.score.toFixed(0)}`}
                  >
                    {stock.score.toFixed(0)}
                  </div>
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2">
                      <span className={`font-medium text-sm truncate ${
                        colors.isDark ? 'text-slate-200' : 'text-slate-700'
                      }`}>
                        {stock.name}
                      </span>
                      <span className={`text-xs font-mono ${
                        colors.isDark ? 'text-slate-500' : 'text-slate-400'
                      }`}>
                        {stock.symbol}
                      </span>
                    </div>
                    {stock.industry && (
                      <span className={`text-xs px-1.5 py-0.5 rounded mt-0.5 inline-block ${
                        colors.isDark ? 'bg-slate-700 text-slate-400' : 'bg-slate-100 text-slate-500'
                      }`}>
                        {stock.industry}
                      </span>
                    )}
                  </div>
                  <div className="text-right">
                    <div className={`font-mono text-sm ${cc.getColorClass(stock.change >= 0)}`}>
                      {stock.price.toFixed(2)}
                    </div>
                    <div className={`text-xs font-mono flex items-center justify-end ${cc.getColorClass(stock.change >= 0)}`}>
                      {stock.change >= 0 ? <TrendingUp size={10} className="mr-0.5" /> : <TrendingDown size={10} className="mr-0.5" />}
                      {stock.change >= 0 ? '+' : ''}{stock.changePercent.toFixed(2)}%
                    </div>
                  </div>
                  <button
                    onClick={async (e) => {
                      e.stopPropagation();
                      const added = await addStocksToWatchlist([stock]);
                      if (added.length > 0 && onStockAdded) {
                        onStockAdded();
                      }
                    }}
                    className={`p-1 rounded transition-colors ${
                      colors.isDark 
                        ? 'hover:bg-green-500/20 text-slate-500 hover:text-green-400' 
                        : 'hover:bg-green-100 text-slate-400 hover:text-green-600'
                    }`}
                    title="添加到自选"
                  >
                    <Plus size={14} />
                  </button>
                </div>
              ))}
            </div>

            {/* 操作按钮 */}
            {result.stocks.length > 0 && (
              <div className="mt-3 flex gap-2">
                <button
                  onClick={handleSaveRecord}
                  disabled={saving}
                  className={`flex-1 flex items-center justify-center gap-1 px-3 py-2 rounded-lg text-xs font-medium transition-colors ${
                    saving
                      ? 'bg-slate-600 text-slate-400 cursor-not-allowed'
                      : colors.isDark
                        ? 'bg-slate-700 text-slate-200 hover:bg-slate-600'
                        : 'bg-slate-200 text-slate-700 hover:bg-slate-300'
                  }`}
                >
                  <Save size={14} />
                  <span>保存记录</span>
                </button>
                <button
                  onClick={handleAddToWatchlist}
                  disabled={saving || selectedStocks.size === 0}
                  className={`flex-1 flex items-center justify-center gap-1 px-3 py-2 rounded-lg text-xs font-medium transition-colors ${
                    saving || selectedStocks.size === 0
                      ? 'bg-slate-600 text-slate-400 cursor-not-allowed'
                      : 'bg-accent text-white hover:bg-accent/80'
                  }`}
                >
                  <Plus size={14} />
                  <span>添加到自选 ({selectedStocks.size})</span>
                </button>
              </div>
            )}
          </div>
        )}

        {/* 取消提示 */}
        {progress && progress.cancelled && !loading && (
          <div className={`p-4 text-center ${colors.isDark ? 'text-slate-400' : 'text-slate-500'}`}>
            <XCircle size={32} className="mx-auto mb-2 text-yellow-500" />
            <p className="text-sm">选股已取消</p>
            {progress.found > 0 && (
              <p className="text-xs mt-1">已找到 {progress.found} 只股票</p>
            )}
          </div>
        )}

        {/* 空状态 */}
        {!result && !loading && !progress && (
          <div className="flex flex-col items-center justify-center h-full text-center p-4">
            <Search size={48} className={`mb-4 ${colors.isDark ? 'text-slate-600' : 'text-slate-300'}`} />
            <p className={`text-sm ${colors.isDark ? 'text-slate-500' : 'text-slate-400'}`}>
              选择策略后点击"开始选股"
            </p>
            <p className={`text-xs mt-1 ${colors.isDark ? 'text-slate-600' : 'text-slate-300'}`}>
              全市场扫描约需1-2分钟
            </p>
          </div>
        )}
      </div>
    </div>
  );
};
