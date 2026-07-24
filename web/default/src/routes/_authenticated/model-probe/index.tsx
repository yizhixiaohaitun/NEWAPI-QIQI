/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { createFileRoute, redirect } from '@tanstack/react-router'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ExternalLink, RefreshCw } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

function ModelProbeEmbed() {
  const { t } = useTranslation()
  const [loading, setLoading] = useState(true)
  const [failed, setFailed] = useState(false)
  const [key, setKey] = useState(0)
  const url = '/model-probe-dashboard'

  return (
    <main className='flex min-h-[calc(100vh-4rem)] flex-col gap-3 p-3'>
      <div className='flex items-center justify-between gap-3'>
        <div><h1 className='text-xl font-semibold'>{t('Model Probe')}</h1><p className='text-muted-foreground text-sm'>{t('Initial consistency check for the model actually returned by a channel.')}</p></div>
        <div className='flex gap-2'><Button variant='outline' onClick={() => { setFailed(false); setLoading(true); setKey((value) => value + 1) }}><RefreshCw />{t('Refresh')}</Button><Button variant='outline' render={<a href={url} target='_blank' rel='noreferrer' />}><ExternalLink />{t('Open in new window')}</Button></div>
      </div>
      <div className='relative min-h-[720px] flex-1 overflow-hidden rounded-xl border'>
        {loading && !failed ? <div className='absolute inset-0 z-10 grid place-items-center bg-background/90 text-muted-foreground'>{t('Loading model probe...')}</div> : null}
        {failed ? <div className='absolute inset-0 z-10 grid place-items-center bg-background p-6 text-center'><div><p className='font-medium'>{t('The model probe page could not be loaded.')}</p><p className='mt-2 text-sm text-muted-foreground'>{t('Check your login state and try refresh or open it in a new window.')}</p></div></div> : null}
        {/* Same-origin is required so the independent dashboard can reuse the admin login session. */}
        {/* oxlint-disable-next-line react/iframe-missing-sandbox */}
        <iframe key={key} title={t('Model Probe')} src={url} sandbox='allow-scripts allow-same-origin allow-forms' className='h-full min-h-[720px] w-full border-0' onLoad={() => setLoading(false)} onError={() => { setLoading(false); setFailed(true) }} />
      </div>
    </main>
  )
}

export const Route = createFileRoute('/_authenticated/model-probe/')({
  beforeLoad: () => {
    const { auth } = useAuthStore.getState()
    if (!auth.user || auth.user.role < ROLE.ADMIN) throw redirect({ to: '/403' })
  },
  component: ModelProbeEmbed,
})
