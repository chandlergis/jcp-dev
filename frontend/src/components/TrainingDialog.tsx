import React, { useState, useEffect, useRef, useCallback } from 'react';
import { X, Play, SkipForward, Coins, Award, Target, RotateCcw, LineChart } from 'lucide-react';
import {
  createChart, IChartApi, ISeriesApi, CandlestickSeries,
  HistogramSeries, CandlestickData, HistogramData, Time,
  LineSeries, LineData,
} from 'lightweight-charts';
import { TrainingSession, KLineData, TradeRecord, PositionLevel, MilestoneInfo } from '../types';
import {
  createTrainingSession, getTrainingSession, executeTrade, nextDay,
  getTrainingKlines, getStats, abortTraining, getAllMilestones,
  getTrainingPrediction,
} from '../services/trainingService';
import { PredictionResult } from '../types';
import { useTheme } from '../contexts/ThemeContext';

interface TrainingDialogProps {
  isOpen: boolean;
  onClose: () => void;
}

const POSITION_OPTIONS: { level: PositionLevel; label: string; ratio: number }[] = [
  { level: 'full', label: '全仓', ratio: 1 },
  { level: 'half', label: '半仓', ratio: 0.5 },
  { level: 'quarter', label: '1/4仓', ratio: 0.25 },
  { level: 'tenth', label: '1/10仓', ratio: 0.1 },
];

