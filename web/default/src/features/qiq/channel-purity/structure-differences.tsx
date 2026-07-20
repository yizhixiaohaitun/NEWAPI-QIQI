/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or (at your option) any later version.
*/
import type { TFunction } from 'i18next'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'

import {
  dimensionDifferenceKind,
  fieldDifferenceKind,
  summarizeDimensionDifferences,
  summarizeFieldDifferences,
} from './structure-difference-state'
import type { DimensionDifferenceKind, FieldDifferenceKind } from './structure-difference-state'
import type { StructureDimensionDifference, StructureSimilarityDetail } from './types'

function kindLabel(t: TFunction, kind: FieldDifferenceKind | DimensionDifferenceKind) {
  switch (kind) {
    case 'missing': return t('Missing from target')
    case 'added': return t('Added by target')
    case 'type': return t('Type changed')
    default: return t('Sample presence changed')
  }
}

function kindStyle(kind: FieldDifferenceKind | DimensionDifferenceKind) {
  switch (kind) {
    case 'missing': return 'border-destructive/40 bg-destructive/10 text-destructive'
    case 'added': return 'border-blue-500/40 bg-blue-500/10 text-blue-700 dark:text-blue-300'
    case 'type': return 'border-orange-500/40 bg-orange-500/10 text-orange-700 dark:text-orange-300'
    default: return 'border-amber-500/40 bg-amber-500/10 text-amber-700 dark:text-amber-300'
  }
}

function typesLabel(t: TFunction, values?: string[], legacyValue?: string) {
  const types = values?.length ? values : legacyValue ? [legacyValue] : []
  if (!types.length) return t('Not present')
  return types.map((value) => t(`JSON type: ${value}`, { defaultValue: value })).join(' / ')
}

function dimensionLabel(t: TFunction, difference: StructureDimensionDifference) {
  switch (difference.dimension) {
    case 'protocol': return t('Response protocol')
    case 'model_family': return t('Model family')
    case 'event_sequence': return t('SSE event sequence')
    case 'event': return t('SSE event')
    case 'finish_reason': return t('Finish reason')
    case 'header_presence': return t('Response header presence')
    case 'metadata': return t('Response metadata')
    default: return difference.dimension
  }
}

function dimensionValue(t: TFunction, difference: StructureDimensionDifference) {
  if (difference.value === 'signature_id_present') return t('Signature ID present')
  if (difference.dimension === 'protocol') return difference.value.toUpperCase()
  return difference.value
}

