import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Activity, RefreshCw, TrendingUp, X } from 'lucide-react';
import { GetBoardFundFlow, GetBoardLeaders, GetStockMoves } from '../../wailsjs/go/main/App';
import { useMarketStatus } from '../hooks/useMarketStatus';

interface MarketMovesDialogProps {
  isOpen: boolean;
  onClose: () => void;
}

type MoveTab = 'stock' | 'board';
type BoardCategory = 'industry' | 'concept' | 'region';
type StockMoveType = 'surge' | 'drop' | 'change_up' | 'change_down' | 'mainflow' | 'turnover';

interface BoardItem {
  code: string;
  name: string;
  changePercent: number;
  mainNetInflow: number;
  superNetInflow: number;
  largeNetInflow: number;
}

interface BoardFlowResult {
  category: string;
  items: BoardItem[];
  updateTime?: string;
}

interface LeaderItem {
  rank: number;
  code: string;
  name: string;
  price: number;
  changePercent: number;
  mainNetInflow: number;
  mainNetInflowRatio: number;
  score: number;
}

interface BoardLeaderResult {
  boardCode: string;
  items: LeaderItem[];
  updateTime?: string;
}

interface StockMoveItem {
  rank: number;
  code: string;
  name: string;
  price: number;
  changePercent: number;
  speed: number;
  turnoverRate: number;
  volume: number;
  amount: number;
  mainNetInflow: number;
  mainNetInflowRatio: number;
  updateTime?: string;
}

interface StockMoveResult {
  moveType: string;
  items: StockMoveItem[];
  updateTime?: string;
}

// 不同市场状态下的自动刷新间隔（秒），0 表示不自动刷新
const REFRESH_INTERVAL_BY_STATUS: Record<string, number> = {
  trading: 30,       // 交易中：30秒
  pre_market: 0,     // 盘前：不刷（数据停留在前一交易日）
  lunch_break: 0,    // 午休：不刷（行情冻结）
  closed: 0,         // 已收盘/休市：不刷（数据基本不变）
};

const DEFAULT_REFRESH_INTERVAL_SECONDS = 30;

// 根据市场状态返回刷新间隔（秒）
function getRefreshIntervalSeconds(status: string | undefined): number {
  if (!status) return DEFAULT_REFRESH_INTERVAL_SECONDS;
  if (status in REFRESH_INTERVAL_BY_STATUS) {
    return REFRESH_INTERVAL_BY_STATUS[status];
  }
  return DEFAULT_REFRESH_INTERVAL_SECONDS;
}

// 根据市场状态生成人类可读的"刷新策略"提示
function getRefreshTip(status: string | undefined, statusText: string | undefined): string {
  if (!status) return '自动刷新';
  switch (status) {
    case 'trading':
      return '交易中 · 30秒自动刷新';
    case 'pre_market':
      return `${statusText || '盘前'} · 已暂停自动刷新`;
    case 'lunch_break':
      return '午休 · 已暂停自动刷新';
    case 'closed':
      return `${statusText || '已收盘'} · 已暂停自动刷新`;
    default:
      return statusText || '自动刷新';
  }
}

const CATEGORY_LABELS: Record<BoardCategory, string> = {
  industry: '行业',
  concept: '概念',
  region: '地域',
};

const STOCK_MOVE_LABELS: Record<StockMoveType, string> = {
  surge: '涨速榜',
  drop: '跌速榜',
  change_up: '涨幅榜',
  change_down: '跌幅榜',
  mainflow: '主力净流入',
  turnover: '换手活跃',
};

const formatAmount = (value: number): string => {
  if (!Number.isFinite(value) || value === 0) return '0';
  const abs = Math.abs(value);
  if (abs >= 1e8) return `${(value / 1e8).toFixed(2)}亿`;
  if (abs >= 1e4) return `${(value / 1e4).toFixed(2)}万`;
  return value.toFixed(0);
};

