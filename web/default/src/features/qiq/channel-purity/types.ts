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

export type SimilarityMetric = { value?: number; sample_size: number }
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
export type TargetResult = {
  id: string
  group_id: string
  target_channel_id: number
  target_channel_name: string
  baseline_channel_id: number
  baseline_channel_name: string
  model: string
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
  updated_at?: string | number
}
export type SamplingSettings = {
  window_minutes: number
  minimum_samples: number
  max_samples_per_window: number
}
export type PurityGroup = {
  id: string
  name: string
  enabled: boolean
  channel_ids: number[]
  baseline_channel_id: number
  interval_minutes: 5 | 10
  random_pairing_enabled: boolean
  sampling: SamplingSettings
  results: TargetResult[]
  last_run_at?: string | number
  next_run_at?: string | number
  last_error?: string
  updated_at?: string | number
}
export type PurityGroupInput = Omit<PurityGroup, 'id' | 'results' | 'last_run_at' | 'next_run_at' | 'last_error' | 'updated_at'>
export type ChannelOption = { id: number; name: string; models?: string[]; groups: string[] }
export type QuickProbeInput = { channel_id: number; model?: string }
export type QuickProbeResult = {
  ok: boolean
  latency_ms?: number
  message: string
  checked_at?: string | number
}
export type ApiEnvelope<T> = { success?: boolean; message?: string; data?: T }
