/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or (at your option) any later version.
*/
import type { ChannelOption, PurityGroup, PurityGroupInput } from './types'

export const ALL_CHANNEL_GROUPS = '__all__'

export function normalizeChannelGroups(value: unknown): string[] {
  const values = Array.isArray(value) ? value : typeof value === 'string' ? value.split(',') : []
  return [...new Set(values.map((item) => String(item).trim()).filter(Boolean))]
}

export function channelGroupNames(channels: ChannelOption[]): string[] {
  return [...new Set(channels.flatMap((channel) => channel.groups))].sort((left, right) => left.localeCompare(right))
}

export function filterChannelsByGroup(channels: ChannelOption[], group: string): ChannelOption[] {
  return group === ALL_CHANNEL_GROUPS ? channels : channels.filter((channel) => channel.groups.includes(group))
}

export function setGroupChannelsSelected(input: PurityGroupInput, channels: ChannelOption[], group: string, checked: boolean): PurityGroupInput {
  const groupIds = new Set(filterChannelsByGroup(channels, group).map((channel) => channel.id))
  const channelIds = checked
    ? [...new Set([...input.channel_ids, ...groupIds])]
    : input.channel_ids.filter((id) => !groupIds.has(id))
  return {
    ...input,
    channel_ids: channelIds,
    baseline_channel_id: channelIds.includes(input.baseline_channel_id) ? input.baseline_channel_id : 0,
  }
}

export function groupToInput(group: PurityGroup): PurityGroupInput {
  return {
    name: group.name,
    enabled: group.enabled,
    channel_ids: [...group.channel_ids],
    baseline_channel_id: group.baseline_channel_id,
    interval_minutes: group.interval_minutes,
    random_pairing_enabled: group.random_pairing_enabled,
    sampling: { ...group.sampling },
  }
}

export function setChannelSelected(input: PurityGroupInput, id: number, checked: boolean): PurityGroupInput {
  const channelIds = checked
    ? input.channel_ids.includes(id) ? input.channel_ids : [...input.channel_ids, id]
    : input.channel_ids.filter((channelId) => channelId !== id)
  return {
    ...input,
    channel_ids: channelIds,
    baseline_channel_id: !checked && input.baseline_channel_id === id ? 0 : input.baseline_channel_id,
  }
}

export function isGroupInputValid(input: PurityGroupInput): boolean {
  return Boolean(input.name.trim() && input.channel_ids.length >= 2 && input.channel_ids.includes(input.baseline_channel_id))
}
