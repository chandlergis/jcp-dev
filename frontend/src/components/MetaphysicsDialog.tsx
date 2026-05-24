import React, { useState, useMemo, useEffect } from 'react';
import { X, Sparkles, Calendar, Clock, Compass, Hash, TrendingUp, AlertTriangle, Coffee } from 'lucide-react';
import { calculateBazi, calculateDailyFortune, type DailyFortune } from '../utils/bazi';
import { PoeDivination } from './PoeDivination';

interface MetaphysicsDialogProps {
  isOpen: boolean;
  onClose: () => void;
}

type TabId = 'bazi' | 'poe';

const STORAGE_KEY = 'jcp_metaphysics_birth_v1';

interface SavedBirth {
  year: number;
  month: number;
  day: number;
  hour: number;
}

const loadSaved = (): SavedBirth | null => {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return null;
    const v = JSON.parse(raw);
    if (typeof v.year === 'number' && typeof v.month === 'number') return v;
  } catch {/* ignore */}
  return null;
};

const saveBirth = (b: SavedBirth) => {
  try { localStorage.setItem(STORAGE_KEY, JSON.stringify(b)); } catch {/* ignore */}
};

const ELEMENT_COLORS: Record<string, string> = {
  木: 'text-emerald-400 bg-emerald-500/15 border-emerald-500/30',
  火: 'text-rose-400 bg-rose-500/15 border-rose-500/30',
  土: 'text-amber-400 bg-amber-500/15 border-amber-500/30',
  金: 'text-slate-200 bg-slate-400/15 border-slate-400/30',
  水: 'text-sky-400 bg-sky-500/15 border-sky-500/30',
};

const SCORE_COLOR = (score: number) => {
  if (score >= 85) return 'text-amber-300';
  if (score >= 70) return 'text-emerald-400';
  if (score >= 55) return 'text-sky-300';
  if (score >= 40) return 'text-slate-300';
  if (score >= 25) return 'text-orange-400';
  return 'text-rose-400';
};

