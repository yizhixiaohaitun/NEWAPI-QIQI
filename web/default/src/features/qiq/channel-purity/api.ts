/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or (at your option) any later version.
*/
import { api } from '@/lib/api'
import { deduplicateChannels, normalizeChannelGroups, shouldContinueChannelPages } from './channel-state'
import type {
  ApiEnvelope,
  ChannelOption,
  DetectorStatus,
  PurityEvidence,
  PurityGroup,
  PurityGroupInput,
  QuickProbeInput,
  QuickProbeResult,
  TargetResult,
  TokenRange,
  TrendPoint,
} from './types'

// Backend contract is intentionally isolated here. The UI only consumes normalized types.
const ROOT = '/api/channel/purity/groups'
const STATUSES = new Set<DetectorStatus>([
  'BASELINE_UNAVAILABLE', 'LOW_SAMPLE', 'NO_TRAFFIC', 'WARMING_UP',
  'HEALTHY', 'SUSPECT', 'ALERT', 'DETECTOR_ERROR',
])
const record = (value: unknown): Record<string, unknown> =>
  value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {}
const array = (value: unknown): unknown[] => Array.isArray(value) ? value : []
const number = (value: unknown, fallback = 0) => {
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : fallback
}
const optionalNumber = (value: unknown) => value === null || value === undefined || value === '' ? undefined : number(value)
const status = (value: unknown): DetectorStatus => STATUSES.has(value as DetectorStatus) ? value as DetectorStatus : 'WARMING_UP'
function unwrap(payload: unknown): unknown {
  const envelope = record(payload)
  if (envelope.success === false) throw new Error(String(envelope.message || 'Request failed'))
  return 'data' in envelope ? envelope.data : payload
}
function range(value: unknown): TokenRange | undefined {
  const item = record(value)
  if (item.min === undefined || item.max === undefined) return undefined
  return { min: number(item.min), max: number(item.max), p50: optionalNumber(item.p50), p95: optionalNumber(item.p95) }
}
function evidence(value: unknown, index: number): PurityEvidence {
  const item = record(value)
  return {
    id: String(item.id ?? index), occurred_at: item.occurred_at as string | number ?? '',
    kind: String(item.kind ?? 'observation'), summary: String(item.summary ?? item.description ?? ''),
    baseline_value: item.baseline_value == null ? undefined : String(item.baseline_value),
    target_value: item.target_value == null ? undefined : String(item.target_value),
    request_id: item.request_id == null ? undefined : String(item.request_id),
  }
}
function trend(value: unknown): TrendPoint[] {
  return array(value).map((raw) => { const item = record(raw); return {
    at: item.at as string | number, status: status(item.status),
    field_similarity: optionalNumber(item.field_similarity), token_similarity: optionalNumber(item.token_similarity),
    confidence: optionalNumber(item.confidence),
  } })
}
function normalizeResult(value: unknown, group: Record<string, unknown>): TargetResult {
  const item = record(value)
  const evidenceItems = array(item.evidence).map(evidence)
  const field = record(item.field_similarity ?? item.structure_similarity)
  const token = record(item.token_similarity ?? item.token_range_similarity)
  return {
    id: String(item.id ?? `${group.id}-${item.target_channel_id}-${item.model}`), group_id: String(group.id),
    target_channel_id: number(item.target_channel_id), target_channel_name: String(item.target_channel_name ?? `#${item.target_channel_id}`),
    baseline_channel_id: number(item.baseline_channel_id ?? group.baseline_channel_id),
    baseline_channel_name: String(item.baseline_channel_name ?? group.baseline_channel_name ?? `#${group.baseline_channel_id}`),
    model: String(item.model ?? '—'),
    baseline_model: String(item.baseline_model ?? item.model ?? '—'), target_model: String(item.target_model ?? item.model ?? '—'),
    status: status(item.status), samples: number(item.samples ?? item.sample_count),
    field_similarity: { value: optionalNumber(field.value ?? item.field_similarity), sample_size: number(field.sample_size ?? item.samples) },
    token_similarity: { value: optionalNumber(token.value ?? item.token_similarity), sample_size: number(token.sample_size ?? item.samples) },
    confidence: optionalNumber(item.confidence), baseline_token_range: range(item.baseline_token_range),
    target_token_range: range(item.target_token_range), deviation_rate: optionalNumber(item.deviation_rate),
    latest_evidence: item.latest_evidence ? evidence(item.latest_evidence, -1) : evidenceItems[0], evidence: evidenceItems,
    alerts: array(item.alerts).map(String), trend: trend(item.trend ?? item.history),
    updated_at: item.updated_at as string | number | undefined,
  }
}
function normalizeGroup(value: unknown): PurityGroup {
  const item = record(value)
  const sampling = record(item.sampling)
  const interval = number(item.interval_minutes, 5)
  return {
    id: String(item.id), name: String(item.name ?? 'Untitled group'), enabled: item.enabled !== false,
    channel_ids: array(item.channel_ids).map(Number), baseline_channel_id: number(item.baseline_channel_id),
    interval_minutes: interval === 10 ? 10 : 5, random_pairing_enabled: Boolean(item.random_pairing_enabled),
    model_comparisons: array(item.model_comparisons).map((raw) => { const comparison = record(raw); return { baseline_model: String(comparison.baseline_model ?? ''), target_model: String(comparison.target_model ?? '') } }),
    model_comparisons_required: Boolean(item.model_comparisons_required),
    sampling: { window_minutes: number(sampling.window_minutes, 30), minimum_samples: number(sampling.minimum_samples, 20), max_samples_per_window: number(sampling.max_samples_per_window, 200) },
    results: array(item.results ?? item.targets).map((result) => normalizeResult(result, item)),
    last_run_at: item.last_run_at as string | number | undefined,
    next_run_at: item.next_run_at as string | number | undefined,
    last_error: item.last_error == null ? undefined : String(item.last_error),
    updated_at: item.updated_at as string | number | undefined,
  }
}
const config = { skipBusinessError: true, skipErrorHandler: true }
export async function listPurityGroups(): Promise<PurityGroup[]> {
  const response = await api.get(ROOT, config)
  const payload = unwrap(response.data)
  return array(Array.isArray(payload) ? payload : record(payload).items).map(normalizeGroup)
}
export async function getPurityGroup(id: string): Promise<PurityGroup> {
  const response = await api.get(`${ROOT}/${id}`, config)
  return normalizeGroup(unwrap(response.data))
}
export async function createPurityGroup(input: PurityGroupInput): Promise<PurityGroup> {
  const response = await api.post(ROOT, input, config)
  return normalizeGroup(unwrap(response.data))
}
export async function updatePurityGroup(id: string, input: PurityGroupInput): Promise<PurityGroup> {
  const response = await api.put(`${ROOT}/${id}`, input, config)
  return normalizeGroup(unwrap(response.data))
}
export async function deletePurityGroup(id: string): Promise<void> {
  const response = await api.delete(`${ROOT}/${id}`, config)
  unwrap(response.data)
}
export async function listChannelOptions(): Promise<ChannelOption[]> {
  const pageSize = 500
  const items: unknown[] = []
  for (let page = 1; ; page += 1) {
    const response = await api.get('/api/channel/search', { params: { p: page, page_size: pageSize }, ...config })
    const payload = unwrap(response.data)
    const body = record(payload)
    const pageItems = array(Array.isArray(payload) ? payload : body.items ?? body.data)
    items.push(...pageItems)
    const total = body.total === undefined || body.total === null ? undefined : number(body.total)
    if (!shouldContinueChannelPages(pageItems.length, items.length, total)) break
  }
  return deduplicateChannels(items.map((raw) => {
    const item = record(raw)
    return {
      id: number(item.id),
      name: String(item.name ?? `#${item.id}`),
      status: number(item.status),
      models: typeof item.models === 'string' ? item.models.split(',') : array(item.models).map(String),
      groups: normalizeChannelGroups(item.group ?? item.groups),
    }
  }))
}
export async function runPurityGroup(id: string): Promise<void> {
  const response = await api.post(`${ROOT}/${id}/run`, undefined, config)
  unwrap(response.data)
}

