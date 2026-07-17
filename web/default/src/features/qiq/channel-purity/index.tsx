/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or (at your option) any later version.
*/
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type { TFunction } from 'i18next'
import { RefreshCw, Search, ShieldAlert, ShieldCheck } from 'lucide-react'
import { useMemo, useState, type ReactNode } from 'react'
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
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import { Progress } from '@/components/ui/progress'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

import {
  getPurityEligibleChannels,
  getPurityResults,
  startPurityScan,
} from './api'
import type {
  PurityEvidence,
  PurityResult,
  PurityRisk,
  PurityStatus,
} from './types'

const resultKey = ['qiq', 'channel-purity', 'results'] as const

function RiskBadge({ risk }: { risk: PurityRisk }) {
  const { t } = useTranslation()
  let variant: 'destructive' | 'secondary' | 'outline' = 'outline'
  if (risk === 'high') variant = 'destructive'
  if (risk === 'low') variant = 'secondary'
  const label = {
    high: t('Purity risk: high'),
    medium: t('Purity risk: medium'),
    low: t('Purity risk: low'),
    unknown: t('Purity risk: unknown'),
  }[risk]
  return <Badge variant={variant}>{label}</Badge>
}

function StatusBadge({ status }: { status: PurityStatus }) {
  const { t } = useTranslation()
  const label = {
    pending: t('Purity status: pending'),
    running: t('Purity status: running'),
    completed: t('Purity status: completed'),
    failed: t('Purity status: failed'),
  }[status]
  return (
    <Badge variant={status === 'failed' ? 'destructive' : 'outline'}>
      {label}
    </Badge>
  )
}

function coveragePercent(value: number) {
  if (!Number.isFinite(value)) return 0
  return Math.max(0, Math.min(100, Math.round(value)))
}

function formatTimestamp(timestamp: string | number | undefined) {
  if (timestamp === undefined || timestamp === '') return '—'
  const normalized =
    typeof timestamp === 'number' && timestamp < 1e12
      ? timestamp * 1000
      : timestamp
  const date = new Date(normalized)
  return Number.isNaN(date.getTime()) ? '—' : date.toLocaleString()
}

function evidenceTitle(t: TFunction, evidence: PurityEvidence) {
  switch (evidence.kind) {
    case 'protocol':
      return t('Protocol response')
    case 'declared_model':
      return t('Declared model')
    case 'usage':
      return t('Usage metadata')
    case 'warning':
      return t('Warning')
    case 'operational':
      return t('Operational status')
    default:
      return evidence.title ?? t('Evidence item')
  }
}

function evidenceText(t: TFunction, value: string) {
  switch (value) {
    case 'A successful OpenAI-compatible response with output':
      return t('A successful OpenAI-compatible response with output')
    case 'Consistent non-negative token usage when provided':
      return t('Consistent non-negative token usage when provided')
    case 'Not returned':
      return t('Not returned')
    case 'declared_model_differs_from_mapped_request':
      return t('The declared model differs from the mapped request.')
    default:
      return value
  }
}

function apiErrorMessage(error: unknown) {
  if (error && typeof error === 'object' && 'response' in error) {
    const response = (error as { response?: { data?: { message?: unknown } } })
      .response
    if (typeof response?.data?.message === 'string') {
      return response.data.message
    }
  }
  return error instanceof Error ? error.message : undefined
}

