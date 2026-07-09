/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { CircleCheck, Lightbulb, Wrench } from 'lucide-react'

import { StatusBadge, type StatusBadgeProps } from '@/components/status-badge'

import { findEnhancedCompatibilityRule } from '../../../qiq/enhanced-compatibility-rules'
import { LOG_TYPE_ENUM } from '../../constants'
import type { UsageLog } from '../../data/schema'
import type { RelayCompatibilityEvent } from '../../types'

type TranslationFunction = (
  key: string,
  options?: Record<string, unknown>
) => string

export function CompatibilityReference({
  log,
  event,
  t,
}: {
  log: UsageLog
  event: RelayCompatibilityEvent
  t: TranslationFunction
}) {
  const rule = findEnhancedCompatibilityRule(event)
  const isRecommendation =
    event.event_type === 'recommendation' || event.outcome === 'disabled'
  const isRecovered =
    !isRecommendation &&
    log.type === LOG_TYPE_ENUM.CONSUME &&
    event.outcome === 'accepted'

  let status: {
    icon: typeof Lightbulb
    label: string
    variant: StatusBadgeProps['variant']
  }
  if (isRecommendation) {
    status = {
      icon: Lightbulb,
      label: t('Enable suggested'),
      variant: 'warning',
    }
  } else if (isRecovered) {
    status = {
      icon: CircleCheck,
      label: t('Recovered'),
      variant: 'success',
    }
  } else {
    status = {
      icon: Wrench,
      label: t('Repair attempted'),
      variant: 'danger',
    }
  }

  let messageKey: string
  if (rule) {
    if (isRecommendation) {
      messageKey = rule.referenceKeys.recommended
    } else if (isRecovered) {
      messageKey = rule.referenceKeys.recovered
    } else {
      messageKey = rule.referenceKeys.attempted
    }
  } else if (isRecommendation) {
    messageKey =
      'This error matches a disabled enhanced compatibility rule. Enable the rule and retry.'
  } else if (isRecovered) {
    messageKey = 'Enhanced compatibility recovered this request.'
  } else {
    messageKey =
      'Enhanced compatibility was applied, but the request still failed.'
  }

  return (
    <div className='flex min-w-0 flex-col gap-1'>
      <div className='flex min-w-0 flex-wrap items-center gap-1.5'>
        <StatusBadge
          label={status.label}
          icon={status.icon}
          variant={status.variant}
          size='sm'
          copyable={false}
        />
        <span className='text-muted-foreground font-mono text-[11px]'>
          {event.rule_id || rule?.id || event.key || '-'}
        </span>
      </div>
      <span className='text-xs leading-snug font-medium'>
        {rule ? t(rule.shortNameKey) : t('Enhanced compatibility')}
      </span>
      <span className='text-muted-foreground text-xs leading-snug'>
        {t(messageKey, { referenceCount: event.count || 1 })}
      </span>
    </div>
  )
}
