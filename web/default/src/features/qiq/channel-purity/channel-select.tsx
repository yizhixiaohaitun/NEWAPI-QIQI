/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or (at your option) any later version.
*/
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Checkbox } from '@/components/ui/checkbox'
import { SelectContent, SelectGroup, SelectItem, SelectLabel, SelectSeparator } from '@/components/ui/select'

import { channelDisplayLabel, partitionChannels } from './channel-state'
import type { ChannelOption } from './types'

export function ChannelGroupBadges({ groups }: { groups: string[] }) {
  const { t } = useTranslation()
  return <span className='flex min-w-0 flex-wrap gap-1'>{groups.length ? groups.map((group) => <Badge key={group} variant='outline' className='max-w-40 truncate text-[10px]'>{group}</Badge>) : <Badge variant='outline' className='text-muted-foreground text-[10px]'>{t('No channel group')}</Badge>}</span>
}

export function ChannelStateBadge({ enabled }: { enabled: boolean }) {
  const { t } = useTranslation()
  return <Badge variant={enabled ? 'secondary' : 'destructive'} className='text-[10px]'>{enabled ? t('Enabled') : t('Disabled')}</Badge>
}

export function ChannelOptionRow({ channel, checked, disabled, onCheckedChange }: { channel: ChannelOption; checked: boolean; disabled: boolean; onCheckedChange: (checked: boolean) => void }) {
  const enabled = channel.status === 1
  return <label className={`flex items-start gap-2 rounded-md border p-2 text-sm ${disabled ? 'cursor-not-allowed opacity-70' : 'cursor-pointer'}`}>
    <Checkbox className='mt-0.5' disabled={disabled} checked={checked} onCheckedChange={(value) => onCheckedChange(value === true)} />
    <span className='min-w-0 flex-1'><span className='flex flex-wrap items-center gap-1'><span className='truncate'>{channelDisplayLabel(channel)}</span><ChannelStateBadge enabled={enabled} /></span><ChannelGroupBadges groups={channel.groups} /></span>
  </label>
}

export function PartitionedSelectContent({ channels, disabledIds = false }: { channels: ChannelOption[]; disabledIds?: boolean }) {
  const { t } = useTranslation()
  const parts = partitionChannels(channels)
  const render = (channel: ChannelOption, disabled: boolean) => <SelectItem key={channel.id} value={String(channel.id)} disabled={disabled}><span className='flex items-center gap-2'><span>{channelDisplayLabel(channel)}</span><ChannelStateBadge enabled={!disabled} /><span className='text-muted-foreground text-xs'>[{channel.groups.join(', ') || t('No channel group')}]</span></span></SelectItem>
  return <SelectContent>
    <SelectGroup><SelectLabel>{t('Enabled channels ({{count}})', { count: parts.enabled.length })}</SelectLabel>{parts.enabled.map((channel) => render(channel, false))}</SelectGroup>
    {parts.disabled.length ? <><SelectSeparator /><SelectGroup><SelectLabel>{t('Disabled channels ({{count}})', { count: parts.disabled.length })}</SelectLabel>{parts.disabled.map((channel) => render(channel, disabledIds))}</SelectGroup></> : null}
  </SelectContent>
}