export const MarketMovesDialog: React.FC<MarketMovesDialogProps> = ({ isOpen, onClose }) => {
  const { status: marketStatus } = useMarketStatus();
  const refreshIntervalSeconds = useMemo(
    () => getRefreshIntervalSeconds(marketStatus?.status),
    [marketStatus?.status]
  );
  const refreshTip = useMemo(
    () => getRefreshTip(marketStatus?.status, marketStatus?.statusText),
    [marketStatus?.status, marketStatus?.statusText]
  );

  const [activeTab, setActiveTab] = useState<MoveTab>('board');
  const [category, setCategory] = useState<BoardCategory>('industry');
  const [stockMoveType, setStockMoveType] = useState<StockMoveType>('surge');
  const [loading, setLoading] = useState(false);
  const [boardFlow, setBoardFlow] = useState<BoardFlowResult | null>(null);
  const [error, setError] = useState<string>('');
  const [stockLoading, setStockLoading] = useState(false);
  const [stockError, setStockError] = useState('');
  const [stockMoves, setStockMoves] = useState<StockMoveResult | null>(null);
  const [selectedBoard, setSelectedBoard] = useState<BoardItem | null>(null);
  const [leaderLoading, setLeaderLoading] = useState(false);
  const [leaderError, setLeaderError] = useState('');
  const [leaderResult, setLeaderResult] = useState<BoardLeaderResult | null>(null);
  const [autoRefreshCountdown, setAutoRefreshCountdown] = useState(refreshIntervalSeconds);

  const resetAutoRefreshCountdown = useCallback(() => {
    setAutoRefreshCountdown(refreshIntervalSeconds);
  }, [refreshIntervalSeconds]);

  const loadBoardLeaders = useCallback(async (boardCode: string) => {
    setLeaderLoading(true);
    setLeaderError('');
    try {
      const result = await GetBoardLeaders(boardCode, 6);
      const normalized: BoardLeaderResult = {
        boardCode: result?.boardCode || boardCode,
        items: Array.isArray(result?.items) ? result.items : [],
        updateTime: result?.updateTime,
      };
      setLeaderResult(normalized);
      if (!normalized.items.length) {
        setLeaderError('该板块暂未返回可用龙头候选');
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : '获取龙头推荐失败';
      setLeaderError(message);
      setLeaderResult(null);
    } finally {
      setLeaderLoading(false);
    }
  }, []);

  const loadBoardFlow = useCallback(async (nextCategory: BoardCategory, preferredBoardCode?: string) => {
    if (loading) return; // 防止重复请求
    setLoading(true);
    setError('');
    try {
      const result = await GetBoardFundFlow(nextCategory, 1, 30);
      const normalized: BoardFlowResult = {
        category: result?.category || nextCategory,
        items: Array.isArray(result?.items) ? result.items : [],
        updateTime: result?.updateTime,
      };
      setBoardFlow(normalized);
      if (normalized.items.length > 0) {
        const nextSelected =
          normalized.items.find(item => item.code === preferredBoardCode) || normalized.items[0];
        setSelectedBoard(nextSelected);
        void loadBoardLeaders(nextSelected.code);
      } else {
        setSelectedBoard(null);
        setLeaderResult(null);
        setLeaderError('');
        setError(marketStatus?.status === 'trading'
          ? '暂无可展示的板块异动数据'
          : '非交易时段暂无实时数据，交易时段（9:30-15:00）将自动更新');
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : '获取板块异动失败';
      setError(message);
      // 不清空现有数据，保持上次的结果
    } finally {
      setLoading(false);
    }
  }, [loadBoardLeaders, loading, marketStatus]);

  const loadStockMoves = useCallback(async (nextType: StockMoveType) => {
    if (stockLoading) return; // 防止重复请求
    setStockLoading(true);
    setStockError('');
    try {
      const result = await GetStockMoves(nextType, 1, 50);
      const normalized: StockMoveResult = {
        moveType: result?.moveType || nextType,
        items: Array.isArray(result?.items) ? result.items : [],
        updateTime: result?.updateTime,
      };
      setStockMoves(normalized);
      if (!normalized.items.length) {
        setStockError(marketStatus?.status === 'trading'
          ? '暂无可展示的盘口异动数据'
          : '非交易时段暂无实时数据，交易时段（9:30-15:00）将自动更新');
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : '获取盘口异动失败';
      setStockError(message);
      // 不清空现有数据，保持上次的结果
    } finally {
      setStockLoading(false);
    }
  }, [stockLoading, marketStatus]);

  useEffect(() => {
    if (!isOpen) return;
    setActiveTab('board');
    setCategory('industry');
    setStockMoveType('surge');
    resetAutoRefreshCountdown();
    void loadBoardFlow('industry');
  }, [isOpen, loadBoardFlow, resetAutoRefreshCountdown]);

  useEffect(() => {
    if (!isOpen || activeTab !== 'stock') return;
    void loadStockMoves(stockMoveType);
  }, [isOpen, activeTab, stockMoveType, loadStockMoves]);

  // 市场状态变化时重置倒计时基线
  useEffect(() => {
    setAutoRefreshCountdown(refreshIntervalSeconds);
  }, [refreshIntervalSeconds]);

  // 倒计时（仅在自动刷新启用时跑）
  useEffect(() => {
    if (!isOpen || refreshIntervalSeconds <= 0) return;
    const timer = window.setInterval(() => {
      setAutoRefreshCountdown(prev => (prev <= 1 ? refreshIntervalSeconds : prev - 1));
    }, 1000);
    return () => window.clearInterval(timer);
  }, [isOpen, refreshIntervalSeconds]);

  // 自动刷新（按市场状态决定是否启用）
  useEffect(() => {
    if (!isOpen || refreshIntervalSeconds <= 0) return;
    const timer = window.setInterval(() => {
      if (activeTab === 'stock') {
        void loadStockMoves(stockMoveType);
      } else {
        void loadBoardFlow(category, selectedBoard?.code);
      }
      resetAutoRefreshCountdown();
    }, refreshIntervalSeconds * 1000);
    return () => window.clearInterval(timer);
  }, [
    isOpen,
    activeTab,
    stockMoveType,
    category,
    selectedBoard?.code,
    loadBoardFlow,
    loadStockMoves,
    resetAutoRefreshCountdown,
    refreshIntervalSeconds,
  ]);

  const onChangeCategory = (nextCategory: BoardCategory) => {
    setCategory(nextCategory);
    resetAutoRefreshCountdown();
    void loadBoardFlow(nextCategory);
  };

  const onChangeStockMoveType = (nextType: StockMoveType) => {
    setStockMoveType(nextType);
    resetAutoRefreshCountdown();
    void loadStockMoves(nextType);
  };

  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="absolute inset-0 bg-black/60 backdrop-blur-sm" onClick={onClose} />
      <div className="relative w-[1120px] h-[760px] max-w-[92vw] max-h-[88vh] fin-panel border fin-divider rounded-xl shadow-2xl flex flex-col overflow-hidden">
        <div className="flex items-center justify-between px-5 py-4 border-b fin-divider">
          <div className="flex items-center gap-3">
            <Activity className="w-5 h-5 text-accent-2" />
            <div>
              <h2 className="text-lg font-semibold fin-text-primary">异动中心</h2>
              <div className="text-[11px] fin-text-tertiary">
                {refreshIntervalSeconds > 0
                  ? `${activeTab === 'stock' ? '盘口异动' : '板块异动'} · ${refreshTip} · ${autoRefreshCountdown}s 后更新`
                  : `${activeTab === 'stock' ? '盘口异动' : '板块异动'} · ${refreshTip}`}
              </div>
            </div>
          </div>
          <div className="flex items-center gap-3">
            <div className="text-[11px] fin-text-secondary">
              {refreshIntervalSeconds > 0
                ? (activeTab === 'stock'
                    ? (stockLoading ? '刷新中...' : '自动刷新中')
                    : (loading || leaderLoading ? '刷新中...' : '自动刷新中'))
                : '手动刷新模式'}
            </div>
            <button onClick={onClose} className="p-2 rounded-lg fin-hover transition-colors">
              <X className="w-4 h-4 fin-text-secondary" />
            </button>
          </div>
        </div>

        <div className="px-4 pt-3">
          <div className="flex items-center gap-2">
            <button
              onClick={() => setActiveTab('board')}
              className={`px-3 py-1.5 rounded-lg text-sm border transition-colors ${
                activeTab === 'board'
                  ? 'bg-accent/20 text-accent border-accent/40'
                  : 'fin-panel fin-text-secondary fin-divider hover:fin-text-primary'
              }`}
            >
              板块异动
            </button>
            <button
              onClick={() => setActiveTab('stock')}
              className={`px-3 py-1.5 rounded-lg text-sm border transition-colors ${
                activeTab === 'stock'
                  ? 'bg-accent/20 text-accent border-accent/40'
                  : 'fin-panel fin-text-secondary fin-divider hover:fin-text-primary'
              }`}
            >
              盘口异动
            </button>
          </div>
        </div>

        <div className="flex-1 px-4 py-3 overflow-hidden">
          {activeTab === 'stock' ? (
            <div className="h-full fin-panel border fin-divider rounded-lg p-4 overflow-auto fin-scrollbar">
              <div className="flex items-center justify-between mb-3">
                <div className="text-sm fin-text-primary font-medium">盘口异动</div>
                <div className="flex items-center gap-2">
                  <button
                    onClick={() => {
                      resetAutoRefreshCountdown();
                      void loadStockMoves(stockMoveType);
                    }}
                    className="p-1.5 rounded fin-hover transition-colors"
                    disabled={stockLoading}
                    title="刷新"
                  >
                    <RefreshCw className={`w-4 h-4 fin-text-secondary ${stockLoading ? 'animate-spin' : ''}`} />
                  </button>
                </div>
              </div>
              <div className="flex flex-wrap items-center gap-2 mb-3">
                {(Object.keys(STOCK_MOVE_LABELS) as StockMoveType[]).map((item) => (
                  <button
                    key={item}
                    onClick={() => onChangeStockMoveType(item)}
                    className={`px-2.5 py-1 rounded text-xs border ${stockMoveType === item
                      ? 'bg-accent/20 text-accent border-accent/40'
                      : 'fin-panel fin-text-secondary fin-divider hover:fin-text-primary'}`}
                  >
                    {STOCK_MOVE_LABELS[item]}
                  </button>
                ))}
                {stockMoves?.updateTime ? (
                  <span className="text-xs fin-text-tertiary">更新: {stockMoves.updateTime}</span>
                ) : null}
              </div>
              {stockLoading ? (
                <div className="h-[460px] flex items-center justify-center">
                  <RefreshCw className="w-5 h-5 fin-text-secondary animate-spin" />
                </div>
              ) : stockError ? (
                <div className="text-sm text-amber-300">{stockError}</div>
              ) : (
                <div className="overflow-auto fin-scrollbar h-[500px] rounded border fin-divider">
                  <table className="w-full text-sm">
                    <thead className="sticky top-0 fin-panel border-b fin-divider z-10">
                      <tr className="text-left">
                        <th className="px-3 py-2 font-medium fin-text-secondary">股票</th>
                        <th className="px-3 py-2 font-medium fin-text-secondary">现价</th>
                        <th className="px-3 py-2 font-medium fin-text-secondary">涨跌幅</th>
                        <th className="px-3 py-2 font-medium fin-text-secondary">涨速</th>
                        <th className="px-3 py-2 font-medium fin-text-secondary">主力净流入</th>
                        <th className="px-3 py-2 font-medium fin-text-secondary">换手率</th>
                      </tr>
                    </thead>
                    <tbody>
                      {(stockMoves?.items || []).map((item) => (
                        <tr key={item.code} className="border-b fin-divider/40 hover:bg-slate-700/20">
                          <td className="px-3 py-2">
                            <div className="font-medium fin-text-primary">{item.rank}. {item.name}</div>
                            <div className="text-xs fin-text-tertiary font-mono">{item.code}</div>
                          </td>
                          <td className="px-3 py-2 font-mono fin-text-primary">{item.price.toFixed(2)}</td>
                          <td className={`px-3 py-2 font-mono ${item.changePercent >= 0 ? 'text-red-400' : 'text-green-400'}`}>
                            {item.changePercent >= 0 ? '+' : ''}
                            {item.changePercent.toFixed(2)}%
                          </td>
                          <td className={`px-3 py-2 font-mono ${item.speed >= 0 ? 'text-red-400' : 'text-green-400'}`}>
                            {item.speed >= 0 ? '+' : ''}
                            {item.speed.toFixed(2)}%
                          </td>
                          <td className={`px-3 py-2 font-mono ${item.mainNetInflow >= 0 ? 'text-red-400' : 'text-green-400'}`}>
                            {formatAmount(item.mainNetInflow)}
                          </td>
                          <td className="px-3 py-2 font-mono fin-text-primary">{item.turnoverRate.toFixed(2)}%</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </div>
          ) : (
            <div className="h-full fin-panel border fin-divider rounded-lg overflow-hidden flex flex-col">
              <div className="px-4 py-3 border-b fin-divider flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <TrendingUp className="w-4 h-4 text-accent-2" />
                  <span className="text-sm fin-text-primary font-medium">板块异动（资金流）</span>
                  {boardFlow?.updateTime ? (
                    <span className="text-xs fin-text-tertiary">更新: {boardFlow.updateTime}</span>
                  ) : null}
                </div>
                <div className="flex items-center gap-2">
                  {(['industry', 'concept', 'region'] as BoardCategory[]).map((item) => (
                    <button
                      key={item}
                      onClick={() => onChangeCategory(item)}
                      className={`px-2.5 py-1 rounded text-xs border ${
                        category === item
                          ? 'bg-accent/20 text-accent border-accent/40'
                          : 'fin-panel fin-text-secondary fin-divider hover:fin-text-primary'
                      }`}
                    >
                      {CATEGORY_LABELS[item]}
                    </button>
                  ))}
                  <button
                    onClick={() => {
                      resetAutoRefreshCountdown();
                      void loadBoardFlow(category, selectedBoard?.code);
                    }}
                    className="p-1.5 rounded fin-hover transition-colors"
                    disabled={loading}
                    title="刷新"
                  >
                    <RefreshCw className={`w-4 h-4 fin-text-secondary ${loading ? 'animate-spin' : ''}`} />
                  </button>
                </div>
              </div>

              {loading ? (
                <div className="flex-1 flex items-center justify-center">
                  <RefreshCw className="w-5 h-5 fin-text-secondary animate-spin" />
                </div>
              ) : error ? (
                <div className="flex-1 px-4 py-3 text-sm text-amber-300">{error}</div>
              ) : (
                <div className="flex-1 min-h-0 flex">
                  <div className="w-[58%] overflow-auto fin-scrollbar border-r fin-divider">
                    <table className="w-full text-sm">
                      <thead className="sticky top-0 fin-panel border-b fin-divider z-10">
                        <tr className="text-left">
                          <th className="px-3 py-2 font-medium fin-text-secondary">板块</th>
                          <th className="px-3 py-2 font-medium fin-text-secondary">涨跌幅</th>
                          <th className="px-3 py-2 font-medium fin-text-secondary">主力净流入</th>
                          <th className="px-3 py-2 font-medium fin-text-secondary">超大单</th>
                          <th className="px-3 py-2 font-medium fin-text-secondary">大单</th>
                        </tr>
                      </thead>
                      <tbody>
                        {(boardFlow?.items || []).map((item) => {
                          const active = selectedBoard?.code === item.code;
                          return (
                            <tr
                              key={item.code}
                              className={`border-b fin-divider/40 cursor-pointer ${
                                active ? 'bg-accent/10' : 'hover:bg-slate-700/20'
                              }`}
                              onClick={() => {
                                setSelectedBoard(item);
                                void loadBoardLeaders(item.code);
                              }}
                            >
                              <td className="px-3 py-2">
                                <div className="font-medium fin-text-primary">{item.name}</div>
                                <div className="text-xs fin-text-tertiary font-mono">{item.code}</div>
                              </td>
                              <td className={`px-3 py-2 font-mono ${item.changePercent >= 0 ? 'text-red-400' : 'text-green-400'}`}>
                                {item.changePercent >= 0 ? '+' : ''}
                                {item.changePercent.toFixed(2)}%
                              </td>
                              <td className={`px-3 py-2 font-mono ${item.mainNetInflow >= 0 ? 'text-red-400' : 'text-green-400'}`}>
                                {formatAmount(item.mainNetInflow)}
                              </td>
                              <td className={`px-3 py-2 font-mono ${item.superNetInflow >= 0 ? 'text-red-400' : 'text-green-400'}`}>
                                {formatAmount(item.superNetInflow)}
                              </td>
                              <td className={`px-3 py-2 font-mono ${item.largeNetInflow >= 0 ? 'text-red-400' : 'text-green-400'}`}>
                                {formatAmount(item.largeNetInflow)}
                              </td>
                            </tr>
                          );
                        })}
                      </tbody>
                    </table>
                  </div>
                  <div className="flex-1 min-w-0 p-3 overflow-auto fin-scrollbar">
                    <div className="flex items-center justify-between mb-2">
                      <div className="text-sm fin-text-primary font-medium">龙头推荐</div>
                      {leaderResult?.updateTime ? (
                        <div className="text-xs fin-text-tertiary">更新: {leaderResult.updateTime}</div>
                      ) : null}
                    </div>
                    <div className="text-xs fin-text-tertiary mb-3">
                      推荐逻辑：涨跌幅 + 主力净流入 + 主力净流入占比综合评分
                    </div>
                    {selectedBoard ? (
                      <div className="text-xs fin-text-secondary mb-3">
                        当前板块：<span className="fin-text-primary">{selectedBoard.name}</span>
                        <span className="ml-1 font-mono fin-text-tertiary">{selectedBoard.code}</span>
                      </div>
                    ) : (
                      <div className="text-sm fin-text-tertiary">请先在左侧选择板块</div>
                    )}
                    {leaderLoading ? (
                      <div className="pt-6 flex items-center justify-center">
                        <RefreshCw className="w-4 h-4 fin-text-secondary animate-spin" />
                      </div>
                    ) : leaderError ? (
                      <div className="text-sm text-amber-300">{leaderError}</div>
                    ) : (
                      <div className="space-y-2">
                        {(leaderResult?.items || []).map((leader) => (
                          <div key={leader.code} className="fin-panel border fin-divider rounded-lg px-3 py-2">
                            <div className="flex items-center justify-between">
                              <div className="text-sm fin-text-primary">
                                {leader.rank}. {leader.name}
                                <span className="ml-1 text-xs font-mono fin-text-tertiary">{leader.code}</span>
                              </div>
                              <div className={`text-sm font-mono ${leader.changePercent >= 0 ? 'text-red-400' : 'text-green-400'}`}>
                                {leader.changePercent >= 0 ? '+' : ''}
                                {leader.changePercent.toFixed(2)}%
                              </div>
                            </div>
                            <div className="mt-1 text-xs fin-text-secondary flex items-center justify-between">
                              <span>主力净流入: {formatAmount(leader.mainNetInflow)}</span>
                              <span>占比: {leader.mainNetInflowRatio.toFixed(2)}%</span>
                              <span>评分: {leader.score.toFixed(2)}</span>
                            </div>
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  );
};
