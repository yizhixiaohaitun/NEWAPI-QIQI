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
import { ExternalLink } from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { formatTimestampToDate } from '@/lib/format'

import { taskActionMapper, taskStatusMapper } from '../../lib/mappers'
import type { TaskLog } from '../../types'

const SECRET_KEY_PATTERN =
  /authorization|apikey|accesstoken|refreshtoken|secret|password|credential|cookie/i
const URL_PATTERN = /https?:\/\/[^\s"'<>]+/g

function isSecretKey(key: string): boolean {
  return SECRET_KEY_PATTERN.test(key.replace(/[^a-z0-9]/gi, ''))
}

function parseUnknown(value: unknown): unknown {
  if (typeof value !== 'string') return value
  const trimmed = value.trim()
  if (!trimmed) return undefined
  try {
    return JSON.parse(trimmed)
  } catch {
    return value
  }
}

function asRecord(value: unknown): Record<string, unknown> | undefined {
  const parsed = parseUnknown(value)
  return parsed && typeof parsed === 'object' && !Array.isArray(parsed)
    ? (parsed as Record<string, unknown>)
    : undefined
}

function getPrompt(log: TaskLog): string | undefined {
  const properties = asRecord(log.properties)
  if (typeof properties?.input === 'string' && properties.input.trim()) {
    return properties.input.trim()
  }
  const data = asRecord(log.data)
  for (const key of ['prompt', 'input', 'text']) {
    const value = data?.[key]
    if (typeof value === 'string' && value.trim()) {
      return value.trim()
    }
  }
  return undefined
}

function getModel(log: TaskLog): string | undefined {
  const properties = asRecord(log.properties)
  for (const key of ['origin_model_name', 'upstream_model_name', 'model']) {
    const value = properties?.[key]
    if (typeof value === 'string' && value) {
      return value
    }
  }
  return undefined
}

function sanitizeResource(value: string): string {
  const trimmed = value.trim()
  try {
    const url = new URL(trimmed)
    if (url.protocol !== 'http:' && url.protocol !== 'https:') return trimmed
    for (const key of [...url.searchParams.keys()]) {
      if (isSecretKey(key)) url.searchParams.set(key, '[REDACTED]')
    }
    return url.toString()
  } catch {
    return trimmed
  }
}

function collectResources(
  value: unknown,
  resources: string[],
  depth = 0,
  resourceContext = false
): void {
  if (depth > 8 || value == null) return
  const parsed = depth === 0 ? parseUnknown(value) : value
  if (Array.isArray(parsed)) {
    parsed.forEach((item) =>
      collectResources(item, resources, depth + 1, resourceContext)
    )
    return
  }
  if (parsed && typeof parsed === 'object') {
    Object.entries(parsed as Record<string, unknown>).forEach(([key, item]) => {
      if (isSecretKey(key)) return
      const normalizedKey = key.replace(/[^a-z0-9]/gi, '')
      const isResourceField =
        !/result|output/i.test(normalizedKey) &&
        /reference|resource|images?|firstimage|lastimage/i.test(normalizedKey)
      collectResources(
        item,
        resources,
        depth + 1,
        resourceContext || isResourceField
      )
    })
    return
  }
  if (typeof parsed !== 'string' || !resourceContext) return
  if (/^(?:assetId|file):\/\//i.test(parsed.trim())) {
    resources.push(sanitizeResource(parsed))
  }
  for (const match of parsed.match(URL_PATTERN) || []) {
    resources.push(sanitizeResource(match.replace(/[),.;]+$/, '')))
  }
}

function getReferenceResources(log: TaskLog, resultUrl?: string): string[] {
  const properties = asRecord(log.properties)
  const resources: string[] = []
  collectResources(properties?.reference_resources, resources, 0, true)
  collectResources(log.data, resources)

  const sanitizedResult = resultUrl ? sanitizeResource(resultUrl) : undefined
  return [...new Set(resources)].filter((resource) => {
    if (resource === sanitizedResult) return false
    if (resource === sanitizeResource(log.result_url || '')) return false
    return resource !== sanitizeResource(log.fail_reason || '')
  })
}

