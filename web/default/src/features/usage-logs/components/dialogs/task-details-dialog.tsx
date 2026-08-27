/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useQuery } from '@tanstack/react-query'
import { ExternalLink, Loader2 } from 'lucide-react'
import { useMemo, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { formatTimestampToDate } from '@/lib/format'

import { getTaskDetail } from '../../api'
import { taskActionMapper, taskStatusMapper } from '../../lib/mappers'
import type { TaskDetail, TaskLog } from '../../types'

const DETAIL_POLL_INTERVAL_MS = 3_000
const ACTIVE_TASK_STATUSES = new Set([
  'NOT_START',
  'SUBMITTED',
  'QUEUED',
  'IN_PROGRESS',
])

function isActiveTaskStatus(status?: string): boolean {
  return ACTIVE_TASK_STATUSES.has(String(status || '').toUpperCase())
}

function asRecord(value: unknown): Record<string, unknown> | undefined {
  if (!value) return undefined
  if (typeof value === 'string') {
    try {
      const parsed = JSON.parse(value)
      return parsed && typeof parsed === 'object' && !Array.isArray(parsed)
        ? (parsed as Record<string, unknown>)
        : undefined
    } catch {
      return undefined
    }
  }
  return typeof value === 'object' && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : undefined
}

function formatTime(value?: number): string {
  return value ? formatTimestampToDate(value, 'seconds') : '-'
}

function formatElapsedTime(log: TaskLog): string {
  const start = log.start_time || log.submit_time
  if (!start || !log.finish_time) return '-'
  const seconds = Math.max(0, log.finish_time - start)
  return `${seconds.toFixed(seconds < 10 ? 2 : 0)} s`
}

function stringify(value: unknown): string | undefined {
  if (value === undefined || value === null || value === '') return undefined
  if (typeof value === 'string') {
    try {
      return JSON.stringify(JSON.parse(value), null, 2)
    } catch {
      return value
    }
  }
  return JSON.stringify(value, null, 2)
}

function requestValue(
  snapshot: Record<string, unknown> | undefined,
  input: Record<string, unknown> | undefined,
  ...keys: string[]
): unknown {
  for (const source of [input, snapshot]) {
    for (const key of keys) {
      if (source?.[key] !== undefined && source[key] !== null) {
        return source[key]
      }
    }
  }
  return undefined
}

function resourceText(value: unknown): string | undefined {
  if (typeof value === 'string' && value.trim()) return value.trim()
  const record = asRecord(value)
  if (!record) return undefined
  for (const key of [
    'url',
    'image_url',
    'video_url',
    'audio_url',
    'uri',
    'asset_id',
  ]) {
    const nested = record[key]
    if (typeof nested === 'string' && nested.trim()) return nested.trim()
    const nestedRecord = asRecord(nested)
    if (typeof nestedRecord?.url === 'string' && nestedRecord.url.trim()) {
      return nestedRecord.url.trim()
    }
  }
  return stringify(record)
}

function collectResourceValues(value: unknown, output: string[]) {
  const values = Array.isArray(value) ? value : [value]
  for (const item of values) {
    const text = resourceText(item)
    if (text) output.push(text)
  }
}

function requestResources(
  snapshot?: Record<string, unknown>,
  input?: Record<string, unknown>
): string[] {
  const values: string[] = []
  const resourceKeys = [
    'images',
    'image_urls',
    'image_references',
    'reference_images',
    'video_references',
    'reference_videos',
    'audio_references',
    'reference_audios',
    'reference_resources',
    'first_image',
    'last_image',
    'start_frames',
    'end_frames',
  ]

  for (const source of [input, snapshot]) {
    for (const key of resourceKeys) {
      if (source?.[key] !== undefined) {
        collectResourceValues(source[key], values)
      }
    }
  }

  const content = snapshot?.content
  if (Array.isArray(content)) {
    for (const item of content) {
      const record = asRecord(item)
      if (!record) continue
      for (const key of ['image_url', 'video_url', 'audio_url']) {
        if (record[key] !== undefined) {
          collectResourceValues(record[key], values)
        }
      }
    }
  }

  return [...new Set(values)]
}

function displayValue(value: unknown): ReactNode {
  if (value === undefined || value === null || value === '') return undefined
  if (typeof value === 'boolean') return value ? 'true' : 'false'
  if (typeof value === 'string' || typeof value === 'number') {
    return String(value)
  }
  return stringify(value)
}

function DetailRow({
  label,
  value,
  mono = false,
}: {
  label: ReactNode
  value: ReactNode
  mono?: boolean
}) {
  const empty = value === undefined || value === null || value === ''
  return (
    <div className='grid min-w-0 grid-cols-[7rem_minmax(0,1fr)] gap-3 text-sm'>
      <span className='text-muted-foreground text-xs'>{label}</span>
      <div
        className={
          mono
            ? 'min-w-0 font-mono text-xs break-all'
            : 'min-w-0 text-xs break-words'
        }
      >
        {empty ? '-' : value}
      </div>
    </div>
  )
}

function JsonBlock({ value }: { value: unknown }) {
  const content = stringify(value)
  if (!content) return <span className='text-muted-foreground'>-</span>
  return (
    <pre className='bg-muted/40 max-h-64 overflow-auto rounded-md border p-3 font-mono text-[11px] break-all whitespace-pre-wrap'>
      {content}
    </pre>
  )
}

export function TaskDetailsDialog({
  log,
  open,
  isAdmin,
  onOpenChange,
}: {
  log: TaskLog
  open: boolean
  isAdmin: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const detailQuery = useQuery({
    queryKey: ['usage-logs', 'task-detail', isAdmin, log.task_id],
    enabled: open && Boolean(log.task_id),
    queryFn: async (): Promise<TaskDetail> => {
      const response = await getTaskDetail(log.task_id, isAdmin)
      if (!response?.success || !response.data) {
        throw new Error(response?.message || t('Request failed'))
      }
      return response.data as TaskDetail
    },
    refetchInterval: (query) =>
      open &&
      isActiveTaskStatus(
        (query.state.data as TaskDetail | undefined)?.status || log.status
      )
        ? DETAIL_POLL_INTERVAL_MS
        : false,
    refetchIntervalInBackground: false,
  })

  const detail = detailQuery.data
  const snapshot = useMemo(
    () => asRecord(detail?.request_snapshot),
    [detail?.request_snapshot]
  )
  const input = useMemo(() => asRecord(snapshot?.input), [snapshot])
  const properties = asRecord(detail?.properties)
  const model =
    displayValue(requestValue(snapshot, input, 'model')) ||
    displayValue(properties?.origin_model_name) ||
    displayValue(properties?.upstream_model_name)
  const prompt =
    displayValue(requestValue(snapshot, input, 'prompt', 'text')) ||
    displayValue(properties?.input)
  const resources = requestResources(snapshot, input)
  const resultUrl = detail?.result_url
  const errorMessage =
    detail?.fail_reason && detail.fail_reason !== resultUrl
      ? detail.fail_reason
      : undefined

  let resourcesValue: ReactNode = t('Not persisted for this historical task')
  if (detail?.detail_source === 'normalized_upstream_request') {
    resourcesValue = t('None')
  }
  if (resources.length > 0) {
    resourcesValue = (
      <div className='space-y-1'>
        {resources.map((resource) =>
          /^https?:\/\//i.test(resource) ? (
            <a
              key={resource}
              href={resource}
              target='_blank'
              rel='noopener noreferrer'
              className='text-primary block break-all hover:underline'
            >
              {resource}
            </a>
          ) : (
            <div key={resource} className='font-mono break-all'>
              {resource}
            </div>
          )
        )}
      </div>
    )
  }

  let resultValue: ReactNode = '-'
  if (detail && isActiveTaskStatus(detail.status)) {
    resultValue = t('Waiting for generation')
  }
  if (resultUrl) {
    resultValue = (
      <a
        href={resultUrl}
        target='_blank'
        rel='noopener noreferrer'
        className='text-primary inline-flex max-w-full items-start gap-1 break-all hover:underline'
      >
        <span>{resultUrl}</span>
        <ExternalLink className='mt-0.5 size-3 shrink-0' />
      </a>
    )
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('Details')}
      contentClassName='sm:max-w-3xl'
      contentHeight='auto'
      bodyClassName='space-y-4'
    >
      {detailQuery.isLoading && !detail && (
        <div className='text-muted-foreground flex items-center justify-center gap-2 py-12 text-sm'>
          <Loader2 className='size-4 animate-spin' />
          {t('Loading')}
        </div>
      )}
      {detailQuery.error && !detail && (
        <div className='border-destructive/40 bg-destructive/5 text-destructive rounded-md border p-3 text-sm'>
          {detailQuery.error instanceof Error
            ? detailQuery.error.message
            : t('Request failed')}
        </div>
      )}
      {detail && (
        <div className='space-y-5 py-2'>
          {detail.detail_source !== 'normalized_upstream_request' && (
            <div className='rounded-md border border-amber-500/40 bg-amber-500/10 p-3 text-xs text-amber-800 dark:text-amber-200'>
              {detail.detail_source === 'legacy_partial'
                ? t(
                    'This is a historical task. Only fields actually saved at submission are shown; the complete normalized request was not persisted.'
                  )
                : t(
                    'This historical task did not persist its request details. Metadata and available response data are shown below.'
                  )}
            </div>
          )}

          <div className='bg-muted/30 space-y-2 rounded-md border p-3'>
            <DetailRow label={t('Task ID')} value={detail.task_id} mono />
            <DetailRow
              label={t('Type')}
              value={`${t(detail.platform || '-')} · ${t(taskActionMapper.getLabel(detail.action))}`}
            />
            <DetailRow label={t('Model')} value={model} />
            {isAdmin && (
              <DetailRow
                label={t('Channel')}
                value={detail.channel_id > 0 ? detail.channel_id : undefined}
              />
            )}
            <DetailRow
              label={t('Status')}
              value={t(
                taskStatusMapper.getLabel(
                  detail.status,
                  detail.status || 'Unknown'
                )
              )}
            />
            <DetailRow
              label={t('Progress')}
              value={
                <span className='inline-flex items-center gap-1.5'>
                  {detail.progress || '-'}
                  {detailQuery.isFetching &&
                    isActiveTaskStatus(detail.status) && (
                      <Loader2 className='text-muted-foreground size-3 animate-spin' />
                    )}
                </span>
              }
            />
            <DetailRow
              label={t('Submit Time')}
              value={formatTime(detail.submit_time)}
              mono
            />
            <DetailRow
              label={t('Start Time')}
              value={formatTime(detail.start_time)}
              mono
            />
            <DetailRow
              label={t('Finish Time')}
              value={formatTime(detail.finish_time)}
              mono
            />
            <DetailRow
              label={t('Elapsed Time')}
              value={formatElapsedTime(detail)}
              mono
            />
          </div>

          <div className='space-y-2 rounded-md border p-3'>
            <div className='text-sm font-medium'>{t('Invocation')}</div>
            <DetailRow label={t('Prompt')} value={prompt} />
            <DetailRow
              label={t('Duration')}
              value={displayValue(
                requestValue(snapshot, input, 'duration', 'seconds')
              )}
            />
            <DetailRow
              label={t('Aspect Ratio')}
              value={displayValue(
                requestValue(snapshot, input, 'aspect_ratio', 'ratio')
              )}
            />
            <DetailRow
              label={t('Resolution')}
              value={displayValue(requestValue(snapshot, input, 'resolution'))}
            />
            <DetailRow
              label={t('Audio')}
              value={displayValue(
                requestValue(snapshot, input, 'audio', 'generate_audio')
              )}
            />
            <DetailRow
              label={t('Output Count')}
              value={displayValue(requestValue(snapshot, input, 'n'))}
            />
            <DetailRow label={t('Reference')} value={resourcesValue} />
          </div>

          {errorMessage && (
            <DetailRow label={t('Error Message')} value={errorMessage} />
          )}
          <DetailRow label={t('Result')} value={resultValue} />

          <div className='space-y-1.5'>
            <div className='text-muted-foreground text-xs'>
              {t('Normalized upstream request')}
            </div>
            <JsonBlock value={snapshot} />
          </div>
          <div className='space-y-1.5'>
            <div className='text-muted-foreground text-xs'>
              {t('Upstream response data')}
            </div>
            <JsonBlock value={detail.data} />
          </div>
        </div>
      )}
    </Dialog>
  )
}
