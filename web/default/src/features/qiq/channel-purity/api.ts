/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or (at your option) any later version.
*/
import { api } from '@/lib/api'

import type {
  ApiEnvelope,
  PurityEvidence,
  PurityEvidenceKind,
  PurityFullScanResponse,
  PurityResult,
  PurityRisk,
  PurityRunStatus,
  PuritySettings,
  PurityStatus,
} from './types'

const PURITY_STATUSES = new Set<PurityStatus>([
  'pending',
  'running',
  'completed',
  'failed',
  'unknown',
])
const PURITY_RISKS = new Set<PurityRisk>(['low', 'medium', 'high', 'unknown'])
const EVIDENCE_KINDS = new Set<PurityEvidenceKind>([
  'protocol',
  'declared_model',
  'usage',
  'warning',
  'operational',
  'generic',
])

function unwrap<T>(payload: ApiEnvelope<T> | T): T {
  if (payload && typeof payload === 'object' && 'data' in payload) {
    return (payload as ApiEnvelope<T>).data as T
  }
  return payload as T
}

function toRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {}
}

function optionalText(value: unknown): string | undefined {
  return value === undefined || value === null || value === ''
    ? undefined
    : String(value)
}

function optionalIdentifier(value: unknown): string | number | undefined {
  if (typeof value === 'number' && Number.isFinite(value)) return value
  if (typeof value === 'string' && value.trim() !== '') return value
  return undefined
}

function normalizeStatus(value: unknown): PurityStatus {
  return PURITY_STATUSES.has(value as PurityStatus)
    ? (value as PurityStatus)
    : 'unknown'
}

function normalizeRisk(value: unknown): PurityRisk {
  return PURITY_RISKS.has(value as PurityRisk)
    ? (value as PurityRisk)
    : 'unknown'
}

function normalizeEvidenceArray(source: unknown[]): PurityEvidence[] {
  return source.map((item, index) => {
    const record = toRecord(item)
    const kind = EVIDENCE_KINDS.has(record.kind as PurityEvidenceKind)
      ? (record.kind as PurityEvidenceKind)
      : 'generic'
    return {
      id: String(
        record.id ??
          `${kind}-${index}-${String(record.title ?? record.description ?? '')}`
      ),
      kind,
      title: optionalText(record.title),
      description: optionalText(record.description),
      expected: optionalText(record.expected),
      actual: optionalText(record.actual),
    }
  })
}

function normalizeEvidence(
  raw: Record<string, unknown>,
  result: Record<string, unknown>
): PurityEvidence[] {
  const source = result.evidence ?? raw.evidence ?? raw.evidences
  if (Array.isArray(source)) return normalizeEvidenceArray(source)

  const evidence = toRecord(source)
  const items: PurityEvidence[] = []
  const httpStatus = Number(result.http_status ?? evidence.http_status ?? 0)
  const responseReceived = Number.isFinite(httpStatus) && httpStatus > 0
  const object = evidence.object
  const hasOutput = evidence.has_output
  const hasChoices = evidence.has_choices

  if (responseReceived) {
    items.push({
      id: 'protocol',
      kind: 'protocol',
      expected: 'A successful OpenAI-compatible response with output',
      actual: [
        `HTTP ${httpStatus}`,
        object === undefined ? null : `object=${String(object)}`,
        hasOutput === undefined ? null : `output=${String(hasOutput)}`,
        hasChoices === undefined ? null : `choices=${String(hasChoices)}`,
      ]
        .filter(Boolean)
        .join(', '),
    })
  }

  const declaredModel = result.declared_model ?? evidence.declared_model
  if (responseReceived && httpStatus >= 200 && httpStatus < 300) {
    items.push({
      id: 'declared-model',
      kind: 'declared_model',
      expected: String(evidence.mapped_model ?? raw.model ?? '-'),
      actual:
        declaredModel === undefined || declaredModel === ''
          ? 'Not returned'
          : String(declaredModel),
    })
  }

  const usage = toRecord(result.usage ?? evidence.usage)
  if (responseReceived && httpStatus >= 200 && httpStatus < 300) {
    items.push({
      id: 'usage',
      kind: 'usage',
      expected: 'Consistent non-negative token usage when provided',
      actual:
        evidence.has_usage === false
          ? 'Not returned'
          : [
              `prompt=${String(usage.prompt_tokens ?? 0)}`,
              `completion=${String(usage.completion_tokens ?? 0)}`,
              `total=${String(usage.total_tokens ?? 0)}`,
            ].join(', '),
    })
  }

  const warnings = evidence.warnings
  if (Array.isArray(warnings)) {
    warnings.forEach((warning, index) => {
      items.push({
        id: `warning-${index}-${String(warning)}`,
        kind: 'warning',
        description: String(warning),
      })
    })
  }

  const errorClass = raw.error_class ?? result.error_class
  if (errorClass) {
    items.push({
      id: 'operational-status',
      kind: 'operational',
      description: String(errorClass),
    })
  }
  return items
}

