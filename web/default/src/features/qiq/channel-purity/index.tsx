/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or (at your option) any later version.
*/
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Activity,
  AlertTriangle,
  Pencil,
  Play,
  Plus,
  RefreshCw,
  Trash2,
} from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

import {
  createPurityGroup,
  deletePurityGroup,
  getPurityGroup,
  getPurityResultDetail,
  listChannelOptions,
  listPurityGroups,
  runPurityGroup,
  updatePurityGroup,
} from './api'
import { GroupForm } from './group-form'
import { QuickProbe } from './quick-probe'
import type { DetectorStatus, PurityGroup, PurityGroupInput, TargetResult, TokenRange } from './types'

const groupsKey = ['qiq', 'channel-purity', 'groups'] as const

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

function tokenRange(value?: TokenRange) {
  if (!value) return '—'
  return `${value.min.toLocaleString()} – ${value.max.toLocaleString()}`
}

function errorMessage(error: unknown) {
  if (error && typeof error === 'object' && 'response' in error) {
    const response = (error as { response?: { data?: { message?: unknown } } }).response
    if (typeof response?.data?.message === 'string') return response.data.message
  }
  return error instanceof Error ? error.message : undefined
}

function Metric({ label, value }: { label: string; value?: number }) {
  return <div><p className='text-muted-foreground text-xs'>{label}</p><p className='font-medium tabular-nums'>{percent(value)}</p></div>
}

function ResultRow({ result, onOpen }: { result: TargetResult; onOpen: () => void }) {
  const { t } = useTranslation()
  return (
    <TableRow>
      <TableCell>
        <div className='font-medium'>{result.target_channel_name}</div>
        <div className='text-muted-foreground text-xs'>{t('Baseline')}: {result.baseline_channel_name}</div>
      </TableCell>
      <TableCell className='font-mono text-xs'>{result.model}</TableCell>
      <TableCell><StatusBadge status={result.status} /></TableCell>
      <TableCell className='tabular-nums'>{result.samples}</TableCell>
      <TableCell>{percent(result.field_similarity.value)}</TableCell>
      <TableCell>{percent(result.token_similarity.value)}</TableCell>
      <TableCell>{percent(result.confidence)}</TableCell>
      <TableCell className='max-w-64'>
        {result.latest_evidence ? <div><p className='truncate text-sm'>{result.latest_evidence.summary}</p><p className='text-muted-foreground text-xs'>{formatTime(result.latest_evidence.occurred_at)}</p></div> : <span className='text-muted-foreground'>—</span>}
        {result.alerts.length ? <Badge variant='destructive' className='mt-1'>{t('{{count}} alerts', { count: result.alerts.length })}</Badge> : null}
      </TableCell>
      <TableCell className='text-right'><Button size='sm' variant='outline' onClick={onOpen}>{t('Details')}</Button></TableCell>
    </TableRow>
  )
}

function ResultCard({ result, onOpen }: { result: TargetResult; onOpen: () => void }) {
  const { t } = useTranslation()
  return <Card className='gap-3 py-4'>
    <CardContent className='space-y-3 px-4'>
      <div className='flex items-start justify-between gap-2'><div><p className='font-medium'>{result.target_channel_name}</p><p className='text-muted-foreground text-xs'>{result.model} · {t('Baseline')}: {result.baseline_channel_name}</p></div><StatusBadge status={result.status} /></div>
      <div className='grid grid-cols-3 gap-2'><Metric label={t('Field / structure')} value={result.field_similarity.value} /><Metric label={t('Token range')} value={result.token_similarity.value} /><Metric label={t('Confidence')} value={result.confidence} /></div>
      <div className='text-sm'><span className='text-muted-foreground'>{t('Samples')}: </span>{result.samples}</div>
      {result.latest_evidence ? <p className='border-l-2 pl-2 text-sm'>{result.latest_evidence.summary}</p> : null}
      <Button className='w-full' size='sm' variant='outline' onClick={onOpen}>{t('View evidence and trend')}</Button>
    </CardContent>
  </Card>
}

