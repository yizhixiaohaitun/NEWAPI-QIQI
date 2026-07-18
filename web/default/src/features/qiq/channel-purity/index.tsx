/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or (at your option) any later version.
*/
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import type { TFunction } from 'i18next'
import { Clock, RefreshCw, Search, ShieldAlert } from 'lucide-react'
import { type ReactNode, useState } from 'react'
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
import { Progress } from '@/components/ui/progress'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

import {
  getPurityResults,
  getPurityRunStatus,
  getPurityScan,
  getPuritySettings,
  startPurityFullScan,
  updatePuritySettings,
} from './api'
import type {
  PurityEvidence,
  PurityResult,
  PurityRisk,
  PuritySettings,
  PurityStatus,
} from './types'

const resultKey = ['qiq', 'channel-purity', 'results'] as const
const statusKey = ['qiq', 'channel-purity', 'status'] as const
const settingsKey = ['qiq', 'channel-purity', 'settings'] as const
const intervalOptions = [15, 60, 360, 720, 1440, 10080]

function RiskBadge(props: { risk: PurityRisk }) {
  const { t } = useTranslation()
  let variant: 'destructive' | 'secondary' | 'outline' = 'outline'
  if (props.risk === 'high' || props.risk === 'medium') variant = 'destructive'
  if (props.risk === 'low') variant = 'secondary'
  const labels = {
    high: t('Purity risk: high'),
    medium: t('Purity risk: medium'),
    low: t('Purity risk: low'),
    unknown: t('Purity risk: unknown'),
  }
  return <Badge variant={variant}>{labels[props.risk]}</Badge>
}

