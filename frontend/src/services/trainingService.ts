import { TrainingSession, TradeRecord, CapitalSnapshot, KLineData, MilestoneInfo, TradeAction, PositionLevel } from '../types';
import { 
  CreateTrainingSession, GetTrainingSession, GetTrainingKlines,
  ExecuteTrainingTrade, NextTrainingDay, AbortTraining,
  GetTrainingTrades, GetTrainingCapitalCurve, GetTrainingStats,
  GetAllMilestones
} from '../../wailsjs/go/main/App';

export async function createTrainingSession(): Promise<TrainingSession | null> {
  try {
    const result = await CreateTrainingSession();
    return result as TrainingSession;
  } catch (err) {
    console.error('Failed to create training session:', err);
    return null;
  }
}

export async function getTrainingSession(sessionId: string): Promise<TrainingSession | null> {
  try {
    const result = await GetTrainingSession(sessionId);
    return result as TrainingSession;
  } catch (err) {
    return null;
  }
}

export async function getTrainingKlines(sessionId: string): Promise<KLineData[]> {
  try {
    const result = await GetTrainingKlines(sessionId);
    return (result || []) as KLineData[];
  } catch (err) {
    return [];
  }
}

export async function executeTrade(
  sessionId: string, 
  action: TradeAction, 
  positionLevel: PositionLevel
): Promise<TradeRecord | null> {
  try {
    const result = await ExecuteTrainingTrade({ sessionId, action, positionLevel });
    return result as TradeRecord;
  } catch (err) {
    console.error('Trade failed:', err);
    return null;
  }
}

export async function nextDay(sessionId: string): Promise<KLineData | null> {
  try {
    const result = await NextTrainingDay(sessionId);
    return result as KLineData;
  } catch (err) {
    return null;
  }
}

export async function abortTraining(sessionId: string): Promise<boolean> {
  try {
    const result = await AbortTraining(sessionId);
    return result === 'success';
  } catch (err) {
    return false;
  }
}

export async function getTrades(sessionId: string): Promise<TradeRecord[]> {
  try {
    const result = await GetTrainingTrades(sessionId);
    return (result || []) as TradeRecord[];
  } catch (err) {
    return [];
  }
}

export async function getCapitalCurve(sessionId: string): Promise<CapitalSnapshot[]> {
  try {
    const result = await GetTrainingCapitalCurve(sessionId);
    return (result || []) as CapitalSnapshot[];
  } catch (err) {
    return [];
  }
}

export async function getStats(sessionId: string): Promise<Record<string, any>> {
  try {
    const result = await GetTrainingStats(sessionId);
    return result || {};
  } catch (err) {
    return {};
  }
}

export async function getAllMilestones(): Promise<MilestoneInfo[]> {
  try {
    const result = await GetAllMilestones();
    return (result || []) as MilestoneInfo[];
  } catch (err) {
    return [];
  }
}