function ResultDetail({ result, loading, error, onRetry, onClose }: { result: TargetResult | null; loading: boolean; error?: string; onRetry: () => void; onClose: () => void }) {
  const { t } = useTranslation()
  return <Dialog open={Boolean(result) || loading || Boolean(error)} onOpenChange={(open) => { if (!open) onClose() }}><DialogContent className='max-h-[90vh] overflow-y-auto sm:max-w-3xl'>{result ? <>
    <DialogHeader><DialogTitle>{result.target_channel_name} · {result.model}</DialogTitle><DialogDescription>{t('Independent comparison against baseline {{baseline}}', { baseline: result.baseline_channel_name })}</DialogDescription></DialogHeader>
    <div className='flex flex-wrap gap-2'><StatusBadge status={result.status} /><Badge variant='outline'>{t('{{count}} samples', { count: result.samples })}</Badge></div>
    <div className='grid gap-3 sm:grid-cols-3'><Card><CardContent className='pt-4'><p className='text-muted-foreground text-xs'>{t('Baseline token interval')}</p><p className='mt-1 font-mono'>{tokenRange(result.baseline_token_range)}</p></CardContent></Card><Card><CardContent className='pt-4'><p className='text-muted-foreground text-xs'>{t('Target token interval')}</p><p className='mt-1 font-mono'>{tokenRange(result.target_token_range)}</p></CardContent></Card><Card><CardContent className='pt-4'><p className='text-muted-foreground text-xs'>{t('Deviation rate')}</p><p className='mt-1 font-medium'>{percent(result.deviation_rate)}</p></CardContent></Card></div>
    <div><h3 className='mb-2 font-medium'>{t('Evidence chain')}</h3>{result.evidence.length ? <div className='space-y-2'>{result.evidence.map((item) => <div key={item.id} className='rounded-lg border p-3'><div className='flex justify-between gap-3'><Badge variant='outline'>{item.kind}</Badge><span className='text-muted-foreground text-xs'>{formatTime(item.occurred_at)}</span></div><p className='mt-2 text-sm'>{item.summary}</p>{item.baseline_value !== undefined || item.target_value !== undefined ? <div className='bg-muted mt-2 grid gap-2 rounded p-2 font-mono text-xs sm:grid-cols-2'><span>{t('Baseline')}: {item.baseline_value ?? '—'}</span><span>{t('Target')}: {item.target_value ?? '—'}</span></div> : null}{item.request_id ? <p className='text-muted-foreground mt-2 text-xs'>Request ID: {item.request_id}</p> : null}</div>)}</div> : <p className='text-muted-foreground text-sm'>{t('No evidence has been recorded yet.')}</p>}</div>
    <div><h3 className='mb-2 font-medium'>{t('Historical trend')}</h3>{result.trend.length ? <div className='flex min-h-28 items-end gap-1 overflow-x-auto rounded-lg border p-3'>{result.trend.map((point, index) => <div key={`${point.at}-${index}`} className='group flex min-w-8 flex-1 flex-col items-center justify-end gap-1' title={`${formatTime(point.at)} · ${percent(point.confidence)}`}><div className={`w-full max-w-8 rounded-t ${point.status === 'ALERT' || point.status === 'DETECTOR_ERROR' ? 'bg-destructive' : point.status === 'SUSPECT' ? 'bg-amber-500' : 'bg-emerald-500'}`} style={{ height: `${Math.max(10, (point.confidence ?? 0.2) * 80)}px` }} /><span className='text-muted-foreground text-[10px]'>{index + 1}</span></div>)}</div> : <p className='text-muted-foreground text-sm'>{t('Trend is unavailable until multiple detection windows are recorded.')}</p>}</div>
    {result.alerts.length ? <div className='border-destructive/40 bg-destructive/5 rounded-lg border p-3'><h3 className='text-destructive font-medium'>{t('Alerts')}</h3><ul className='mt-2 list-disc space-y-1 pl-5 text-sm'>{result.alerts.map((alert) => <li key={alert}>{alert}</li>)}</ul></div> : null}
  </> : loading ? <div className='py-10 text-center'><RefreshCw className='mx-auto mb-2 size-5 animate-spin' /><p>{t('Loading latest assessment and history…')}</p></div> : <div role='alert' className='py-6'><p className='text-destructive'>{error || t('Failed to load result details')}</p><Button className='mt-3' variant='outline' onClick={onRetry}>{t('Try again')}</Button></div>}</DialogContent></Dialog>
}

