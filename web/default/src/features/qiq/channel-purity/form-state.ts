/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or (at your option) any later version.
*/
import type { ChannelOption, PurityGroup, PurityGroupInput } from './types'

export function groupToInput(group: PurityGroup): PurityGroupInput {
  return {
    name: group.name,
    enabled: group.enabled,
    channel_ids: [...group.channel_ids],
    baseline_channel_id: group.baseline_channel_id,
    interval_minutes: group.interval_minutes,
    random_pairing_enabled: group.random_pairing_enabled,
    model_comparisons: group.model_comparisons.map((comparison) => ({ ...comparison })),
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

export function normalizeModelComparisons(comparisons: PurityGroupInput['model_comparisons']) {
  return comparisons.map((comparison) => ({ baseline_model: comparison.baseline_model.trim(), target_model: comparison.target_model.trim() }))
}

function uniqueModels(models?: string[]): string[] {
  return [...new Set((models ?? []).map((model) => model.trim()).filter(Boolean))].sort((a, b) => a.localeCompare(b))
}

export function modelComparisonOptions(input: PurityGroupInput, channels: ChannelOption[]) {
  const baselineModels = uniqueModels(channels.find((channel) => channel.id === input.baseline_channel_id && channel.status === 1)?.models)
  const targets = channels.filter((channel) => input.channel_ids.includes(channel.id) && channel.id !== input.baseline_channel_id)
  const targetModels = targets.length
    ? uniqueModels(targets[0].models).filter((model) => targets.every((target) => uniqueModels(target.models).includes(model)))
    : []
  return { baselineModels, targetModels }
}

export function reconcileModelComparisons(input: PurityGroupInput, channels: ChannelOption[]): PurityGroupInput {
  const { baselineModels, targetModels } = modelComparisonOptions(input, channels)
  return {
    ...input,
    model_comparisons: input.model_comparisons.map((comparison) => ({
      baseline_model: baselineModels.includes(comparison.baseline_model) ? comparison.baseline_model : '',
      target_model: targetModels.includes(comparison.target_model) ? comparison.target_model : '',
    })),
  }
}

export function modelComparisonError(input: PurityGroupInput, channels: { id: number; models?: string[] }[]): string | undefined {
  const comparisons = normalizeModelComparisons(input.model_comparisons)
  if (!comparisons.length || comparisons.some((item) => !item.baseline_model || !item.target_model)) return 'Model comparisons are required'
  const keys = comparisons.map((item) => `${item.baseline_model}\u0000${item.target_model}`)
  if (new Set(keys).size !== keys.length) return 'Duplicate model comparison'
  const baseline = channels.find((channel) => channel.id === input.baseline_channel_id)
  if (!baseline) return 'Baseline channel is required'
  const baselineModels = new Set(uniqueModels(baseline.models))
  for (const item of comparisons) {
    if (!baselineModels.has(item.baseline_model)) return 'Baseline model is unavailable'
    for (const target of channels.filter((channel) => input.channel_ids.includes(channel.id) && channel.id !== input.baseline_channel_id)) {
      if (!new Set(uniqueModels(target.models)).has(item.target_model)) return 'Target model is unavailable'
    }
  }
  return undefined
}

export function isGroupInputValid(input: PurityGroupInput): boolean {
  return Boolean(input.name.trim() && input.channel_ids.length >= 2 && input.channel_ids.includes(input.baseline_channel_id) && input.model_comparisons.length)
}
