/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or (at your option) any later version.
*/
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CartesianGrid,
  Legend,
  Line,
  LineChart,
  ReferenceLine,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'

import { Button } from '@/components/ui/button'
import { ChartContainer, type ChartConfig } from '@/components/ui/chart'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

import type { DetectorStatus, TrendPoint } from './types'

type Range = 10 | 30 | 100

const chartConfig = {
  field_similarity: { label: 'Structure similarity', color: 'var(--chart-1)' },
  token_similarity: { label: 'Token similarity', color: 'var(--chart-2)' },
  confidence: { label: 'Confidence', color: 'var(--chart-3)' },
} satisfies ChartConfig

function timestamp(value: string | number) {
  const normalized = typeof value === 'number' && value < 1e12 ? value * 1000 : value
  const result = new Date(normalized).getTime()
  return Number.isFinite(result) ? result : 0
}

function shortTime(value: string | number) {
  const date = new Date(timestamp(value))
  if (Number.isNaN(date.getTime())) return '—'
  return date.toLocaleString(undefined, {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

function percent(value?: number | null) {
  if (value === undefined || value === null || !Number.isFinite(value)) return '—'
  return `${Math.round(Math.max(0, Math.min(1, value)) * 100)}%`
}

function statusColor(status: DetectorStatus) {
  if (status === 'ALERT' || status === 'DETECTOR_ERROR') return 'var(--destructive)'
  if (status === 'SUSPECT' || status === 'LOW_SAMPLE') return '#f59e0b'
  if (status === 'BASELINE_UNAVAILABLE' || status === 'NO_TRAFFIC') return '#64748b'
  if (status === 'WARMING_UP') return '#3b82f6'
  return '#10b981'
}

function StatusDot({ cx, cy, payload }: { cx?: number; cy?: number; payload?: { status?: DetectorStatus } }) {
  if (cx === undefined || cy === undefined || !payload?.status) return null
  return <circle cx={cx} cy={cy} r={4} fill={statusColor(payload.status)} stroke='var(--background)' strokeWidth={2} />
}

export function PurityTrendChart({
  points,
  suspectThreshold,
  alertThreshold,
}: {
  points: TrendPoint[]
  suspectThreshold?: number
  alertThreshold?: number
}) {
  const { t } = useTranslation()
  const [range, setRange] = useState<Range>(30)
  const data = useMemo(() => points.slice(-range).map((point) => ({
    ...point,
    timestamp: timestamp(point.at),
    label: shortTime(point.at),
  })), [points, range])

  if (!data.length) {
    return <p className='text-muted-foreground text-sm'>{t('Trend is unavailable until multiple detection windows are recorded.')}</p>
  }

  return <div className='space-y-3'>
    <div className='flex flex-wrap items-center justify-between gap-2'>
      <p className='text-muted-foreground text-xs'>{t('Missing values are left blank instead of being drawn as 0%. Status dots show the conclusion for each window.')}</p>
      <div className='flex rounded-lg bg-muted p-1'>
        {([10, 30, 100] as Range[]).map((value) => <Button key={value} size='sm' variant={range === value ? 'secondary' : 'ghost'} aria-pressed={range === value} onClick={() => setRange(value)}>{t('Last {{count}}', { count: value })}</Button>)}
      </div>
    </div>
    <ChartContainer config={chartConfig} className='h-72 w-full aspect-auto'>
      <LineChart data={data} margin={{ top: 12, right: 12, bottom: 8, left: 0 }} accessibilityLayer>
        <CartesianGrid vertical={false} />
        <XAxis dataKey='timestamp' type='number' scale='time' domain={['dataMin', 'dataMax']} tickFormatter={(value) => shortTime(Number(value))} minTickGap={32} />
        <YAxis domain={[0, 1]} tickFormatter={(value) => `${Math.round(Number(value) * 100)}%`} width={42} />
        <Tooltip
          labelFormatter={(value) => shortTime(Number(value))}
          formatter={(value, name) => [percent(Number(value)), t(String(chartConfig[String(name) as keyof typeof chartConfig]?.label ?? name))]}
          contentStyle={{ borderRadius: 8 }}
        />
        <Legend formatter={(value) => t(String(chartConfig[value as keyof typeof chartConfig]?.label ?? value))} />
        {suspectThreshold !== undefined ? <ReferenceLine y={suspectThreshold} stroke='#f59e0b' strokeDasharray='4 4' label={{ value: t('Suspect threshold'), fill: '#f59e0b', fontSize: 11 }} /> : null}
        {alertThreshold !== undefined ? <ReferenceLine y={alertThreshold} stroke='var(--destructive)' strokeDasharray='4 4' label={{ value: t('Alert threshold'), fill: 'var(--destructive)', fontSize: 11 }} /> : null}
        <Line type='monotone' dataKey='field_similarity' connectNulls={false} stroke='var(--color-field_similarity)' strokeWidth={2} dot={<StatusDot />} activeDot={{ r: 5 }} />
        <Line type='monotone' dataKey='token_similarity' connectNulls={false} stroke='var(--color-token_similarity)' strokeWidth={2} dot={false} activeDot={{ r: 5 }} />
        <Line type='monotone' dataKey='confidence' connectNulls={false} stroke='var(--color-confidence)' strokeWidth={1.5} strokeDasharray='5 4' dot={false} activeDot={{ r: 5 }} />
      </LineChart>
    </ChartContainer>
    <details>
      <summary className='cursor-pointer text-sm font-medium'>{t('Accessible trend table')}</summary>
      <div className='mt-2 max-h-64 overflow-auto rounded-lg border'>
        <Table>
          <TableHeader><TableRow><TableHead>{t('Window end')}</TableHead><TableHead>{t('Status')}</TableHead><TableHead>{t('Structure')}</TableHead><TableHead>{t('Token')}</TableHead><TableHead>{t('Confidence')}</TableHead></TableRow></TableHeader>
          <TableBody>{data.slice().reverse().map((point, index) => <TableRow key={`${point.timestamp}-${index}`}><TableCell>{point.label}</TableCell><TableCell>{t(`Purity detector status: ${point.status}`)}</TableCell><TableCell>{percent(point.field_similarity)}</TableCell><TableCell>{percent(point.token_similarity)}</TableCell><TableCell>{percent(point.confidence)}</TableCell></TableRow>)}</TableBody>
        </Table>
      </div>
    </details>
  </div>
}
