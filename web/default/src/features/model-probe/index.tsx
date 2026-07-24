/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { getChannels } from '@/features/channels/api'
import type { Channel } from '@/features/channels/types'
import { api } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

type ProbeResult = {
  id: number
  created_at: number
  channel_id: number
  channel_name: string
  declared_model: string
  actual_model: string
  id_status: string
  expected_tokens: number | null
  actual_tokens: number | null
  token_delta: number | null
  token_tolerance: number
  token_status: string
  conclusion: string
  error: string
}

const statusClass = (status: string) => {
  if (status === 'passed' || status === 'match') return 'text-emerald-600'
  if (status === 'unknown') return 'text-amber-600'
  return 'text-red-600'
}

export function ModelProbeDashboard() {
  const { t } = useTranslation()
  const [channels, setChannels] = useState<Channel[]>([])
  const [channelId, setChannelId] = useState('')
  const [modelId, setModelId] = useState('')
  const [officialModelId, setOfficialModelId] = useState('')
  const [results, setResults] = useState<ProbeResult[]>([])
  const [loading, setLoading] = useState(true)
  const [probing, setProbing] = useState(false)

  const selected = useMemo(
    () => channels.find((channel) => String(channel.id) === channelId),
    [channelId, channels]
  )

  const load = useCallback(async (showLoading = true) => {
    if (showLoading) setLoading(true)
    try {
      const [channelResponse, probeResponse] = await Promise.all([
        getChannels({ p: 1, page_size: 100 }),
        api.get('/api/channel/model_probe'),
      ])
      setChannels(channelResponse.data?.items ?? [])
      setResults(probeResponse.data.data?.results ?? [])
    } catch (error: unknown) {
      toast.error(error instanceof Error ? error.message : t('Failed to load model probe'))
    } finally {
      if (showLoading) setLoading(false)
    }
  }, [t])

  useEffect(() => {
    void load()
  }, [load])

  useEffect(() => {
    const firstModel = selected?.test_model?.trim() || selected?.models.split(',')[0]?.trim() || ''
    setModelId(firstModel)
    setOfficialModelId(firstModel)
  }, [selected])

  const runProbe = async () => {
    if (!channelId || !modelId.trim() || !officialModelId.trim()) {
      toast.error(t('Select a channel and model first'))
      return
    }
    setProbing(true)
    try {
      const response = await api.post(
        `/api/channel/model_officiality_probe/${channelId}`,
        { target_model_id: modelId.trim(), official_model_id: officialModelId.trim() },
        { skipBusinessError: true }
      )
      if (!response.data.success) toast.error(response.data.message || t('Probe failed'))
      await load(false)
    } catch (error: unknown) {
      toast.error(error instanceof Error ? error.message : t('Probe failed'))
    } finally {
      setProbing(false)
    }
  }

  return (
    <main className='min-h-screen space-y-5 bg-background p-4 md:p-6'>
      <header className='space-y-2'>
        <h1 className='text-2xl font-semibold'>{t('Model Probe')}</h1>
        <p className='text-muted-foreground'>
          {t('Send one low-cost fixed request through the selected channel to initially check whether the returned model matches its declaration.')}
        </p>
        <p className='text-muted-foreground text-sm'>
          {t('This is not cryptographic proof: proxies can rewrite fields, and hidden prompts or wrappers can affect token usage. The fixed probe body is not persisted.')}
        </p>
      </header>

      <section className='grid gap-4 rounded-xl border p-4 md:grid-cols-4'>
        <div className='space-y-2'>
          <Label htmlFor='probe-channel'>{t('Channel')}</Label>
          <select id='probe-channel' className='h-9 w-full rounded-lg border bg-background px-3' value={channelId} onChange={(event) => setChannelId(event.target.value)}>
            <option value=''>{t('Select channel')}</option>
            {channels.map((channel) => <option key={channel.id} value={channel.id}>{channel.name} (#{channel.id})</option>)}
          </select>
        </div>
        <div className='space-y-2'><Label htmlFor='probe-model'>{t('Request model')}</Label><Input id='probe-model' value={modelId} onChange={(event) => setModelId(event.target.value)} /></div>
        <div className='space-y-2'><Label htmlFor='official-model'>{t('Declared model ID')}</Label><Input id='official-model' value={officialModelId} onChange={(event) => setOfficialModelId(event.target.value)} /></div>
        <div className='flex items-end'><Button className='w-full' disabled={probing || loading} onClick={runProbe}>{probing ? t('Probing...') : t('Run probe now')}</Button></div>
      </section>

      <section className='overflow-hidden rounded-xl border'>
        <div className='flex items-center justify-between border-b p-4'><h2 className='font-semibold'>{t('Recent results (anomalies first)')}</h2><Button variant='outline' onClick={() => void load(true)}>{t('Refresh')}</Button></div>
        {loading ? <p className='p-6 text-muted-foreground'>{t('Loading...')}</p> : (
          <div className='overflow-x-auto'>
            <table className='w-full min-w-[980px] text-sm'>
              <thead className='bg-muted/50 text-left'><tr><th className='p-3'>{t('Time')}</th><th className='p-3'>{t('Channel')}</th><th className='p-3'>{t('Declared / actual model')}</th><th className='p-3'>{t('ID match')}</th><th className='p-3'>{t('Expected / actual input tokens')}</th><th className='p-3'>{t('Delta / tolerance')}</th><th className='p-3'>{t('Token result')}</th><th className='p-3'>{t('Conclusion')}</th></tr></thead>
              <tbody>{results.map((result) => <tr key={result.id} className='border-t align-top'><td className='p-3'>{new Date(result.created_at * 1000).toLocaleString()}</td><td className='p-3'>{result.channel_name || `#${result.channel_id}`}</td><td className='p-3'>{result.declared_model}<br/><span className='text-muted-foreground'>{result.actual_model || t('Not returned')}</span></td><td className={`p-3 font-medium ${statusClass(result.id_status)}`}>{t(result.id_status)}</td><td className='p-3'>{result.expected_tokens ?? t('unknown')} / {result.actual_tokens ?? t('unknown')}</td><td className='p-3'>{result.token_delta ?? '—'} / ±{result.token_tolerance || '—'}</td><td className={`p-3 font-medium ${statusClass(result.token_status)}`}>{t(result.token_status)}</td><td className={`p-3 font-medium ${statusClass(result.conclusion)}`}>{t(result.conclusion)}{result.error ? <div className='mt-1 max-w-xs text-xs font-normal text-muted-foreground'>{result.error}</div> : null}</td></tr>)}</tbody>
            </table>
            {results.length === 0 ? <p className='p-6 text-center text-muted-foreground'>{t('No probe results yet')}</p> : null}
          </div>
        )}
      </section>
    </main>
  )
}
