/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or (at your option) any later version.
*/

export type PurityRisk = 'low' | 'medium' | 'high' | 'unknown'
export type PurityStatus =
  | 'pending'
  | 'running'
  | 'completed'
  | 'failed'
  | 'unknown'
export type PurityEvidenceKind =
  | 'protocol'
  | 'declared_model'
  | 'usage'
  | 'warning'
  | 'operational'
  | 'generic'

export type PurityEvidence = {
  id: string
  kind: PurityEvidenceKind
  title?: string
  description?: string
  expected?: string
  actual?: string
}

export type PurityResult = {
  id: string | number
  scan_id?: string | number
  channel_id: number
  channel_name?: string
  model: string
  risk: PurityRisk
  coverage: number
  status: PurityStatus
  summary?: string
  error_class?: string
  evidence?: PurityEvidence[]
  created_at?: string | number
  updated_at?: string | number
}

export type PuritySettings = {
  enabled: boolean
  interval_minutes: number
}

export type PurityRunStatus = {
  status: PurityStatus
  run_id?: string | number
  enabled_channels: number
  model_combinations: number
  completed: number
  failed: number
  last_run_at?: string | number
  next_run_at?: string | number
  started_at?: string | number
  finished_at?: string | number
  error?: string
}

export type PurityFullScanResponse = {
  run_id?: string | number
  status?: PurityStatus
}

export type ApiEnvelope<T> = {
  success?: boolean
  message?: string
  data?: T
}
