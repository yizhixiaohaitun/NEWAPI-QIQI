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
  IncidentAction,
  PurityEvidence,
  PurityGroup,
  PurityGroupInput,
  PurityHistoryPage,
  PurityHistoryPreview,
  PurityIncident,
  PurityRunTask,
  QuickProbeInput,
  QuickProbeResult,
  TargetResult,
  StructureSimilarityDetail,
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
function structureDetail(value: unknown): StructureSimilarityDetail | undefined {
  const item = record(value)
  if (!Object.keys(item).length) return undefined
  return {
    version: String(item.version ?? ''),
    method: String(item.method ?? 'multiset_jaccard'),
    window_started_at: item.window_started_at as string | number,
    window_ended_at: item.window_ended_at as string | number,
    paired_sample_count: number(item.paired_sample_count),
    matched_count: number(item.matched_count), baseline_only_count: number(item.baseline_only_count), target_only_count: number(item.target_only_count),
    intersection_count: number(item.intersection_count), union_count: number(item.union_count),
    differences: array(item.differences).map((raw) => { const difference = record(raw); return {
      signature: String(difference.signature ?? ''), baseline_count: number(difference.baseline_count),
      target_count: number(difference.target_count), matched_count: number(difference.matched_count),
    } }),
    field_paths_available: Boolean(item.field_paths_available),
    detail_available: item.detail_available == null ? undefined : Boolean(item.detail_available),
    score_available: item.score_available == null ? undefined : Boolean(item.score_available),
    field_differences: array(item.field_differences).map((raw) => { const difference = record(raw); return {
      path: String(difference.path ?? ''), change: difference.change == null ? undefined : String(difference.change),
      baseline_types: array(difference.baseline_types).map(String), target_types: array(difference.target_types).map(String),
      baseline_type: difference.baseline_type == null ? undefined : String(difference.baseline_type),
      target_type: difference.target_type == null ? undefined : String(difference.target_type),
      baseline_count: number(difference.baseline_count), target_count: number(difference.target_count),
    } }),
    dimension_differences: array(item.dimension_differences).map((raw) => { const difference = record(raw); return {
      dimension: String(difference.dimension ?? ''), value: String(difference.value ?? ''),
      change: difference.change == null ? undefined : String(difference.change),
      baseline_count: number(difference.baseline_count), target_count: number(difference.target_count),
    } }),
    limitation: item.limitation == null ? undefined : String(item.limitation),
  }
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
function incident(value: unknown): PurityIncident {
  const item = record(value)
  const rawStatus = String(item.status ?? 'OPEN')
  const allowed = ['OPEN', 'ACKNOWLEDGED', 'SILENCED', 'FALSE_POSITIVE', 'RESOLVED']
  return {
    id: number(item.id), status: (allowed.includes(rawStatus) ? rawStatus : 'OPEN') as PurityIncident['status'],
    note: item.note == null ? undefined : String(item.note), silence_until: item.silence_until as string | number | undefined,
    opened_at: item.opened_at as string | number, resolved_at: item.resolved_at as string | number | undefined,
    audit: array(item.audit).map((raw) => { const entry = record(raw); return {
      id: number(entry.id), action: String(entry.action ?? ''), note: entry.note == null ? undefined : String(entry.note),
      created_at: entry.created_at as string | number,
    } }),
  }
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
    alerts: array(item.alerts).map((alert) => typeof alert === 'string' ? alert : String(record(alert).message ?? record(alert).status ?? '')),
    incidents: array(item.incidents ?? item.alert_records).map(incident),
    explanation: Object.keys(record(item.explanation)).length ? (() => { const value = record(item.explanation); return {
      code: String(value.code ?? item.status ?? ''), summary: String(value.summary ?? ''), suggested_action: String(value.suggested_action ?? ''),
      combined_similarity: optionalNumber(value.combined_similarity), suspect_threshold: optionalNumber(value.suspect_threshold),
      alert_threshold: optionalNumber(value.alert_threshold), consecutive_anomalies: optionalNumber(value.consecutive_anomalies),
      consecutive_healthy: optionalNumber(value.consecutive_healthy), baseline_available: value.baseline_available == null ? undefined : Boolean(value.baseline_available),
    } })() : undefined,
    trend: trend(item.trend ?? item.history),
    updated_at: item.updated_at as string | number | undefined,
  }
}
function normalizeGroup(value: unknown): PurityGroup {
  const item = record(value)
  const sampling = record(item.sampling)
  const policy = record(item.policy)
  const retention = record(item.retention)
  const interval = number(item.interval_minutes, 5)
  return {
    id: String(item.id), name: String(item.name ?? 'Untitled group'), enabled: item.enabled !== false,
    channel_ids: array(item.channel_ids).map(Number), baseline_channel_id: number(item.baseline_channel_id),
    interval_minutes: interval === 10 ? 10 : 5, random_pairing_enabled: Boolean(item.random_pairing_enabled),
    model_comparisons: array(item.model_comparisons).map((raw) => { const comparison = record(raw); return { baseline_model: String(comparison.baseline_model ?? ''), target_model: String(comparison.target_model ?? '') } }),
    model_comparisons_required: Boolean(item.model_comparisons_required),
    sampling: { window_minutes: number(sampling.window_minutes, 30), minimum_samples: number(sampling.minimum_samples, 20), max_samples_per_window: number(sampling.max_samples_per_window, 200) },
    policy: { suspect_threshold: number(policy.suspect_threshold, .72), alert_threshold: number(policy.alert_threshold, .55), alert_windows: number(policy.alert_windows, 3), recovery_windows: number(policy.recovery_windows, 2) },
    retention: { max_windows_per_target_model: number(retention.max_windows_per_target_model, 100), policy: String(retention.policy ?? 'latest_windows') },
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
export async function clearPurityGroupHistory(id: string): Promise<void> {
  const response = await api.delete(`${ROOT}/${id}/history`, config)
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
function normalizeRunTask(value: unknown): PurityRunTask {
  const item = record(value)
  const rawStatus = String(item.status ?? 'pending')
  const status = ['pending', 'running', 'succeeded', 'failed'].includes(rawStatus) ? rawStatus as PurityRunTask['status'] : 'pending'
  return { task_id: String(item.task_id ?? ''), status, error: item.error == null || item.error === '' ? undefined : String(item.error) }
}
export async function runPurityGroup(id: string): Promise<PurityRunTask> {
  const response = await api.post(`${ROOT}/${id}/run`, undefined, config)
  return normalizeRunTask(unwrap(response.data))
}
export async function getPurityRunTask(groupId: string, taskId: string): Promise<PurityRunTask> {
  const response = await api.get(`${ROOT}/${groupId}/run/${encodeURIComponent(taskId)}`, config)
  return normalizeRunTask(unwrap(response.data))
}
export async function waitForPurityRun(groupId: string, task: PurityRunTask, onStatus?: (task: PurityRunTask) => void): Promise<PurityRunTask> {
  let current = task
  const deadline = Date.now() + 10 * 60_000
  while (current.status === 'pending' || current.status === 'running') {
    if (Date.now() >= deadline) throw new Error('Manual detection timed out while waiting for completion')
    onStatus?.(current)
    await new Promise((resolve) => setTimeout(resolve, 1500))
    current = await getPurityRunTask(groupId, current.task_id)
  }
  onStatus?.(current)
  if (current.status === 'failed') throw new Error(current.error || 'Manual detection failed')
  return current
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
  const detail = structureDetail(latest.structure_similarity_detail)
  const pairRun = record(latest.pair_run)
  const run = Object.keys(pairRun).length ? pairRun : latest
  const evidenceItems = array(latest.evidence ?? latest.anomaly_evidence).map(evidence)
  return {
    ...result,
    status: status(latest.state ?? latest.status ?? run.state ?? result.status),
    confidence: optionalNumber(latest.confidence) ?? optionalNumber(run.confidence) ?? result.confidence,
    samples: number(run.paired_sample_count ?? detail?.paired_sample_count ?? result.samples),
    field_similarity: {
      ...result.field_similarity,
      value: detail?.score_available === false ? undefined : optionalNumber(latest.structure_similarity ?? run.structure_similarity) ?? result.field_similarity.value,
      sample_size: number(run.paired_sample_count ?? detail?.paired_sample_count ?? result.field_similarity.sample_size),
      detail,
    },
    token_similarity: { ...result.token_similarity, value: optionalNumber(run.token_similarity) ?? result.token_similarity.value },
    baseline_token_range: run.baseline_token_min === undefined ? result.baseline_token_range : { min: number(run.baseline_token_min), max: number(run.baseline_token_max) },
    target_token_range: run.target_token_min === undefined ? result.target_token_range : { min: number(run.target_token_min), max: number(run.target_token_max) },
    deviation_rate: optionalNumber(run.token_deviation_rate) ?? result.deviation_rate,
    evidence: evidenceItems.length ? evidenceItems : result.evidence,
    incidents: array(latest.incidents ?? latest.alert_records).length ? array(latest.incidents ?? latest.alert_records).map(incident) : result.incidents,
    explanation: Object.keys(record(latest.explanation)).length ? (() => { const value = record(latest.explanation); return {
      code: String(value.code ?? latest.state ?? ''), summary: String(value.summary ?? ''), suggested_action: String(value.suggested_action ?? ''),
      combined_similarity: optionalNumber(value.combined_similarity), suspect_threshold: optionalNumber(value.suspect_threshold), alert_threshold: optionalNumber(value.alert_threshold),
      consecutive_anomalies: optionalNumber(value.consecutive_anomalies), consecutive_healthy: optionalNumber(value.consecutive_healthy),
      baseline_available: value.baseline_available == null ? undefined : Boolean(value.baseline_available),
    } })() : result.explanation,
    error_class: run.error_class == null ? undefined : String(run.error_class),
    pair_run: Object.keys(run).some((key) => ['id', 'latest_pair_run_id', 'window_started_at', 'window_ended_at', 'paired_sample_count', 'created_at'].includes(key)) || Boolean(detail) ? {
      id: optionalNumber(run.id ?? latest.latest_pair_run_id),
      baseline_sample_count: optionalNumber(run.baseline_sample_count), target_sample_count: optionalNumber(run.target_sample_count),
      paired_sample_count: optionalNumber(run.paired_sample_count ?? detail?.paired_sample_count),
      baseline_invalid_count: optionalNumber(run.baseline_invalid_count), target_invalid_count: optionalNumber(run.target_invalid_count),
      unmatched_baseline_count: optionalNumber(run.unmatched_baseline_count), unmatched_target_count: optionalNumber(run.unmatched_target_count),
      window_started_at: (run.window_started_at ?? detail?.window_started_at) as string | number | undefined,
      window_ended_at: (run.window_ended_at ?? detail?.window_ended_at) as string | number | undefined,
      state: run.state === undefined && latest.state === undefined ? undefined : status(run.state ?? latest.state),
      error_class: run.error_class == null ? undefined : String(run.error_class), created_at: run.created_at as string | number | undefined,
    } : undefined,
    updated_at: (latest.updated_at ?? run.created_at ?? result.updated_at) as string | number | undefined,
    trend: historyItems.slice().reverse().map(historyPoint),
  }
}

export async function listPurityHistory(params: { page: number; page_size: number; group_id?: string; status?: string; query?: string }): Promise<PurityHistoryPage> {
  const response = await api.get('/api/channel/purity/history', { params, ...config })
  const body = record(unwrap(response.data))
  return {
    items: array(body.items).map((raw) => { const item = record(raw); return {
      id: number(item.id), group_id: String(item.group_id), group_name: String(item.group_name ?? `#${item.group_id}`),
      target_channel_id: number(item.target_channel_id), target_channel_name: String(item.target_channel_name ?? `#${item.target_channel_id}`),
      baseline_model: String(item.baseline_model ?? ''), target_model: String(item.target_model ?? ''), status: status(item.state ?? item.status),
      paired_sample_count: number(item.paired_sample_count), structure_similarity: optionalNumber(item.structure_similarity),
      token_similarity: optionalNumber(item.token_similarity), confidence: optionalNumber(item.confidence),
      window_ended_at: item.window_ended_at as string | number,
    } }),
    total: number(body.total), page: number(body.page, params.page), page_size: number(body.page_size, params.page_size),
  }
}
export async function getPurityHistoryPreview(groupId: string): Promise<PurityHistoryPreview> {
  const response = await api.get(`${ROOT}/${groupId}/history/preview`, config)
  const item = record(unwrap(response.data))
  return { samples: number(item.samples), pair_runs: number(item.pair_runs), assessments: number(item.assessments), alerts: number(item.alerts), audits: optionalNumber(item.audits) }
}
export async function updatePurityIncident(groupId: string, alertId: number, action: IncidentAction, input: { note?: string; silence_until?: string | number } = {}): Promise<PurityIncident> {
  const response = await api.post(`${ROOT}/${groupId}/alerts/${alertId}/actions`, { action, ...input }, config)
  return incident(unwrap(response.data))
}

export async function runQuickProbe(input: QuickProbeInput): Promise<QuickProbeResult> {
  const response = await api.post('/api/channel/purity/quick-probe', input, config)
  const item = record(unwrap(response.data))
  return { ok: Boolean(item.ok ?? item.success), latency_ms: optionalNumber(item.latency_ms), message: String(item.message ?? ''), checked_at: item.checked_at as string | number | undefined }
}
export type { ApiEnvelope }
