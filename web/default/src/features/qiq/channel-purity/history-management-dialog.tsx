/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or (at your option) any later version.
*/
import { useQuery } from '@tanstack/react-query'
import { ChevronLeft, ChevronRight, RefreshCw, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'

import { getPurityHistoryPreview, listPurityHistory } from './api'
import type { DetectorStatus, PurityGroup } from './types'

const statuses: Array<DetectorStatus | 'all'> = ['all', 'ALERT', 'SUSPECT', 'DETECTOR_ERROR', 'BASELINE_UNAVAILABLE', 'LOW_SAMPLE', 'NO_TRAFFIC', 'WARMING_UP', 'HEALTHY']

function formatTime(value?: string | number) {
  if (value === undefined || value === '') return '—'
  const normalized = typeof value === 'number' && value < 1e12 ? value * 1000 : value
  const date = new Date(normalized)
  return Number.isNaN(date.getTime()) ? '—' : date.toLocaleString()
}

function percent(value?: number) {
  if (value === undefined) return '—'
  return `${Math.round(Math.max(0, Math.min(1, value)) * 100)}%`
}

export function HistoryManagementDialog({ group, clearing, onClear, onOpenChange }: { group: PurityGroup | null; clearing: boolean; onClear: (id: string) => void; onOpenChange: (open: boolean) => void }) {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const [status, setStatus] = useState<DetectorStatus | 'all'>('all')
  const [query, setQuery] = useState('')
  const [confirming, setConfirming] = useState(false)
  const pageSize = 20
  const historyQuery = useQuery({
    queryKey: ['qiq', 'channel-purity', 'history-management', group?.id, page, status, query],
    queryFn: () => listPurityHistory({ page, page_size: pageSize, group_id: group!.id, status: status === 'all' ? undefined : status, query: query.trim() || undefined }),
    enabled: Boolean(group),
  })
  const previewQuery = useQuery({ queryKey: ['qiq', 'channel-purity', 'history-preview', group?.id], queryFn: () => getPurityHistoryPreview(group!.id), enabled: Boolean(group) })
  const totalPages = Math.max(1, Math.ceil((historyQuery.data?.total ?? 0) / pageSize))
  const preview = previewQuery.data
  return <>
    <Dialog open={Boolean(group)} onOpenChange={(open) => { if (!open && !clearing) onOpenChange(false) }}><DialogContent className='max-h-[90vh] overflow-y-auto sm:max-w-5xl'>
      <DialogHeader><DialogTitle>{t('Record management')} · {group?.name}</DialogTitle><DialogDescription>{t('Search retained detector runs, review the retention policy, or clear this group’s detector data without deleting its configuration.')}</DialogDescription></DialogHeader>
      {group ? <div className='space-y-4'>
        <div className='grid gap-3 rounded-lg border bg-muted/30 p-3 sm:grid-cols-2 lg:grid-cols-5'><div><p className='text-muted-foreground text-xs'>{t('Retention policy')}</p><p className='font-medium'>{t('Newest {{count}} windows / comparison', { count: group.retention.max_windows_per_target_model })}</p></div><div><p className='text-muted-foreground text-xs'>{t('Samples')}</p><p className='font-medium'>{preview?.samples ?? '—'}</p></div><div><p className='text-muted-foreground text-xs'>{t('Pair runs')}</p><p className='font-medium'>{preview?.pair_runs ?? '—'}</p></div><div><p className='text-muted-foreground text-xs'>{t('Assessments / incidents')}</p><p className='font-medium'>{preview ? `${preview.assessments} / ${preview.alerts}` : '—'}</p></div><div className='flex items-end'><Button size='sm' variant='destructive' disabled={clearing || previewQuery.isLoading} onClick={() => setConfirming(true)}><Trash2 />{clearing ? t('Clearing…') : t('Clear all detector records')}</Button></div></div>
        <div className='flex flex-wrap gap-2'><Input className='min-w-52 flex-1' value={query} placeholder={t('Search channel or model')} onChange={(event) => { setQuery(event.target.value); setPage(1) }} /><Select value={status} onValueChange={(value) => { setStatus((value ?? 'all') as DetectorStatus | 'all'); setPage(1) }}><SelectTrigger className='w-52'><SelectValue /></SelectTrigger><SelectContent>{statuses.map((item) => <SelectItem key={item} value={item}>{item === 'all' ? t('All statuses') : t(`Purity detector status: ${item}`)}</SelectItem>)}</SelectContent></Select><Button variant='outline' disabled={historyQuery.isFetching} onClick={() => void historyQuery.refetch()}><RefreshCw className={historyQuery.isFetching ? 'animate-spin' : ''} />{t('Refresh')}</Button></div>
        <div className='overflow-x-auto rounded-lg border'><Table><TableHeader><TableRow><TableHead>{t('Time')}</TableHead><TableHead>{t('Target channel')}</TableHead><TableHead>{t('Model comparison')}</TableHead><TableHead>{t('Status')}</TableHead><TableHead>{t('Paired samples')}</TableHead><TableHead>{t('Structure')}</TableHead><TableHead>{t('Token')}</TableHead><TableHead>{t('Confidence')}</TableHead></TableRow></TableHeader><TableBody>{historyQuery.data?.items.map((item) => <TableRow key={item.id}><TableCell className='whitespace-nowrap'>{formatTime(item.window_ended_at)}</TableCell><TableCell>{item.target_channel_name}</TableCell><TableCell className='max-w-72 truncate font-mono text-xs' title={`${item.baseline_model} → ${item.target_model}`}>{item.baseline_model} → {item.target_model}</TableCell><TableCell><Badge variant='outline'>{t(`Purity detector status: ${item.status}`)}</Badge></TableCell><TableCell>{item.paired_sample_count}</TableCell><TableCell>{percent(item.structure_similarity)}</TableCell><TableCell>{percent(item.token_similarity)}</TableCell><TableCell>{percent(item.confidence)}</TableCell></TableRow>)}{!historyQuery.isLoading && !historyQuery.data?.items.length ? <TableRow><TableCell colSpan={8} className='h-24 text-center text-muted-foreground'>{t('No retained records match the current filters.')}</TableCell></TableRow> : null}</TableBody></Table></div>
        {historyQuery.isError ? <p role='alert' className='text-destructive text-sm'>{t('Failed to load retained detector records.')}</p> : null}
        <div className='flex items-center justify-between'><p className='text-muted-foreground text-sm'>{t('{{count}} records', { count: historyQuery.data?.total ?? 0 })}</p><div className='flex items-center gap-2'><Button size='sm' variant='outline' disabled={page <= 1} onClick={() => setPage((value) => value - 1)}><ChevronLeft />{t('Previous')}</Button><span className='text-sm'>{page} / {totalPages}</span><Button size='sm' variant='outline' disabled={page >= totalPages} onClick={() => setPage((value) => value + 1)}>{t('Next')}<ChevronRight /></Button></div></div>
      </div> : null}
    </DialogContent></Dialog>
    <ConfirmDialog open={confirming} onOpenChange={(open) => { if (!clearing) setConfirming(open) }} title={t('Clear detector records')} desc={t('This clears {{samples}} samples, {{runs}} pair runs, {{assessments}} assessments, and {{alerts}} incidents. The group, schedule, channels, model comparisons, and decision rules are kept. This action cannot be undone.', { samples: preview?.samples ?? 0, runs: preview?.pair_runs ?? 0, assessments: preview?.assessments ?? 0, alerts: preview?.alerts ?? 0 })} confirmText={clearing ? t('Clearing…') : t('Clear records')} destructive isLoading={clearing} handleConfirm={() => { if (group) onClear(group.id) }} />
  </>
}