export function ChannelPurity() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [editing, setEditing] = useState<PurityGroup | null | undefined>(undefined)
  const [formError, setFormError] = useState<string>()
  const [detailSeed, setDetailSeed] = useState<TargetResult | null>(null)
  const groupsQuery = useQuery({ queryKey: groupsKey, queryFn: listPurityGroups, refetchInterval: 30_000 })
  const channelsQuery = useQuery({ queryKey: ['qiq', 'channels', 'options'], queryFn: listChannelOptions })
  const detailQuery = useQuery({ queryKey: ['qiq', 'channel-purity', 'detail', detailSeed?.group_id, detailSeed?.target_channel_id, detailSeed?.model], queryFn: () => getPurityResultDetail(detailSeed!), enabled: Boolean(detailSeed), retry: false })
  const refresh = () => groupsQuery.refetch()
  const openDetail = (result: TargetResult) => setDetailSeed(result)
  const closeDetail = () => setDetailSeed(null)
  const saveMutation = useMutation({
    mutationFn: ({ group, input }: { group: PurityGroup | null; input: PurityGroupInput }) => group ? updatePurityGroup(group.id, input) : createPurityGroup(input),
    onMutate: () => setFormError(undefined),
    onSuccess: async () => { setEditing(undefined); setFormError(undefined); toast.success(t('Benchmark group saved')); await refresh() },
    onError: (error) => { const message = errorMessage(error) || t('Failed to save benchmark group'); setFormError(message); toast.error(message) },
  })
  const deleteMutation = useMutation({ mutationFn: deletePurityGroup, onSuccess: async () => { toast.success(t('Benchmark group deleted')); await refresh() }, onError: (error) => toast.error(errorMessage(error) || t('Failed to delete benchmark group')) })
  const runMutation = useMutation({ mutationFn: runPurityGroup, onSuccess: async () => { toast.success(t('Formal detection started')); await refresh() }, onError: (error) => toast.error(errorMessage(error) || t('Failed to start formal detection')) })
  const toggleMutation = useMutation({ mutationFn: async (group: PurityGroup) => updatePurityGroup(group.id, { name: group.name, enabled: !group.enabled, channel_ids: group.channel_ids, baseline_channel_id: group.baseline_channel_id, interval_minutes: group.interval_minutes, random_pairing_enabled: group.random_pairing_enabled, sampling: { ...group.sampling } }), onSuccess: async () => { toast.success(t('Group status updated')); await refresh() }, onError: (error) => toast.error(errorMessage(error) || t('Failed to update group status')) })
  const editMutation = useMutation({ mutationFn: getPurityGroup, onSuccess: (group) => { setFormError(undefined); setEditing(group) }, onError: (error) => toast.error(errorMessage(error) || t('Failed to load group configuration')) })
  const groups = groupsQuery.data ?? []
  const results = useMemo(() => groups.flatMap((group) => group.results.map((result) => ({ group, result }))), [groups])
  const detailResult = detailSeed && detailQuery.data ? detailQuery.data : null
  return <SectionPageLayout>
    <SectionPageLayout.Title>{t('Channel purity')}</SectionPageLayout.Title>
    <SectionPageLayout.Content><div className='space-y-4'>
      <div className='flex flex-wrap items-start justify-between gap-3'><div><h2 className='text-lg font-semibold'>{t('Grouped baseline detection')}</h2><p className='text-muted-foreground max-w-3xl text-sm'>{t('Compare every target with its designated baseline independently. Results are bucketed by actual model; no whole-group average is calculated.')}</p></div><div className='flex gap-2'><Button variant='outline' disabled={groupsQuery.isFetching} onClick={() => void refresh()}><RefreshCw className={groupsQuery.isFetching ? 'animate-spin' : ''} />{groupsQuery.isFetching ? t('Refreshing…') : t('Refresh')}</Button><Button disabled={editMutation.isPending} onClick={() => { setFormError(undefined); setEditing(null) }}><Plus />{t('Create group')}</Button></div></div>
      {groupsQuery.isError ? <div className='border-destructive/40 bg-destructive/5 rounded-lg border p-4'><p className='text-destructive font-medium'>{t('Failed to load benchmark groups')}</p><p className='text-muted-foreground text-sm'>{errorMessage(groupsQuery.error)}</p><Button className='mt-2' size='sm' variant='outline' onClick={() => void groupsQuery.refetch()}>{t('Try again')}</Button></div> : null}
      <div className='grid gap-3 md:grid-cols-2 xl:grid-cols-3'>{groups.map((group) => <Card key={group.id} className='gap-3 py-4'><CardContent className='space-y-3 px-4'><div className='flex items-start justify-between gap-2'><div><p className='font-medium'>{group.name}</p><p className='text-muted-foreground text-xs'>{t('{{count}} channels', { count: group.channel_ids.length })} · {t('Baseline')} #{group.baseline_channel_id}</p></div><Badge variant={group.enabled ? 'secondary' : 'outline'}>{group.enabled ? t('Enabled') : t('Disabled')}</Badge></div><div className='text-muted-foreground flex flex-wrap gap-x-3 gap-y-1 text-xs'><span>{t('Every {{count}} minutes', { count: group.interval_minutes })}</span><span>{group.random_pairing_enabled ? t('Random pairing on') : t('Random pairing off')}</span><span>{t('Minimum {{count}} samples', { count: group.sampling.minimum_samples })}</span></div><div className='rounded-md bg-muted/50 p-2 text-xs'><div>{t('Last run')}: {formatTime(group.last_run_at)}</div><div>{t('Next run')}: {formatTime(group.next_run_at)}</div>{group.last_error ? <div className='text-destructive mt-1'>{group.last_error}</div> : null}</div><div className='flex flex-wrap gap-2'><Button size='sm' variant='outline' disabled={editMutation.isPending || deleteMutation.isPending || runMutation.isPending || toggleMutation.isPending} onClick={() => editMutation.mutate(group.id)}><Pencil />{editMutation.isPending && editMutation.variables === group.id ? t('Loading…') : t('Edit')}</Button><Button size='sm' variant='outline' disabled={toggleMutation.isPending || deleteMutation.isPending || runMutation.isPending} onClick={() => toggleMutation.mutate(group)}>{toggleMutation.isPending && toggleMutation.variables?.id === group.id ? t('Updating…') : group.enabled ? t('Disable') : t('Enable')}</Button><Button size='sm' variant='outline' disabled={!group.enabled || runMutation.isPending || toggleMutation.isPending || deleteMutation.isPending} onClick={() => runMutation.mutate(group.id)}><Play />{runMutation.isPending && runMutation.variables === group.id ? t('Starting…') : t('Run now')}</Button><Button size='sm' variant='outline' disabled={deleteMutation.isPending || runMutation.isPending || toggleMutation.isPending} onClick={() => { if (window.confirm(t('Delete this benchmark group and its configuration?'))) deleteMutation.mutate(group.id) }}><Trash2 />{deleteMutation.isPending && deleteMutation.variables === group.id ? t('Deleting…') : t('Delete')}</Button></div></CardContent></Card>)}{!groupsQuery.isLoading && !groups.length ? <Card className='md:col-span-2 xl:col-span-3'><CardContent className='flex flex-col items-center py-10 text-center'><Activity className='text-muted-foreground mb-3 size-8' /><p className='font-medium'>{t('No benchmark groups yet')}</p><p className='text-muted-foreground text-sm'>{t('Create a group to begin collecting model-bucketed comparisons.')}</p></CardContent></Card> : null}</div>
      <Card><CardHeader><CardTitle>{t('Independent target results')}</CardTitle></CardHeader><CardContent className='p-0'><div className='hidden overflow-x-auto md:block'><Table><TableHeader><TableRow><TableHead>{t('Target / baseline')}</TableHead><TableHead>{t('Actual model')}</TableHead><TableHead>{t('Status')}</TableHead><TableHead>{t('Samples')}</TableHead><TableHead>{t('Field / structure')}</TableHead><TableHead>{t('Token range')}</TableHead><TableHead>{t('Confidence')}</TableHead><TableHead>{t('Latest evidence / alert')}</TableHead><TableHead /></TableRow></TableHeader><TableBody>{results.map(({ result }) => <ResultRow key={result.id} result={result} onOpen={() => openDetail(result)} />)}{!results.length ? <TableRow><TableCell colSpan={9} className='text-muted-foreground h-24 text-center'>{groupsQuery.isLoading ? t('Loading…') : t('No formal detection results yet. Waiting states are shown when returned by the detector; missing data is not displayed as 0%.')}</TableCell></TableRow> : null}</TableBody></Table></div><div className='space-y-3 p-3 md:hidden'>{results.map(({ result }) => <ResultCard key={result.id} result={result} onOpen={() => openDetail(result)} />)}{!results.length ? <p className='text-muted-foreground py-8 text-center text-sm'>{t('No formal detection results yet. Waiting states are shown when returned by the detector; missing data is not displayed as 0%.')}</p> : null}</div></CardContent></Card>
      <div className='flex items-start gap-2 rounded-lg border border-amber-500/30 bg-amber-500/5 p-3 text-sm'><AlertTriangle className='mt-0.5 size-4 shrink-0 text-amber-600' /><p>{t('Status is reported per target and model bucket: BASELINE_UNAVAILABLE, LOW_SAMPLE, NO_TRAFFIC, WARMING_UP, HEALTHY, SUSPECT, ALERT, or DETECTOR_ERROR.')}</p></div>
      <QuickProbe channels={channelsQuery.data ?? []} channelsLoading={channelsQuery.isLoading} channelsError={channelsQuery.isError ? errorMessage(channelsQuery.error) || t('Failed to load channels') : undefined} onRetryChannels={() => void channelsQuery.refetch()} />
      {editing !== undefined ? <GroupForm key={editing?.id ?? 'new'} open group={editing} channels={channelsQuery.data ?? []} channelsLoading={channelsQuery.isLoading} channelsError={channelsQuery.isError ? errorMessage(channelsQuery.error) || t('Failed to load channels') : undefined} saving={saveMutation.isPending} saveError={formError} onRetryChannels={() => void channelsQuery.refetch()} onOpenChange={(open) => { if (!open && !saveMutation.isPending) { setEditing(undefined); setFormError(undefined) } }} onSave={(input) => saveMutation.mutate({ group: editing, input })} /> : null}
      <ResultDetail result={detailResult} loading={detailQuery.isFetching} error={detailQuery.isError ? errorMessage(detailQuery.error) || t('Failed to load result details') : undefined} onRetry={() => void detailQuery.refetch()} onClose={closeDetail} />
    </div></SectionPageLayout.Content>
  </SectionPageLayout>
}
