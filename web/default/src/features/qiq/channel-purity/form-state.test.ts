import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { groupToInput, isGroupInputValid, modelComparisonError, modelComparisonOptions, normalizeModelComparisons, reconcileModelComparisons, setChannelSelected } from './form-state.ts'
import type { PurityGroupInput } from './types.ts'

const input = (): PurityGroupInput => ({ name: 'Production', enabled: true, channel_ids: [1, 2], baseline_channel_id: 1, interval_minutes: 5, random_pairing_enabled: false, model_comparisons: [{ baseline_model: ' gpt-4o ', target_model: 'gpt-4o-mini ' }], sampling: { window_minutes: 30, minimum_samples: 20, max_samples_per_window: 200 }, policy: { suspect_threshold: 0.72, alert_threshold: 0.55, alert_windows: 3, recovery_windows: 2 }, retention: { max_windows_per_target_model: 100, policy: 'latest_windows' } })

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
    const channels = [{ id: 1, name: 'baseline', status: 1, groups: [], models: [' gpt-4o '] }, { id: 2, name: 'target', status: 1, groups: [], models: ['gpt-4o-mini '] }]
    assert.equal(modelComparisonError(input(), channels), undefined)
    assert.equal(modelComparisonError({ ...input(), model_comparisons: [...input().model_comparisons, ...input().model_comparisons] }, channels), 'Duplicate model comparison')
    assert.equal(modelComparisonError({ ...input(), model_comparisons: [] }, channels), 'Model comparisons are required')
  })
  test('uses the same trimmed, empty-filtered and deduplicated models for options and validation', () => {
    const channels = [
      { id: 1, name: 'baseline', status: 1, groups: [], models: [' shared ', '', 'shared'] },
      { id: 2, name: 'target', status: 1, groups: [], models: ['shared ', ' '] },
    ]
    const current = { ...input(), model_comparisons: [{ baseline_model: ' shared ', target_model: ' shared ' }] }
    assert.deepEqual(modelComparisonOptions(current, channels), { baselineModels: ['shared'], targetModels: ['shared'] })
    assert.equal(modelComparisonError(current, channels), undefined)
    assert.deepEqual(reconcileModelComparisons(current, channels).model_comparisons, [{ baseline_model: 'shared', target_model: 'shared' }])
  })
  test('offers baseline models and the intersection supported by every target', () => {
    const channels = [
      { id: 1, name: 'baseline', status: 1, groups: [], models: ['z-model', 'gpt-4o', 'gpt-4o'] },
      { id: 2, name: 'target-a', status: 1, groups: [], models: ['shared', 'only-a'] },
      { id: 3, name: 'target-b', status: 1, groups: [], models: ['shared', 'only-b'] },
    ]
    assert.deepEqual(modelComparisonOptions({ ...input(), channel_ids: [1, 2, 3] }, channels), {
      baselineModels: ['gpt-4o', 'z-model'],
      targetModels: ['shared'],
    })
  })
  test('clears model values that become invalid after channels change', () => {
    const current = { ...input(), model_comparisons: [{ baseline_model: 'gpt-4o', target_model: 'shared' }] }
    const channels = [
      { id: 1, name: 'baseline', status: 1, groups: [], models: ['replacement'] },
      { id: 2, name: 'target', status: 1, groups: [], models: ['other'] },
    ]
    assert.deepEqual(reconcileModelComparisons(current, channels).model_comparisons, [{ baseline_model: '', target_model: '' }])
  })
  test('returns no target option until at least one target channel is selected', () => {
    const channels = [{ id: 1, name: 'baseline', status: 1, groups: [], models: ['gpt-4o'] }]
    assert.deepEqual(modelComparisonOptions({ ...input(), channel_ids: [1] }, channels).targetModels, [])
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