export const MetaphysicsDialog: React.FC<MetaphysicsDialogProps> = ({ isOpen, onClose }) => {
  const saved = useMemo(loadSaved, []);
  const now = new Date();
  const [year, setYear] = useState<number>(saved?.year ?? 1990);
  const [month, setMonth] = useState<number>(saved?.month ?? 1);
  const [day, setDay] = useState<number>(saved?.day ?? 1);
  const [hour, setHour] = useState<number>(saved?.hour ?? 12);
  const [fortune, setFortune] = useState<DailyFortune | null>(null);
  const [activeTab, setActiveTab] = useState<TabId>('bazi');
  const [question, setQuestion] = useState<string>('今日大盘走势如何？');

  // 自动计算（参数变化时）
  useEffect(() => {
    if (!isOpen) return;
    try {
      const pillars = calculateBazi(year, month, day, hour);
      setFortune(calculateDailyFortune(pillars, now));
    } catch (err) {
      console.error('八字计算失败:', err);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isOpen, year, month, day, hour]);

  const handleCompute = () => {
    saveBirth({ year, month, day, hour });
    const pillars = calculateBazi(year, month, day, hour);
    setFortune(calculateDailyFortune(pillars, new Date()));
  };

  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="absolute inset-0 bg-black/70 backdrop-blur-sm" onClick={onClose} />
      <div
        className="relative w-[1100px] h-[760px] max-w-[94vw] max-h-[90vh] fin-panel border fin-divider rounded-xl shadow-2xl flex flex-col overflow-hidden"
        style={{
          background:
            'linear-gradient(135deg, rgba(120,53,15,0.10) 0%, rgba(15,23,42,0.95) 50%, rgba(127,29,29,0.10) 100%)',
        }}
      >
        {/* Header */}
        <div className="flex items-center justify-between px-5 py-4 border-b fin-divider">
          <div className="flex items-center gap-3">
            <div className="w-9 h-9 rounded-lg bg-gradient-to-br from-amber-500 to-rose-500 flex items-center justify-center">
              <Sparkles className="w-4 h-4 text-white" />
            </div>
            <div>
              <h2 className="text-lg font-semibold fin-text-primary">玄学占卜</h2>
              <div className="text-[11px] fin-text-tertiary italic">
                祖传秘技 · 仅供娱乐 · 投资有风险 · 决策需理性
              </div>
            </div>
          </div>
          <button onClick={onClose} className="p-2 rounded-lg fin-hover transition-colors">
            <X className="w-4 h-4 fin-text-secondary" />
          </button>
        </div>

        {/* Tabs */}
        <div className="px-5 pt-3 flex items-center gap-2">
          <button
            onClick={() => setActiveTab('bazi')}
            className={`px-3 py-1.5 rounded-lg text-sm border transition-colors ${
              activeTab === 'bazi'
                ? 'bg-amber-500/20 text-amber-300 border-amber-500/40'
                : 'fin-panel fin-text-secondary fin-divider hover:fin-text-primary'
            }`}
          >
            八字今日运势
          </button>
          <button
            onClick={() => setActiveTab('poe')}
            className={`px-3 py-1.5 rounded-lg text-sm border transition-colors ${
              activeTab === 'poe'
                ? 'bg-rose-500/20 text-rose-300 border-rose-500/40'
                : 'fin-panel fin-text-secondary fin-divider hover:fin-text-primary'
            }`}
          >
            妈祖圣杯请示
          </button>
        </div>

        {/* Body */}
        <div className="flex-1 px-5 py-4 overflow-hidden">
          {activeTab === 'bazi' ? (
            <div className="h-full flex gap-4 overflow-hidden">
              {/* 左侧输入 */}
              <div className="w-72 flex-shrink-0 fin-panel border fin-divider rounded-lg p-4 overflow-auto fin-scrollbar">
                <div className="text-sm fin-text-primary font-medium mb-3 flex items-center gap-2">
                  <Calendar className="w-4 h-4 text-amber-300" />
                  生辰八字
                </div>

                <div className="space-y-3">
                  <div>
                    <label className="text-xs fin-text-secondary block mb-1">出生年份</label>
                    <input
                      type="number"
                      value={year}
                      onChange={e => setYear(parseInt(e.target.value) || 1990)}
                      min={1900} max={now.getFullYear()}
                      className="w-full px-3 py-1.5 rounded bg-slate-800/60 border fin-divider text-sm fin-text-primary"
                    />
                  </div>
                  <div className="flex gap-2">
                    <div className="flex-1">
                      <label className="text-xs fin-text-secondary block mb-1">月</label>
                      <input
                        type="number"
                        value={month}
                        onChange={e => setMonth(Math.max(1, Math.min(12, parseInt(e.target.value) || 1)))}
                        min={1} max={12}
                        className="w-full px-3 py-1.5 rounded bg-slate-800/60 border fin-divider text-sm fin-text-primary"
                      />
                    </div>
                    <div className="flex-1">
                      <label className="text-xs fin-text-secondary block mb-1">日</label>
                      <input
                        type="number"
                        value={day}
                        onChange={e => setDay(Math.max(1, Math.min(31, parseInt(e.target.value) || 1)))}
                        min={1} max={31}
                        className="w-full px-3 py-1.5 rounded bg-slate-800/60 border fin-divider text-sm fin-text-primary"
                      />
                    </div>
                  </div>
                  <div>
                    <label className="text-xs fin-text-secondary block mb-1 flex items-center gap-1">
                      <Clock className="w-3 h-3" />
                      出生时辰 (0-23)
                    </label>
                    <input
                      type="number"
                      value={hour}
                      onChange={e => setHour(Math.max(0, Math.min(23, parseInt(e.target.value) || 0)))}
                      min={0} max={23}
                      className="w-full px-3 py-1.5 rounded bg-slate-800/60 border fin-divider text-sm fin-text-primary"
                    />
                  </div>
                  <button
                    onClick={handleCompute}
                    className="w-full px-3 py-2 rounded-lg bg-gradient-to-r from-amber-500 to-rose-500 text-white text-sm font-semibold hover:opacity-90 transition-opacity"
                  >
                    🔮 排盘 · 算今日财运
                  </button>
                </div>

                {/* 四柱展示 */}
                {fortune && (
                  <div className="mt-5">
                    <div className="text-xs fin-text-tertiary mb-2">您的四柱</div>
                    <div className="grid grid-cols-4 gap-1.5 text-center">
                      {[
                        { label: '年柱', p: fortune.pillars.year },
                        { label: '月柱', p: fortune.pillars.month },
                        { label: '日柱', p: fortune.pillars.day },
                        { label: '时柱', p: fortune.pillars.hour },
                      ].map(item => (
                        <div key={item.label} className="fin-panel border fin-divider rounded p-1.5">
                          <div className="text-[10px] fin-text-tertiary mb-0.5">{item.label}</div>
                          <div className="text-base font-bold fin-text-primary leading-tight">
                            {item.p.stem}
                          </div>
                          <div className="text-base font-bold fin-text-primary leading-tight">
                            {item.p.branch}
                          </div>
                        </div>
                      ))}
                    </div>
                    <div className="mt-3 text-xs fin-text-secondary text-center">
                      日主：
                      <span className="font-bold text-amber-300">
                        {fortune.pillars.dayMaster}
                      </span>
                      ·五行属
                      <span className={`ml-1 px-1.5 py-0.5 rounded text-xs ${ELEMENT_COLORS[fortune.pillars.dayMasterElement]}`}>
                        {fortune.pillars.dayMasterElement}
                      </span>
                    </div>
                  </div>
                )}
              </div>

              {/* 右侧运势详情 */}
              <div className="flex-1 overflow-auto fin-scrollbar pr-1">
                {fortune ? (
                  <FortunePanel fortune={fortune} />
                ) : (
                  <div className="h-full flex items-center justify-center text-sm fin-text-tertiary">
                    请先填写生辰信息
                  </div>
                )}
              </div>
            </div>
          ) : (
            <div className="h-full flex flex-col gap-3 overflow-hidden">
              {/* 问题输入 */}
              <div className="flex items-center gap-2">
                <input
                  type="text"
                  value={question}
                  onChange={e => setQuestion(e.target.value)}
                  placeholder="例如：今日大盘走势如何？/ 该不该买入XX？"
                  className="flex-1 px-3 py-2 rounded-lg bg-slate-800/60 border fin-divider text-sm fin-text-primary"
                />
                <div className="text-xs fin-text-tertiary">
                  心中默念三遍后掷杯
                </div>
              </div>
              <div className="flex-1 fin-panel border fin-divider rounded-lg overflow-hidden">
                <PoeDivination question={question} />
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
};

// === 运势详情面板 ===
const FortunePanel: React.FC<{ fortune: DailyFortune }> = ({ fortune }) => {
  const todayStr = new Date().toLocaleDateString('zh-CN', {
    year: 'numeric', month: 'long', day: 'numeric', weekday: 'long',
  });

  return (
    <div className="space-y-3">
      {/* 顶部大评分 */}
      <div className="fin-panel border fin-divider rounded-lg p-4 bg-gradient-to-br from-amber-500/10 to-rose-500/5">
        <div className="text-xs fin-text-tertiary mb-1">{todayStr}</div>
        <div className="flex items-center gap-4">
          <div>
            <div className="text-xs fin-text-secondary mb-1">今日财运指数</div>
            <div className={`text-5xl font-bold ${SCORE_COLOR(fortune.wealthScore)}`}>
              {fortune.wealthScore}
            </div>
            <div className={`mt-1 text-sm font-bold ${SCORE_COLOR(fortune.wealthScore)}`}>
              {fortune.wealthLevel}
            </div>
          </div>
          <div className="flex-1 border-l fin-divider pl-4">
            <div className="text-xs fin-text-tertiary mb-1">今日干支</div>
            <div className="text-2xl font-bold fin-text-primary">
              {fortune.todayPillar.stem}{fortune.todayPillar.branch}
            </div>
            <div className="text-xs fin-text-secondary mt-1">
              {fortune.relation}
            </div>
          </div>
        </div>
        <div className="mt-3 px-3 py-2 rounded bg-slate-900/50 text-sm fin-text-primary leading-relaxed">
          {fortune.marketAdvice}
        </div>
      </div>

      {/* 五行分布 */}
      <div className="fin-panel border fin-divider rounded-lg p-3">
        <div className="text-xs fin-text-secondary font-medium mb-2">命局五行分布</div>
        <div className="grid grid-cols-5 gap-2">
          {(['木', '火', '土', '金', '水'] as const).map(el => {
            const cnt = fortune.elementCounts[el] || 0;
            const isUseful = fortune.usefulElement === el;
            return (
              <div
                key={el}
                className={`px-2 py-2 rounded text-center border ${ELEMENT_COLORS[el]} ${isUseful ? 'ring-2 ring-amber-400/60' : ''}`}
              >
                <div className="text-lg font-bold">{el}</div>
                <div className="text-xs">{cnt}个</div>
                {isUseful && <div className="text-[10px] mt-0.5">用神</div>}
              </div>
            );
          })}
        </div>
      </div>

      {/* 宜忌 */}
      <div className="grid grid-cols-2 gap-3">
        <div className="fin-panel border border-emerald-500/30 rounded-lg p-3 bg-emerald-500/5">
          <div className="text-xs text-emerald-300 font-medium mb-2 flex items-center gap-1">
            <TrendingUp className="w-3 h-3" />
            今日宜
          </div>
          <div className="space-y-1">
            {fortune.suitable.map(s => (
              <div key={s} className="text-sm fin-text-primary flex items-center gap-2">
                <span className="text-emerald-400">✓</span>
                {s}
              </div>
            ))}
          </div>
        </div>
        <div className="fin-panel border border-rose-500/30 rounded-lg p-3 bg-rose-500/5">
          <div className="text-xs text-rose-300 font-medium mb-2 flex items-center gap-1">
            <AlertTriangle className="w-3 h-3" />
            今日忌
          </div>
          <div className="space-y-1">
            {fortune.avoid.map(s => (
              <div key={s} className="text-sm fin-text-primary flex items-center gap-2">
                <span className="text-rose-400">✗</span>
                {s}
              </div>
            ))}
          </div>
        </div>
      </div>

      {/* 幸运元素 */}
      <div className="grid grid-cols-3 gap-3">
        <div className="fin-panel border fin-divider rounded-lg p-3">
          <div className="text-xs fin-text-tertiary mb-1 flex items-center gap-1">
            <Compass className="w-3 h-3" />
            幸运方位
          </div>
          <div className="text-base fin-text-primary font-bold">{fortune.luckyDirection}</div>
        </div>
        <div className="fin-panel border fin-divider rounded-lg p-3">
          <div className="text-xs fin-text-tertiary mb-1">幸运色</div>
          <div className="text-base fin-text-primary font-bold">{fortune.luckyColor}</div>
        </div>
        <div className="fin-panel border fin-divider rounded-lg p-3">
          <div className="text-xs fin-text-tertiary mb-1 flex items-center gap-1">
            <Hash className="w-3 h-3" />
            幸运数字
          </div>
          <div className="text-base fin-text-primary font-bold font-mono">
            {fortune.luckyNumbers.join(' / ')}
          </div>
        </div>
      </div>

      {/* 板块提示 */}
      <div className="fin-panel border border-amber-500/30 rounded-lg p-3 bg-amber-500/5">
        <div className="text-xs text-amber-300 font-medium mb-1.5 flex items-center gap-1">
          <Coffee className="w-3 h-3" />
          今日可关注板块（用神：{fortune.usefulElement}）
        </div>
        <div className="text-sm fin-text-primary">{fortune.sectorHint}</div>
      </div>

      {/* 诗句 */}
      <div className="fin-panel border fin-divider rounded-lg p-3 text-center italic text-sm fin-text-secondary">
        {fortune.poem}
      </div>
    </div>
  );
};
