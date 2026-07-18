/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or (at your option) any later version.
*/
import type { ChannelOption, PurityGroupInput } from './types'

export const ALL_CHANNEL_GROUPS = '__all__'
// Mirrors common.ChannelStatusEnabled. Manual/automatic disabled are 2/3.
export const CHANNEL_STATUS_ENABLED = 1

export function normalizeChannelGroups(value: unknown): string[] {
  const values = Array.isArray(value) ? value : typeof value === 'string' ? value.split(',') : []
  return [...new Set(values.map((item) => String(item).trim()).filter(Boolean))]
}

export function isChannelEnabled(channel: ChannelOption): boolean {
  return channel.status === CHANNEL_STATUS_ENABLED
}

export function channelGroupNames(channels: ChannelOption[]): string[] {
  return [...new Set(channels.flatMap((channel) => channel.groups))].sort((left, right) => left.localeCompare(right))
}

export function filterChannelsByGroup(channels: ChannelOption[], group: string): ChannelOption[] {
  return group === ALL_CHANNEL_GROUPS ? channels : channels.filter((channel) => channel.groups.includes(group))
}

export function partitionChannels(channels: ChannelOption[]): { enabled: ChannelOption[]; disabled: ChannelOption[] } {
  return { enabled: channels.filter(isChannelEnabled), disabled: channels.filter((channel) => !isChannelEnabled(channel)) }
}

export function setGroupChannelsSelected(input: PurityGroupInput, channels: ChannelOption[], group: string, checked: boolean): PurityGroupInput {
  const groupIds = new Set(filterChannelsByGroup(channels, group).filter(isChannelEnabled).map((channel) => channel.id))
  const channelIds = checked ? [...new Set([...input.channel_ids, ...groupIds])] : input.channel_ids.filter((id) => !groupIds.has(id))
  return { ...input, channel_ids: channelIds, baseline_channel_id: channelIds.includes(input.baseline_channel_id) ? input.baseline_channel_id : 0 }
}

export function selectedUnavailableIds(input: PurityGroupInput, channels: ChannelOption[]): number[] {
  const byId = new Map(channels.map((channel) => [channel.id, channel]))
  return input.channel_ids.filter((id) => !byId.has(id) || !isChannelEnabled(byId.get(id)!))
}

export function enabledBaselineChannels(input: PurityGroupInput, channels: ChannelOption[]): ChannelOption[] {
  const selected = new Set(input.channel_ids)
  return channels.filter((channel) => selected.has(channel.id) && isChannelEnabled(channel))
}

export function channelDisplayLabel(channel: ChannelOption): string {
  return `${channel.name} #${channel.id}`
}
