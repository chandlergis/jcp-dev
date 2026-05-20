# K线训练营 - 功能规划

## 核心概念
用户通过模拟交易训练K线形态识别能力，系统随机提供股票K线数据，用户进行买入/持有/卖出操作，以收盘价成交。

## 已完成后端

### 数据模型 (`internal/models/training.go`)
- TrainingSession - 训练会话
- TradeRecord - 交易记录  
- MilestoneInfo - 里程碑信息
- CapitalSnapshot - 资金快照
- TrainingRecord - 训练记录（持久化）

### 训练管理器 (`internal/training/manager.go`)
- CreateSession - 创建训练会话（随机股票+时间段）
- ExecuteTrade - 执行买入/卖出
- NextDay - 推进到下一天
- checkMilestones - 检查里程碑
- updateCapitalCurve - 更新资金曲线
- CalculateStats - 计算统计数据

### 训练服务 (`internal/services/training_service.go`)
- 封装训练管理器
- 持久化训练记录到 `data/training_records.json`

### API接口 (`app.go`)
```
CreateTrainingSession() -> TrainingSession
GetTrainingSession(sessionId) -> TrainingSession
GetTrainingKlines(sessionId) -> KLineData[]
ExecuteTrainingTrade({sessionId, action, positionLevel}) -> TradeRecord
NextTrainingDay(sessionId) -> KLineData
AbortTraining(sessionId) -> string
GetTrainingTrades(sessionId) -> TradeRecord[]
GetTrainingCapitalCurve(sessionId) -> CapitalSnapshot[]
GetTrainingStats(sessionId) -> Record<string, any>
GetTrainingRecords() -> TrainingRecord[]
GetBestTrainingRecord() -> TrainingRecord
GetMilestoneInfo(type) -> MilestoneInfo
GetAllMilestones() -> MilestoneInfo[]
```

### Wails绑定已更新
- `frontend/wailsjs/go/main/App.js` - 已添加所有训练API
- `frontend/wailsjs/go/main/App.d.ts` - 已添加类型定义
- `frontend/wailsjs/go/models.ts` - 已添加TrainingSession, TradeRecord等类型

---

## 待完成前端

### 1. 创建前端服务 `frontend/src/services/trainingService.ts`

```typescript
import { TrainingSession, TradeRecord, CapitalSnapshot, KLineData, MilestoneInfo } from '../types';
import { 
  CreateTrainingSession, GetTrainingSession, GetTrainingKlines,
  ExecuteTrainingTrade, NextTrainingDay, AbortTraining,
  GetTrainingTrades, GetTrainingCapitalCurve, GetTrainingStats,
  GetTrainingRecords, GetBestTrainingRecord, GetAllMilestones
} from '../../wailsjs/go/main/App';

// 创建训练会话
export async function createTrainingSession(): Promise<TrainingSession | null> {
  try {
    return await CreateTrainingSession();
  } catch (err) {
    console.error('Failed to create training session:', err);
    return null;
  }
}

// 获取训练会话
export async function getTrainingSession(sessionId: string): Promise<TrainingSession | null> {
  try {
    return await GetTrainingSession(sessionId);
  } catch (err) {
    return null;
  }
}

// 获取可见K线数据
export async function getTrainingKlines(sessionId: string): Promise<KLineData[]> {
  try {
    return await GetTrainingKlines(sessionId) || [];
  } catch (err) {
    return [];
  }
}

// 执行交易
export async function executeTrade(
  sessionId: string, 
  action: 'buy' | 'sell', 
  positionLevel: 'full' | 'half' | 'quarter' | 'tenth'
): Promise<TradeRecord | null> {
  try {
    return await ExecuteTrainingTrade({ sessionId, action, positionLevel });
  } catch (err) {
    console.error('Trade failed:', err);
    return null;
  }
}

// 推进到下一天
export async function nextDay(sessionId: string): Promise<KLineData | null> {
  try {
    return await NextTrainingDay(sessionId);
  } catch (err) {
    return null;
  }
}

// 中止训练
export async function abortTraining(sessionId: string): Promise<boolean> {
  try {
    const result = await AbortTraining(sessionId);
    return result === 'success';
  } catch (err) {
    return false;
  }
}

// 获取交易记录
export async function getTrades(sessionId: string): Promise<TradeRecord[]> {
  try {
    return await GetTrainingTrades(sessionId) || [];
  } catch (err) {
    return [];
  }
}

// 获取资金曲线
export async function getCapitalCurve(sessionId: string): Promise<CapitalSnapshot[]> {
  try {
    return await GetTrainingCapitalCurve(sessionId) || [];
  } catch (err) {
    return [];
  }
}

// 获取统计数据
export async function getStats(sessionId: string): Promise<Record<string, any>> {
  try {
    return await GetTrainingStats(sessionId) || {};
  } catch (err) {
    return {};
  }
}

// 获取所有里程碑
export async function getAllMilestones(): Promise<MilestoneInfo[]> {
  try {
    return await GetAllMilestones() || [];
  } catch (err) {
    return [];
  }
}
```

