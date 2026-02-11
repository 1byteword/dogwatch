export interface RecordingRule {
  id: string;
  name: string;
  expression: string;
  interval: number; // nanoseconds from Go, we'll normalize
  labels: Record<string, string>;
  enabled: boolean;
  last_eval: string;
  last_error: string;
  last_value: number;
  description: string;
  created_at: string;
  updated_at: string;
  created_by: string;
}

export interface EvaluationHistory {
  id: string;
  rule_id: string;
  timestamp: string;
  value: number;
  duration_ms: number;
  error: string;
  success: boolean;
}

export interface RecordingRulesStatus {
  total_rules: number;
  enabled_rules: number;
  total_evaluations: number;
  successful_evaluations: number;
  failed_evaluations: number;
  last_eval_duration: number;
  avg_eval_duration: number;
}
