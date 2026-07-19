import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { ALL_CHANNEL_GROUPS, deduplicateChannels, enabledBaselineChannels, filterChannelsByGroup, normalizeChannelGroups, partitionChannels, selectedUnavailableIds, setGroupChannelsSelected, shouldContinueChannelPages } from './channel-state.ts'
import type { ChannelOption, PurityGroupInput } from './types.ts'

const channels: ChannelOption[] = [
  { id: 1, name: 'Enabled shared', status: 1, groups: ['prod', 'openai'] },
  { id: 2, name: 'Disabled prod', status: 2, groups: ['prod'] },
  { id: 3, name: 'Enabled backup', status: 1, groups: ['backup', 'openai'] },
  { id: 4, name: 'Auto disabled', status: 3, groups: [] },
]
const input = (ids = [1, 2, 99]): PurityGroupInput => ({ name: 'Production', enabled: true, channel_ids: ids, baseline_channel_id: 1, interval_minutes: 5, random_pairing_enabled: false, model_comparisons: [{ baseline_model: 'gpt-4o', target_model: 'gpt-4o' }], sampling: { window_minutes: 30, minimum_samples: 20, max_samples_per_window: 200 } })

describe('channel state', () => {
  test('normalizes comma-separated and array groups', () => {
    assert.deepEqual(normalizeChannelGroups(' prod, openai,prod, '), ['prod', 'openai'])
    assert.deepEqual(normalizeChannelGroups(['prod', ' openai ', 'prod']), ['prod', 'openai'])
  })
  test('filters multi-group channels and partitions by real status semantics', () => {
    assert.deepEqual(filterChannelsByGroup(channels, 'openai').map(({ id }) => id), [1, 3])
    assert.equal(filterChannelsByGroup(channels, ALL_CHANNEL_GROUPS).length, 4)
    assert.deepEqual(partitionChannels(channels).enabled.map(({ id }) => id), [1, 3])
    assert.deepEqual(partitionChannels(channels).disabled.map(({ id }) => id), [2, 4])
  })
  test('batch selection only adds enabled channels and clearing preserves other groups', () => {
    const selected = setGroupChannelsSelected(input([3]), channels, 'prod', true)
    assert.deepEqual(selected.channel_ids, [3, 1])
    const cleared = setGroupChannelsSelected(selected, channels, 'prod', false)
    assert.deepEqual(cleared.channel_ids, [3])
  })
  test('surfaces disabled and missing legacy selections', () => {
    assert.deepEqual(selectedUnavailableIds(input(), channels), [2, 99])
    assert.deepEqual(enabledBaselineChannels(input(), channels).map(({ id }) => id), [1])
  })
  test('continues short pages when total says more rows exist and deduplicates ids', () => {
    assert.equal(shouldContinueChannelPages(2, 2, 5), true)
    assert.equal(shouldContinueChannelPages(1, 5, 5), false)
    assert.equal(shouldContinueChannelPages(0, 2, 5), false)
    assert.deepEqual(deduplicateChannels([channels[0], { ...channels[0], name: 'Latest' }, channels[1]]).map(({ id, name }) => [id, name]), [[1, 'Latest'], [2, 'Disabled prod']])
  })
})