function historyPoint(value: unknown): TrendPoint {
  const item = record(value)
  return {
    at: (item.window_ended_at ?? item.created_at) as string | number,
    status: status(item.state ?? item.status),
    field_similarity: optionalNumber(item.structure_similarity ?? item.field_similarity),
    token_similarity: optionalNumber(item.token_similarity),
    confidence: optionalNumber(item.confidence),
  }
}

export async function getPurityResultDetail(result: TargetResult): Promise<TargetResult> {
  const params = { target_channel_id: result.target_channel_id, actual_model: result.model }
  const [latestResponse, historyResponse] = await Promise.all([
    api.get(`${ROOT}/${result.group_id}/latest`, { params, ...config }),
    api.get(`${ROOT}/${result.group_id}/history`, { params: { ...params, p: 1, page_size: 100 }, ...config }),
  ])
  const latest = record(unwrap(latestResponse.data))
  const historyPayload = unwrap(historyResponse.data)
  const historyItems = array(Array.isArray(historyPayload) ? historyPayload : record(historyPayload).items)
  return {
    ...result,
    status: status(latest.state ?? latest.status ?? result.status),
    confidence: optionalNumber(latest.confidence) ?? result.confidence,
    updated_at: (latest.updated_at ?? result.updated_at) as string | number | undefined,
    trend: historyItems.slice().reverse().map(historyPoint),
  }
}

export async function runQuickProbe(input: QuickProbeInput): Promise<QuickProbeResult> {
  const response = await api.post('/api/channel/purity/quick-probe', input, config)
  const item = record(unwrap(response.data))
  return { ok: Boolean(item.ok ?? item.success), latency_ms: optionalNumber(item.latency_ms), message: String(item.message ?? ''), checked_at: item.checked_at as string | number | undefined }
}
export type { ApiEnvelope }
