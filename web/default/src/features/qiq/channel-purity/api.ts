/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or (at your option) any later version.
*/
import { api } from '@/lib/api'

import type {
  ApiEnvelope,
  PurityResult,
  PurityScanRequest,
  PurityStatus,
} from './types'

function unwrap<T>(payload: ApiEnvelope<T> | T): T {
  if (payload && typeof payload === 'object' && 'data' in payload) {
    return (payload as ApiEnvelope<T>).data ?? (payload as T)
  }
  return payload as T
}

function normalizeResult(raw: Record<string, unknown>): PurityResult {
  const channel = (raw.channel ?? {}) as Record<string, unknown>
  return {
    id: String(raw.id ?? raw.scan_id ?? `${raw.channel_id}-${raw.model}`),
    scan_id: (raw.scan_id ?? raw.id) as string | number,
    channel_id: Number(raw.channel_id ?? channel.id ?? 0),
    channel_name: String(raw.channel_name ?? channel.name ?? '-'),
    model: String(raw.model ?? '-'),
    risk: (raw.risk ?? raw.risk_level ?? 'unknown') as PurityResult['risk'],
    coverage: Number(raw.coverage ?? raw.coverage_rate ?? 0),
    status: (raw.status ?? 'pending') as PurityStatus,
    summary: raw.summary as string | undefined,
    created_at: (raw.created_at ?? raw.created_time) as string | number,
    updated_at: (raw.updated_at ?? raw.updated_time) as string | number,
    evidence: (raw.evidence ?? raw.evidences ?? []) as PurityResult['evidence'],
  }
}

export async function getPurityResults(): Promise<PurityResult[]> {
  const response = await api.get('/api/channel/purity/results')
  const payload = unwrap<unknown>(response.data)
  const records = Array.isArray(payload)
    ? payload
    : (((payload as Record<string, unknown>)?.items ??
        (payload as Record<string, unknown>)?.results ??
        []) as unknown[])
  return records.map((record) =>
    normalizeResult(record as Record<string, unknown>)
  )
}

export async function startPurityScan(
  input: PurityScanRequest
): Promise<ApiEnvelope<{ id?: string | number; scan_id?: string | number }>> {
  const response = await api.post('/api/channel/purity/scans', input)
  return response.data
}

export async function getPurityScan(id: string): Promise<PurityResult> {
  const response = await api.get(`/api/channel/purity/scans/${id}`)
  return normalizeResult(unwrap<Record<string, unknown>>(response.data))
}
