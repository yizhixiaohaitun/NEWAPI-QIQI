/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or (at your option) any later version.
*/
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'

import { ChannelOptionRow, PartitionedSelectContent } from './channel-select'
import { ALL_CHANNEL_GROUPS, channelDisplayLabel, channelGroupNames, enabledBaselineChannels, filterChannelsByGroup, partitionChannels, selectedUnavailableIds, setGroupChannelsSelected } from './channel-state'
import { groupToInput, isGroupInputValid, setChannelSelected } from './form-state'
import type { ChannelOption, PurityGroup, PurityGroupInput } from './types'

const emptyInput = (): PurityGroupInput => ({ name: '', enabled: true, channel_ids: [], baseline_channel_id: 0, interval_minutes: 5, random_pairing_enabled: false, sampling: { window_minutes: 30, minimum_samples: 20, max_samples_per_window: 200 } })

export function GroupForm({ open, group, channels, channelsLoading, channelsError, saving, saveError, onRetryChannels, onOpenChange, onSave }: { open: boolean; group: PurityGroup | null; channels: ChannelOption[]; channelsLoading: boolean; channelsError?: string; saving: boolean; saveError?: string; onRetryChannels: () => void; onOpenChange: (open: boolean) => void; onSave: (input: PurityGroupInput) => void }) {
  const { t } = useTranslation()
  const [input, setInput] = useState<PurityGroupInput>(() => group ? groupToInput(group) : emptyInput())
  const [channelGroup, setChannelGroup] = useState(ALL_CHANNEL_GROUPS)
  const selected = new Set(input.channel_ids)
  const availableGroups = channelGroupNames(channels)
  const visible = filterChannelsByGroup(channels, channelGroup)
  const parts = partitionChannels(visible)
  const unavailable = selectedUnavailableIds(input, channels)
  const known = new Map(channels.map((channel) => [channel.id, channel]))
  const baselines = enabledBaselineChannels(input, channels)
  const baseline = baselines.find((channel) => channel.id === input.baseline_channel_id)
  const valid = isGroupInputValid(input) && unavailable.length === 0 && Boolean(baseline)
  const toggle = (id: number, checked: boolean) => setInput((current) => setChannelSelected(current, id, checked))
  const renderSection = (title: string, items: ChannelOption[], disabled: boolean) => <div className='space-y-2'><div className='flex items-center justify-between'><p className='text-sm font-medium'>{title}</p><span className='text-muted-foreground text-xs'>{items.length}</span></div><div className='grid gap-2 sm:grid-cols-2'>{items.map((channel) => <ChannelOptionRow key={channel.id} channel={channel} checked={selected.has(channel.id)} disabled={saving || (disabled && !selected.has(channel.id))} onCheckedChange={(checked) => toggle(channel.id, checked)} />)}{!items.length ? <p className='text-muted-foreground text-sm'>{t('None')}</p> : null}</div></div>
  return <Dialog open={open} onOpenChange={onOpenChange}><DialogContent className='max-h-[90vh] overflow-y-auto sm:max-w-2xl'>
    <DialogHeader><DialogTitle>{group ? t('Edit benchmark group') : t('Create benchmark group')}</DialogTitle><DialogDescription>{t('Choose at least two enabled channels and designate exactly one baseline. Disabled or missing legacy channels must be removed before saving.')}</DialogDescription></DialogHeader>
    <div className='grid gap-4'>
      <div className='space-y-2'><Label htmlFor='group-name'>{t('Detection group name')}</Label><Input id='group-name' value={input.name} onChange={(event) => setInput({ ...input, name: event.target.value })} /></div>
      <div className='space-y-2'><Label>{t('Channel group (from channel management)')}</Label><Select value={channelGroup} onValueChange={setChannelGroup}><SelectTrigger className='w-full'><SelectValue>{channelGroup === ALL_CHANNEL_GROUPS ? t('All channel groups') : channelGroup}</SelectValue></SelectTrigger><SelectContent><SelectItem value={ALL_CHANNEL_GROUPS}>{t('All channel groups')}</SelectItem>{availableGroups.map((name) => <SelectItem key={name} value={name}>{name}</SelectItem>)}</SelectContent></Select><p className='text-muted-foreground text-xs'>{t('Filtering preserves manual selections in other channel groups. Batch selection only adds enabled channels.')}</p><div className='flex flex-wrap gap-2'><Button type='button' size='sm' variant='outline' disabled={saving || !parts.enabled.some((channel) => !selected.has(channel.id))} onClick={() => setInput((current) => setGroupChannelsSelected(current, channels, channelGroup, true))}>{t('Select enabled in current channel group')}</Button><Button type='button' size='sm' variant='outline' disabled={saving || !parts.enabled.some((channel) => selected.has(channel.id))} onClick={() => setInput((current) => setGroupChannelsSelected(current, channels, channelGroup, false))}>{t('Clear enabled in current channel group')}</Button><span className='text-muted-foreground self-center text-xs'>{t('{{selected}} selected in total', { selected: input.channel_ids.length })}</span></div></div>
      <div className='max-h-72 space-y-4 overflow-y-auto rounded-lg border p-3'>{channelsLoading ? <p>{t('Loading channels…')}</p> : channelsError ? <div><p role='alert' className='text-destructive'>{channelsError}</p><Button size='sm' variant='outline' onClick={onRetryChannels}>{t('Retry channel loading')}</Button></div> : <>{renderSection(t('Enabled channels ({{count}})', { count: parts.enabled.length }), parts.enabled, false)}{renderSection(t('Disabled channels ({{count}})', { count: parts.disabled.length }), parts.disabled, true)}</>}</div>
      {unavailable.length ? <div role='alert' className='border-destructive/40 bg-destructive/5 rounded-lg border p-3 text-sm'><p className='text-destructive font-medium'>{t('This legacy detection group contains disabled or missing channels. They cannot participate in formal detection; remove them before saving.')}</p><div className='mt-2 flex flex-wrap gap-2'>{unavailable.map((id) => <Button key={id} size='sm' variant='outline' onClick={() => toggle(id, false)}>{known.get(id) ? channelDisplayLabel(known.get(id)!) : t('Missing channel #{{id}}', { id })} · {t('Remove')}</Button>)}</div></div> : null}
      <div className='space-y-2'><Label>{t('Baseline channel (selected and enabled only)')}</Label><Select value={baseline ? String(baseline.id) : ''} onValueChange={(value) => setInput({ ...input, baseline_channel_id: Number(value) })}><SelectTrigger className='w-full'><SelectValue placeholder={t('Select a baseline from selected enabled channels')}>{baseline ? channelDisplayLabel(baseline) : undefined}</SelectValue></SelectTrigger><PartitionedSelectContent channels={baselines} disabledIds /></Select></div>
      <div className='grid gap-4 sm:grid-cols-2'><div className='space-y-2'><Label>{t('Detection interval')}</Label><Select value={String(input.interval_minutes)} onValueChange={(value) => setInput({ ...input, interval_minutes: Number(value) as 5 | 10 })}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent><SelectItem value='5'>{t('Every 5 minutes')}</SelectItem><SelectItem value='10'>{t('Every 10 minutes')}</SelectItem></SelectContent></Select></div><div className='flex items-center justify-between rounded-lg border p-3'><Label>{t('Random pairing detection')}</Label><Switch checked={input.random_pairing_enabled} onCheckedChange={(checked) => setInput({ ...input, random_pairing_enabled: checked })} /></div></div>
      <div><Label>{t('Sampling settings')}</Label><div className='mt-2 grid gap-3 sm:grid-cols-3'>{([['window_minutes', t('Window (minutes)')], ['minimum_samples', t('Minimum samples')], ['max_samples_per_window', t('Maximum samples / window')]] as const).map(([key, label]) => <div key={key}><Label className='text-xs'>{label}</Label><Input type='number' min={1} value={input.sampling[key]} onChange={(event) => setInput({ ...input, sampling: { ...input.sampling, [key]: Math.max(1, Number(event.target.value)) } })} /></div>)}</div></div>
      <div className='flex items-center justify-between rounded-lg border p-3'><Label>{t('Enable this group')}</Label><Switch checked={input.enabled} onCheckedChange={(checked) => setInput({ ...input, enabled: checked })} /></div>
    </div>
    {saveError ? <p role='alert' className='text-destructive text-sm'>{saveError}</p> : null}<DialogFooter><Button variant='outline' disabled={saving} onClick={() => onOpenChange(false)}>{t('Cancel')}</Button><Button disabled={!valid || saving || channelsLoading || Boolean(channelsError)} onClick={() => onSave({ ...input, name: input.name.trim() })}>{saving ? t('Saving…') : t('Save group')}</Button></DialogFooter>
  </DialogContent></Dialog>
}