export function ChannelPurity() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [scanOpen, setScanOpen] = useState(false)
  const [selectedChannel, setSelectedChannel] = useState('')
  const [selectedModel, setSelectedModel] = useState('')
  const [detail, setDetail] = useState<PurityResult | null>(null)

  const resultsQuery = useQuery({
    queryKey: resultKey,
    queryFn: getPurityResults,
    refetchInterval: (query) =>
      query.state.data?.some(
        (item) => item.status === 'pending' || item.status === 'running'
      )
        ? 5000
        : false,
  })
  const channelsQuery = useQuery({
    queryKey: ['qiq', 'channel-purity', 'channels'],
    queryFn: getPurityEligibleChannels,
    staleTime: 30_000,
  })
  const channels = channelsQuery.data ?? []
  const channel = channels.find((item) => String(item.id) === selectedChannel)
  const models = useMemo(
    () => [
      ...new Set(
        (channel?.models ?? '')
          .split(',')
          .map((model) => model.trim())
          .filter(Boolean)
      ),
    ],
    [channel?.models]
  )

  const scanMutation = useMutation({
    mutationFn: startPurityScan,
    onSuccess: async (response) => {
      if (response.success === false) {
        toast.error(response.message || t('Failed to start purity scan'))
        return
      }
      toast.success(t('Channel purity scan started'))
      setScanOpen(false)
      await queryClient.invalidateQueries({ queryKey: resultKey })
    },
    onError: (error) =>
      toast.error(apiErrorMessage(error) || t('Failed to start purity scan')),
  })

  const results = resultsQuery.data ?? []
  const highRisk = results.filter((item) => item.risk === 'high').length
  const completed = results.filter((item) => item.status === 'completed')
  const averageCoverage = completed.length
    ? Math.round(
        completed.reduce(
          (sum, item) => sum + coveragePercent(item.coverage),
          0
        ) / completed.length
      )
    : 0
  const active = results.filter(
    (item) => item.status === 'pending' || item.status === 'running'
  ).length

  const beginScan = () => {
    if (!selectedChannel || !selectedModel) return
    scanMutation.mutate({
      channel_id: Number(selectedChannel),
      model: selectedModel,
    })
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Channel purity')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='space-y-4'>
          <div className='flex flex-wrap items-center justify-between gap-3'>
            <p className='text-muted-foreground max-w-2xl text-sm'>
              {t(
                'Inspect whether channel responses match the selected model and review supporting evidence.'
              )}
            </p>
            <div className='flex gap-2'>
              <Button
                variant='outline'
                onClick={() => resultsQuery.refetch()}
                disabled={resultsQuery.isFetching}
              >
                <RefreshCw
                  className={resultsQuery.isFetching ? 'animate-spin' : ''}
                />
                {t('Refresh')}
              </Button>
              <Button onClick={() => setScanOpen(true)}>
                <Search />
                {t('Start purity scan')}
              </Button>
            </div>
          </div>

          {resultsQuery.isError ? (
            <div className='border-destructive/40 bg-destructive/5 flex flex-wrap items-center justify-between gap-3 rounded-md border px-4 py-3'>
              <div>
                <p className='text-destructive text-sm font-medium'>
                  {t('Failed to load purity scan results.')}
                </p>
                {apiErrorMessage(resultsQuery.error) ? (
                  <p className='text-muted-foreground mt-1 text-xs'>
                    {apiErrorMessage(resultsQuery.error)}
                  </p>
                ) : null}
              </div>
              <Button
                type='button'
                size='sm'
                variant='outline'
                onClick={() => void resultsQuery.refetch()}
              >
                {t('Try again')}
              </Button>
            </div>
          ) : null}

          <div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-4'>
            <SummaryCard
              title={t('Scans')}
              value={results.length}
              icon={<Search className='size-4' />}
            />
            <SummaryCard
              title={t('High risk')}
              value={highRisk}
              icon={<ShieldAlert className='size-4' />}
            />
            <SummaryCard
              title={t('Average coverage')}
              value={`${averageCoverage}%`}
              icon={<ShieldCheck className='size-4' />}
            />
            <SummaryCard
              title={t('Active scans')}
              value={active}
              icon={<RefreshCw className='size-4' />}
            />
          </div>

          <Card>
            <CardHeader>
              <CardTitle>{t('Purity scan results')}</CardTitle>
            </CardHeader>
            <CardContent className='overflow-x-auto p-0'>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('Channel')}</TableHead>
                    <TableHead>{t('Model')}</TableHead>
                    <TableHead>{t('Risk')}</TableHead>
                    <TableHead>{t('Coverage')}</TableHead>
                    <TableHead>{t('Status')}</TableHead>
                    <TableHead>{t('Updated at')}</TableHead>
                    <TableHead className='text-right'>{t('Actions')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {results.map((result) => {
                    const coverage = coveragePercent(result.coverage)
                    const timestamp = result.updated_at ?? result.created_at
                    return (
                      <TableRow key={result.id}>
                        <TableCell className='font-medium'>
                          {result.channel_name ?? `#${result.channel_id}`}
                        </TableCell>
                        <TableCell className='font-mono text-xs'>
                          {result.model}
                        </TableCell>
                        <TableCell>
                          <RiskBadge risk={result.risk} />
                        </TableCell>
                        <TableCell>
                          <div className='flex min-w-28 items-center gap-2'>
                            <Progress value={coverage} />
                            <span className='text-xs'>{coverage}%</span>
                          </div>
                        </TableCell>
                        <TableCell>
                          <StatusBadge status={result.status} />
                        </TableCell>
                        <TableCell className='text-muted-foreground text-xs whitespace-nowrap'>
                          {formatTimestamp(timestamp)}
                        </TableCell>
                        <TableCell className='text-right'>
                          <Button
                            size='sm'
                            variant='ghost'
                            onClick={() => setDetail(result)}
                          >
                            {t('View evidence')}
                          </Button>
                        </TableCell>
                      </TableRow>
                    )
                  })}
                  {!resultsQuery.isLoading &&
                  !resultsQuery.isError &&
                  results.length === 0 ? (
                    <TableRow>
                      <TableCell
                        colSpan={7}
                        className='text-muted-foreground h-28 text-center'
                      >
                        {t('No purity scan results yet.')}
                      </TableCell>
                    </TableRow>
                  ) : null}
                  {resultsQuery.isLoading ? (
                    <TableRow>
                      <TableCell
                        colSpan={7}
                        className='text-muted-foreground h-28 text-center'
                      >
                        {t('Loading...')}
                      </TableCell>
                    </TableRow>
                  ) : null}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
        </div>
      </SectionPageLayout.Content>

      <Dialog open={scanOpen} onOpenChange={setScanOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('Start purity scan')}</DialogTitle>
            <DialogDescription>
              {t('Select an enabled channel and one of its configured models.')}
            </DialogDescription>
          </DialogHeader>
          <div className='space-y-4'>
            <div className='space-y-2'>
              <Label>{t('Channel')}</Label>
              {channelsQuery.isError ? (
                <div className='border-destructive/40 rounded-md border p-3'>
                  <p className='text-destructive text-sm'>
                    {t('Failed to load enabled channels.')}
                  </p>
                  <Button
                    type='button'
                    size='sm'
                    variant='outline'
                    className='mt-2'
                    onClick={() => void channelsQuery.refetch()}
                  >
                    {t('Try again')}
                  </Button>
                </div>
              ) : (
                <Select
                  value={selectedChannel}
                  onValueChange={(value) => {
                    setSelectedChannel(value ?? '')
                    setSelectedModel('')
                  }}
                  disabled={channelsQuery.isLoading || channels.length === 0}
                >
                  <SelectTrigger className='w-full'>
                    <SelectValue
                      placeholder={
                        channelsQuery.isLoading
                          ? t('Loading...')
                          : t('Select channel')
                      }
                    />
                  </SelectTrigger>
                  <SelectContent>
                    {channels.map((item) => (
                      <SelectItem key={item.id} value={String(item.id)}>
                        {item.name} (#{item.id})
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
              {!channelsQuery.isLoading &&
              !channelsQuery.isError &&
              channels.length === 0 ? (
                <p className='text-muted-foreground text-xs'>
                  {t('No enabled channels are available for scanning.')}
                </p>
              ) : null}
            </div>
            <div className='space-y-2'>
              <Label>{t('Model')}</Label>
              <Select
                value={selectedModel}
                onValueChange={(value) => setSelectedModel(value ?? '')}
                disabled={!selectedChannel}
              >
                <SelectTrigger className='w-full'>
                  <SelectValue placeholder={t('Select model')} />
                </SelectTrigger>
                <SelectContent>
                  {models.map((model) => (
                    <SelectItem key={model} value={model}>
                      {model}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
          <DialogFooter>
            <Button variant='outline' onClick={() => setScanOpen(false)}>
              {t('Cancel')}
            </Button>
            <Button
              onClick={beginScan}
              disabled={
                !selectedChannel ||
                !selectedModel ||
                channelsQuery.isError ||
                scanMutation.isPending
              }
            >
              {scanMutation.isPending ? t('Starting...') : t('Start scan')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog
        open={detail !== null}
        onOpenChange={(open) => !open && setDetail(null)}
      >
        <DialogContent className='sm:max-w-2xl'>
          <DialogHeader>
            <DialogTitle>{t('Purity evidence')}</DialogTitle>
            <DialogDescription>
              {detail?.summary ||
                t('Signals and observations collected during this scan.')}
            </DialogDescription>
          </DialogHeader>
          <div className='max-h-[60vh] space-y-3 overflow-y-auto'>
            {detail?.evidence?.map((evidence) => (
              <div
                key={evidence.id}
                className='bg-muted/40 rounded-lg border p-3'
              >
                <p className='font-medium'>{evidenceTitle(t, evidence)}</p>
                {evidence.description ? (
                  <p className='text-muted-foreground mt-1 text-sm'>
                    {evidenceText(t, evidence.description)}
                  </p>
                ) : null}
                {evidence.expected ? (
                  <p className='mt-2 text-xs'>
                    <span className='font-medium'>{t('Expected')}:</span>{' '}
                    {evidenceText(t, evidence.expected)}
                  </p>
                ) : null}
                {evidence.actual ? (
                  <p className='mt-1 text-xs'>
                    <span className='font-medium'>{t('Observed')}:</span>{' '}
                    {evidenceText(t, evidence.actual)}
                  </p>
                ) : null}
              </div>
            ))}
            {!detail?.evidence?.length ? (
              <p className='text-muted-foreground py-8 text-center text-sm'>
                {t('No evidence was returned for this scan.')}
              </p>
            ) : null}
          </div>
        </DialogContent>
      </Dialog>
    </SectionPageLayout>
  )
}

function SummaryCard({
  title,
  value,
  icon,
}: {
  title: string
  value: string | number
  icon: ReactNode
}) {
  return (
    <Card>
      <CardContent className='flex items-center justify-between p-4'>
        <div>
          <p className='text-muted-foreground text-sm'>{title}</p>
          <p className='mt-1 text-2xl font-semibold'>{value}</p>
        </div>
        <div className='bg-muted rounded-full p-2'>{icon}</div>
      </CardContent>
    </Card>
  )
}