export const TrainingDialog: React.FC<TrainingDialogProps> = ({ isOpen, onClose }) => {
  const { colors } = useTheme();
  const chartContainerRef = useRef<HTMLDivElement>(null);
  const chartRef = useRef<IChartApi | null>(null);
  const candleSeriesRef = useRef<ISeriesApi<'Candlestick'> | null>(null);
  const volumeSeriesRef = useRef<ISeriesApi<'Histogram'> | null>(null);
  const buySeriesRef = useRef<ISeriesApi<'Line'> | null>(null);
  const sellSeriesRef = useRef<ISeriesApi<'Line'> | null>(null);
  const kSeriesRef = useRef<ISeriesApi<'Line'> | null>(null);
  const dSeriesRef = useRef<ISeriesApi<'Line'> | null>(null);
  const jSeriesRef = useRef<ISeriesApi<'Line'> | null>(null);

  const [session, setSession] = useState<TrainingSession | null>(null);
  const [trades, setTrades] = useState<TradeRecord[]>([]);
  const [loading, setLoading] = useState(false);
  const [finished, setFinished] = useState(false);
  const [stats, setStats] = useState<Record<string, any> | null>(null);
  const [milestones, setMilestones] = useState<MilestoneInfo[]>([]);
  const [showMilestone, setShowMilestone] = useState<MilestoneInfo | null>(null);
  const [prediction, setPrediction] = useState<PredictionResult | null>(null);

  const klineCacheRef = useRef<KLineData[]>([]);
  const lastLoadedIndexRef = useRef<number>(-1);
  const buyDataRef = useRef<LineData[]>([]);
  const sellDataRef = useRef<LineData[]>([]);

  useEffect(() => {
    getAllMilestones().then(setMilestones);
  }, []);

  // Init chart
  useEffect(() => {
    if (!chartContainerRef.current || !isOpen) return;
    if (chartRef.current) return;

    const chart = createChart(chartContainerRef.current, {
      layout: { 
        textColor: colors.isDark ? '#9ca88b' : '#64748b',
      },
      grid: {
        vertLines: { color: colors.isDark ? 'rgba(107, 142, 35, 0.08)' : 'rgba(226, 232, 240, 0.5)' },
        horzLines: { color: colors.isDark ? 'rgba(107, 142, 35, 0.08)' : 'rgba(226, 232, 240, 0.5)' },
      },
      crosshair: { mode: 0 },
      rightPriceScale: { borderColor: colors.isDark ? 'rgba(107, 142, 35, 0.18)' : '#cbd5e1' },
      timeScale: { borderColor: colors.isDark ? 'rgba(107, 142, 35, 0.18)' : '#cbd5e1', timeVisible: true },
      width: chartContainerRef.current.clientWidth,
      height: chartContainerRef.current.clientHeight,
    });
    chartRef.current = chart;

    const candleSeries = chart.addSeries(CandlestickSeries, {
      upColor: '#22c55e',
      downColor: '#ef4444',
      borderUpColor: '#22c55e',
      borderDownColor: '#ef4444',
      wickUpColor: '#22c55e',
      wickDownColor: '#ef4444',
    });
    candleSeriesRef.current = candleSeries;

    const volumeSeries = chart.addSeries(HistogramSeries, {
      color: '#64748b',
      priceFormat: { type: 'volume' },
      priceScaleId: 'volume',
    });
    volumeSeriesRef.current = volumeSeries;
    chart.priceScale('volume').applyOptions({ scaleMargins: { top: 0.8, bottom: 0 } });

    const buySeries = chart.addSeries(LineSeries, {
      color: '#22c55e',
      lineWidth: 1,
      pointMarkersVisible: true,
      pointMarkersRadius: 8,
      lastValueVisible: false,
      priceLineVisible: false,
    });
    buySeriesRef.current = buySeries;

    const sellSeries = chart.addSeries(LineSeries, {
      color: '#ef4444',
      lineWidth: 1,
      pointMarkersVisible: true,
      pointMarkersRadius: 8,
      lastValueVisible: false,
      priceLineVisible: false,
    });
    sellSeriesRef.current = sellSeries;

    // KDJ
    const kSeries = chart.addSeries(LineSeries, {
      color: '#facc15',
      lineWidth: 1,
      lastValueVisible: false,
      priceLineVisible: false,
      priceScaleId: 'kdj',
    });
    kSeriesRef.current = kSeries;

    const dSeries = chart.addSeries(LineSeries, {
      color: '#a855f7',
      lineWidth: 1,
      lastValueVisible: false,
      priceLineVisible: false,
      priceScaleId: 'kdj',
    });
    dSeriesRef.current = dSeries;

    const jSeries = chart.addSeries(LineSeries, {
      color: '#22d3ee',
      lineWidth: 1,
      lastValueVisible: false,
      priceLineVisible: false,
      priceScaleId: 'kdj',
    });
    jSeriesRef.current = jSeries;

    chart.priceScale('kdj').applyOptions({
      scaleMargins: { top: 0.85, bottom: 0 },
    });

    const handleResize = () => {
      if (chartContainerRef.current) {
        chart.applyOptions({ 
          width: chartContainerRef.current.clientWidth,
          height: chartContainerRef.current.clientHeight,
        });
      }
    };
    window.addEventListener('resize', handleResize);

    return () => {
      window.removeEventListener('resize', handleResize);
      chart.remove();
      chartRef.current = null;
      candleSeriesRef.current = null;
      volumeSeriesRef.current = null;
      buySeriesRef.current = null;
      sellSeriesRef.current = null;
      kSeriesRef.current = null;
      dSeriesRef.current = null;
      jSeriesRef.current = null;
    };
  }, [isOpen]);

  // 计算KDJ
  const calcKDJ = useCallback((data: KLineData[], n: number = 9): { k: LineData[]; d: LineData[]; j: LineData[] } => {
    const kArr: LineData[] = [];
    const dArr: LineData[] = [];
    const jArr: LineData[] = [];
    let kVal = 50, dVal = 50;

    for (let i = 0; i < data.length; i++) {
      const start = Math.max(0, i - n + 1);
      let lowN = Infinity, highN = -Infinity;
      for (let idx = start; idx <= i; idx++) {
        if (data[idx].low < lowN) lowN = data[idx].low;
        if (data[idx].high > highN) highN = data[idx].high;
      }
      const rsv = highN - lowN > 0 ? ((data[i].close - lowN) / (highN - lowN)) * 100 : 50;
      kVal = (2 / 3) * kVal + (1 / 3) * rsv;
      dVal = (2 / 3) * dVal + (1 / 3) * kVal;
      const jVal = 3 * kVal - 2 * dVal;
      const time = data[i].time.split(' ')[0] as Time;
      kArr.push({ time, value: kVal });
      dArr.push({ time, value: dVal });
      jArr.push({ time, value: jVal });
    }
    return { k: kArr, d: dArr, j: jArr };
  }, []);

  // 全量加载K线
  const loadAllKlines = useCallback((data: KLineData[]) => {
    if (!candleSeriesRef.current || !volumeSeriesRef.current) return;
    klineCacheRef.current = data;
    lastLoadedIndexRef.current = data.length - 1;

    const candleData: CandlestickData[] = data.map(d => ({
      time: d.time.split(' ')[0] as Time,
      open: d.open, high: d.high, low: d.low, close: d.close,
    }));
    const volumeData: HistogramData[] = data.map((d, i) => ({
      time: d.time.split(' ')[0] as Time,
      value: d.volume,
      color: d.close >= (i > 0 ? data[i-1].close : d.close) ? 'rgba(34, 197, 94, 0.4)' : 'rgba(239, 68, 68, 0.4)',
    }));

    candleSeriesRef.current.setData(candleData);
    volumeSeriesRef.current.setData(volumeData);
    if (buySeriesRef.current) buySeriesRef.current.setData([]);
    if (sellSeriesRef.current) sellSeriesRef.current.setData([]);
    buyDataRef.current = [];
    sellDataRef.current = [];

    const kdj = calcKDJ(data);
    if (kSeriesRef.current) kSeriesRef.current.setData(kdj.k);
    if (dSeriesRef.current) dSeriesRef.current.setData(kdj.d);
    if (jSeriesRef.current) jSeriesRef.current.setData(kdj.j);

    chartRef.current?.timeScale().fitContent();
  }, [calcKDJ]);

  // 增量追加K线
  const appendKline = useCallback((kline: KLineData) => {
    if (!candleSeriesRef.current || !volumeSeriesRef.current) return;
    klineCacheRef.current.push(kline);
    lastLoadedIndexRef.current++;

    const timeStr = kline.time.split(' ')[0] as Time;
    candleSeriesRef.current.update({
      time: timeStr,
      open: kline.open, high: kline.high, low: kline.low, close: kline.close,
    });
    const prev = klineCacheRef.current.length > 1 ? klineCacheRef.current[klineCacheRef.current.length - 2].close : kline.close;
    volumeSeriesRef.current.update({
      time: timeStr,
      value: kline.volume,
      color: kline.close >= prev ? 'rgba(34, 197, 94, 0.4)' : 'rgba(239, 68, 68, 0.4)',
    });

    const tail = klineCacheRef.current.slice(-30);
    const kdj = calcKDJ(tail);
    const lastK = kdj.k[kdj.k.length - 1];
    const lastD = kdj.d[kdj.d.length - 1];
    const lastJ = kdj.j[kdj.j.length - 1];
    if (kSeriesRef.current && lastK) kSeriesRef.current.update(lastK);
    if (dSeriesRef.current && lastD) dSeriesRef.current.update(lastD);
    if (jSeriesRef.current && lastJ) jSeriesRef.current.update(lastJ);
  }, [calcKDJ]);

  // 添加交易标记
  const addTradeMarker = useCallback((trade: TradeRecord) => {
    const dateStr = trade.date.split(' ')[0] as Time;
    if (trade.action === 'buy') {
      const kline = klineCacheRef.current.find(k => k.time.split(' ')[0] === trade.date.split(' ')[0]);
      if (kline && buySeriesRef.current) {
        buyDataRef.current.push({ time: dateStr, value: kline.low * 0.99 });
        buySeriesRef.current.update({ time: dateStr, value: kline.low * 0.99 });
      }
    } else {
      const kline = klineCacheRef.current.find(k => k.time.split(' ')[0] === trade.date.split(' ')[0]);
      if (kline && sellSeriesRef.current) {
        sellDataRef.current.push({ time: dateStr, value: kline.high * 1.01 });
        sellSeriesRef.current.update({ time: dateStr, value: kline.high * 1.01 });
      }
    }
  }, []);

  // 开始训练
  const handleStart = async () => {
    setLoading(true);
    setFinished(false);
    setStats(null);
    setTrades([]);
    setPrediction(null);
    try {
      const s = await createTrainingSession();
      if (s) {
        setSession(s);
        const k = await getTrainingKlines(s.id);
        if (candleSeriesRef.current) {
          loadAllKlines(k);
        } else {
          const wait = () => {
            if (candleSeriesRef.current) {
              loadAllKlines(k);
            } else {
              setTimeout(wait, 50);
            }
          };
          setTimeout(wait, 100);
        }
        // 获取AI预测
        const pred = await getTrainingPrediction(s.id);
        setPrediction(pred);
      }
    } finally {
      setLoading(false);
    }
  };

  // 买入
  const handleBuy = async (level: PositionLevel) => {
    if (!session) return;
    const trade = await executeTrade(session.id, 'buy', level);
    if (trade) {
      const updated = await getTrainingSession(session.id);
      if (updated) setSession(updated);
      setTrades(prev => [...prev, trade]);
      addTradeMarker(trade);
    }
  };

  // 卖出
  const handleSell = async (level: PositionLevel) => {
    if (!session) return;
    const trade = await executeTrade(session.id, 'sell', level);
    if (trade) {
      const updated = await getTrainingSession(session.id);
      if (updated) {
        setSession(updated);
        const newMilestones = (updated.milestones || []).filter(
          (m: string) => !(session.milestones || []).includes(m)
        );
        if (newMilestones.length > 0) {
          const info = milestones.find(m => m.type === newMilestones[0]);
          if (info) setShowMilestone(info);
        }
      }
      setTrades(prev => [...prev, trade]);
      addTradeMarker(trade);
    }
  };

  // 下一天
  const handleNextDay = async () => {
    if (!session) return;
    const kline = await nextDay(session.id);
    if (kline) {
      appendKline(kline);
      const s = await getTrainingSession(session.id);
      if (s) {
        setSession(s);
        if (s.status === 'finished') {
          setFinished(true);
          setPrediction(null);
          const st = await getStats(s.id);
          setStats(st);
        } else {
          // 获取新一天的AI预测
          const pred = await getTrainingPrediction(s.id);
          setPrediction(pred);
        }
      }
    } else {
      const s = await getTrainingSession(session.id);
      if (s) {
        setSession(s);
        setFinished(true);
        setPrediction(null);
        const st = await getStats(s.id);
        setStats(st);
      }
    }
  };

  const handleAbort = async () => {
    if (!session) return;
    await abortTraining(session.id);
    setFinished(true);
    const s = await getTrainingSession(session.id);
    if (s) {
      setSession(s);
      const st = await getStats(s.id);
      setStats(st);
    }
  };

  const formatMoney = (v: number) => {
    if (Math.abs(v) >= 100000000) return (v / 100000000).toFixed(2) + '亿';
    if (Math.abs(v) >= 10000) return (v / 10000).toFixed(2) + '万';
    return v.toFixed(2);
  };

  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-50 flex fin-app">
      {/* Left: Chart */}
      <div className="flex-1 flex flex-col min-w-0 p-3">
        {/* Header */}
        <div className="flex items-center justify-between mb-3 px-4 py-2 fin-panel rounded-lg border fin-divider">
          <div className="flex items-center gap-3">
            {session ? (
              <>
                <span className={`font-bold ${colors.isDark ? 'text-white' : 'text-slate-800'}`}>
                  {session.stockName}
                </span>
                <span className={`text-sm font-mono ${colors.isDark ? 'text-slate-400' : 'text-slate-500'}`}>
                  {session.stockCode}
                </span>
                <span className="text-xs text-accent-2">{session.currentDate}</span>
                <span className={`text-xs ${colors.isDark ? 'text-slate-500' : 'text-slate-400'}`}>
                  {session.currentIndex + 1}/{session.totalDays}天
                </span>
              </>
            ) : (
              <span className={`font-bold ${colors.isDark ? 'text-white' : 'text-slate-800'}`}>
                K线训练营
              </span>
            )}
          </div>
          <button onClick={onClose} className={`p-1.5 rounded transition-colors ${colors.isDark ? 'hover:bg-slate-700/50 text-slate-400' : 'hover:bg-slate-200/50 text-slate-500'}`}>
            <X size={18} />
          </button>
        </div>

        {/* Chart */}
        <div className="flex-1 relative min-h-0">
          <div ref={chartContainerRef} className="absolute inset-0 fin-panel rounded-lg overflow-hidden" />
          {!session && (
            <div className="absolute inset-0 flex flex-col items-center justify-center fin-panel-strong rounded-lg z-10">
              <LineChart size={64} className="mb-4 text-accent-2" />
              <h2 className={`text-xl font-bold mb-2 ${colors.isDark ? 'text-white' : 'text-slate-800'}`}>K线训练营</h2>
              <p className={`text-sm mb-6 ${colors.isDark ? 'text-slate-400' : 'text-slate-500'}`}>
                随机股票+随机时间段，训练K线形态识别
              </p>
              <button 
                onClick={handleStart} 
                disabled={loading} 
                className="flex items-center gap-2 px-6 py-2.5 rounded-lg bg-accent text-white font-medium hover:bg-accent/80 transition-colors disabled:opacity-50"
              >
                <Play size={18} />
                {loading ? '创建中...' : '开始训练'}
              </button>
              <div className="flex items-center gap-4 mt-4">
                <span className={`text-xs ${colors.isDark ? 'text-slate-500' : 'text-slate-400'}`}>初始资金100万</span>
                <span className={`text-xs ${colors.isDark ? 'text-slate-500' : 'text-slate-400'}`}>日线交易</span>
                <span className={`text-xs ${colors.isDark ? 'text-slate-500' : 'text-slate-400'}`}>收盘价成交</span>
              </div>
            </div>
          )}
        </div>
      </div>

      {/* Right Panel */}
      <div className={`w-80 flex flex-col overflow-y-auto fin-panel border-l fin-divider`}>
        {!session && !finished ? (
          <div className="flex-1 flex items-center justify-center p-6">
            <p className={`text-sm ${colors.isDark ? 'text-slate-500' : 'text-slate-400'}`}>点击"开始训练"</p>
          </div>
        ) : finished && stats ? (
          <div className="p-4">
            <h3 className={`text-base font-bold mb-4 ${colors.isDark ? 'text-white' : 'text-slate-800'}`}>训练结果</h3>
            
            <div className="space-y-2 mb-4">
              <div className="flex items-center justify-between p-2 fin-panel-soft rounded">
                <span className={`text-xs ${colors.isDark ? 'text-slate-400' : 'text-slate-500'}`}>总收益率</span>
                <span className={`text-sm font-bold ${stats.totalReturn >= 0 ? 'text-green-500' : 'text-red-500'}`}>
                  {stats.totalReturn >= 0 ? '+' : ''}{stats.totalReturn?.toFixed(2) || 0}%
                </span>
              </div>
              <div className="flex items-center justify-between p-2 fin-panel-soft rounded">
                <span className={`text-xs ${colors.isDark ? 'text-slate-400' : 'text-slate-500'}`}>总盈亏</span>
                <span className={`text-sm font-bold ${stats.totalProfit >= 0 ? 'text-green-500' : 'text-red-500'}`}>
                  {stats.totalProfit >= 0 ? '+' : ''}{formatMoney(stats.totalProfit || 0)}
                </span>
              </div>
              <div className="grid grid-cols-2 gap-2">
                <div className="p-2 fin-panel-soft rounded">
                  <div className={`text-xs ${colors.isDark ? 'text-slate-500' : 'text-slate-400'}`}>交易</div>
                  <div className={`text-sm font-bold ${colors.isDark ? 'text-white' : 'text-slate-800'}`}>{stats.tradeCount || 0}</div>
                </div>
                <div className="p-2 fin-panel-soft rounded">
                  <div className={`text-xs ${colors.isDark ? 'text-slate-500' : 'text-slate-400'}`}>胜率</div>
                  <div className={`text-sm font-bold ${stats.winRate >= 50 ? 'text-green-500' : 'text-red-500'}`}>{stats.winRate?.toFixed(1) || 0}%</div>
                </div>
                <div className="p-2 fin-panel-soft rounded">
                  <div className={`text-xs ${colors.isDark ? 'text-slate-500' : 'text-slate-400'}`}>回撤</div>
                  <div className="text-sm font-bold text-red-500">{stats.maxDrawdown?.toFixed(2) || 0}%</div>
                </div>
                <div className="p-2 fin-panel-soft rounded">
                  <div className={`text-xs ${colors.isDark ? 'text-slate-500' : 'text-slate-400'}`}>连胜</div>
                  <div className={`text-sm font-bold ${colors.isDark ? 'text-white' : 'text-slate-800'}`}>{stats.maxWinStreak || 0}</div>
                </div>
              </div>
            </div>
            
            <button onClick={handleStart} className="w-full flex items-center justify-center gap-2 py-2 rounded-lg bg-accent text-white font-medium hover:bg-accent/80 transition-colors">
              <RotateCcw size={16} /> 再来一次
            </button>
            <button onClick={onClose} className={`w-full mt-2 py-2 rounded-lg fin-panel border fin-divider text-sm ${colors.isDark ? 'text-slate-300' : 'text-slate-600'}`}>
              返回
            </button>
          </div>
        ) : session ? (
          <>
            {/* 账户 */}
            <div className="p-3 border-b fin-divider">
              <div className="flex items-center gap-2 mb-2">
                <Coins size={14} className="text-accent-2" />
                <span className={`text-sm font-bold ${colors.isDark ? 'text-white' : 'text-slate-800'}`}>账户</span>
              </div>
              <div className="space-y-1.5">
                <div className="flex items-center justify-between">
                  <span className={`text-xs ${colors.isDark ? 'text-slate-400' : 'text-slate-500'}`}>总资产</span>
                  <span className={`text-sm font-mono font-bold ${colors.isDark ? 'text-white' : 'text-slate-800'}`}>{formatMoney(session.totalAsset)}</span>
                </div>
                <div className="flex items-center justify-between">
                  <span className={`text-xs ${colors.isDark ? 'text-slate-400' : 'text-slate-500'}`}>可用</span>
                  <span className={`text-sm font-mono ${colors.isDark ? 'text-slate-300' : 'text-slate-600'}`}>{formatMoney(session.cash)}</span>
                </div>
                {session.position > 0 && (
                  <>
                    <div className="flex items-center justify-between">
                      <span className={`text-xs ${colors.isDark ? 'text-slate-400' : 'text-slate-500'}`}>持仓</span>
                      <span className={`text-sm font-mono ${colors.isDark ? 'text-slate-300' : 'text-slate-600'}`}>{session.position}股</span>
                    </div>
                    <div className="flex items-center justify-between">
                      <span className={`text-xs ${colors.isDark ? 'text-slate-400' : 'text-slate-500'}`}>盈亏</span>
                      <span className={`text-sm font-mono font-bold ${
                        session.positionValue >= session.position * session.avgCost ? 'text-green-500' : 'text-red-500'
                      }`}>
                        {((session.positionValue - session.position * session.avgCost) / (session.position * session.avgCost) * 100).toFixed(2)}%
                      </span>
                    </div>
                  </>
                )}
              </div>
            </div>

            {/* 进度 */}
            <div className="px-3 py-2 border-b fin-divider">
              <div className="flex items-center justify-between mb-1">
                <span className={`text-xs ${colors.isDark ? 'text-slate-400' : 'text-slate-500'}`}>进度</span>
                <span className={`text-xs font-mono ${colors.isDark ? 'text-slate-300' : 'text-slate-600'}`}>{session.currentIndex + 1}/{session.totalDays}</span>
              </div>
              <div className={`h-1.5 rounded-full overflow-hidden ${colors.isDark ? 'bg-slate-700' : 'bg-slate-200'}`}>
                <div className="h-full bg-accent rounded-full transition-all" style={{ width: `${((session.currentIndex + 1) / session.totalDays) * 100}%` }} />
              </div>
            </div>

            {/* 交易按钮 */}
            {!finished && (
              <div className="p-3 border-b fin-divider">
                {/* AI预测提示 */}
                {prediction && (
                  <div className={`mb-2 p-2 rounded-lg border ${
                    prediction.signal === '强买入' || prediction.signal === '买入'
                      ? 'bg-green-500/10 border-green-500/30'
                      : prediction.signal === '强卖出' || prediction.signal === '卖出'
                        ? 'bg-red-500/10 border-red-500/30'
                        : 'bg-slate-500/10 border-slate-500/30'
                  }`}>
                    <div className="flex items-center justify-between mb-1">
                      <span className={`text-xs font-bold ${
                        prediction.signal === '强买入' || prediction.signal === '买入'
                          ? 'text-green-500'
                          : prediction.signal === '强卖出' || prediction.signal === '卖出'
                            ? 'text-red-500'
                            : 'text-slate-400'
                      }`}>
                        AI 预测: {prediction.signal}
                      </span>
                      <span className={`text-xs font-mono ${prediction.direction === '涨' ? 'text-green-500' : 'text-red-500'}`}>
                        {prediction.direction} {Math.abs(prediction.return).toFixed(2)}%
                      </span>
                    </div>
                    <div className="flex items-center gap-2">
                      <div className={`flex-1 h-1 rounded-full overflow-hidden ${colors.isDark ? 'bg-slate-700' : 'bg-slate-200'}`}>
                        <div
                          className={`h-full rounded-full ${
                            prediction.confidence > 0.5 ? 'bg-green-500' : prediction.confidence > 0.3 ? 'bg-yellow-500' : 'bg-slate-400'
                          }`}
                          style={{ width: `${Math.min(prediction.confidence * 100, 100)}%` }}
                        />
                      </div>
                      <span className={`text-[10px] ${colors.isDark ? 'text-slate-500' : 'text-slate-400'}`}>
                        置信度 {(prediction.confidence * 100).toFixed(0)}%
                      </span>
                    </div>
                  </div>
                )}
                <div className="grid grid-cols-2 gap-1.5 mb-2">
                  {POSITION_OPTIONS.map(opt => (
                    <button 
                      key={opt.level} 
                      onClick={() => handleBuy(opt.level)} 
                      disabled={session.cash * opt.ratio < 100}
                      className="px-2 py-1.5 rounded text-xs font-medium bg-green-500/10 text-green-500 hover:bg-green-500/20 border border-green-500/20 disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
                    >
                      {opt.label}买入
                    </button>
                  ))}
                </div>
                {session.position > 0 && (
                  <div className="grid grid-cols-2 gap-1.5">
                    {POSITION_OPTIONS.map(opt => (
                      <button 
                        key={opt.level} 
                        onClick={() => handleSell(opt.level)}
                        className="px-2 py-1.5 rounded text-xs font-medium bg-red-500/10 text-red-500 hover:bg-red-500/20 border border-red-500/20 transition-colors"
                      >
                        {opt.label}卖出
                      </button>
                    ))}
                  </div>
                )}
              </div>
            )}

            {/* 下一天 */}
            {!finished && (
              <div className="p-3 border-b fin-divider">
                <button onClick={handleNextDay} className="w-full flex items-center justify-center gap-2 py-2 rounded-lg bg-accent text-white font-medium hover:bg-accent/80 transition-colors">
                  <SkipForward size={16} /> 下一天
                </button>
                <button onClick={handleAbort} className={`w-full mt-1 py-1 text-xs ${colors.isDark ? 'text-slate-500 hover:text-slate-300' : 'text-slate-400 hover:text-slate-600'}`}>
                  结束训练
                </button>
              </div>
            )}

            {/* 里程碑 */}
            {session.milestones && session.milestones.length > 0 && (
              <div className="p-3 border-b fin-divider">
                <div className="flex items-center gap-1.5 mb-2">
                  <Award size={14} className="text-yellow-500" />
                  <span className={`text-xs font-bold ${colors.isDark ? 'text-slate-300' : 'text-slate-600'}`}>里程碑</span>
                </div>
                <div className="space-y-1">
                  {session.milestones.map(m => {
                    const info = milestones.find(i => i.type === m);
                    return (
                      <div key={m} className={`flex items-center gap-2 text-xs ${colors.isDark ? 'text-slate-400' : 'text-slate-500'}`}>
                        <span>{info?.icon}</span>
                        <span>{info?.name}</span>
                      </div>
                    );
                  })}
                </div>
              </div>
            )}

            {/* 交易记录 */}
            <div className="p-3 flex-1">
              <div className="flex items-center gap-1.5 mb-2">
                <Target size={14} className="text-accent-2" />
                <span className={`text-xs font-bold ${colors.isDark ? 'text-slate-300' : 'text-slate-600'}`}>交易记录</span>
              </div>
              <div className="space-y-1 max-h-48 overflow-y-auto fin-scrollbar">
                {trades.length === 0 && <span className={`text-xs ${colors.isDark ? 'text-slate-600' : 'text-slate-400'}`}>暂无交易</span>}
                {trades.map(trade => (
                  <div key={trade.id} className={`flex items-center justify-between py-1 text-xs ${colors.isDark ? 'text-slate-400' : 'text-slate-500'}`}>
                    <div className="flex items-center gap-1.5">
                      <span className={trade.action === 'buy' ? 'text-green-500' : 'text-red-500'}>
                        {trade.action === 'buy' ? '买' : '卖'}
                      </span>
                      <span>{trade.quantity}股</span>
                      <span className="font-mono">{trade.price.toFixed(2)}</span>
                    </div>
                    {trade.profit != null && (
                      <span className={`font-mono ${trade.profit >= 0 ? 'text-green-500' : 'text-red-500'}`}>
                        {trade.profit >= 0 ? '+' : ''}{trade.profitPercent?.toFixed(2)}%
                      </span>
                    )}
                  </div>
                ))}
              </div>
            </div>
          </>
        ) : null}
      </div>

      {/* Milestone Alert */}
      {showMilestone && (
        <div className="fixed inset-0 z-[60] flex items-center justify-center bg-black/50" onClick={() => setShowMilestone(null)}>
          <div className={`p-6 rounded-lg text-center fin-panel-strong border fin-divider`} onClick={e => e.stopPropagation()}>
            <div className="text-5xl mb-3">{showMilestone.icon}</div>
            <div className={`text-xl font-bold mb-2 ${colors.isDark ? 'text-yellow-400' : 'text-yellow-600'}`}>{showMilestone.name}</div>
            <div className={`text-sm mb-4 ${colors.isDark ? 'text-slate-300' : 'text-slate-600'}`}>{showMilestone.description}</div>
            <button onClick={() => setShowMilestone(null)} className="px-4 py-1.5 rounded bg-accent text-white font-medium hover:bg-accent/80">
              继续交易
            </button>
          </div>
        </div>
      )}
    </div>
  );
};
