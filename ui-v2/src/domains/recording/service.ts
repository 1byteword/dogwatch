import { api } from "../../core/api";
import type { RecordingRule, EvaluationHistory, RecordingRulesStatus } from "./types";

export async function loadRecordingRules(): Promise<RecordingRule[]> {
  try {
    return await api.get<RecordingRule[]>("/api/recording-rules");
  } catch {
    return [];
  }
}

export async function loadRecordingRuleHistory(ruleId: string): Promise<EvaluationHistory[]> {
  if (!ruleId) return [];
  try {
    return await api.get<EvaluationHistory[]>(`/api/recording-rules/${ruleId}/history?limit=50`);
  } catch {
    return [];
  }
}

export async function loadRecordingRulesStatus(): Promise<RecordingRulesStatus | null> {
  try {
    return await api.get<RecordingRulesStatus>("/api/recording-rules-status");
  } catch {
    return null;
  }
}
