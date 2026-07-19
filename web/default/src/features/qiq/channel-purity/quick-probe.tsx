/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or (at your option) any later version.
*/
import { useMutation } from '@tanstack/react-query'
import { FlaskConical } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Select, SelectTrigger, SelectValue } from '@/components/ui/select'

import { runQuickProbe } from './api'
import { channelDisplayLabel, isChannelEnabled } from './channel-state'
import { PartitionedSelectContent } from './channel-select'
import type { ChannelOption, QuickProbeResult } from './types'

function message(error: unknown) { return error instanceof Error ? error.message : undefined }

export function QuickProbe({ channels, channelsLoading, channelsError, onRetryChannels }: { channels: ChannelOption[]; channelsLoading: boolean; channelsError?: string; onRetryChannels: () => void }) {
  const { t } = useTranslation()
  const [channelId, setChannelId] = useState('')
  const [model, setModel] = useState('')
  const [result, setResult] = useState<QuickProbeResult | null>(null)
  const selected = channels.find((channel) => String(channel.id) === channelId)
  const usable = Boolean(selected && isChannelEnabled(selected))
  const mutation = useMutation({ mutationFn: runQuickProbe, onSuccess: setResult, onError: (error) => toast.error(message(error) || t('Quick probe failed')) })
  return <Card><CardHeader><CardTitle className='flex items-center gap-2'><FlaskConical className='size-5' />{t('Quick Probe — manual connectivity diagnosis')}</CardTitle></CardHeader><CardContent className='space-y-3'><p className='text-muted-foreground text-sm'>{t('This sends a manual connectivity check only. Its output is never included in scheduled benchmark results, evidence, or alerts.')}</p>{channelsError ? <div role='alert' className='text-destructive text-sm'>{channelsError}<Button className='ml-2' size='sm' variant='outline' onClick={onRetryChannels}>{t('Retry')}</Button></div> : null}<div className='grid gap-2 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto]'><Select value={channelId} onValueChange={(value) => setChannelId(value ?? '')}><SelectTrigger className='w-full'><SelectValue placeholder={t('Select channel')}>{selected ? channelDisplayLabel(selected) : undefined}</SelectValue></SelectTrigger><PartitionedSelectContent channels={channels} disabledIds /></Select><Input value={model} onChange={(event) => setModel(event.target.value)} placeholder={t('Optional model')} /><Button disabled={!usable || mutation.isPending || channelsLoading || Boolean(channelsError)} onClick={() => mutation.mutate({ channel_id: Number(channelId), model: model || undefined })}>{mutation.isPending ? t('Diagnosing…') : t('Run diagnosis')}</Button></div>{result ? <div className='rounded-lg border p-3 text-sm'><Badge variant={result.ok ? 'secondary' : 'destructive'}>{result.ok ? t('Connected') : t('Connection failed')}</Badge><span className='ml-2'>{result.message || '—'}</span>{result.latency_ms !== undefined ? <span className='text-muted-foreground ml-2'>{result.latency_ms} ms</span> : null}</div> : null}</CardContent></Card>
}
