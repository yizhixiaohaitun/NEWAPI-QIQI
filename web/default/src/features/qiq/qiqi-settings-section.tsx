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
import { useQueryClient } from '@tanstack/react-query'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import * as z from 'zod'

import { Badge } from '@/components/ui/badge'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormLabel,
} from '@/components/ui/form'
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '@/features/system-settings/components/settings-form-layout'
import { SettingsPageFormActions } from '@/features/system-settings/components/settings-page-context'
import { SettingsSection } from '@/features/system-settings/components/settings-section'
import { useResetForm } from '@/features/system-settings/hooks/use-reset-form'
import { useUpdateOption } from '@/features/system-settings/hooks/use-update-option'

import { RESPONSES_MISSING_REASONING_ITEM_RULE } from './enhanced-compatibility-rules'

const qiqiContextRequestLoggingOption =
  'qiqi_setting.context_request_logging_enabled' as const
const qiqiResponsesMissingReasoningItemRetryOption =
  RESPONSES_MISSING_REASONING_ITEM_RULE.settingKey

const qiqiSettingsSchema = z.object({
  contextRequestLoggingEnabled: z.boolean(),
  responsesMissingReasoningItemRetryEnabled: z.boolean(),
})

type QiqiSettingsFormValues = z.infer<typeof qiqiSettingsSchema>

type QiqiSettingsSectionProps = {
  defaultValues: {
    [qiqiContextRequestLoggingOption]: boolean
    [qiqiResponsesMissingReasoningItemRetryOption]: boolean
  }
}

export function QiqiSettingsSection(props: QiqiSettingsSectionProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const updateOption = useUpdateOption()
  const formDefaults: QiqiSettingsFormValues = {
    contextRequestLoggingEnabled:
      props.defaultValues[qiqiContextRequestLoggingOption],
    responsesMissingReasoningItemRetryEnabled:
      props.defaultValues[qiqiResponsesMissingReasoningItemRetryOption],
  }

  const form = useForm<QiqiSettingsFormValues>({
    resolver: zodResolver(qiqiSettingsSchema),
    defaultValues: formDefaults,
  })
  const { dirtyFields, isDirty, isSubmitting } = form.formState
  const contextLoggingEnabled = form.watch('contextRequestLoggingEnabled')
  const responsesMissingReasoningItemRetryEnabled = form.watch(
    'responsesMissingReasoningItemRetryEnabled'
  )

  useResetForm(form, formDefaults)

  const onSubmit = async (values: QiqiSettingsFormValues) => {
    const updates = [
      {
        key: qiqiContextRequestLoggingOption,
        value: values.contextRequestLoggingEnabled,
        savedValue: props.defaultValues[qiqiContextRequestLoggingOption],
      },
      {
        key: qiqiResponsesMissingReasoningItemRetryOption,
        value: values.responsesMissingReasoningItemRetryEnabled,
        savedValue:
          props.defaultValues[qiqiResponsesMissingReasoningItemRetryOption],
      },
    ].filter((entry) => entry.value !== entry.savedValue)

    if (updates.length === 0) {
      toast.info(t('No changes to save'))
      return
    }

    for (const update of updates) {
      const response = await updateOption.mutateAsync({
        key: update.key,
        value: update.value,
      })
      if (!response.success) break
    }
    await queryClient.refetchQueries({ queryKey: ['system-options'] })
  }

  return (
    <SettingsSection title={t('Qiqi Settings')} className='w-full max-w-2xl'>
      <Form {...form}>
        <SettingsForm
          onSubmit={form.handleSubmit(onSubmit)}
          className='bg-card rounded-lg border p-4 shadow-sm sm:p-5 lg:grid-cols-1'
        >
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending || isSubmitting}
            isSaveDisabled={!isDirty}
            saveLabel='Save Qiqi settings'
          />

          <Tabs defaultValue='general' className='min-w-0'>
            <TabsList variant='line' className='max-w-full justify-start'>
              <TabsTrigger value='general'>{t('General')}</TabsTrigger>
              <TabsTrigger value='enhanced-compatibility'>
                {t('Enhanced compatibility')}
              </TabsTrigger>
            </TabsList>

            <TabsContent value='general' className='pt-4'>
              <FormField
                control={form.control}
                name='contextRequestLoggingEnabled'
                render={({ field }) => (
                  <SettingsSwitchItem className='bg-muted/30 items-start rounded-md px-3 py-3 sm:px-4'>
                    <SettingsSwitchContent className='max-w-xl space-y-1'>
                      <div className='flex flex-wrap items-center gap-2'>
                        <FormLabel>{t('Save full relay context')}</FormLabel>
                        <Badge variant='secondary'>
                          {t(contextLoggingEnabled ? 'Enabled' : 'Disabled')}
                        </Badge>
                        {dirtyFields.contextRequestLoggingEnabled ? (
                          <Badge variant='outline'>
                            {t('Unsaved changes')}
                          </Badge>
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
                        onCheckedChange={field.onChange}
                        disabled={updateOption.isPending || isSubmitting}
                      />
                    </FormControl>
                  </SettingsSwitchItem>
                )}
              />
            </TabsContent>

            <TabsContent value='enhanced-compatibility' className='pt-4'>
              <FormField
                control={form.control}
                name='responsesMissingReasoningItemRetryEnabled'
                render={({ field }) => (
                  <SettingsSwitchItem className='bg-muted/30 items-start rounded-md px-3 py-3 sm:px-4'>
                    <SettingsSwitchContent className='max-w-xl space-y-1'>
                      <div className='flex flex-wrap items-center gap-2'>
                        <Badge variant='outline' className='font-mono'>
                          {RESPONSES_MISSING_REASONING_ITEM_RULE.id}
                        </Badge>
                        <FormLabel>
                          {t(
                            RESPONSES_MISSING_REASONING_ITEM_RULE.shortNameKey
                          )}
                        </FormLabel>
                        <Badge variant='secondary'>
                          {t(
                            responsesMissingReasoningItemRetryEnabled
                              ? 'Enabled'
                              : 'Disabled'
                          )}
                        </Badge>
                        {dirtyFields.responsesMissingReasoningItemRetryEnabled ? (
                          <Badge variant='outline'>
                            {t('Unsaved changes')}
                          </Badge>
                        ) : null}
                      </div>
                      <FormDescription>
                        {t(
                          RESPONSES_MISSING_REASONING_ITEM_RULE.descriptionKey
                        )}
                      </FormDescription>
                    </SettingsSwitchContent>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                        disabled={updateOption.isPending || isSubmitting}
                      />
                    </FormControl>
                  </SettingsSwitchItem>
                )}
              />
            </TabsContent>
          </Tabs>
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