function normalizeResult(raw: Record<string, unknown>): PurityResult {
  const channel = toRecord(raw.channel)
  const result = toRecord(raw.result)
  const channelID = Number(raw.channel_id ?? channel.id ?? 0)
  const model = String(raw.model ?? '-')
  const scanID = optionalIdentifier(raw.scan_id ?? raw.id)
  return {
    id: scanID ?? `${channelID}-${model}`,
    scan_id: scanID,
    channel_id: Number.isFinite(channelID) ? channelID : 0,
    channel_name: String(raw.channel_name ?? channel.name ?? '-'),
    model,
    risk: normalizeRisk(raw.risk ?? raw.risk_level),
    coverage: Number(raw.coverage ?? raw.coverage_rate ?? 0),
    status: normalizeStatus(raw.status),
    summary: optionalText(raw.summary),
    error_class: optionalText(raw.error_class ?? result.error_class),
    created_at: (raw.created_at ?? raw.created_time) as
      | string
      | number
      | undefined,
    updated_at: (raw.updated_at ??
      raw.updated_time ??
      raw.completed_at ??
      raw.started_at) as string | number | undefined,
    evidence: normalizeEvidence(raw, result),
  }
}

function assertSuccess<T>(payload: ApiEnvelope<T>, fallback: string): void {
  if (payload.success === false) {
    throw new Error(payload.message || fallback)
  }
}

export async function getPurityResults(): Promise<PurityResult[]> {
  const response = await api.get('/api/channel/purity/results', {
    params: { p: 1, page_size: 100 },
    skipBusinessError: true,
    skipErrorHandler: true,
  })
  const envelope = response.data as ApiEnvelope<unknown>
  assertSuccess(envelope, 'Failed to load purity scan results')
  const payload = unwrap<unknown>(envelope)
  const records = Array.isArray(payload)
    ? payload
    : (((payload as Record<string, unknown>)?.items ??
        (payload as Record<string, unknown>)?.results ??
        []) as unknown[])
  return records.map((record) => normalizeResult(toRecord(record)))
}

export async function startPurityFullScan(): Promise<
  ApiEnvelope<PurityFullScanResponse>
> {
  const response = await api.post(
    '/api/channel/purity/inspections',
    {},
    { skipBusinessError: true, skipErrorHandler: true }
  )
  return response.data
}

export async function getPuritySettings(): Promise<PuritySettings> {
  const response = await api.get('/api/channel/purity/inspection/settings', {
    skipBusinessError: true,
    skipErrorHandler: true,
  })
  const envelope = response.data as ApiEnvelope<Record<string, unknown>>
  assertSuccess(envelope, 'Failed to load automatic inspection settings')
  const payload = toRecord(unwrap(envelope))
  return {
    enabled: Boolean(payload.enabled),
    interval_minutes: Math.max(1, Number(payload.interval_minutes ?? 1440)),
  }
}

export async function updatePuritySettings(
  settings: PuritySettings
): Promise<PuritySettings> {
  const response = await api.put(
    '/api/channel/purity/inspection/settings',
    settings,
    {
      skipBusinessError: true,
      skipErrorHandler: true,
    }
  )
  const envelope = response.data as ApiEnvelope<Record<string, unknown>>
  assertSuccess(envelope, 'Failed to update automatic inspection settings')
  const payload = toRecord(unwrap(envelope))
  return {
    enabled: Boolean(payload.enabled ?? settings.enabled),
    interval_minutes: Math.max(
      1,
      Number(payload.interval_minutes ?? settings.interval_minutes)
    ),
  }
}

export async function getPurityRunStatus(): Promise<PurityRunStatus> {
  const response = await api.get('/api/channel/purity/inspection/status', {
    skipBusinessError: true,
    skipErrorHandler: true,
  })
  const envelope = response.data as ApiEnvelope<Record<string, unknown>>
  assertSuccess(envelope, 'Failed to load automatic inspection status')
  const payload = toRecord(unwrap(envelope))
  const task = toRecord(payload.task)
  const state = toRecord(task.state)
  const result = toRecord(task.result)
  const rawStatus = String(
    task.status ?? (payload.running ? 'running' : 'unknown')
  )
  let status = normalizeStatus(rawStatus)
  if (rawStatus === 'succeeded') status = 'completed'
  if (rawStatus === 'failed') status = 'failed'
  const total = Math.max(
    0,
    Number(result.total ?? state.total ?? payload.model_combinations ?? 0)
  )
  const completed = Math.max(
    0,
    Number(
      result.completed ??
        state.processed ??
        (status === 'completed' ? total : 0)
    )
  )
  return {
    status,
    run_id: optionalIdentifier(task.task_id ?? task.id),
    enabled_channels: Math.max(0, Number(payload.enabled_channels ?? 0)),
    model_combinations: total,
    completed,
    failed: Math.max(0, Number(result.failed ?? 0)),
    last_run_at: payload.last_run_at as string | number | undefined,
    next_run_at: payload.next_run_at as string | number | undefined,
    started_at: task.created_at as string | number | undefined,
    finished_at:
      status === 'completed' || status === 'failed'
        ? (task.updated_at as string | number | undefined)
        : undefined,
    error: optionalText(task.error ?? payload.error),
  }
}

export async function getPurityScan(id: string): Promise<PurityResult> {
  const response = await api.get(`/api/channel/purity/scans/${id}`, {
    skipBusinessError: true,
    skipErrorHandler: true,
  })
  const envelope = response.data as ApiEnvelope<Record<string, unknown>>
  assertSuccess(envelope, 'Failed to load purity scan')
  return normalizeResult(unwrap(envelope))
}