function StatusBadge(props: { status: PurityStatus }) {
  const { t } = useTranslation()
  const labels = {
    pending: t('Purity status: pending'),
    running: t('Purity status: running'),
    completed: t('Purity status: completed'),
    failed: t('Purity status: failed'),
    unknown: t('Purity status: unknown'),
  }
  return (
    <Badge variant={props.status === 'failed' ? 'destructive' : 'outline'}>
      {labels[props.status]}
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

function isUnauthorizedError(error: unknown) {
  if (error && typeof error === 'object' && 'response' in error) {
    if (
      (error as { response?: { status?: number } }).response?.status === 401
    ) {
      return true
    }
  }
  return (
    error instanceof Error && /\b(?:401|unauthorized)\b/i.test(error.message)
  )
}

function SignInAction() {
  const { t } = useTranslation()
  return (
    <Button
      type='button'
      size='sm'
      render={
        <Link to='/sign-in' search={{ redirect: '/qiq/channel-purity' }} />
      }
    >
      {t('Sign in')}
    </Button>
  )
}

function ErrorPanel(props: {
  error: unknown
  fallback: string
  onRetry: () => void
}) {
  const { t } = useTranslation()
  const unauthorized = isUnauthorizedError(props.error)
  return (
    <div className='border-destructive/40 bg-destructive/5 flex flex-wrap items-center justify-between gap-3 rounded-md border px-4 py-3'>
      <div>
        <p className='text-destructive text-sm font-medium'>
          {unauthorized
            ? t('Your session has expired. Please sign in again.')
            : props.fallback}
        </p>
        {apiErrorMessage(props.error) ? (
          <p className='text-muted-foreground mt-1 text-xs'>
            {apiErrorMessage(props.error)}
          </p>
        ) : null}
      </div>
      <div className='flex gap-2'>
        {unauthorized ? <SignInAction /> : null}
        <Button
          type='button'
          size='sm'
          variant='outline'
          onClick={props.onRetry}
        >
          {t('Try again')}
        </Button>
      </div>
    </div>
  )
}

function operationalErrorLabel(t: TFunction, errorClass?: string) {
  const labels: Record<string, string> = {
    invalid_base_url: t('Invalid channel base URL'),
    credential_unavailable: t('Channel credential is unavailable'),
    unsupported_channel_type: t('Unsupported channel type'),
    rate_limit: t('Upstream rate limit'),
    authentication_error: t('Upstream authentication failed'),
    timeout: t('Upstream request timed out'),
  }
  return (
    (errorClass && labels[errorClass]) ||
    errorClass ||
    t('Scan failed before risk could be determined')
  )
}

function evidenceTitle(t: TFunction, evidence: PurityEvidence) {
  const labels = {
    protocol: t('Protocol response'),
    declared_model: t('Declared model'),
    usage: t('Usage metadata'),
    warning: t('Warning'),
    operational: t('Operational status'),
    generic: evidence.title ?? t('Evidence item'),
  }
  return labels[evidence.kind]
}

export function ChannelPurity() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [detail, setDetail] = useState<PurityResult | null>(null)

  const resultsQuery = useQuery({
    queryKey: resultKey,
    queryFn: getPurityResults,
  })
  const settingsQuery = useQuery({
    queryKey: settingsKey,
    queryFn: getPuritySettings,
  })
  const statusQuery = useQuery({
    queryKey: statusKey,
    queryFn: getPurityRunStatus,
    refetchInterval: (query) => {
      const status = query.state.data?.status
      return status === 'pending' || status === 'running' ? 3000 : 30_000
    },
  })
  const detailScanId = detail?.scan_id
  const detailQuery = useQuery({
    queryKey: ['qiq', 'channel-purity', 'scan', detailScanId],
    queryFn: () => getPurityScan(String(detailScanId)),
    enabled: detailScanId !== undefined,
    retry: false,
  })

  const refreshAll = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: resultKey }),
      queryClient.invalidateQueries({ queryKey: statusKey }),
    ])
  }
  const fullScanMutation = useMutation({
    mutationFn: startPurityFullScan,
    onSuccess: async (response) => {
      if (response.success === false) {
        toast.error(response.message || t('Failed to start full purity scan'))
        return
      }
      toast.success(t('Full channel purity scan started'))
      await refreshAll()
    },
    onError: (error) =>
      toast.error(
        isUnauthorizedError(error)
          ? t('Your session has expired. Please sign in again.')
          : apiErrorMessage(error) || t('Failed to start full purity scan')
      ),
  })
  const settingsMutation = useMutation({
    mutationFn: updatePuritySettings,
    onSuccess: (settings) => {
      queryClient.setQueryData(settingsKey, settings)
      toast.success(t('Automatic inspection settings updated'))
    },
    onError: (error) =>
      toast.error(
        isUnauthorizedError(error)
          ? t('Your session has expired. Please sign in again.')
          : apiErrorMessage(error) ||
              t('Failed to update automatic inspection settings')
      ),
  })

  const settings = settingsQuery.data
  const run = statusQuery.data
  const running = run?.status === 'pending' || run?.status === 'running'
  const total = run?.model_combinations ?? 0
  const processed = Math.min(total, run?.completed ?? 0)
  const progress = total > 0 ? Math.round((processed / total) * 100) : 0
  const updateSettings = (patch: Partial<PuritySettings>) => {
    if (!settings) return
    settingsMutation.mutate({ ...settings, ...patch })
  }
  const results = resultsQuery.data ?? []
  const detailResult = detailQuery.data ?? detail

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Channel purity')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='space-y-4'>
          <div className='flex flex-wrap items-center justify-between gap-3'>
            <p className='text-muted-foreground max-w-2xl text-sm'>
              {t(
                'Automatically inspect every enabled channel and configured model, then review failures and supporting evidence.'
              )}
            </p>
            <div className='flex gap-2'>
              <Button variant='outline' onClick={() => void refreshAll()}>
                <RefreshCw
                  className={statusQuery.isFetching ? 'animate-spin' : ''}
                />
                {t('Refresh')}
              </Button>
              <Button
                onClick={() => fullScanMutation.mutate()}
                disabled={running || fullScanMutation.isPending}
              >
                <Search />
                {running || fullScanMutation.isPending
                  ? t('Full scan in progress')
                  : t('Scan all now')}
              </Button>
            </div>
          </div>

          {resultsQuery.isError ? (
            <ErrorPanel
              error={resultsQuery.error}
              fallback={t('Failed to load purity scan results.')}
              onRetry={() => void resultsQuery.refetch()}
            />
          ) : null}
          {statusQuery.isError ? (
            <ErrorPanel
              error={statusQuery.error}
              fallback={t('Failed to load automatic inspection status.')}
              onRetry={() => void statusQuery.refetch()}
            />
          ) : null}

          <div className='grid gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,2fr)]'>
            <Card>
              <CardHeader>
                <CardTitle>{t('Automatic inspection')}</CardTitle>
              </CardHeader>
              <CardContent className='space-y-5'>
                {settingsQuery.isError ? (
                  <ErrorPanel
                    error={settingsQuery.error}
                    fallback={t(
                      'Failed to load automatic inspection settings.'
                    )}
                    onRetry={() => void settingsQuery.refetch()}
                  />
                ) : (
                  <>
                    <div className='flex items-center justify-between gap-3'>
                      <div>
                        <p className='text-sm font-medium'>
                          {t('Enable scheduled inspection')}
                        </p>
                        <p className='text-muted-foreground text-xs'>
                          {t(
                            'Run a full inspection automatically at the configured interval.'
                          )}
                        </p>
                      </div>
                      <Switch
                        checked={settings?.enabled ?? false}
                        disabled={!settings || settingsMutation.isPending}
                        onCheckedChange={(checked) =>
                          updateSettings({ enabled: checked })
                        }
                        aria-label={t('Enable scheduled inspection')}
                      />
                    </div>
                    <div className='space-y-2'>
                      <p className='text-sm font-medium'>
                        {t('Inspection interval')}
                      </p>
                      <Select
                        value={String(settings?.interval_minutes ?? 1440)}
                        disabled={!settings || settingsMutation.isPending}
                        onValueChange={(value) =>
                          updateSettings({ interval_minutes: Number(value) })
                        }
                      >
                        <SelectTrigger className='w-full'>
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          {intervalOptions.map((minutes) => (
                            <SelectItem key={minutes} value={String(minutes)}>
                              {t('{{count}} minutes', { count: minutes })}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>
                    <div className='grid grid-cols-2 gap-3 text-sm'>
                      <div>
                        <p className='text-muted-foreground'>{t('Last run')}</p>
                        <p>{formatTimestamp(run?.last_run_at)}</p>
                      </div>
                      <div>
                        <p className='text-muted-foreground'>{t('Next run')}</p>
                        <p>
                          {settings?.enabled
                            ? formatTimestamp(run?.next_run_at)
                            : t('Disabled')}
                        </p>
                      </div>
                    </div>
                  </>
                )}
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle>{t('Inspection progress and coverage')}</CardTitle>
              </CardHeader>
              <CardContent className='space-y-4'>
                <div className='flex items-center justify-between gap-3'>
                  <StatusBadge status={run?.status ?? 'unknown'} />
                  <span className='text-muted-foreground text-sm'>
                    {processed}/{total}
                  </span>
                </div>
                <Progress value={progress} />
                <div className='grid gap-3 sm:grid-cols-4'>
                  <SummaryCard
                    title={t('Enabled channels')}
                    value={run?.enabled_channels ?? 0}
                    icon={<ShieldAlert className='size-4' />}
                  />
                  <SummaryCard
                    title={t('Model combinations')}
                    value={total}
                    icon={<Search className='size-4' />}
                  />
                  <SummaryCard
                    title={t('Completed')}
                    value={run?.completed ?? 0}
                    icon={<RefreshCw className='size-4' />}
                  />
                  <SummaryCard
                    title={t('Failed')}
                    value={run?.failed ?? 0}
                    icon={<Clock className='size-4' />}
                    danger={Boolean(run?.failed)}
                  />
                </div>
                {run?.error ? (
                  <div className='border-destructive/40 bg-destructive/5 rounded-md border p-3'>
                    <p className='text-destructive text-sm font-medium'>
                      {t('Inspection failed')}
                    </p>
                    <p className='text-muted-foreground mt-1 text-xs'>
                      {run.error}
                    </p>
                  </div>
                ) : null}
              </CardContent>
            </Card>
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
                    const failed =
                      result.status === 'failed' || Boolean(result.error_class)
                    return (
                      <TableRow key={result.id}>
                        <TableCell className='font-medium'>
                          {result.channel_name ?? `#${result.channel_id}`}
                        </TableCell>
                        <TableCell className='font-mono text-xs'>
                          {result.model}
                        </TableCell>
                        <TableCell>
                          {failed ? (
                            <div className='space-y-1'>
                              <Badge variant='destructive'>
                                {t('Risk not determined')}
                              </Badge>
                              <p className='text-destructive max-w-52 text-xs'>
                                {operationalErrorLabel(t, result.error_class)}
                              </p>
                            </div>
                          ) : (
                            <RiskBadge risk={result.risk} />
                          )}
                        </TableCell>
                        <TableCell>
                          {failed ? (
                            <span className='text-muted-foreground text-xs'>
                              {t('Not available')}
                            </span>
                          ) : (
                            <div className='flex min-w-28 items-center gap-2'>
                              <Progress value={coverage} />
                              <span className='text-xs'>{coverage}%</span>
                            </div>
                          )}
                        </TableCell>
                        <TableCell>
                          <StatusBadge
                            status={failed ? 'failed' : result.status}
                          />
                        </TableCell>
                        <TableCell className='text-muted-foreground text-xs whitespace-nowrap'>
                          {formatTimestamp(
                            result.updated_at ?? result.created_at
                          )}
                        </TableCell>
                        <TableCell className='text-right'>
                          <Button
                            size='sm'
                            variant='ghost'
                            disabled={result.scan_id === undefined}
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

        <Dialog
          open={detail !== null}
          onOpenChange={(open) => !open && setDetail(null)}
        >
          <DialogContent className='sm:max-w-2xl'>
            <DialogHeader>
              <DialogTitle>{t('Purity evidence')}</DialogTitle>
              <DialogDescription>
                {detailResult?.summary ||
                  t('Signals and observations collected during this scan.')}
              </DialogDescription>
            </DialogHeader>
            <div className='max-h-[60vh] space-y-3 overflow-y-auto'>
              {detailQuery.isLoading ? (
                <p className='text-muted-foreground py-8 text-center text-sm'>
                  {t('Loading...')}
                </p>
              ) : null}
              {detailQuery.isError ? (
                <ErrorPanel
                  error={detailQuery.error}
                  fallback={t('Failed to load purity scan details.')}
                  onRetry={() => void detailQuery.refetch()}
                />
              ) : null}
              {!detailQuery.isLoading && !detailQuery.isError
                ? detailResult?.evidence?.map((evidence) => (
                    <div
                      key={evidence.id}
                      className='bg-muted/40 rounded-lg border p-3'
                    >
                      <p className='font-medium'>
                        {evidenceTitle(t, evidence)}
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
                  ))
                : null}
              {!detailQuery.isLoading &&
              !detailQuery.isError &&
              !detailResult?.evidence?.length ? (
                <p className='text-muted-foreground py-8 text-center text-sm'>
                  {t('No evidence was returned for this scan.')}
                </p>
              ) : null}
            </div>
          </DialogContent>
        </Dialog>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

function SummaryCard(props: {
  title: string
  value: string | number
  icon: ReactNode
  danger?: boolean
}) {
  return (
    <div className='bg-muted/30 flex items-center justify-between rounded-lg border p-3'>
      <div>
        <p className='text-muted-foreground text-xs'>{props.title}</p>
        <p
          className={
            props.danger
              ? 'text-destructive mt-1 text-xl font-semibold'
              : 'mt-1 text-xl font-semibold'
          }
        >
          {props.value}
        </p>
      </div>
      <div className='bg-muted rounded-full p-2'>{props.icon}</div>
    </div>
  )
}
