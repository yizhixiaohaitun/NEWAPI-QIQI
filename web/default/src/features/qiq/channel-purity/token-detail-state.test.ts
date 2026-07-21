/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or (at your option) any later version.
*/
import assert from 'node:assert/strict'
import test from 'node:test'
import { tokenMetricAvailability } from './token-detail-state.ts'
import type { TokenSimilarityDetail } from './types.ts'

const detail = (overrides: Partial<TokenSimilarityDetail> = {}): TokenSimilarityDetail => ({
  version: 'token_similarity.v1', baseline_valid_samples: 5, target_valid_samples: 5,
  paired_count: 5, baseline_min: 10, baseline_max: 20, baseline_p50: 15, baseline_p95: 20,
  target_min: 11, target_max: 22, target_p50: 16, target_p95: 22, ratio_median: 1.1,
  q1: 1, q3: 1.2, mad: 0.1, robust_lower: 0.7, robust_upper: 1.4,
  outside_count: 1, deviation_rate: 0.2, score_available: true, pairs: [], ...overrides,
})

test('token metric is unavailable without valid paired token data', () => {
  assert.deepEqual(tokenMetricAvailability(detail({ paired_count: 0, score_available: false })), { available: false, partial: true, outside: 1 })
  assert.deepEqual(tokenMetricAvailability(undefined), { available: false, partial: false, outside: 0 })
})

test('token metric reports complete and partial windows', () => {
  assert.deepEqual(tokenMetricAvailability(detail()), { available: true, partial: false, outside: 1 })
  assert.equal(tokenMetricAvailability(detail({ target_valid_samples: 4 })).partial, true)
})