### 2. 更新类型定义 `frontend/src/types.ts`

在文件末尾添加：

```typescript
// ========== K线训练营类型 ==========

export type TradeAction = 'buy' | 'sell';
export type PositionLevel = 'full' | 'half' | 'quarter' | 'tenth';
export type TrainingStatus = 'running' | 'finished' | 'aborted';

export interface TrainingSession {
  id: string;
  stockCode: string;
  stockName: string;
  startDate: string;
  endDate: string;
  currentDate: string;
  currentIndex: number;
  totalDays: number;
  initialCapital: number;
  cash: number;
  totalAsset: number;
  position: number;
  avgCost: number;
  positionValue: number;
  totalReturn: number;
  totalProfit: number;
  status: TrainingStatus;
  isTradingDay: boolean;
  milestones: string[];
  tradeCount: number;
  winCount: number;
  maxDrawdown: number;
  winStreak: number;
  maxWinStreak: number;
  createdAt: string;
  finishedAt?: string;
}

export interface TradeRecord {
  id: string;
  sessionId: string;
  action: TradeAction;
  date: string;
  price: number;
  quantity: number;
  amount: number;
  positionLevel: PositionLevel;
  cashAfter: number;
  positionAfter: number;
  assetAfter: number;
  profit?: number;
  profitPercent?: number;
  createdAt: string;
}

export interface CapitalSnapshot {
  date: string;
  totalAsset: number;
  cash: number;
  position: number;
  price: number;
}

export interface MilestoneInfo {
  type: string;
  name: string;
  description: string;
  icon: string;
}

export interface TrainingStats {
  totalReturn: number;
  totalProfit: number;
  tradeCount: number;
  winCount: number;
  winRate: number;
  maxDrawdown: number;
  avgHoldingDays: number;
  sharpeRatio: number;
  maxWinStreak: number;
}
```

### 3. 创建训练主组件 `frontend/src/components/TrainingMode.tsx`

功能要求：
- 全屏模式，左侧K线图，右侧交易面板
- 顶部显示股票信息和当前日期
- 底部显示资金曲线和交易记录

主要状态：
```typescript
const [session, setSession] = useState<TrainingSession | null>(null);
const [klines, setKlines] = useState<KLineData[]>([]);
const [loading, setLoading] = useState(false);
const [showResult, setShowResult] = useState(false);
const [stats, setStats] = useState<TrainingStats | null>(null);
```

核心功能：
1. **开始训练** - 调用createTrainingSession
2. **显示K线** - 使用StockChartLW组件，传入可见K线数据
3. **买入操作** - 选择仓位后调用executeTrade('buy', level)
4. **卖出操作** - 选择仓位后调用executeTrade('sell', level)
5. **下一天** - 调用nextDay，更新K线显示
6. **结束训练** - 显示统计结果

