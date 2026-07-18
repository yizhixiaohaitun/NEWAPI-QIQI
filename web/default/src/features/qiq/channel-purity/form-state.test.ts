import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { groupToInput, isGroupInputValid, setChannelSelected } from './form-state.ts'
import type { PurityGroupInput } from './types.ts'

const input = (): PurityGroupInput => ({
  name: 'Production', enabled: true, channel_ids: [1, 2], baseline_channel_id: 1,
  interval_minutes: 5, random_pairing_enabled: false,
  sampling: { window_minutes: 30, minimum_samples: 20, max_samples_per_window: 200 },
})

describe('channel purity group form state', () => {
  test('removing the baseline clears it and makes the form invalid', () => {
    const next = setChannelSelected(input(), 1, false)
    assert.deepEqual(next.channel_ids, [2])
    assert.equal(next.baseline_channel_id, 0)
    assert.equal(isGroupInputValid(next), false)
  })

  test('adding an existing channel never duplicates it', () => {
    assert.deepEqual(setChannelSelected(input(), 2, true).channel_ids, [1, 2])
  })

  test('requires a name, two channels, and a selected baseline', () => {
    assert.equal(isGroupInputValid(input()), true)
    assert.equal(isGroupInputValid({ ...input(), name: '  ' }), false)
    assert.equal(isGroupInputValid({ ...input(), baseline_channel_id: 3 }), false)
  })

  test('editing clones nested and channel form state', () => {
    const group = { ...input(), id: '7', results: [] }
    const next = groupToInput(group)
    next.channel_ids.push(3)
    next.sampling.minimum_samples = 99
    assert.deepEqual(group.channel_ids, [1, 2])
    assert.equal(group.sampling.minimum_samples, 20)
  })
})
