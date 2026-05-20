import React, { useState, useEffect, useCallback } from 'react';
import { SelectorRecord, SelectorStock } from '../types';
import { getSelectorRecords, deleteSelectorRecord, addStocksToWatchlist } from '../services/selectorService';
import { useTheme } from '../contexts/ThemeContext';
import { useCandleColor } from '../contexts/CandleColorContext';
import { X, Trash2, Plus, Calendar, TrendingUp, TrendingDown, ChevronDown, ChevronRight } from 'lucide-react';

interface SelectorRecordDialogProps {
  isOpen: boolean;
  onClose: () => void;
  onStockSelect: (symbol: string) => void;
}

export const SelectorRecordDialog: React.FC<SelectorRecordDialogProps> = ({
  isOpen,
  onClose,
  onStockSelect,
}) => {
  const { colors } = useTheme();
  const cc = useCandleColor();
  const [records, setRecords] = useState<SelectorRecord[]>([]);
  const [loading, setLoading] = useState<boolean>(false);
  const [expandedDates, setExpandedDates] = useState<Set<string>>(new Set());
  const [expandedRecords, setExpandedRecords] = useState<Set<string>>(new Set());

  useEffect(() => {
    if (isOpen) {
      loadRecords();
    }
  }, [isOpen]);

  const loadRecords = async () => {
    setLoading(true);
    try {
      const data = await getSelectorRecords();
      setRecords(data);
      // 默认展开最近一天
      if (data.length > 0) {
        const latestDate = data[0].date;
        setExpandedDates(new Set([latestDate]));
      }
    } catch (err) {
      console.error('Failed to load records:', err);
    } finally {
      setLoading(false);
    }
  };

  const handleDelete = useCallback(async (date: string, strategy: string) => {
    try {
      await deleteSelectorRecord(date, strategy as any);
      await loadRecords();
    } catch (err) {
      console.error('Failed to delete record:', err);
    }
  }, []);

  const handleAddToWatchlist = useCallback(async (stocks: SelectorStock[]) => {
    try {
      await addStocksToWatchlist(stocks);
    } catch (err) {
      console.error('Failed to add to watchlist:', err);
    }
  }, []);

  const toggleDate = (date: string) => {
    setExpandedDates(prev => {
      const next = new Set(prev);
      if (next.has(date)) {
        next.delete(date);
      } else {
        next.add(date);
      }
      return next;
    });
  };

  const toggleRecord = (key: string) => {
    setExpandedRecords(prev => {
      const next = new Set(prev);
      if (next.has(key)) {
        next.delete(key);
      } else {
        next.add(key);
      }
      return next;
    });
  };

  // 按日期分组
  const groupedRecords = records.reduce((acc, record) => {
    if (!acc[record.date]) {
      acc[record.date] = [];
    }
    acc[record.date].push(record);
    return acc;
  }, {} as Record<string, SelectorRecord[]>);

  const dates = Object.keys(groupedRecords).sort((a, b) => b.localeCompare(a));

  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
      <div className={`w-full max-w-2xl max-h-[80vh] rounded-xl shadow-2xl flex flex-col ${
        colors.isDark ? 'bg-slate-900' : 'bg-white'
      }`}>
        {/* Header */}
        <div className={`flex items-center justify-between p-4 border-b ${
          colors.isDark ? 'border-slate-700' : 'border-slate-200'
        }`}>
          <div className="flex items-center gap-2">
            <Calendar size={20} className="text-accent-2" />
            <h2 className={`text-lg font-bold ${colors.isDark ? 'text-white' : 'text-slate-800'}`}>
              选股记录
            </h2>
          </div>
          <button
            onClick={onClose}
            className={`p-1.5 rounded-lg transition-colors ${
              colors.isDark 
                ? 'hover:bg-slate-800 text-slate-400 hover:text-white' 
                : 'hover:bg-slate-100 text-slate-500 hover:text-slate-800'
            }`}
          >
            <X size={20} />
          </button>
        </div>

        {/* Content */}
        <div className="flex-1 overflow-y-auto p-4">
          {loading ? (
            <div className="flex items-center justify-center h-32">
              <div className="text-sm text-slate-500">加载中...</div>
            </div>
          ) : dates.length === 0 ? (
            <div className="flex flex-col items-center justify-center h-32">
              <Calendar size={48} className={`mb-4 ${colors.isDark ? 'text-slate-600' : 'text-slate-300'}`} />
              <p className={`text-sm ${colors.isDark ? 'text-slate-500' : 'text-slate-400'}`}>
                暂无选股记录
              </p>
            </div>
          ) : (
            <div className="space-y-3">
              {dates.map(date => (
                <div key={date} className={`rounded-lg border ${
                  colors.isDark ? 'border-slate-700' : 'border-slate-200'
                }`}>
                  {/* Date Header */}
                  <button
                    onClick={() => toggleDate(date)}
                    className={`w-full flex items-center justify-between p-3 ${
                      colors.isDark ? 'hover:bg-slate-800' : 'hover:bg-slate-50'
                    }`}
                  >
                    <div className="flex items-center gap-2">
                      {expandedDates.has(date) ? (
                        <ChevronDown size={16} className="text-slate-400" />
                      ) : (
                        <ChevronRight size={16} className="text-slate-400" />
                      )}
                      <span className={`font-medium ${colors.isDark ? 'text-slate-200' : 'text-slate-700'}`}>
                        {date}
                      </span>
                      <span className={`text-xs ${colors.isDark ? 'text-slate-500' : 'text-slate-400'}`}>
                        ({groupedRecords[date].length} 个策略)
                      </span>
                    </div>
                  </button>

                  {/* Records for this date */}
                  {expandedDates.has(date) && (
                    <div className={`border-t ${colors.isDark ? 'border-slate-700' : 'border-slate-200'}`}>
                      {groupedRecords[date].map(record => {
                        const recordKey = `${record.date}-${record.strategy}`;
                        const isExpanded = expandedRecords.has(recordKey);
                        
                        return (
                          <div key={recordKey} className={`border-b last:border-b-0 ${
                            colors.isDark ? 'border-slate-800' : 'border-slate-100'
                          }`}>
                            {/* Record Header */}
                            <div
                              className={`flex items-center justify-between p-3 cursor-pointer ${
                                colors.isDark ? 'hover:bg-slate-800/50' : 'hover:bg-slate-50'
                              }`}
                              onClick={() => toggleRecord(recordKey)}
                            >
                              <div className="flex items-center gap-2">
                                {isExpanded ? (
                                  <ChevronDown size={14} className="text-slate-400" />
                                ) : (
                                  <ChevronRight size={14} className="text-slate-400" />
                                )}
                                <span className={`text-sm font-medium ${
                                  colors.isDark ? 'text-slate-300' : 'text-slate-600'
                                }`}>
                                  {record.strategyName}
                                </span>
                                <span className={`text-xs px-2 py-0.5 rounded ${
                                  colors.isDark ? 'bg-slate-700 text-slate-400' : 'bg-slate-100 text-slate-500'
                                }`}>
                                  {record.stocks.length} 只
                                </span>
                              </div>
                              <div className="flex items-center gap-1">
                                <button
                                  onClick={(e) => {
                                    e.stopPropagation();
                                    handleAddToWatchlist(record.stocks);
                                  }}
                                  className={`p-1.5 rounded transition-colors ${
                                    colors.isDark 
                                      ? 'hover:bg-slate-700 text-slate-400 hover:text-accent-2' 
                                      : 'hover:bg-slate-200 text-slate-500 hover:text-accent-2'
                                  }`}
                                  title="添加到自选股"
                                >
                                  <Plus size={14} />
                                </button>
                                <button
                                  onClick={(e) => {
                                    e.stopPropagation();
                                    handleDelete(record.date, record.strategy);
                                  }}
                                  className={`p-1.5 rounded transition-colors ${
                                    colors.isDark 
                                      ? 'hover:bg-red-500/20 text-slate-400 hover:text-red-400' 
                                      : 'hover:bg-red-100 text-slate-500 hover:text-red-500'
                                  }`}
                                  title="删除记录"
                                >
                                  <Trash2 size={14} />
                                </button>
                              </div>
                            </div>

                            {/* Stock List */}
                            {isExpanded && (
                              <div className={`px-3 pb-3 ${colors.isDark ? 'text-slate-400' : 'text-slate-500'}`}>
                                <div className="space-y-1">
                                  {record.stocks.map(stock => (
                                    <div
                                      key={stock.symbol}
                                      className={`flex items-center justify-between py-1.5 px-2 rounded cursor-pointer ${
                                        colors.isDark ? 'hover:bg-slate-800' : 'hover:bg-slate-50'
                                      }`}
                                      onClick={() => {
                                        onStockSelect(stock.symbol);
                                        onClose();
                                      }}
                                    >
                                      <div className="flex items-center gap-2">
                                        <span className={`text-sm ${colors.isDark ? 'text-slate-300' : 'text-slate-600'}`}>
                                          {stock.name}
                                        </span>
                                        <span className={`text-xs font-mono ${
                                          colors.isDark ? 'text-slate-500' : 'text-slate-400'
                                        }`}>
                                          {stock.symbol}
                                        </span>
                                      </div>
                                      <div className="flex items-center gap-2">
                                        <span className={`text-sm font-mono ${cc.getColorClass(stock.change >= 0)}`}>
                                          {stock.price.toFixed(2)}
                                        </span>
                                        <span className={`text-xs font-mono flex items-center ${cc.getColorClass(stock.change >= 0)}`}>
                                          {stock.change >= 0 ? <TrendingUp size={10} className="mr-0.5" /> : <TrendingDown size={10} className="mr-0.5" />}
                                          {stock.change >= 0 ? '+' : ''}{stock.changePercent.toFixed(2)}%
                                        </span>
                                      </div>
                                    </div>
                                  ))}
                                </div>
                                <div className={`text-xs mt-2 ${colors.isDark ? 'text-slate-600' : 'text-slate-400'}`}>
                                  执行时间: {new Date(record.executedAt).toLocaleString('zh-CN')}
                                </div>
                              </div>
                            )}
                          </div>
                        );
                      })}
                    </div>
                  )}
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  );
};