function formatTime(value?: number): string {
  return value ? formatTimestampToDate(value, 'seconds') : '-'
}

function formatDuration(log: TaskLog): string {
  const start = log.start_time || log.submit_time
  if (!start || !log.finish_time) return '-'
  const seconds = Math.max(0, log.finish_time - start)
  return `${seconds.toFixed(seconds < 10 ? 2 : 0)} s`
}

function isHttpResource(value: string): boolean {
  return /^https?:\/\//i.test(value)
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
            ? 'min-w-0 break-all font-mono text-xs'
            : 'min-w-0 break-words text-xs'
        }
      >
        {empty ? '-' : value}
      </div>
    </div>
  )
}

export function TaskDetailsDialog({
  log,
  open,
  onOpenChange,
}: {
  log: TaskLog
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const model = getModel(log)
  const prompt = getPrompt(log)
  const resultCandidate =
    log.result_url ||
    (log.status === 'SUCCESS' && /^https?:\/\//i.test(log.fail_reason || '')
      ? log.fail_reason
      : undefined)
  const resultUrl = resultCandidate
    ? sanitizeResource(resultCandidate)
    : undefined
  const referenceResources = getReferenceResources(log, resultCandidate)
  const errorMessage =
    log.fail_reason && log.fail_reason !== resultCandidate
      ? log.fail_reason
      : undefined

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('Details')}
      contentClassName='sm:max-w-2xl'
      contentHeight='auto'
      bodyClassName='space-y-4'
    >
      <div className='space-y-5 py-2'>
        <div className='space-y-2 rounded-md border bg-muted/30 p-3'>
          <DetailRow label={t('Task ID')} value={log.task_id} mono />
          <DetailRow
            label={t('Type')}
            value={`${t(log.platform || '-')} · ${t(taskActionMapper.getLabel(log.action))}`}
          />
          <DetailRow label={t('Model')} value={model} />
          <DetailRow
            label={t('Channel')}
            value={log.channel_id > 0 ? log.channel_id : undefined}
          />
          <DetailRow
            label={t('Status')}
            value={t(
              taskStatusMapper.getLabel(log.status, log.status || 'Unknown')
            )}
          />
          <DetailRow label={t('Progress')} value={log.progress} />
          <DetailRow
            label={t('Submit Time')}
            value={formatTime(log.submit_time)}
            mono
          />
          <DetailRow
            label={t('Start Time')}
            value={formatTime(log.start_time)}
            mono
          />
          <DetailRow
            label={t('Finish Time')}
            value={formatTime(log.finish_time)}
            mono
          />
          <DetailRow label={t('Duration')} value={formatDuration(log)} mono />
        </div>

        {errorMessage && (
          <DetailRow label={t('Error Message')} value={errorMessage} />
        )}
        {prompt && (
          <DetailRow
            label={t('Prompt')}
            value={<span className='whitespace-pre-wrap'>{prompt}</span>}
          />
        )}
        {referenceResources.length > 0 && (
          <DetailRow
            label={t('Reference')}
            value={
              <div className='space-y-1'>
                {referenceResources.map((resource) =>
                  isHttpResource(resource) ? (
                    <a
                      key={resource}
                      href={resource}
                      target='_blank'
                      rel='noopener noreferrer'
                      className='block break-all text-primary hover:underline'
                    >
                      {resource}
                    </a>
                  ) : (
                    <div key={resource} className='break-all font-mono'>
                      {resource}
                    </div>
                  )
                )}
              </div>
            }
          />
        )}
        {resultUrl && (
          <DetailRow
            label={t('Result')}
            value={
              <a
                href={resultUrl}
                target='_blank'
                rel='noopener noreferrer'
                className='inline-flex max-w-full items-start gap-1 break-all text-primary hover:underline'
              >
                <span>{resultUrl}</span>
                <ExternalLink className='mt-0.5 size-3 shrink-0' />
              </a>
            }
          />
        )}
      </div>
    </Dialog>
  )
}
