/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or (at your option) any later version.
*/
import { useMutation, useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import type { TFunction } from 'i18next'
import {
  Activity,
  AlertTriangle,
  Ban,
  BellOff,
  CheckCheck,
  CheckCircle2,
  Clock3,
  MessageSquare,
  Pencil,
  Play,
  Plus,
  RefreshCw,
  Settings2,
  ShieldAlert,
  Trash2,
} from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import {
  Progress,
  ProgressLabel,
} from '@/components/ui/progress'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

import {
  clearPurityGroupHistory,
  createPurityGroup,
  deletePurityGroup,
  getPurityGroup,
  getPurityResultDetail,
  listChannelOptions,
  listPurityGroups,
  runPurityGroup,
  updatePurityIncident,
  waitForPurityRun,
  updatePurityGroup,
} from './api'
import { GroupForm } from './group-form'
import { HistoryManagementDialog } from './history-management-dialog'
import { QuickProbe } from './quick-probe'
import { StructureDifferencePanel } from './structure-differences'
import { tokenMetricAvailability } from './token-detail-state'
import { PurityTrendChart } from './trend-chart'
import type { DetectorStatus, PurityGroup, PurityGroupInput, TargetResult, TokenRange, TokenSimilarityDetail } from './types'

const groupsKey = ['qiq', 'channel-purity', 'groups'] as const

type ResultEntry = { group: PurityGroup; result: TargetResult }
type ResultFilter = 'all' | 'issue' | 'unavailable' | 'collecting' | 'healthy'

const issueStatuses = new Set<DetectorStatus>(['ALERT', 'SUSPECT'])
const unavailableStatuses = new Set<DetectorStatus>(['DETECTOR_ERROR', 'BASELINE_UNAVAILABLE'])
const collectingStatuses = new Set<DetectorStatus>(['LOW_SAMPLE', 'NO_TRAFFIC', 'WARMING_UP'])

const statusStyle: Record<DetectorStatus, string> = {
  BASELINE_UNAVAILABLE: 'border-orange-500/50 bg-orange-500/10 text-orange-700 dark:text-orange-300',
  LOW_SAMPLE: 'border-amber-500/50 bg-amber-500/10 text-amber-700 dark:text-amber-300',
  NO_TRAFFIC: 'border-slate-500/40 bg-slate-500/10 text-slate-700 dark:text-slate-300',
  WARMING_UP: 'border-blue-500/40 bg-blue-500/10 text-blue-700 dark:text-blue-300',
  HEALTHY: 'border-emerald-500/40 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300',
  SUSPECT: 'border-amber-500/50 bg-amber-500/10 text-amber-700 dark:text-amber-300',
  ALERT: 'border-destructive/50 bg-destructive/10 text-destructive',
  DETECTOR_ERROR: 'border-destructive/50 bg-destructive/10 text-destructive',
}

function StatusBadge({ status }: { status: DetectorStatus }) {
  const { t } = useTranslation()
  return <Badge variant='outline' className={statusStyle[status]}>{t(`Purity detector status: ${status}`)}</Badge>
}

function evidenceKindLabel(t: TFunction, kind: string) {
  switch (kind) {
    case 'structure_distribution_shift': return t('Structure distribution changed')
    case 'token_interval_shift': return t('Token interval changed')
    case 'missing_comparable_samples': return t('Comparable samples missing')
    case 'protocol_mismatch': return t('Response protocol changed')
    default: return kind
  }
}

function formatTime(value?: string | number) {
  if (value === undefined || value === '') return '—'
  const normalized = typeof value === 'number' && value < 1e12 ? value * 1000 : value
  const date = new Date(normalized)
  return Number.isNaN(date.getTime()) ? '—' : date.toLocaleString()
}

function percent(value?: number) {
  if (value === undefined || !Number.isFinite(value)) return '—'
  return `${Math.round(Math.max(0, Math.min(1, value)) * 100)}%`
}

function pairingUtilization(baseline?: number, target?: number, paired?: number) {
  const available = Math.min(baseline ?? 0, target ?? 0)
  if (available <= 0 || paired === undefined) return undefined
  return Math.min(1, paired / available)
}

function tokenRange(value?: TokenRange) {
  if (!value) return '—'
  return `${value.min.toLocaleString()} – ${value.max.toLocaleString()}`
}

function TokenWindowDetail({ detail }: { detail?: TokenSimilarityDetail }) {
  const { t } = useTranslation()
  if (!detail) return <p className='text-muted-foreground text-sm'>{t('Token detail is unavailable for this historical result.')}</p>
  const availability = tokenMetricAvailability(detail)
  const value = (number: number) => Number.isFinite(number) ? number.toFixed(3) : '—'
  return <section className='space-y-3 rounded-lg border p-3 text-sm'><div><h3 className='font-medium'>{t('Complete token comparison for this window')}</h3><p className='text-muted-foreground text-xs'>{t('Only anonymous token counts and ratios are retained; response content and identifiers are never included.')}</p>{!availability.available ? <p className='mt-1 text-xs text-amber-700'>{t('No valid paired token data is available, so no token score is produced.')}</p> : availability.partial ? <p className='mt-1 text-xs text-amber-700'>{t('Token statistics cover only the valid paired samples shown below.')}</p> : null}</div><dl className='grid gap-2 sm:grid-cols-3'><div><dt className='text-muted-foreground text-xs'>{t('Valid samples (baseline / target / paired)')}</dt><dd>{detail.baseline_valid_samples} / {detail.target_valid_samples} / {detail.paired_count}</dd></div><div><dt className='text-muted-foreground text-xs'>{t('Baseline min / p50 / p95 / max')}</dt><dd>{detail.baseline_min} / {value(detail.baseline_p50)} / {value(detail.baseline_p95)} / {detail.baseline_max}</dd></div><div><dt className='text-muted-foreground text-xs'>{t('Target min / p50 / p95 / max')}</dt><dd>{detail.target_min} / {value(detail.target_p50)} / {value(detail.target_p95)} / {detail.target_max}</dd></div><div><dt className='text-muted-foreground text-xs'>{t('Ratio median / Q1 / Q3')}</dt><dd>{value(detail.ratio_median)} / {value(detail.q1)} / {value(detail.q3)}</dd></div><div><dt className='text-muted-foreground text-xs'>{t('MAD / robust lower / upper')}</dt><dd>{value(detail.mad)} / {value(detail.robust_lower)} / {value(detail.robust_upper)}</dd></div><div><dt className='text-muted-foreground text-xs'>{t('Outside interval / deviation rate')}</dt><dd>{availability.outside} / {detail.paired_count} · {percent(detail.deviation_rate)}</dd></div></dl>{availability.available ? <div><h4 className='font-medium'>{t('All valid anonymous pairs in this window')}</h4><p className='text-muted-foreground mt-1 text-xs'>{t('Each row is one anonymous paired sample from the complete window, not a single response body.')}</p><div className='mt-2 max-h-72 overflow-auto rounded-md border'><table className='w-full text-left text-xs'><thead className='bg-muted sticky top-0'><tr><th className='p-2'>#</th><th className='p-2'>{t('Baseline tokens')}</th><th className='p-2'>{t('Target tokens')}</th><th className='p-2'>{t('Ratio')}</th><th className='p-2'>{t('Robust interval')}</th></tr></thead><tbody>{detail.pairs.map((pair, index) => <tr key={index} className='border-t'><td className='p-2'>{index + 1}</td><td className='p-2 font-mono'>{pair.baseline_tokens}</td><td className='p-2 font-mono'>{pair.target_tokens}</td><td className='p-2 font-mono'>{value(pair.ratio)}</td><td className='p-2'><Badge variant='outline' className={pair.outside ? 'border-destructive/40 text-destructive' : 'border-emerald-500/40 text-emerald-700 dark:text-emerald-300'}>{pair.outside ? t('Outside') : t('Inside')}</Badge></td></tr>)}</tbody></table></div></div> : <p className='rounded bg-amber-500/10 p-2 text-xs text-amber-800 dark:text-amber-200'>{t('No valid paired token data is available for this window.')}</p>}</section>
}

function errorMessage(error: unknown) {
  if (error && typeof error === 'object' && 'response' in error) {
    const response = (error as { response?: { data?: { message?: unknown } } }).response
    if (typeof response?.data?.message === 'string') return response.data.message
  }
  return error instanceof Error ? error.message : undefined
}

function resultCategory(status: DetectorStatus): Exclude<ResultFilter, 'all'> {
  if (issueStatuses.has(status)) return 'issue'
  if (unavailableStatuses.has(status)) return 'unavailable'
  if (collectingStatuses.has(status)) return 'collecting'
  return 'healthy'
}

function resultPriority(status: DetectorStatus) {
  const priorities: Record<DetectorStatus, number> = {
    ALERT: 0,
    DETECTOR_ERROR: 1,
    BASELINE_UNAVAILABLE: 2,
    SUSPECT: 3,
    LOW_SAMPLE: 4,
    NO_TRAFFIC: 5,
    WARMING_UP: 6,
    HEALTHY: 7,
  }
  return priorities[status]
}

function resultSummary(t: TFunction, result: TargetResult, minimumSamples: number) {
  switch (result.status) {
    case 'BASELINE_UNAVAILABLE':
      return t('The baseline does not currently have comparable samples. Check baseline traffic and availability.')
    case 'LOW_SAMPLE':
      return t('{{count}} more paired samples are needed before a conclusion is made.', {
        count: Math.max(0, minimumSamples - result.samples),
      })
    case 'NO_TRAFFIC':
      return t('No comparable requests were collected in the current window.')
    case 'WARMING_UP':
      return t('The detection window is still forming. No conclusion is made yet.')
    case 'HEALTHY':
      return t('No significant difference was found in the current window.')
    case 'SUSPECT':
      return t('Persistent differences were found. Review the supporting evidence.')
    case 'ALERT':
      return t('A significant difference was detected. Prioritize this comparison for review.')
    case 'DETECTOR_ERROR':
      return t('This comparison failed to run. Review the error and retry.')
  }
}

function SampleCoverage({ samples, minimumSamples, compact = false }: { samples: number; minimumSamples: number; compact?: boolean }) {
  const { t } = useTranslation()
  const target = Math.max(1, minimumSamples)
  const value = Math.min(100, (samples / target) * 100)
  return <Progress value={value} className={compact ? 'gap-1.5' : undefined}>
    <ProgressLabel className={compact ? 'text-xs' : undefined}>{t('Sample coverage')}</ProgressLabel>
    <span className={`text-muted-foreground ml-auto tabular-nums ${compact ? 'text-xs' : 'text-sm'}`}>{samples} / {minimumSamples}</span>
  </Progress>
}

function SummaryCard({ icon, label, value, note, tone = 'default' }: { icon: React.ReactNode; label: string; value: number; note: string; tone?: 'default' | 'attention' | 'unavailable' | 'collecting' | 'healthy' }) {
  const toneClass = tone === 'attention'
    ? 'border-destructive/35 bg-destructive/5'
    : tone === 'unavailable'
      ? 'border-orange-500/35 bg-orange-500/5'
      : tone === 'collecting'
        ? 'border-amber-500/35 bg-amber-500/5'
        : tone === 'healthy'
          ? 'border-emerald-500/35 bg-emerald-500/5'
          : ''
  return <Card className={`gap-2 py-4 ${toneClass}`}><CardContent className='flex items-start gap-3 px-4'>
    <div className='bg-background mt-0.5 rounded-md border p-2'>{icon}</div>
    <div><p className='text-muted-foreground text-xs'>{label}</p><p className='text-2xl font-semibold tabular-nums'>{value}</p><p className='text-muted-foreground text-xs'>{note}</p></div>
  </CardContent></Card>
}

function ComparisonResult({ entry, onOpen }: { entry: ResultEntry; onOpen: () => void }) {
  const { t } = useTranslation()
  const { group, result } = entry
  const category = resultCategory(result.status)
  const canJudge = category !== 'collecting' && result.status !== 'BASELINE_UNAVAILABLE' && result.status !== 'DETECTOR_ERROR'
  return <div className='grid gap-3 rounded-lg border bg-background p-3 lg:grid-cols-[minmax(210px,1.25fr)_minmax(220px,1.35fr)_minmax(180px,1fr)_minmax(220px,1fr)_auto] lg:items-center'>
    <div className='min-w-0'>
      <p className='text-muted-foreground text-xs'>{t('Model comparison')}</p>
      <p className='truncate font-mono text-sm font-medium' title={`${result.baseline_model} → ${result.target_model}`}>{result.baseline_model} → {result.target_model}</p>
      <p className='text-muted-foreground mt-1 text-[11px]'>{t('Last detected {{time}}', { time: formatTime(result.updated_at) })}</p>
    </div>
    <div className='min-w-0'>
      <div className='flex flex-wrap items-center gap-2'><StatusBadge status={result.status} />{result.alerts.length ? <Badge variant='destructive'>{t('{{count}} alerts', { count: result.alerts.length })}</Badge> : null}</div>
      <p className='text-muted-foreground mt-1 text-xs leading-relaxed'>{resultSummary(t, result, group.sampling.minimum_samples)}</p>
      {result.latest_evidence ? <p className='mt-1 truncate text-xs' title={t(result.latest_evidence.summary)}><span className='font-medium'>{t('Latest evidence')}:</span> {t(result.latest_evidence.summary)}</p> : null}
    </div>
    <SampleCoverage samples={result.samples} minimumSamples={group.sampling.minimum_samples} compact />
    <div>
      {canJudge ? <div className='grid grid-cols-3 gap-2 text-sm'>
        <div><p className='text-muted-foreground text-[11px]'>{t('Structure')}</p><p className='font-medium tabular-nums'>{percent(result.field_similarity.value)}</p></div>
        <div><p className='text-muted-foreground text-[11px]'>{t('Token')}</p><p className='font-medium tabular-nums'>{percent(result.token_similarity.value)}</p></div>
        <div><p className='text-muted-foreground text-[11px]'>{t('Confidence')}</p><p className='font-medium tabular-nums'>{percent(result.confidence)}</p></div>
      </div> : category === 'collecting' ? <div><p className='font-medium'>{t('Not evaluated yet')}</p><p className='text-muted-foreground text-xs'>{t('Observed values: structure {{structure}} · token {{token}}', { structure: percent(result.field_similarity.value), token: percent(result.token_similarity.value) })}</p></div> : <div><p className='font-medium'>{t('Result unavailable')}</p><p className='text-muted-foreground text-xs'>{t('Scores are hidden because this run cannot produce a valid conclusion.')}</p></div>}
    </div>
    <Button size='sm' variant='outline' onClick={onOpen}>{t('Details')}</Button>
  </div>
}

function TargetResultGroup({ entries, onOpen }: { entries: ResultEntry[]; onOpen: (result: TargetResult) => void }) {
  const { t } = useTranslation()
  const first = entries[0]
  const attentionCount = entries.filter(({ result }) => ['issue', 'unavailable'].includes(resultCategory(result.status))).length
  const collectingCount = entries.filter(({ result }) => resultCategory(result.status) === 'collecting').length
  const worst = [...entries].sort((a, b) => resultPriority(a.result.status) - resultPriority(b.result.status))[0].result.status
  return <Card className='gap-0 overflow-hidden py-0'>
    <CardHeader className='bg-muted/30 border-b py-4'>
      <div className='flex flex-wrap items-start justify-between gap-3'>
        <div><CardTitle className='text-base'>{first.result.target_channel_name}</CardTitle><p className='text-muted-foreground mt-1 text-xs'>{t('Baseline')}: {first.result.baseline_channel_name} · {t('Detection group')}: {first.group.name} · {t('{{count}} model comparisons', { count: entries.length })}</p></div>
        <div className='flex flex-wrap items-center gap-2'>{attentionCount ? <Badge variant='destructive'>{t('{{count}} need attention', { count: attentionCount })}</Badge> : null}{collectingCount ? <Badge variant='outline' className='border-amber-500/40 text-amber-700 dark:text-amber-300'>{t('{{count}} collecting data', { count: collectingCount })}</Badge> : null}<StatusBadge status={worst} /></div>
      </div>
    </CardHeader>
    <CardContent className='space-y-2 p-3'>{entries.map((entry) => <ComparisonResult key={entry.result.id} entry={entry} onOpen={() => onOpen(entry.result)} />)}</CardContent>
  </Card>
}

function ScorePanel({ label, value, note }: { label: string; value: string; note?: string }) {
  return <Card className='gap-2 py-4'><CardContent className='px-4'><p className='text-muted-foreground text-xs'>{label}</p><p className='mt-1 text-2xl font-semibold tabular-nums'>{value}</p>{note ? <p className='text-muted-foreground mt-1 text-xs'>{note}</p> : null}</CardContent></Card>
}

function ResultDetail({ result, minimumSamples, loading, error, running, runStatus, onRun, onRetry, onClose }: { result: TargetResult | null; minimumSamples: number; loading: boolean; error?: string; running: boolean; runStatus?: string; onRun: () => void; onRetry: () => void; onClose: () => void }) {
  const { t } = useTranslation()
  const [incidentNote, setIncidentNote] = useState('')
  const category = result ? resultCategory(result.status) : 'collecting'
  const observingOnly = category === 'collecting'
  const incidentMutation = useMutation({
    mutationFn: ({ alertId, action, note, silenceUntil }: { alertId: number; action: 'acknowledge' | 'silence' | 'note' | 'false_positive' | 'resolve'; note?: string; silenceUntil?: number }) => updatePurityIncident(result!.group_id, alertId, action, { note, silence_until: silenceUntil }),
    onSuccess: async () => { setIncidentNote(''); toast.success(t('Incident updated')); await onRetry() },
    onError: (actionError) => toast.error(errorMessage(actionError) || t('Failed to update incident')),
  })
  return <Sheet open={Boolean(result) || loading || Boolean(error)} onOpenChange={(open) => { if (!open) onClose() }}><SheetContent className='w-full sm:max-w-3xl'>{result ? <div className='flex min-h-0 flex-1 flex-col'>
    <SheetHeader className='shrink-0 border-b pr-14'><SheetTitle className='break-all'>{result.target_channel_name} · {result.baseline_model} → {result.target_model}</SheetTitle><SheetDescription>{t('Independent comparison against baseline {{baseline}}', { baseline: result.baseline_channel_name })}</SheetDescription></SheetHeader>
    <div className='min-h-0 flex-1 space-y-4 overflow-y-auto p-4'>
    <div className='flex flex-wrap items-center gap-2'><Button size='sm' variant='outline' disabled={running} onClick={onRun}><Play />{running ? t('Starting manual detection…') : t('Run manual detection')}</Button><Button size='sm' variant='outline' render={<Link to='/channels' search={{ filter: result.target_channel_name, model: result.target_model }} />}><Settings2 />{t('Open target channel settings')}</Button>{runStatus ? <span role='status' aria-live='polite' className='text-muted-foreground text-sm'>{runStatus}</span> : null}</div>
    <div className={`rounded-lg border p-4 ${category === 'issue' ? 'border-destructive/35 bg-destructive/5' : category === 'unavailable' ? 'border-orange-500/35 bg-orange-500/5' : category === 'collecting' ? 'border-amber-500/35 bg-amber-500/5' : 'border-emerald-500/35 bg-emerald-500/5'}`}>
      <div className='flex flex-wrap items-start justify-between gap-3'><div><div className='flex flex-wrap items-center gap-2'><StatusBadge status={result.status} />{result.alerts.length ? <Badge variant='destructive'>{t('{{count}} alerts', { count: result.alerts.length })}</Badge> : null}</div><p className='mt-2 font-medium'>{resultSummary(t, result, minimumSamples)}</p></div><div className='min-w-48'><SampleCoverage samples={result.samples} minimumSamples={minimumSamples} compact /></div></div>
      {observingOnly ? <p className='text-muted-foreground mt-3 text-xs'>{t('Similarity values are observations only until the minimum sample count is reached; they are not a health conclusion.')}</p> : null}
    </div>
    <div className='grid gap-3 sm:grid-cols-3'>
      <ScorePanel label={t('Structure similarity')} value={observingOnly ? t('Insufficient evidence') : percent(result.field_similarity.value)} note={observingOnly ? t('Current observation: {{value}} from {{paired}} / {{required}} paired samples', { value: percent(result.field_similarity.value), paired: result.samples, required: minimumSamples }) : t('Compared with the designated baseline')} />
      <ScorePanel label={t('Token similarity')} value={observingOnly ? t('Insufficient evidence') : percent(result.token_similarity.value)} note={observingOnly ? t('Current observation: {{value}} from {{paired}} / {{required}} paired samples', { value: percent(result.token_similarity.value), paired: result.samples, required: minimumSamples }) : t('Compared with the designated baseline')} />
      <ScorePanel label={t('Confidence')} value={observingOnly ? t('Pending') : percent(result.confidence)} note={observingOnly ? t('Available after enough paired samples') : t('Confidence in the current conclusion')} />
    </div>
    <Tabs defaultValue='overview'>
      <TabsList className='grid h-auto w-full grid-cols-2 sm:grid-cols-4'>
        <TabsTrigger value='overview'>{t('Overview')}</TabsTrigger>
        <TabsTrigger value='evidence'>{t('Evidence')}</TabsTrigger>
        <TabsTrigger value='history'>{t('History')}</TabsTrigger>
        <TabsTrigger value='technical'>{t('Technical details')}</TabsTrigger>
      </TabsList>
      <TabsContent value='overview' className='space-y-3 pt-2'>
        {result.explanation ? <div className='rounded-lg border p-3 text-sm'><h3 className='font-medium'>{t('Why this status')}</h3><p className='mt-1'>{t(result.explanation.summary || resultSummary(t, result, minimumSamples))}</p>{result.explanation.suggested_action ? <p className='text-muted-foreground mt-1'>{t('Suggested action')}: {t(result.explanation.suggested_action)}</p> : null}<dl className='mt-3 grid gap-2 sm:grid-cols-4'><div><dt className='text-muted-foreground text-xs'>{t('Combined similarity')}</dt><dd>{percent(result.explanation.combined_similarity)}</dd></div><div><dt className='text-muted-foreground text-xs'>{t('Suspect / alert thresholds')}</dt><dd>{percent(result.explanation.suspect_threshold)} / {percent(result.explanation.alert_threshold)}</dd></div><div><dt className='text-muted-foreground text-xs'>{t('Consecutive anomalous windows')}</dt><dd>{result.explanation.consecutive_anomalies ?? '—'}</dd></div><div><dt className='text-muted-foreground text-xs'>{t('Consecutive healthy windows')}</dt><dd>{result.explanation.consecutive_healthy ?? '—'}</dd></div></dl></div> : null}
        <StructureDifferencePanel detail={result.field_similarity.detail} score={result.field_similarity.value} pairedSamples={result.samples} minimumSamples={minimumSamples} />
        <TokenWindowDetail detail={result.token_detail} />
        {result.pair_run ? <div className='rounded-lg border p-3 text-sm'><h3 className='font-medium'>{t('Latest pair run')}</h3><dl className='mt-2 grid gap-x-4 gap-y-2 sm:grid-cols-2'><div><dt className='text-muted-foreground'>{t('Pair run ID')}</dt><dd>{result.pair_run.id ?? '—'}</dd></div><div><dt className='text-muted-foreground'>{t('Status / error')}</dt><dd>{result.pair_run.state ? t(`Purity detector status: ${result.pair_run.state}`) : t(`Purity detector status: ${result.status}`)}{result.pair_run.error_class ? ` · ${result.pair_run.error_class}` : ''}</dd></div><div><dt className='text-muted-foreground'>{t('Samples (baseline / target / paired)')}</dt><dd>{result.pair_run.baseline_sample_count ?? '—'} / {result.pair_run.target_sample_count ?? '—'} / {result.pair_run.paired_sample_count ?? '—'}</dd><p className='text-muted-foreground mt-1 text-xs'>{t('Threshold coverage {{coverage}} · pairing utilization {{utilization}}', { coverage: percent((result.pair_run.paired_sample_count ?? result.samples) / Math.max(1, minimumSamples)), utilization: percent(pairingUtilization(result.pair_run.baseline_sample_count, result.pair_run.target_sample_count, result.pair_run.paired_sample_count)) })}</p>{result.pair_run.baseline_invalid_count !== undefined || result.pair_run.target_invalid_count !== undefined ? <p className='text-muted-foreground mt-1 text-xs'>{t('Invalid baseline / target: {{baseline}} / {{target}} · unmatched baseline / target: {{unmatchedBaseline}} / {{unmatchedTarget}}', { baseline: result.pair_run.baseline_invalid_count ?? 0, target: result.pair_run.target_invalid_count ?? 0, unmatchedBaseline: result.pair_run.unmatched_baseline_count ?? 0, unmatchedTarget: result.pair_run.unmatched_target_count ?? 0 })}</p> : null}</div><div><dt className='text-muted-foreground'>{t('Time window')}</dt><dd>{formatTime(result.pair_run.window_started_at)} – {formatTime(result.pair_run.window_ended_at)}</dd></div><div><dt className='text-muted-foreground'>{t('Recorded at')}</dt><dd>{formatTime(result.pair_run.created_at ?? result.updated_at)}</dd></div></dl></div> : <p className='text-muted-foreground text-sm'>{t('Pair-run metadata is unavailable for this historical result.')}</p>}
        <div className='grid gap-3 sm:grid-cols-3'><Card><CardContent className='pt-4'><p className='text-muted-foreground text-xs'>{t('Baseline token interval')}</p><p className='mt-1 font-mono'>{tokenRange(result.baseline_token_range)}</p></CardContent></Card><Card><CardContent className='pt-4'><p className='text-muted-foreground text-xs'>{t('Target token interval')}</p><p className='mt-1 font-mono'>{tokenRange(result.target_token_range)}</p></CardContent></Card><Card><CardContent className='pt-4'><p className='text-muted-foreground text-xs'>{t('Deviation rate')}</p><p className='mt-1 font-medium'>{percent(result.deviation_rate)}</p></CardContent></Card></div>
      </TabsContent>
      <TabsContent value='evidence' className='space-y-3 pt-2'>
        {result.alerts.length ? <div className='border-destructive/40 bg-destructive/5 rounded-lg border p-3'><h3 className='text-destructive font-medium'>{t('Alerts')}</h3><ul className='mt-2 list-disc space-y-1 pl-5 text-sm'>{result.alerts.map((alert) => <li key={alert}>{t(alert)}</li>)}</ul></div> : null}
        {result.incidents.length ? <div className='space-y-2'><h3 className='font-medium'>{t('Incident handling')}</h3>{result.incidents.map((incident) => <div key={incident.id} className='rounded-lg border p-3'><div className='flex flex-wrap items-start justify-between gap-2'><div><Badge variant={incident.status === 'OPEN' ? 'destructive' : 'outline'}>{t(`Incident status: ${incident.status}`)}</Badge><p className='text-muted-foreground mt-1 text-xs'>{t('Opened at {{time}}', { time: formatTime(incident.opened_at) })}{incident.silence_until ? ` · ${t('Silenced until {{time}}', { time: formatTime(incident.silence_until) })}` : ''}</p></div><div className='flex flex-wrap gap-1'><Button size='sm' variant='outline' disabled={incidentMutation.isPending} onClick={() => incidentMutation.mutate({ alertId: incident.id, action: 'acknowledge' })}><CheckCheck />{t('Acknowledge')}</Button><Button size='sm' variant='outline' disabled={incidentMutation.isPending} onClick={() => incidentMutation.mutate({ alertId: incident.id, action: 'silence', silenceUntil: Math.floor(Date.now() / 1000) + 86400 })}><BellOff />{t('Silence 24h')}</Button><Button size='sm' variant='outline' disabled={incidentMutation.isPending} onClick={() => incidentMutation.mutate({ alertId: incident.id, action: 'false_positive', note: incidentNote.trim() || undefined })}><Ban />{t('Mark false positive')}</Button></div></div>{incident.note ? <p className='mt-2 text-sm'>{incident.note}</p> : null}<div className='mt-3 flex gap-2'><Input value={incidentNote} placeholder={t('Add an investigation note')} onChange={(event) => setIncidentNote(event.target.value)} /><Button size='sm' variant='outline' disabled={incidentMutation.isPending || !incidentNote.trim()} onClick={() => incidentMutation.mutate({ alertId: incident.id, action: 'note', note: incidentNote.trim() })}><MessageSquare />{t('Save note')}</Button></div>{incident.audit?.length ? <div className='mt-3 border-l pl-3'><p className='text-muted-foreground text-xs font-medium'>{t('Audit trail')}</p>{incident.audit.map((entry) => <p key={entry.id} className='mt-1 text-xs'>{formatTime(entry.created_at)} · {t(entry.action)}{entry.note ? ` · ${entry.note}` : ''}</p>)}</div> : null}</div>)}</div> : null}
        <div><h3 className='mb-2 font-medium'>{t('Original detector evidence')}</h3>{result.evidence.length ? <div className='space-y-2'>{result.evidence.map((item) => <div key={item.id} className='rounded-lg border p-3'><div className='flex justify-between gap-3'><Badge variant='outline'>{evidenceKindLabel(t, item.kind)}</Badge><span className='text-muted-foreground text-xs'>{formatTime(item.occurred_at)}</span></div><p className='mt-2 text-sm'>{t(item.summary)}</p>{item.baseline_value !== undefined || item.target_value !== undefined ? <div className='bg-muted mt-2 grid gap-2 rounded p-2 font-mono text-xs sm:grid-cols-2'><span>{t('Baseline')}: {item.baseline_value ?? '—'}</span><span>{t('Target')}: {item.target_value ?? '—'}</span></div> : null}</div>)}</div> : <p className='text-muted-foreground text-sm'>{t('No evidence has been recorded yet.')}</p>}</div>
      </TabsContent>
      <TabsContent value='history' className='pt-2'>
        <h3 className='mb-2 font-medium'>{t('Historical trend')}</h3>
        <PurityTrendChart points={result.trend} suspectThreshold={result.explanation?.suspect_threshold} alertThreshold={result.explanation?.alert_threshold} />
      </TabsContent>
      <TabsContent value='technical' className='space-y-3 pt-2'>
        <div className='rounded-lg border p-4'>
          <div className='flex flex-wrap items-end justify-between gap-2'><div><p className='text-muted-foreground text-xs'>{t('Field / structure similarity')}</p><p className='text-2xl font-semibold tabular-nums'>{percent(result.field_similarity.value)}</p></div><p className='text-muted-foreground text-xs'>{t('Scoring method')}: {t('multiset Jaccard similarity')}</p></div>
          {result.field_similarity.detail ? <div className='mt-4 space-y-3'>
            <div className='grid grid-cols-3 gap-2 text-center'><div className='rounded bg-emerald-500/10 p-2'><p className='text-xs text-emerald-700'>{t('Matched structure samples')}</p><p className='font-semibold'>{result.field_similarity.detail.matched_count}</p></div><div className='rounded bg-amber-500/10 p-2'><p className='text-xs text-amber-700'>{t('Baseline-only structure samples')}</p><p className='font-semibold'>{result.field_similarity.detail.baseline_only_count}</p></div><div className='rounded bg-blue-500/10 p-2'><p className='text-xs text-blue-700'>{t('Target-only structure samples')}</p><p className='font-semibold'>{result.field_similarity.detail.target_only_count}</p></div></div>
            {result.field_similarity.detail.score_available === false ? <p className='rounded bg-amber-500/10 p-2 text-sm text-amber-800 dark:text-amber-200'>{t('No comparable structure samples are available, so no structure score is produced.')}</p> : <p className='text-sm'>{t('Score basis')}: {result.field_similarity.detail.intersection_count} / {result.field_similarity.detail.union_count} = {percent(result.field_similarity.value)}</p>}
            <p className='text-muted-foreground text-xs'>{t('Scoring detail version')}: {result.field_similarity.detail.version} · {t('Window')}: {formatTime(result.field_similarity.detail.window_started_at)} - {formatTime(result.field_similarity.detail.window_ended_at)} · {t('{{count}} paired samples', { count: result.field_similarity.detail.paired_sample_count })}</p>
            {result.field_similarity.detail.differences.length ? <div><p className='mb-1 text-sm font-medium'>{t('Structure signature frequencies')}</p><div className='max-h-48 overflow-auto rounded border'><Table><TableHeader><TableRow><TableHead>{t('Anonymous structure signature')}</TableHead><TableHead>{t('Baseline')}</TableHead><TableHead>{t('Target')}</TableHead><TableHead>{t('Matched')}</TableHead></TableRow></TableHeader><TableBody>{result.field_similarity.detail.differences.map((difference) => <TableRow key={difference.signature}><TableCell className='max-w-56 truncate font-mono text-xs' title={difference.signature}>{difference.signature}</TableCell><TableCell>{difference.baseline_count}</TableCell><TableCell>{difference.target_count}</TableCell><TableCell>{difference.matched_count}</TableCell></TableRow>)}</TableBody></Table></div></div> : null}
            {result.field_similarity.detail.field_paths_available && result.field_similarity.detail.field_differences?.length ? <div><p className='mb-1 text-sm font-medium'>{t('Sanitized field and type differences')}</p><div className='max-h-48 overflow-auto rounded border'><Table><TableHeader><TableRow><TableHead>{t('Field path')}</TableHead><TableHead>{t('Baseline type')}</TableHead><TableHead>{t('Target type')}</TableHead><TableHead>{t('Baseline / target count')}</TableHead></TableRow></TableHeader><TableBody>{result.field_similarity.detail.field_differences.map((difference) => <TableRow key={`${difference.path}:${difference.baseline_types?.join(',') ?? difference.baseline_type}:${difference.target_types?.join(',') ?? difference.target_type}`}><TableCell className='font-mono text-xs'>{difference.path}</TableCell><TableCell>{difference.baseline_types?.join(' / ') || difference.baseline_type || '—'}</TableCell><TableCell>{difference.target_types?.join(' / ') || difference.target_type || '—'}</TableCell><TableCell>{difference.baseline_count} / {difference.target_count}</TableCell></TableRow>)}</TableBody></Table></div><p className='text-muted-foreground mt-1 text-xs'>{t('Only sanitized structural paths and data types are retained; response values are never stored here.')}</p></div> : null}
            {!result.field_similarity.detail.field_paths_available ? <p className='rounded bg-amber-500/10 p-2 text-xs text-amber-800'>{t('Field-level evidence gap: existing samples retain only anonymous structure hashes, so exact matched, missing, or added field names cannot be recovered. The counts above are real matched structure occurrences, not inferred field names.')}</p> : null}
          </div> : <p className='text-muted-foreground mt-3 text-sm'>{t('Detailed scoring inputs are unavailable for this historical result.')}</p>}
        </div>
      </TabsContent>
    </Tabs>
    </div>
  </div> : loading ? <div className='py-10 text-center'><RefreshCw className='mx-auto mb-2 size-5 animate-spin' /><p>{t('Loading latest assessment and history…')}</p></div> : <div role='alert' className='p-6'><p className='text-destructive'>{error || t('Failed to load result details')}</p><Button className='mt-3' variant='outline' onClick={onRetry}>{t('Try again')}</Button></div>}</SheetContent></Sheet>
}

export function ChannelPurity() {
  const { t } = useTranslation()
  const [editing, setEditing] = useState<PurityGroup | null | undefined>(undefined)
  const [formError, setFormError] = useState<string>()
  const [detailSeed, setDetailSeed] = useState<TargetResult | null>(null)
  const [runStatus, setRunStatus] = useState<string>()
  const [resultFilter, setResultFilter] = useState<ResultFilter>('all')
  const [deleteCandidate, setDeleteCandidate] = useState<PurityGroup | null>(null)
  const [recordCandidate, setRecordCandidate] = useState<PurityGroup | null>(null)
  const groupsQuery = useQuery({ queryKey: groupsKey, queryFn: listPurityGroups, refetchInterval: 30_000 })
  const channelsQuery = useQuery({ queryKey: ['qiq', 'channels', 'options'], queryFn: listChannelOptions })
  const detailQuery = useQuery({ queryKey: ['qiq', 'channel-purity', 'detail', detailSeed?.group_id, detailSeed?.target_channel_id, detailSeed?.model], queryFn: () => getPurityResultDetail(detailSeed!), enabled: Boolean(detailSeed), retry: false })
  const refresh = () => groupsQuery.refetch()
  const openDetail = (result: TargetResult) => { setRunStatus(undefined); setDetailSeed(result) }
  const closeDetail = () => { setDetailSeed(null); setRunStatus(undefined) }
  const saveMutation = useMutation({
    mutationFn: ({ group, input }: { group: PurityGroup | null; input: PurityGroupInput }) => group ? updatePurityGroup(group.id, input) : createPurityGroup(input),
    onMutate: () => setFormError(undefined),
    onSuccess: async () => { setEditing(undefined); setFormError(undefined); toast.success(t('Benchmark group saved')); await refresh() },
    onError: (error) => { const message = errorMessage(error) || t('Failed to save benchmark group'); setFormError(message); toast.error(message) },
  })
  const deleteMutation = useMutation({ mutationFn: deletePurityGroup, onSuccess: async () => { setDeleteCandidate(null); toast.success(t('Benchmark group deleted')); await refresh() }, onError: (error) => toast.error(errorMessage(error) || t('Failed to delete benchmark group')) })
  const runMutation = useMutation({ mutationFn: async (groupId: string) => {
    const task = await runPurityGroup(groupId)
    setRunStatus(t('Manual detection queued…'))
    return waitForPurityRun(groupId, task, (current) => setRunStatus(current.status === 'running' ? t('Manual detection is running…') : t('Manual detection queued…')))
  }, onMutate: () => setRunStatus(t('Submitting manual detection…')), onSuccess: async () => { setRunStatus(t('Manual detection succeeded; refreshing results…')); await refresh(); if (detailSeed) await detailQuery.refetch(); setRunStatus(t('Manual detection completed and results refreshed.')); toast.success(t('Manual detection completed')) }, onError: (error) => { const message = errorMessage(error) || t('Manual detection failed'); setRunStatus(message); toast.error(message) } })
  const clearHistoryMutation = useMutation({ mutationFn: clearPurityGroupHistory, onSuccess: async () => { toast.success(t('Detection history cleared')); setRecordCandidate(null); closeDetail(); await refresh() }, onError: (error) => toast.error(errorMessage(error) || t('Failed to clear detection history')) })
  const toggleMutation = useMutation({ mutationFn: async (group: PurityGroup) => updatePurityGroup(group.id, { name: group.name, enabled: !group.enabled, channel_ids: group.channel_ids, baseline_channel_id: group.baseline_channel_id, interval_minutes: group.interval_minutes, random_pairing_enabled: group.random_pairing_enabled, model_comparisons: group.model_comparisons.map((comparison) => ({ ...comparison })), sampling: { ...group.sampling }, policy: { ...group.policy }, retention: { ...group.retention } }), onSuccess: async () => { toast.success(t('Group status updated')); await refresh() }, onError: (error) => toast.error(errorMessage(error) || t('Failed to update group status')) })
  const editMutation = useMutation({ mutationFn: getPurityGroup, onSuccess: (group) => { setFormError(undefined); setEditing(group) }, onError: (error) => toast.error(errorMessage(error) || t('Failed to load group configuration')) })
  const groups = groupsQuery.data ?? []
  const results = useMemo<ResultEntry[]>(() => groups.flatMap((group) => group.results.map((result) => ({ group, result }))), [groups])
  const counts = useMemo(() => ({
    issue: results.filter(({ result }) => resultCategory(result.status) === 'issue').length,
    unavailable: results.filter(({ result }) => resultCategory(result.status) === 'unavailable').length,
    collecting: results.filter(({ result }) => resultCategory(result.status) === 'collecting').length,
    healthy: results.filter(({ result }) => resultCategory(result.status) === 'healthy').length,
  }), [results])
  const visibleResults = useMemo(() => results
    .filter(({ result }) => resultFilter === 'all' || resultCategory(result.status) === resultFilter)
    .sort((a, b) => resultPriority(a.result.status) - resultPriority(b.result.status)), [results, resultFilter])
  const groupedResults = useMemo(() => {
    const grouped = new Map<string, ResultEntry[]>()
    for (const entry of visibleResults) {
      const key = `${entry.group.id}:${entry.result.target_channel_id}`
      const current = grouped.get(key) ?? []
      current.push(entry)
      grouped.set(key, current)
    }
    return [...grouped.values()]
  }, [visibleResults])
  const detailResult = detailSeed && detailQuery.data ? detailQuery.data : null
  const detailGroup = detailSeed ? groups.find((group) => group.id === detailSeed.group_id) : undefined
  const latestUpdate = results.reduce<string | number | undefined>((latest, { result }) => {
    if (!result.updated_at) return latest
    if (!latest) return result.updated_at
    return new Date(result.updated_at).getTime() > new Date(latest).getTime() ? result.updated_at : latest
  }, undefined)
  return <SectionPageLayout>
    <SectionPageLayout.Title>{t('Channel purity')}</SectionPageLayout.Title>
    <SectionPageLayout.Content><div className='space-y-5'>
      <div className='flex flex-wrap items-start justify-between gap-3'><div><h2 className='text-lg font-semibold'>{t('Grouped baseline detection')}</h2><p className='text-muted-foreground max-w-3xl text-sm'>{t('Start with the conclusion, then inspect samples, evidence, and history. Every target and model comparison is evaluated independently.')}</p></div><div className='flex gap-2'><Button variant='outline' disabled={groupsQuery.isFetching} onClick={() => void refresh()}><RefreshCw className={groupsQuery.isFetching ? 'animate-spin' : ''} />{groupsQuery.isFetching ? t('Refreshing…') : t('Refresh')}</Button><Button disabled={editMutation.isPending} onClick={() => { setFormError(undefined); setEditing(null) }}><Plus />{t('Create group')}</Button></div></div>
      {groupsQuery.isError ? <div className='border-destructive/40 bg-destructive/5 rounded-lg border p-4'><p className='text-destructive font-medium'>{t('Failed to load benchmark groups')}</p><p className='text-muted-foreground text-sm'>{errorMessage(groupsQuery.error)}</p><Button className='mt-2' size='sm' variant='outline' onClick={() => void groupsQuery.refetch()}>{t('Try again')}</Button></div> : null}

      <div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-5'>
        <SummaryCard icon={<Activity className='size-4' />} label={t('Total comparisons')} value={results.length} note={t('Last updated {{time}}', { time: formatTime(latestUpdate) })} />
        <SummaryCard icon={<ShieldAlert className='text-destructive size-4' />} label={t('Confirmed anomalies')} value={counts.issue} note={t('Alerts and suspect comparisons that need review')} tone='attention' />
        <SummaryCard icon={<AlertTriangle className='size-4 text-orange-600' />} label={t('Detection unavailable')} value={counts.unavailable} note={t('Detector errors and unavailable baselines are not channel anomalies')} tone='unavailable' />
        <SummaryCard icon={<Clock3 className='size-4 text-amber-600' />} label={t('Collecting data')} value={counts.collecting} note={t('No conclusion until enough comparable samples exist')} tone='collecting' />
        <SummaryCard icon={<CheckCircle2 className='size-4 text-emerald-600' />} label={t('No anomaly found')} value={counts.healthy} note={t('Current windows are within configured thresholds')} tone='healthy' />
      </div>

      <Card>
        <CardHeader className='border-b'><div className='flex flex-wrap items-start justify-between gap-3'><div><CardTitle>{t('Detection results')}</CardTitle><p className='text-muted-foreground mt-1 text-sm'>{t('Results needing attention are shown first. Similarity percentages are not treated as a conclusion while samples are still accumulating.')}</p></div><div className='flex flex-wrap gap-1 rounded-lg bg-muted p-1'>{([
          ['all', t('All {{count}}', { count: results.length })],
          ['issue', t('Anomalies {{count}}', { count: counts.issue })],
          ['unavailable', t('Unavailable {{count}}', { count: counts.unavailable })],
          ['collecting', t('Collecting {{count}}', { count: counts.collecting })],
          ['healthy', t('Normal {{count}}', { count: counts.healthy })],
        ] as [ResultFilter, string][]).map(([value, label]) => <Button key={value} size='sm' variant={resultFilter === value ? 'secondary' : 'ghost'} aria-pressed={resultFilter === value} onClick={() => setResultFilter(value)}>{label}</Button>)}</div></div></CardHeader>
        <CardContent className='space-y-3 p-3'>{groupedResults.map((entries) => <TargetResultGroup key={`${entries[0].group.id}:${entries[0].result.target_channel_id}`} entries={entries} onOpen={openDetail} />)}{!visibleResults.length ? <div className='text-muted-foreground flex min-h-32 flex-col items-center justify-center text-center text-sm'><Activity className='mb-2 size-6' /><p>{groupsQuery.isLoading ? t('Loading…') : resultFilter === 'all' ? t('No formal detection results yet. Waiting states are shown when returned by the detector; missing data is not displayed as 0%.') : t('No results match this filter.')}</p></div> : null}</CardContent>
      </Card>

      <div><div className='mb-3'><h2 className='font-semibold'>{t('Detection groups and scheduling')}</h2><p className='text-muted-foreground text-sm'>{t('Manage configuration and manual runs here. Historical results are automatically limited to the latest 100 windows per target and model comparison.')}</p></div><div className='grid gap-3 md:grid-cols-2 xl:grid-cols-3'>{groups.map((group) => <Card key={group.id} className='gap-3 py-4'><CardContent className='space-y-3 px-4'><div className='flex items-start justify-between gap-2'><div><p className='font-medium'>{group.name}</p><p className='text-muted-foreground text-xs'>{t('{{count}} channels', { count: group.channel_ids.length })} · {t('Baseline')} #{group.baseline_channel_id}</p></div><Badge variant={group.enabled ? 'secondary' : 'outline'}>{group.enabled ? t('Enabled') : t('Disabled')}</Badge></div><div className='text-muted-foreground flex flex-wrap gap-x-3 gap-y-1 text-xs'><span>{t('Every {{count}} minutes', { count: group.interval_minutes })}</span><span>{group.random_pairing_enabled ? t('Random pairing on') : t('Random pairing off')}</span><span>{t('Minimum {{count}} samples', { count: group.sampling.minimum_samples })}</span><span>{t('{{count}} model comparisons', { count: group.model_comparisons.length })}</span><span>{t('Alert below {{threshold}} for {{windows}} windows', { threshold: percent(group.policy.alert_threshold), windows: group.policy.alert_windows })}</span><span>{t('Retain {{count}} windows / comparison', { count: group.retention.max_windows_per_target_model })}</span></div>{group.model_comparisons_required ? <div className='rounded-md border border-amber-500/40 bg-amber-500/10 p-2 text-xs text-amber-700'>{t('This legacy group has no model comparison list. Configure one before saving or running formal detection.')}</div> : null}<div className='rounded-md bg-muted/50 p-2 text-xs'><div>{t('Last run')}: {formatTime(group.last_run_at)}</div><div>{t('Next run')}: {formatTime(group.next_run_at)}</div>{group.last_error ? <div className='text-destructive mt-1'>{group.last_error}</div> : null}</div><div className='flex flex-wrap gap-2'><Button size='sm' variant='outline' disabled={editMutation.isPending || deleteMutation.isPending || runMutation.isPending || toggleMutation.isPending} onClick={() => editMutation.mutate(group.id)}><Pencil />{editMutation.isPending && editMutation.variables === group.id ? t('Loading…') : t('Edit')}</Button><Button size='sm' variant='outline' disabled={toggleMutation.isPending || deleteMutation.isPending || runMutation.isPending} onClick={() => toggleMutation.mutate(group)}>{toggleMutation.isPending && toggleMutation.variables?.id === group.id ? t('Updating…') : group.enabled ? t('Disable') : t('Enable')}</Button><Button size='sm' variant='outline' disabled={!group.enabled || group.model_comparisons_required || runMutation.isPending || toggleMutation.isPending || deleteMutation.isPending} onClick={() => runMutation.mutate(group.id)}><Play />{runMutation.isPending && runMutation.variables === group.id ? t('Starting…') : t('Manual detection')}</Button><Button size='sm' variant='outline' aria-label={t('Manage detection history for {{name}}', { name: group.name })} disabled={clearHistoryMutation.isPending || runMutation.isPending || deleteMutation.isPending} onClick={() => setRecordCandidate(group)}><Activity />{t('Record management')}</Button><Button size='sm' variant='outline' disabled={deleteMutation.isPending || runMutation.isPending || toggleMutation.isPending} onClick={() => setDeleteCandidate(group)}><Trash2 />{deleteMutation.isPending && deleteMutation.variables === group.id ? t('Deleting…') : t('Delete')}</Button></div></CardContent></Card>)}{!groupsQuery.isLoading && !groups.length ? <Card className='md:col-span-2 xl:col-span-3'><CardContent className='flex flex-col items-center py-10 text-center'><Activity className='text-muted-foreground mb-3 size-8' /><p className='font-medium'>{t('No benchmark groups yet')}</p><p className='text-muted-foreground text-sm'>{t('Create a group to begin collecting model-bucketed comparisons.')}</p></CardContent></Card> : null}</div></div>

      <div className='flex items-start gap-2 rounded-lg border border-blue-500/30 bg-blue-500/5 p-3 text-sm'><AlertTriangle className='mt-0.5 size-4 shrink-0 text-blue-600' /><p>{t('Read the status first. Scores shown during sample collection are observations only; open details for the evidence chain, history, and technical scoring inputs.')}</p></div>
      <QuickProbe channels={channelsQuery.data ?? []} channelsLoading={channelsQuery.isLoading} channelsError={channelsQuery.isError ? errorMessage(channelsQuery.error) || t('Failed to load channels') : undefined} onRetryChannels={() => void channelsQuery.refetch()} />
      {editing !== undefined ? <GroupForm key={editing?.id ?? 'new'} open group={editing} channels={channelsQuery.data ?? []} channelsLoading={channelsQuery.isLoading} channelsError={channelsQuery.isError ? errorMessage(channelsQuery.error) || t('Failed to load channels') : undefined} saving={saveMutation.isPending} saveError={formError} onRetryChannels={() => void channelsQuery.refetch()} onOpenChange={(open) => { if (!open && !saveMutation.isPending) { setEditing(undefined); setFormError(undefined) } }} onSave={(input) => saveMutation.mutate({ group: editing, input })} /> : null}
      <ResultDetail result={detailResult} minimumSamples={detailGroup?.sampling.minimum_samples ?? 0} loading={detailQuery.isFetching} running={runMutation.isPending} runStatus={runStatus} onRun={() => { if (detailSeed) runMutation.mutate(detailSeed.group_id) }} error={detailQuery.isError ? errorMessage(detailQuery.error) || t('Failed to load result details') : undefined} onRetry={() => void detailQuery.refetch()} onClose={closeDetail} />
      <HistoryManagementDialog group={recordCandidate} clearing={clearHistoryMutation.isPending} onClear={(id) => clearHistoryMutation.mutate(id)} onOpenChange={(open) => { if (!open && !clearHistoryMutation.isPending) setRecordCandidate(null) }} />
      <ConfirmDialog open={Boolean(deleteCandidate)} onOpenChange={(open) => { if (!open && !deleteMutation.isPending) setDeleteCandidate(null) }} title={t('Delete detection group')} desc={t('This permanently deletes “{{name}}”, its schedule, configuration, history, and alerts. This action cannot be undone.', { name: deleteCandidate?.name ?? '' })} confirmText={deleteMutation.isPending ? t('Deleting…') : t('Delete group')} destructive isLoading={deleteMutation.isPending} handleConfirm={() => { if (deleteCandidate) deleteMutation.mutate(deleteCandidate.id) }} />
    </div></SectionPageLayout.Content>
  </SectionPageLayout>
}
