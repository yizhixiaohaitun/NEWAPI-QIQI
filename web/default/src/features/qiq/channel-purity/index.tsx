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

function ChannelGroupBadges({ groups }: { groups: string[] }) {
  const { t } = useTranslation()
  return <span className='flex min-w-0 flex-wrap gap-1'>{groups.length ? groups.map((group) => <Badge key={group} variant='outline' className='max-w-40 truncate text-[10px]'>{group}</Badge>) : <Badge variant='outline' className='text-muted-foreground text-[10px]'>{t('No channel group')}</Badge>}</span>
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

function GroupForm({ open, group, channels, channelsLoading, channelsError, saving, saveError, onRetryChannels, onOpenChange, onSave }: {
  open: boolean
  group: PurityGroup | null
  channels: ChannelOption[]
  channelsLoading: boolean
  channelsError?: string
  saving: boolean
  saveError?: string
  onRetryChannels: () => void
  onOpenChange: (open: boolean) => void
  onSave: (input: PurityGroupInput) => void
}) {
  const { t } = useTranslation()
  const [input, setInput] = useState<PurityGroupInput>(() => group ? groupToInput(group) : emptyInput())
  const [channelGroup, setChannelGroup] = useState(ALL_CHANNEL_GROUPS)
  const selected = new Set(input.channel_ids)
  const busy = saving
  const availableGroups = channelGroupNames(channels)
  const visibleChannels = filterChannelsByGroup(channels, channelGroup)
  const visibleIds = visibleChannels.map((channel) => channel.id)
  const allVisibleSelected = visibleIds.length > 0 && visibleIds.every((id) => selected.has(id))
  const toggleChannel = (id: number, checked: boolean) => setInput((current) => setChannelSelected(current, id, checked))
  const valid = isGroupInputValid(input)
  return <Dialog open={open} onOpenChange={onOpenChange}>
    <DialogContent className='max-h-[90vh] overflow-y-auto sm:max-w-2xl'>
      <DialogHeader><DialogTitle>{group ? t('Edit benchmark group') : t('Create benchmark group')}</DialogTitle><DialogDescription>{t('Choose at least two channels and designate exactly one baseline. Each target is compared independently by actual model.')}</DialogDescription></DialogHeader>
      <div className='grid gap-4'>
        <div className='space-y-2'><Label htmlFor='group-name'>{t('Detection group name')}</Label><Input id='group-name' value={input.name} onChange={(event) => setInput({ ...input, name: event.target.value })} placeholder={t('Example: Production OpenAI routes')} /></div>
        <div className='space-y-2'>
          <Label>{t('Channel group (from channel management)')}</Label>
          <Select value={channelGroup} onValueChange={setChannelGroup}><SelectTrigger className='w-full'><SelectValue /></SelectTrigger><SelectContent><SelectItem value={ALL_CHANNEL_GROUPS}>{t('All channel groups')}</SelectItem>{availableGroups.map((name) => <SelectItem key={name} value={name}>{name}</SelectItem>)}</SelectContent></Select>
          <p className='text-muted-foreground text-xs'>{t('This filter only changes the visible channel list. Manually selected channels in other channel groups are retained.')}</p>
          <div className='flex flex-wrap gap-2'><Button type='button' size='sm' variant='outline' disabled={busy || !visibleIds.length || allVisibleSelected} onClick={() => setInput((current) => setGroupChannelsSelected(current, channels, channelGroup, true))}>{t('Select current channel group')}</Button><Button type='button' size='sm' variant='outline' disabled={busy || !visibleIds.some((id) => selected.has(id))} onClick={() => setInput((current) => setGroupChannelsSelected(current, channels, channelGroup, false))}>{t('Clear current channel group')}</Button><span className='text-muted-foreground self-center text-xs'>{t('{{selected}} selected in total; {{visible}} visible', { selected: input.channel_ids.length, visible: visibleChannels.length })}</span></div>
        </div>
        <div className='space-y-2'><Label>{t('Channels in detection group')}</Label><div className='grid max-h-56 gap-2 overflow-y-auto rounded-lg border p-3 sm:grid-cols-2'>{channelsLoading ? <p className='text-muted-foreground text-sm sm:col-span-2'>{t('Loading channels…')}</p> : channelsError ? <div className='sm:col-span-2'><p role='alert' className='text-destructive text-sm'>{channelsError}</p><Button type='button' className='mt-2' size='sm' variant='outline' onClick={onRetryChannels}>{t('Retry channel loading')}</Button></div> : visibleChannels.length ? visibleChannels.map((channel) => <label key={channel.id} className='flex cursor-pointer items-start gap-2 rounded-md border p-2 text-sm'><Checkbox className='mt-0.5' disabled={busy} checked={selected.has(channel.id)} onCheckedChange={(checked) => toggleChannel(channel.id, checked === true)} /><span className='min-w-0'><span className='block truncate'>{channel.name} <span className='text-muted-foreground'>#{channel.id}</span></span><ChannelGroupBadges groups={channel.groups} /></span></label>) : <p className='text-muted-foreground text-sm sm:col-span-2'>{channels.length ? t('No channels match this channel group.') : t('No channels are available.')}</p>}</div></div>
        <div className='space-y-2'><Label>{t('Baseline channel (unique within detection group)')}</Label><Select value={input.baseline_channel_id ? String(input.baseline_channel_id) : ''} onValueChange={(value) => setInput({ ...input, baseline_channel_id: Number(value) })}><SelectTrigger className='w-full'><SelectValue placeholder={t('Select a baseline from selected channels')} /></SelectTrigger><SelectContent>{channels.filter((channel) => selected.has(channel.id)).map((channel) => <SelectItem key={channel.id} value={String(channel.id)}><span className='flex items-center gap-2'><span>{channel.name} #{channel.id}</span><span className='text-muted-foreground text-xs'>{channel.groups.length ? `[${channel.groups.join(', ')}]` : `[${t('No channel group')}]`}</span></span></SelectItem>)}</SelectContent></Select></div>
        <div className='grid gap-4 sm:grid-cols-2'><div className='space-y-2'><Label>{t('Detection interval')}</Label><Select value={String(input.interval_minutes)} onValueChange={(value) => setInput({ ...input, interval_minutes: Number(value) as 5 | 10 })}><SelectTrigger className='w-full'><SelectValue /></SelectTrigger><SelectContent><SelectItem value='5'>{t('Every 5 minutes')}</SelectItem><SelectItem value='10'>{t('Every 10 minutes')}</SelectItem></SelectContent></Select></div><div className='flex items-center justify-between rounded-lg border p-3'><div><p className='text-sm font-medium'>{t('Random pairing detection')}</p><p className='text-muted-foreground text-xs'>{t('Randomly pair eligible observations within the same model bucket.')}</p></div><Switch checked={input.random_pairing_enabled} onCheckedChange={(checked) => setInput({ ...input, random_pairing_enabled: checked })} /></div></div>
        <div><Label>{t('Sampling settings')}</Label><div className='mt-2 grid gap-3 rounded-lg border p-3 sm:grid-cols-3'>{([['window_minutes', t('Window (minutes)')], ['minimum_samples', t('Minimum samples')], ['max_samples_per_window', t('Maximum samples / window')]] as const).map(([key, label]) => <div className='space-y-1' key={key}><Label className='text-xs' htmlFor={key}>{label}</Label><Input id={key} type='number' min={1} value={input.sampling[key]} onChange={(event) => setInput({ ...input, sampling: { ...input.sampling, [key]: Math.max(1, Number(event.target.value)) } })} /></div>)}</div></div>
        <div className='flex items-center justify-between rounded-lg border p-3'><div><p className='text-sm font-medium'>{t('Enable this group')}</p><p className='text-muted-foreground text-xs'>{t('Disabled groups retain history but do not schedule detection.')}</p></div><Switch checked={input.enabled} onCheckedChange={(checked) => setInput({ ...input, enabled: checked })} /></div>
      </div>
      {saveError ? <p role='alert' className='text-destructive text-sm'>{saveError}</p> : null}
      <DialogFooter><Button variant='outline' disabled={busy} onClick={() => onOpenChange(false)}>{t('Cancel')}</Button><Button disabled={!valid || busy || channelsLoading || Boolean(channelsError)} onClick={() => onSave({ ...input, name: input.name.trim() })}>{saving ? t('Saving…') : t('Save group')}</Button></DialogFooter>
    </DialogContent>
  </Dialog>
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

function QuickProbe({ channels, channelsLoading, channelsError, onRetryChannels }: { channels: ChannelOption[]; channelsLoading: boolean; channelsError?: string; onRetryChannels: () => void }) {
  const { t } = useTranslation()
  const [channelId, setChannelId] = useState('')
  const [model, setModel] = useState('')
  const [result, setResult] = useState<QuickProbeResult | null>(null)
  const mutation = useMutation({ mutationFn: runQuickProbe, onSuccess: setResult, onError: (error) => toast.error(errorMessage(error) || t('Quick probe failed')) })
  return <Card><CardHeader><CardTitle className='flex items-center gap-2'><FlaskConical className='size-5' />{t('Quick Probe — manual connectivity diagnosis')}</CardTitle></CardHeader><CardContent className='space-y-3'><p className='text-muted-foreground text-sm'>{t('This sends a manual connectivity check only. Its output is never included in scheduled benchmark results, evidence, or alerts.')}</p>{channelsError ? <div role='alert' className='text-destructive text-sm'>{channelsError}<Button className='ml-2' size='sm' variant='outline' onClick={onRetryChannels}>{t('Retry')}</Button></div> : null}<div className='grid gap-2 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto]'><Select value={channelId} onValueChange={setChannelId}><SelectTrigger className='w-full'><SelectValue placeholder={t('Select channel')} /></SelectTrigger><SelectContent>{channels.map((channel) => <SelectItem key={channel.id} value={String(channel.id)}><span>{channel.name} #{channel.id} <span className='text-muted-foreground text-xs'>{channel.groups.length ? `[${channel.groups.join(', ')}]` : `[${t('No channel group')}]`}</span></span></SelectItem>)}</SelectContent></Select><Input value={model} onChange={(event) => setModel(event.target.value)} placeholder={t('Optional model')} /><Button disabled={!channelId || mutation.isPending || channelsLoading || Boolean(channelsError)} onClick={() => mutation.mutate({ channel_id: Number(channelId), model: model || undefined })}>{mutation.isPending ? t('Diagnosing…') : t('Run diagnosis')}</Button></div>{result ? <div className='rounded-lg border p-3 text-sm'><Badge variant={result.ok ? 'secondary' : 'destructive'}>{result.ok ? t('Connected') : t('Connection failed')}</Badge><span className='ml-2'>{result.message || '—'}</span>{result.latency_ms !== undefined ? <span className='text-muted-foreground ml-2'>{result.latency_ms} ms</span> : null}</div> : null}</CardContent></Card>
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
