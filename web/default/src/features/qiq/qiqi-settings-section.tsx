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
import { zodResolver } from '@hookform/resolvers/zod'
import { useEffect, useRef } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import * as z from 'zod'

import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormLabel,
} from '@/components/ui/form'
import { Switch } from '@/components/ui/switch'

import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '@/features/system-settings/components/settings-form-layout'
import { SettingsPageFormActions } from '@/features/system-settings/components/settings-page-context'
import { SettingsSection } from '@/features/system-settings/components/settings-section'
import { useResetForm } from '@/features/system-settings/hooks/use-reset-form'
import { useUpdateOption } from '@/features/system-settings/hooks/use-update-option'

const qiqiSettingsSchema = z.object({
  'qiqi_setting.context_request_logging_enabled': z.boolean(),
})

type QiqiSettingsFormValues = z.infer<typeof qiqiSettingsSchema>

type QiqiSettingsSectionProps = {
  defaultValues: QiqiSettingsFormValues
}

export function QiqiSettingsSection({
  defaultValues,
}: QiqiSettingsSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const baselineRef = useRef<QiqiSettingsFormValues>(defaultValues)
  const baselineSerializedRef = useRef<string>(JSON.stringify(defaultValues))

  const form = useForm<QiqiSettingsFormValues>({
    resolver: zodResolver(qiqiSettingsSchema),
    defaultValues,
  })
  const { isDirty, isSubmitting } = form.formState
  const contextLoggingEnabled = form.watch(
    'qiqi_setting.context_request_logging_enabled'
  )

  useResetForm(form, defaultValues)

  useEffect(() => {
    const serialized = JSON.stringify(defaultValues)
    if (serialized === baselineSerializedRef.current) return

    baselineRef.current = defaultValues
    baselineSerializedRef.current = serialized
  }, [defaultValues])

  const onSubmit = async (values: QiqiSettingsFormValues) => {
    const key = 'qiqi_setting.context_request_logging_enabled'
    if (values[key] === baselineRef.current[key]) {
      toast.info(t('No changes to save'))
      return
    }

    const response = await updateOption.mutateAsync({
      key,
      value: values[key],
    })

    if (response.success) {
      const savedValues = { ...values }
      baselineRef.current = savedValues
      baselineSerializedRef.current = JSON.stringify(savedValues)
      form.reset(savedValues)
    }
  }

  return (
    <SettingsSection title={t('Qiqi Settings')} className='w-full max-w-2xl'>
      <Form {...form}>
        <SettingsForm
          onSubmit={form.handleSubmit(onSubmit)}
          className='rounded-lg border bg-card p-4 shadow-sm sm:p-5 lg:grid-cols-1'
        >
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending || isSubmitting}
            isSaveDisabled={!isDirty}
            saveLabel='Save Qiqi settings'
          />

          <FormField
            control={form.control}
            name='qiqi_setting.context_request_logging_enabled'
            render={({ field }) => (
              <SettingsSwitchItem className='items-start rounded-md bg-muted/30 px-3 py-3 sm:px-4'>
                <SettingsSwitchContent className='max-w-xl space-y-1'>
                  <div className='flex flex-wrap items-center gap-2'>
                    <FormLabel>{t('Save full relay context')}</FormLabel>
                    <span
                      className={
                        contextLoggingEnabled
                          ? 'rounded-md bg-emerald-500/10 px-1.5 py-0.5 text-[11px] font-medium text-emerald-600 ring-1 ring-emerald-500/20 dark:text-emerald-400'
                          : 'bg-muted text-muted-foreground ring-border rounded-md px-1.5 py-0.5 text-[11px] font-medium ring-1'
                      }
                    >
                      {t(contextLoggingEnabled ? 'Enabled' : 'Disabled')}
                    </span>
                    {isDirty ? (
                      <span className='bg-primary/10 text-primary rounded-md px-1.5 py-0.5 text-[11px] font-medium ring-1 ring-primary/20'>
                        {t('Unsaved changes')}
                      </span>
                    ) : null}
                  </div>
                  <FormDescription>
                    {t(
                      'Persist relay request and response payloads for debugging.'
                    )}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={(checked) => field.onChange(checked)}
                    disabled={updateOption.isPending || isSubmitting}
                  />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
