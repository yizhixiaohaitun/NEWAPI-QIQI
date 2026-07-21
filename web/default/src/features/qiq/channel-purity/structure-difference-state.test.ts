/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or (at your option) any later version.
*/
import assert from 'node:assert/strict'
import test from 'node:test'

import {
  dimensionDifferenceKind,
  fieldDifferenceKind,
  summarizeDimensionDifferences,
  summarizeFieldDifferences,
} from './structure-difference-state.ts'

test('classifies missing, added, type, and frequency structural differences', () => {
  assert.equal(fieldDifferenceKind({ path: 'choices[].message.content', baseline_type: 'string', baseline_count: 5, target_count: 0 }), 'missing')
  assert.equal(fieldDifferenceKind({ path: 'output[].reasoning', target_type: 'array', baseline_count: 0, target_count: 5 }), 'added')
  assert.equal(fieldDifferenceKind({ path: 'usage.total_tokens', change: 'type_changed', baseline_types: ['number'], target_types: ['string'], baseline_count: 5, target_count: 5 }), 'type')
  assert.equal(fieldDifferenceKind({ path: 'choices[].finish_reason', change: 'frequency_changed', baseline_types: ['string'], target_types: ['string'], baseline_count: 5, target_count: 3 }), 'frequency')
  assert.equal(fieldDifferenceKind({ path: 'model', change: 'matched', baseline_type: 'string', target_type: 'string', baseline_count: 5, target_count: 5 }), 'matched')
})

test('summarizes structural difference categories', () => {
  assert.deepEqual(summarizeFieldDifferences([
    { path: 'a', baseline_count: 2, target_count: 0 },
    { path: 'b', baseline_count: 0, target_count: 2 },
    { path: 'c', baseline_type: 'number', target_type: 'string', baseline_count: 2, target_count: 2 },
    { path: 'd', baseline_type: 'string', target_type: 'string', baseline_count: 2, target_count: 1 },
    { path: 'e', change: 'matched', baseline_type: 'string', target_type: 'string', baseline_count: 2, target_count: 2 },
  ]), { matched: 1, missing: 1, added: 1, type: 1, frequency: 1 })
})

test('classifies and summarizes protocol metadata differences', () => {
  const differences = [
    { dimension: 'protocol', value: 'json', change: 'missing', baseline_count: 5, target_count: 0 },
    { dimension: 'protocol', value: 'sse', change: 'added', baseline_count: 0, target_count: 5 },
    { dimension: 'finish_reason', value: 'stop', baseline_count: 5, target_count: 3 },
    { dimension: 'status_code', value: '200', change: 'matched', baseline_count: 5, target_count: 5 },
  ]
  assert.equal(dimensionDifferenceKind(differences[0]), 'missing')
  assert.equal(dimensionDifferenceKind(differences[1]), 'added')
  assert.equal(dimensionDifferenceKind(differences[2]), 'frequency')
  assert.equal(dimensionDifferenceKind(differences[3]), 'matched')
  assert.deepEqual(summarizeDimensionDifferences(differences), { matched: 1, missing: 1, added: 1, frequency: 1 })
})