export function StructureDifferencePanel({ detail, pairedSamples, minimumSamples }: { detail?: StructureSimilarityDetail; pairedSamples: number; minimumSamples: number }) {
  const { t } = useTranslation()
  const differences = detail?.field_differences ?? []
  const dimensionDifferences = detail?.dimension_differences ?? []
  const summary = summarizeFieldDifferences(differences)
  const dimensionSummary = summarizeDimensionDifferences(dimensionDifferences)
  const provisional = pairedSamples < minimumSamples

  return <section className='space-y-4 rounded-lg border p-3'>
    <div>
      <h3 className='font-medium'>{t('Where the response structure differs')}</h3>
      <p className='text-muted-foreground mt-1 text-xs'>{t('This section shows sanitized field paths, JSON types, and protocol metadata only. Response values, content, reasoning, identifiers, and header values are never stored.')}</p>
      {provisional ? <p className='mt-2 rounded bg-amber-500/10 p-2 text-xs text-amber-800 dark:text-amber-200'>{t('Only {{paired}} / {{required}} paired samples are available. The differences below are provisional and are not yet a health conclusion.', { paired: pairedSamples, required: minimumSamples })}</p> : null}
    </div>

    {differences.length ? <div className='space-y-3'>
      <div>
        <h4 className='text-sm font-medium'>{t('Field path and type differences')}</h4>
        <div className='mt-2 flex flex-wrap gap-2 text-xs'>
          {summary.missing ? <Badge variant='outline' className={kindStyle('missing')}>{t('{{count}} missing from target', { count: summary.missing })}</Badge> : null}
          {summary.added ? <Badge variant='outline' className={kindStyle('added')}>{t('{{count}} added by target', { count: summary.added })}</Badge> : null}
          {summary.type ? <Badge variant='outline' className={kindStyle('type')}>{t('{{count}} type changes', { count: summary.type })}</Badge> : null}
          {summary.frequency ? <Badge variant='outline' className={kindStyle('frequency')}>{t('{{count}} sample-presence changes', { count: summary.frequency })}</Badge> : null}
        </div>
      </div>
      <div className='max-h-72 space-y-2 overflow-auto pr-1'>
        {differences.map((difference) => {
          const kind = fieldDifferenceKind(difference)
          return <div key={`${difference.path}:${difference.baseline_types?.join(',') ?? difference.baseline_type}:${difference.target_types?.join(',') ?? difference.target_type}`} className='rounded-md border p-3'>
            <div className='flex flex-wrap items-start justify-between gap-2'>
              <code className='break-all text-xs font-medium'>{difference.path}</code>
              <Badge variant='outline' className={kindStyle(kind)}>{kindLabel(t, kind)}</Badge>
            </div>
            <div className='text-muted-foreground mt-2 grid gap-1 text-xs sm:grid-cols-[1fr_auto_1fr] sm:items-center'>
              <span>{t('Baseline')}: <strong className='text-foreground'>{typesLabel(t, difference.baseline_types, difference.baseline_type)}</strong> · {t('seen in {{count}} samples', { count: difference.baseline_count })}</span>
              <span aria-hidden='true'>→</span>
              <span>{t('Target')}: <strong className='text-foreground'>{typesLabel(t, difference.target_types, difference.target_type)}</strong> · {t('seen in {{count}} samples', { count: difference.target_count })}</span>
            </div>
          </div>
        })}
      </div>
    </div> : detail?.field_paths_available ? <p className='rounded bg-emerald-500/10 p-2 text-sm text-emerald-800 dark:text-emerald-200'>{t('No field-path or field-type difference was found in this window.')}</p> : <p className='rounded bg-amber-500/10 p-2 text-sm text-amber-800 dark:text-amber-200'>{t(detail?.limitation || 'Exact field differences are unavailable for these samples. Run a new detection after upgrading to collect sanitized field paths and types.')}</p>}

    {dimensionDifferences.length ? <div className='space-y-3 border-t pt-3'>
      <div>
        <h4 className='text-sm font-medium'>{t('Protocol and stream metadata differences')}</h4>
        <p className='text-muted-foreground mt-1 text-xs'>{t('These differences explain structural signature changes that do not come from JSON field paths, such as JSON versus SSE, event order, finish reasons, and header presence.')}</p>
        <div className='mt-2 flex flex-wrap gap-2 text-xs'>
          {dimensionSummary.missing ? <Badge variant='outline' className={kindStyle('missing')}>{t('{{count}} baseline-only metadata items', { count: dimensionSummary.missing })}</Badge> : null}
          {dimensionSummary.added ? <Badge variant='outline' className={kindStyle('added')}>{t('{{count}} target-only metadata items', { count: dimensionSummary.added })}</Badge> : null}
          {dimensionSummary.frequency ? <Badge variant='outline' className={kindStyle('frequency')}>{t('{{count}} metadata frequency changes', { count: dimensionSummary.frequency })}</Badge> : null}
        </div>
      </div>
      <div className='max-h-64 space-y-2 overflow-auto pr-1'>
        {dimensionDifferences.map((difference) => {
          const kind = dimensionDifferenceKind(difference)
          return <div key={`${difference.dimension}:${difference.value}:${difference.change}`} className='rounded-md border p-3'>
            <div className='flex flex-wrap items-start justify-between gap-2'>
              <div><p className='text-xs font-medium'>{dimensionLabel(t, difference)}</p><code className='text-muted-foreground mt-1 block break-all text-xs'>{dimensionValue(t, difference)}</code></div>
              <Badge variant='outline' className={kindStyle(kind)}>{kindLabel(t, kind)}</Badge>
            </div>
            <p className='text-muted-foreground mt-2 text-xs'>{t('Baseline samples')}: {difference.baseline_count} · {t('Target samples')}: {difference.target_count}</p>
          </div>
        })}
      </div>
    </div> : null}

    {detail?.detail_available && !differences.length && !dimensionDifferences.length ? <p className='text-muted-foreground text-xs'>{t('No explainable field, protocol, or stream-metadata difference was found in this window.')}</p> : null}
  </section>
}
