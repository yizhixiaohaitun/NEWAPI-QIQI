/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or (at your option) any later version.
*/
import type { TFunction } from 'i18next'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'

import { dimensionDifferenceKind, fieldDifferenceKind, summarizeDimensionDifferences, summarizeFieldDifferences } from './structure-difference-state'
import type { DimensionDifferenceKind, FieldDifferenceKind } from './structure-difference-state'
import type { StructureDimensionDifference, StructureSimilarityDetail } from './types'

function kindLabel(t: TFunction, kind: FieldDifferenceKind | DimensionDifferenceKind) {
  switch (kind) {
    case 'matched': return t('Consistent')
    case 'missing': return t('Missing from target')
    case 'added': return t('Added by target')
    case 'type': return t('Type changed')
    default: return t('Sample presence changed')
  }
}
function kindStyle(kind: FieldDifferenceKind | DimensionDifferenceKind) {
  switch (kind) {
    case 'matched': return 'border-emerald-500/40 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300'
    case 'missing': return 'border-destructive/40 bg-destructive/10 text-destructive'
    case 'added': return 'border-blue-500/40 bg-blue-500/10 text-blue-700 dark:text-blue-300'
    case 'type': return 'border-orange-500/40 bg-orange-500/10 text-orange-700 dark:text-orange-300'
    default: return 'border-amber-500/40 bg-amber-500/10 text-amber-700 dark:text-amber-300'
  }
}
function typesLabel(t: TFunction, values?: string[], legacyValue?: string) {
  const types = values?.length ? values : legacyValue ? [legacyValue] : []
  return types.length ? types.map((value) => t(`JSON type: ${value}`, { defaultValue: value })).join(' / ') : t('Not present')
}
function dimensionLabel(t: TFunction, difference: StructureDimensionDifference) {
  switch (difference.dimension) {
    case 'protocol': return t('Response protocol')
    case 'status_code': return t('HTTP status code')
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
  return difference.dimension === 'protocol' ? difference.value.toUpperCase() : difference.value
}
function scorePercent(value?: number) {
  return value === undefined || !Number.isFinite(value) ? '—' : `${Math.round(Math.max(0, Math.min(1, value)) * 100)}%`
}

export function StructureDifferencePanel({ detail, score, pairedSamples, minimumSamples }: { detail?: StructureSimilarityDetail; score?: number; pairedSamples: number; minimumSamples: number }) {
  const { t } = useTranslation()
  const fields = detail?.field_differences ?? []
  const dimensions = detail?.dimension_differences ?? []
  const fieldSummary = summarizeFieldDifferences(fields)
  const dimensionSummary = summarizeDimensionDifferences(dimensions)
  const provisional = pairedSamples < minimumSamples
  const exactZero = score === 0 && pairedSamples > 0 && detail?.score_available !== false
  const partialCoverage = detail?.field_profile_coverage_complete === false || detail?.metadata_coverage_complete === false
  return <section className='space-y-4 rounded-lg border p-3'>
    <div><h3 className='font-medium'>{t('Complete comparison parameters for all paired samples')}</h3><p className='text-muted-foreground mt-1 text-xs'>{t('The values below aggregate the entire paired window on both sides; they are not taken from a single response.')}</p><p className='text-muted-foreground mt-1 text-xs'>{t('Only sanitized field paths, JSON types, protocol metadata, anonymous signatures, and occurrence counts are shown. Response values, content, reasoning, identifiers, and header values are never stored.')}</p>{provisional ? <p className='mt-2 rounded bg-amber-500/10 p-2 text-xs text-amber-800 dark:text-amber-200'>{t('Only {{paired}} / {{required}} paired samples are available. The comparison below is provisional and is not yet a health conclusion.', { paired: pairedSamples, required: minimumSamples })}</p> : null}</div>
    {detail ? <div className={`rounded-md border p-3 ${exactZero ? 'border-destructive/35 bg-destructive/5' : 'bg-muted/20'}`}><div className='flex flex-wrap justify-between gap-2'><div><h4 className='text-sm font-medium'>{t('Why the structure score is {{score}}', { score: scorePercent(score) })}</h4><p className='mt-1 text-sm'>{exactZero ? t('None of the complete anonymous structure signatures matched exactly in this window.') : t('{{matched}} complete anonymous structure occurrences matched exactly in this window.', { matched: detail.matched_count })}</p></div><Badge variant='outline' className={exactZero ? kindStyle('missing') : kindStyle('matched')}>{t('Exact intersection / union: {{intersection}} / {{union}}', { intersection: detail.intersection_count, union: detail.union_count })}</Badge></div><p className='text-muted-foreground mt-2 text-xs'>{t('This is a strict complete-signature score: a field path, JSON type, protocol, model family, SSE event order, finish reason, or metadata-presence change can make the whole sample signature different. The parameter tables below show which parts caused that result.')}</p></div> : <p className='rounded bg-amber-500/10 p-2 text-sm text-amber-800 dark:text-amber-200'>{t('Detailed scoring inputs are unavailable for this historical result.')}</p>}
    {detail ? <div className='space-y-2'><div className='grid gap-2 sm:grid-cols-3'><div className='rounded-md border p-2'><p className='text-muted-foreground text-xs'>{t('Exact matched occurrences')}</p><p className='font-medium'>{detail.matched_count}</p></div><div className='rounded-md border p-2'><p className='text-muted-foreground text-xs'>{t('Baseline-only occurrences')}</p><p className='font-medium'>{detail.baseline_only_count}</p></div><div className='rounded-md border p-2'><p className='text-muted-foreground text-xs'>{t('Target-only occurrences')}</p><p className='font-medium'>{detail.target_only_count}</p></div></div>{detail.differences.length ? <div><h4 className='text-sm font-medium'>{t('Complete anonymous signature distribution')}</h4><div className='mt-2 max-h-56 overflow-auto rounded-md border'><table className='w-full text-left text-xs'><thead className='bg-muted sticky top-0'><tr><th className='p-2'>{t('Anonymous structure signature')}</th><th className='p-2'>{t('Baseline')}</th><th className='p-2'>{t('Target')}</th><th className='p-2'>{t('Matched')}</th></tr></thead><tbody>{detail.differences.map((difference) => <tr key={difference.signature} className='border-t'><td className='max-w-56 truncate p-2 font-mono' title={difference.signature}>{difference.signature}</td><td className='p-2'>{difference.baseline_count}</td><td className='p-2'>{difference.target_count}</td><td className='p-2'>{difference.matched_count}</td></tr>)}</tbody></table></div></div> : null}</div> : null}
    {partialCoverage ? <p className='rounded bg-amber-500/10 p-2 text-xs text-amber-800'>{t('Some samples were collected before detailed parameter capture was available. The exact signature score still covers all paired samples, while the field and metadata tables cover only the sample counts shown below.')}</p> : null}
    <div className='space-y-3 border-t pt-3'><div className='flex flex-wrap items-start justify-between gap-2'><div><h4 className='text-sm font-medium'>{t('Field paths and JSON types on both sides')}</h4><p className='text-muted-foreground text-xs'>{t('Every safely captured field is listed, including consistent fields and differences.')}</p></div><Badge variant='outline'>{t('Coverage B {{baseline}} / T {{target}} of {{paired}}', { baseline: detail?.baseline_field_profile_samples ?? 0, target: detail?.target_field_profile_samples ?? 0, paired: pairedSamples })}</Badge></div><div className='flex flex-wrap gap-2 text-xs'>{fieldSummary.matched ? <Badge variant='outline' className={kindStyle('matched')}>{t('{{count}} consistent dimensions', { count: fieldSummary.matched })}</Badge> : null}{fieldSummary.missing ? <Badge variant='outline' className={kindStyle('missing')}>{t('{{count}} missing from target', { count: fieldSummary.missing })}</Badge> : null}{fieldSummary.added ? <Badge variant='outline' className={kindStyle('added')}>{t('{{count}} added by target', { count: fieldSummary.added })}</Badge> : null}{fieldSummary.type ? <Badge variant='outline' className={kindStyle('type')}>{t('{{count}} type changes', { count: fieldSummary.type })}</Badge> : null}</div>{fields.length ? <div className='max-h-80 space-y-2 overflow-auto'>{fields.map((field) => { const kind = fieldDifferenceKind(field); return <div key={`${field.path}:${field.change}`} className='rounded-md border p-3'><div className='flex justify-between gap-2'><code className='break-all text-xs font-medium'>{field.path}</code><Badge variant='outline' className={kindStyle(kind)}>{kindLabel(t, kind)}</Badge></div><div className='text-muted-foreground mt-2 grid gap-1 text-xs sm:grid-cols-2'><span>{t('Baseline')}: <strong className='text-foreground'>{typesLabel(t, field.baseline_types, field.baseline_type)}</strong> · {field.baseline_count}</span><span>{t('Target')}: <strong className='text-foreground'>{typesLabel(t, field.target_types, field.target_type)}</strong> · {field.target_count}</span></div></div> })}</div> : <p className='text-muted-foreground text-sm'>{t(detail?.limitation || 'Exact field differences are unavailable for these samples. Run a new detection after upgrading to collect sanitized field paths and types.')}</p>}</div>
    <div className='space-y-3 border-t pt-3'><div className='flex flex-wrap items-start justify-between gap-2'><div><h4 className='text-sm font-medium'>{t('Protocol and stream metadata on both sides')}</h4><p className='text-muted-foreground text-xs'>{t('These differences explain structural signature changes that do not come from JSON field paths, such as JSON versus SSE, event order, finish reasons, and header presence.')}</p></div><Badge variant='outline'>{t('Coverage B {{baseline}} / T {{target}} of {{paired}}', { baseline: detail?.baseline_metadata_samples ?? 0, target: detail?.target_metadata_samples ?? 0, paired: pairedSamples })}</Badge></div>{dimensionSummary.matched ? <Badge variant='outline' className={kindStyle('matched')}>{t('{{count}} consistent metadata items', { count: dimensionSummary.matched })}</Badge> : null}{dimensions.length ? <div className='max-h-72 space-y-2 overflow-auto'>{dimensions.map((dimension) => { const kind = dimensionDifferenceKind(dimension); return <div key={`${dimension.dimension}:${dimension.value}:${dimension.change}`} className='rounded-md border p-3'><div className='flex justify-between gap-2'><div><p className='text-xs font-medium'>{dimensionLabel(t, dimension)}</p><code className='text-muted-foreground break-all text-xs'>{dimensionValue(t, dimension)}</code></div><Badge variant='outline' className={kindStyle(kind)}>{kindLabel(t, kind)}</Badge></div><p className='text-muted-foreground mt-2 text-xs'>{t('Baseline samples')}: {dimension.baseline_count} · {t('Target samples')}: {dimension.target_count}</p></div> })}</div> : <p className='text-muted-foreground text-sm'>{t('No protocol or stream metadata parameters are available for these samples.')}</p>}</div>
  </section>
}
