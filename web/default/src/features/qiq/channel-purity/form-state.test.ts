import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { groupToInput, isGroupInputValid, modelComparisonError, normalizeModelComparisons, setChannelSelected } from './form-state.ts'
import type { PurityGroupInput } from './types.ts'

const input = (): PurityGroupInput => ({ name: 'Production', enabled: true, channel_ids: [1, 2], baseline_channel_id: 1, interval_minutes: 5, random_pairing_enabled: false, model_comparisons: [{ baseline_model: ' gpt-4o ', target_model: 'gpt-4o-mini ' }], sampling: { window_minutes: 30, minimum_samples: 20, max_samples_per_window: 200 } })

describe('channel purity group form state', () => {
  test('removing the baseline clears it and makes the form invalid', () => {
    const next = setChannelSelected(input(), 1, false)
    assert.deepEqual(next.channel_ids, [2])
    assert.equal(next.baseline_channel_id, 0)
    assert.equal(isGroupInputValid(next), false)
  })
  test('adding an existing channel never duplicates it', () => assert.deepEqual(setChannelSelected(input(), 2, true).channel_ids, [1, 2]))
  test('requires a name, two channels, and a selected baseline', () => {
    assert.equal(isGroupInputValid(input()), true)
    assert.equal(isGroupInputValid({ ...input(), name: '  ' }), false)
    assert.equal(isGroupInputValid({ ...input(), baseline_channel_id: 3 }), false)
  })
  test('normalizes and validates explicit model comparisons', () => {
    assert.deepEqual(normalizeModelComparisons(input().model_comparisons), [{ baseline_model: 'gpt-4o', target_model: 'gpt-4o-mini' }])
    const channels = [{ id: 1, models: ['gpt-4o'] }, { id: 2, models: ['gpt-4o-mini'] }]
    assert.equal(modelComparisonError(input(), channels), undefined)
    assert.equal(modelComparisonError({ ...input(), model_comparisons: [...input().model_comparisons, ...input().model_comparisons] }, channels), 'Duplicate model comparison')
    assert.equal(modelComparisonError({ ...input(), model_comparisons: [] }, channels), 'Model comparisons are required')
  })
  test('editing clones nested and channel form state', () => {
    const group = { ...input(), id: '7', results: [] }
    const next = groupToInput(group)
    next.channel_ids.push(3)
    next.sampling.minimum_samples = 99
    next.model_comparisons[0].baseline_model = 'changed'
    assert.deepEqual(group.channel_ids, [1, 2])
    assert.equal(group.model_comparisons[0].baseline_model, ' gpt-4o ')
    assert.equal(group.sampling.minimum_samples, 20)
  })
})
