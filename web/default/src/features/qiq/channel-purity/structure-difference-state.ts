/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or (at your option) any later version.
*/
import type { FieldProfileDifference, StructureDimensionDifference } from './types'

export type FieldDifferenceKind = 'missing' | 'added' | 'type' | 'frequency'
export type DimensionDifferenceKind = 'missing' | 'added' | 'frequency'

export function fieldDifferenceKind(difference: FieldProfileDifference): FieldDifferenceKind {
  if (difference.change === 'missing' || (difference.baseline_count > 0 && difference.target_count === 0)) return 'missing'
  if (difference.change === 'added' || (difference.baseline_count === 0 && difference.target_count > 0)) return 'added'
  const baselineTypes = difference.baseline_types?.length ? difference.baseline_types : difference.baseline_type ? [difference.baseline_type] : []
  const targetTypes = difference.target_types?.length ? difference.target_types : difference.target_type ? [difference.target_type] : []
  if (difference.change === 'type_changed' || baselineTypes.join('\u0000') !== targetTypes.join('\u0000')) return 'type'
  return 'frequency'
}

export function summarizeFieldDifferences(differences: FieldProfileDifference[]) {
  return differences.reduce((summary, difference) => {
    summary[fieldDifferenceKind(difference)] += 1
    return summary
  }, { missing: 0, added: 0, type: 0, frequency: 0 })
}

export function dimensionDifferenceKind(difference: StructureDimensionDifference): DimensionDifferenceKind {
  if (difference.change === 'missing' || (difference.baseline_count > 0 && difference.target_count === 0)) return 'missing'
  if (difference.change === 'added' || (difference.baseline_count === 0 && difference.target_count > 0)) return 'added'
  return 'frequency'
}

export function summarizeDimensionDifferences(differences: StructureDimensionDifference[]) {
  return differences.reduce((summary, difference) => {
    summary[dimensionDifferenceKind(difference)] += 1
    return summary
  }, { missing: 0, added: 0, frequency: 0 })
}
