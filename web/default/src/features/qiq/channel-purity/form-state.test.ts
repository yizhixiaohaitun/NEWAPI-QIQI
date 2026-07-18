import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  ALL_CHANNEL_GROUPS,
  filterChannelsByGroup,
  groupToInput,
  isGroupInputValid,
  normalizeChannelGroups,
  setChannelSelected,
  setGroupChannelsSelected,
} from './form-state.ts'
import type { ChannelOption, PurityGroupInput } from './types.ts'

const input = (): PurityGroupInput => ({
  name: 'Production', enabled: true, channel_ids: [1, 2], baseline_channel_id: 1,
  interval_minutes: 5, random_pairing_enabled: false,
  sampling: { window_minutes: 30, minimum_samples: 20, max_samples_per_window: 200 },
})

const channels: ChannelOption[] = [
  { id: 1, name: 'Shared', groups: ['prod', 'openai'] },
  { id: 2, name: 'Prod only', groups: ['prod'] },
  { id: 3, name: 'Backup', groups: ['backup', 'openai'] },
  { id: 4, name: 'Ungrouped', groups: [] },
]

describe('channel purity group form state', () => {
  test('normalizes comma-separated and array channel groups', () => {
    assert.deepEqual(normalizeChannelGroups(' prod, openai,prod, , backup '), ['prod', 'openai', 'backup'])
    assert.deepEqual(normalizeChannelGroups(['prod', ' openai ', 'prod']), ['prod', 'openai'])
    assert.deepEqual(normalizeChannelGroups(undefined), [])
  })

  test('filters channels with multiple group membership and supports all groups', () => {
    assert.deepEqual(filterChannelsByGroup(channels, 'openai').map(({ id }) => id), [1, 3])
    assert.deepEqual(filterChannelsByGroup(channels, 'prod').map(({ id }) => id), [1, 2])
    assert.deepEqual(filterChannelsByGroup(channels, ALL_CHANNEL_GROUPS).map(({ id }) => id), [1, 2, 3, 4])
  })

  test('selects and clears the current channel group without losing other selections', () => {
    const initial = { ...input(), channel_ids: [3, 4], baseline_channel_id: 3 }
    const selected = setGroupChannelsSelected(initial, channels, 'prod', true)
    assert.deepEqual(selected.channel_ids, [3, 4, 1, 2])
    assert.equal(selected.baseline_channel_id, 3)

    const cleared = setGroupChannelsSelected(selected, channels, 'prod', false)
    assert.deepEqual(cleared.channel_ids, [3, 4])
    assert.equal(cleared.baseline_channel_id, 3)
  })

  test('clearing a channel group also clears a baseline contained in it', () => {
    const cleared = setGroupChannelsSelected(input(), channels, 'prod', false)
    assert.deepEqual(cleared.channel_ids, [])
    assert.equal(cleared.baseline_channel_id, 0)
  })
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