### 4. 创建交易面板组件 `frontend/src/components/TradingPanel.tsx`

显示内容：
- 当前持仓信息（数量、成本、市值、盈亏）
- 可用资金
- 买入/卖出按钮（带仓位选择）
- 下一天按钮
- 交易记录列表

UI布局：
```
┌─────────────────────────────┐
│ 持仓信息                     │
│ 数量: 1000股  成本: 15.50    │
│ 市值: 16000   盈亏: +500    │
├─────────────────────────────┤
│ 可用资金: 84000              │
├─────────────────────────────┤
│ [全仓买入] [半仓买入]        │
│ [1/4仓买入] [1/10仓买入]     │
├─────────────────────────────┤
│ [全仓卖出] [半仓卖出]        │
│ [1/4仓卖出] [1/10仓卖出]     │
├─────────────────────────────┤
│ [下一天 ▶]                   │
├─────────────────────────────┤
│ 交易记录                     │
│ 2024-01-15 买入 1000股 15.50 │
│ 2024-01-20 卖出 1000股 16.00 │
└─────────────────────────────┘
```

### 5. 创建资金曲线组件 `frontend/src/components/CapitalCurve.tsx`

使用Lightweight Charts绘制资金曲线：
- X轴：日期
- Y轴：总资产
- 显示买入/卖出标记

### 6. 创建里程碑弹窗 `frontend/src/components/MilestoneDialog.tsx`

当达成里程碑时显示：
- 里程碑图标和名称
- 描述文字
- 庆祝动画效果
- 关闭按钮

### 7. 创建训练结果组件 `frontend/src/components/TrainingResult.tsx`

训练结束后显示：
- 总收益率
- 总盈亏
- 交易次数/胜率
- 最大回撤
- 夏普比率
- 最大连胜
- 资金曲线图
- 再来一次/返回按钮

### 8. 添加入口到主界面

修改 `frontend/src/App.tsx`：
1. 添加状态 `const [showTraining, setShowTraining] = useState(false)`
2. 在顶部导航栏添加"训练营"按钮
3. 添加训练模式的条件渲染

```tsx
// 在顶部导航栏添加按钮
<button
  onClick={() => setShowTraining(true)}
  className={`p-2 rounded-lg fin-panel border fin-divider transition-colors ${colors.isDark ? 'text-slate-300 hover:text-white' : 'text-slate-600 hover:text-slate-900'} hover:border-green-400/40`}
  title="K线训练营"
>
  <GraduationCap className="h-4 w-4" />
</button>

// 在主内容区域添加条件渲染
{showTraining && (
  <TrainingMode onClose={() => setShowTraining(false)} />
)}
```

---

## 里程碑定义

| 类型 | 名称 | 描述 | 图标 |
|------|------|------|------|
| return_10 | 初级交易员 | 累计收益达到10% | 🌱 |
| return_30 | 中级交易员 | 累计收益达到30% | 📈 |
| return_50 | 高级交易员 | 累计收益达到50% | 🏆 |
| return_100 | 交易大师 | 累计收益达到100% | 👑 |
| win_streak_3 | 稳定盈利者 | 连续3次盈利交易 | 🎯 |
| single_profit_20 | 精准抄底 | 单笔收益超过20% | 💎 |

## 交易规则

1. **T+1规则** - 买入当天不能卖出
2. **收盘价成交** - 以当日收盘价买入/卖出
3. **仓位管理** - 支持全仓/半仓/1/4仓/1/10仓
4. **初始资金** - 100万虚拟资金
5. **按手交易** - 最小交易单位100股
6. **随机股票** - 系统随机选择股票
7. **随机时间段** - 60-120个交易日
8. **逐K线推进** - 用户无法看到未来数据

## 数据存储

训练记录保存在 `data/training_records.json`：
```json
{
  "records": [
    {
      "session": { ... },
      "trades": [ ... ],
      "capitalCurve": [ ... ]
    }
  ]
}
```
