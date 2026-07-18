/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or (at your option) any later version.
*/
import type { PurityGroup, PurityGroupInput } from './types'

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
