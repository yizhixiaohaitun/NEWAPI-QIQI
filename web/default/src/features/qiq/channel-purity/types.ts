/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or (at your option) any later version.
*/

export type DetectorStatus =
  | 'BASELINE_UNAVAILABLE'
  | 'LOW_SAMPLE'
  | 'NO_TRAFFIC'
  | 'WARMING_UP'
  | 'HEALTHY'
  | 'SUSPECT'
  | 'ALERT'
  | 'DETECTOR_ERROR'

export type FieldProfileDifference = {
  path: string
  change?: 'missing' | 'added' | 'type_changed' | 'frequency_changed' | string
  baseline_types?: string[]
  target_types?: string[]
  baseline_type?: string
  target_type?: string
  baseline_count: number
  target_count: number
}
export type StructureDifference = {
  signature: string
  baseline_count: number
  target_count: number
  matched_count: number
}
export type StructureDimensionDifference = {
  dimension: 'protocol' | 'model_family' | 'event_sequence' | 'event' | 'finish_reason' | 'header_presence' | 'metadata' | string
  value: string
  change?: 'missing' | 'added' | 'frequency_changed' | string
  baseline_count: number
  target_count: number
}
export type StructureSimilarityDetail = {
  version: string
  method: 'multiset_jaccard' | string
  window_started_at: string | number
  window_ended_at: string | number
  paired_sample_count: number
  matched_count: number
  baseline_only_count: number
  target_only_count: number
  intersection_count: number
  union_count: number
  differences: StructureDifference[]
  field_paths_available: boolean
  detail_available?: boolean
  score_available?: boolean
  field_differences?: FieldProfileDifference[]
  dimension_differences?: StructureDimensionDifference[]
  limitation?: string
}
export type SimilarityMetric = { value?: number; sample_size: number; detail?: StructureSimilarityDetail }
export type TokenRange = { min: number; max: number; p50?: number; p95?: number }
export type PurityEvidence = {
  id: string
  occurred_at: string | number
  kind: string
  summary: string
  baseline_value?: string
  target_value?: string
  request_id?: string
}
export type TrendPoint = {
  at: string | number
  status: DetectorStatus
  field_similarity?: number
  token_similarity?: number
  confidence?: number
}
export type SystemTaskStatus = 'pending' | 'running' | 'succeeded' | 'failed'
export type PurityRunTask = { task_id: string; status: SystemTaskStatus; error?: string }
export type PairRunDetail = {
  id?: number
  baseline_sample_count?: number
  target_sample_count?: number
  paired_sample_count?: number
  baseline_invalid_count?: number
  target_invalid_count?: number
  unmatched_baseline_count?: number
  unmatched_target_count?: number
  window_started_at?: string | number
  window_ended_at?: string | number
  state?: DetectorStatus
  error_class?: string
  created_at?: string | number
}
export type PurityPolicy = {
  suspect_threshold: number
  alert_threshold: number
  alert_windows: number
  recovery_windows: number
}
export type RetentionPolicy = {
  max_windows_per_target_model: number
  policy: string
}
export type StatusExplanation = {
  code: string
  summary: string
  suggested_action: string
  combined_similarity?: number
  suspect_threshold?: number
  alert_threshold?: number
  consecutive_anomalies?: number
  consecutive_healthy?: number
  baseline_available?: boolean
}
export type PurityIncident = {
  id: number
  status: 'OPEN' | 'ACKNOWLEDGED' | 'SILENCED' | 'FALSE_POSITIVE' | 'RESOLVED'
  note?: string
  silence_until?: string | number
  opened_at: string | number
  resolved_at?: string | number
  audit?: PurityIncidentAudit[]
}
export type PurityIncidentAudit = {
  id: number
  action: string
  note?: string
  created_at: string | number
}
export type TargetResult = {
  id: string
  group_id: string
  target_channel_id: number
  target_channel_name: string
  baseline_channel_id: number
  baseline_channel_name: string
  model: string
  baseline_model: string
  target_model: string
  status: DetectorStatus
  samples: number
  field_similarity: SimilarityMetric
  token_similarity: SimilarityMetric
  confidence?: number
  baseline_token_range?: TokenRange
  target_token_range?: TokenRange
  deviation_rate?: number
  latest_evidence?: PurityEvidence
  evidence: PurityEvidence[]
  alerts: string[]
  trend: TrendPoint[]
  pair_run?: PairRunDetail
  explanation?: StatusExplanation
  incidents: PurityIncident[]
  error_class?: string
  updated_at?: string | number
}
export type SamplingSettings = {
  window_minutes: number
  minimum_samples: number
  max_samples_per_window: number
}
export type ModelComparison = { baseline_model: string; target_model: string }
export type PurityGroup = {
  id: string
  name: string
  enabled: boolean
  channel_ids: number[]
  baseline_channel_id: number
  interval_minutes: 5 | 10
  random_pairing_enabled: boolean
  model_comparisons: ModelComparison[]
  model_comparisons_required?: boolean
  sampling: SamplingSettings
  policy: PurityPolicy
  retention: RetentionPolicy
  results: TargetResult[]
  last_run_at?: string | number
  next_run_at?: string | number
  last_error?: string
  updated_at?: string | number
}
export type PurityGroupInput = Omit<PurityGroup, 'id' | 'results' | 'last_run_at' | 'next_run_at' | 'last_error' | 'updated_at'>
export type ChannelOption = { id: number; name: string; status: number; models?: string[]; groups: string[] }
export type PurityHistoryRecord = {
  id: number
  group_id: string
  group_name: string
  target_channel_id: number
  target_channel_name: string
  baseline_model: string
  target_model: string
  status: DetectorStatus
  paired_sample_count: number
  structure_similarity?: number
  token_similarity?: number
  confidence?: number
  window_ended_at: string | number
}
export type PurityHistoryPage = { items: PurityHistoryRecord[]; total: number; page: number; page_size: number }
export type PurityHistoryPreview = { samples: number; pair_runs: number; assessments: number; alerts: number; audits?: number }
export type IncidentAction = 'acknowledge' | 'silence' | 'note' | 'false_positive' | 'resolve'
export type QuickProbeInput = { channel_id: number; model?: string }
export type QuickProbeResult = {
  ok: boolean
  latency_ms?: number
  message: string
  checked_at?: string | number
}
export type ApiEnvelope<T> = { success?: boolean; message?: string; data?: T }
