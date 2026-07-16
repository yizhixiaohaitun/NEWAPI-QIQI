/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or (at your option) any later version.
*/
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { RefreshCw, Search, ShieldAlert, ShieldCheck } from 'lucide-react'
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
import { getChannels } from '@/features/channels/api'

import { getPurityResults, getPurityScan, startPurityScan } from './api'
import type { PurityResult, PurityRisk, PurityStatus } from './types'

const resultKey = ['qiq', 'channel-purity', 'results'] as const

function RiskBadge({ risk }: { risk: PurityRisk }) {
  const { t } = useTranslation()
  const variant =
    risk === 'high' ? 'destructive' : risk === 'low' ? 'secondary' : 'outline'
  return <Badge variant={variant}>{t(`Purity risk: ${risk}`)}</Badge>
}

function StatusBadge({ status }: { status: PurityStatus }) {
  const { t } = useTranslation()
  return (
    <Badge variant={status === 'failed' ? 'destructive' : 'outline'}>
      {t(`Purity status: ${status}`)}
    </Badge>
  )
}

function coveragePercent(value: number) {
  const normalized = value <= 1 ? value * 100 : value
  return Math.max(0, Math.min(100, Math.round(normalized)))
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
    queryFn: () => getChannels({ p: 1, page_size: 1000, status: 'enabled' }),
  })
  const channels = channelsQuery.data?.data?.items ?? []
  const channel = channels.find((item) => String(item.id) === selectedChannel)
  const models = useMemo(
    () =>
      Array.from(
        new Set(
          (channel?.models ?? '')
            .split(',')
            .map((model) => model.trim())
            .filter(Boolean)
        )
      ),
    [channel?.models]
  )

  const scanMutation = useMutation({
    mutationFn: startPurityScan,
    onSuccess: async (response) => {
      if (response.success === false) return
      toast.success(t('Channel purity scan started'))
      setScanOpen(false)
      const scan = response.data
      const id = scan?.id ?? scan?.scan_id
      if (id != null) await getPurityScan(String(id)).catch(() => undefined)
      await queryClient.invalidateQueries({ queryKey: resultKey })
    },
    onError: () => toast.error(t('Failed to start purity scan')),
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
                          {timestamp
                            ? new Date(
                                typeof timestamp === 'number' &&
                                  timestamp < 1e12
                                  ? timestamp * 1000
                                  : timestamp
                              ).toLocaleString()
                            : '—'}
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
                  {!resultsQuery.isLoading && results.length === 0 ? (
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
              <Select
                value={selectedChannel}
                onValueChange={(value) => {
                  setSelectedChannel(value ?? '')
                  setSelectedModel('')
                }}
              >
                <SelectTrigger className='w-full'>
                  <SelectValue placeholder={t('Select channel')} />
                </SelectTrigger>
                <SelectContent>
                  {channels.map((item) => (
                    <SelectItem key={item.id} value={String(item.id)}>
                      {item.name} (#{item.id})
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
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
                !selectedChannel || !selectedModel || scanMutation.isPending
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
            {detail?.evidence?.map((evidence, index) => (
              <div
                key={`${evidence.title ?? 'evidence'}-${index}`}
                className='bg-muted/40 rounded-lg border p-3'
              >
                <p className='font-medium'>
                  {evidence.title ?? t('Evidence item')}
                </p>
                {evidence.description ? (
                  <p className='text-muted-foreground mt-1 text-sm'>
                    {evidence.description}
                  </p>
                ) : null}
                {evidence.expected ? (
                  <p className='mt-2 text-xs'>
                    <span className='font-medium'>{t('Expected')}:</span>{' '}
                    {evidence.expected}
                  </p>
                ) : null}
                {evidence.actual ? (
                  <p className='mt-1 text-xs'>
                    <span className='font-medium'>{t('Observed')}:</span>{' '}
                    {evidence.actual}
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
  icon: React.ReactNode
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
