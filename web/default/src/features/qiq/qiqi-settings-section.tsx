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
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
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
import { safeNumberFieldProps } from '@/features/system-settings/utils/numeric-field'

import {
  AZURE_RESPONSES_RESOURCE_AFFINITY_RULE,
  RESPONSES_MISSING_REASONING_ITEM_RULE,
  RESPONSES_STREAM_ERROR_RETRY_RULE,
} from './enhanced-compatibility-rules'

const qiqiContextRequestLoggingOption =
  'qiqi_setting.context_request_logging_enabled' as const
const qiqiResponsesMissingReasoningItemRetryOption =
  RESPONSES_MISSING_REASONING_ITEM_RULE.settingKey
const qiqiResponsesStreamErrorRetryOption =
  RESPONSES_STREAM_ERROR_RETRY_RULE.settingKey
const qiqiResponsesStreamErrorRetryTimesOption =
  RESPONSES_STREAM_ERROR_RETRY_RULE.retryTimesSettingKey
const qiqiAzureResponsesResourceAffinityOption =
  AZURE_RESPONSES_RESOURCE_AFFINITY_RULE.settingKey

const qiqiSettingsSchema = z.object({
  contextRequestLoggingEnabled: z.boolean(),
  responsesMissingReasoningItemRetryEnabled: z.boolean(),
  responsesStreamErrorRetryEnabled: z.boolean(),
  responsesStreamErrorRetryTimes: z
    .number({
      error: 'Retry attempts must be an integer from 0 to 5.',
    })
    .int('Retry attempts must be an integer from 0 to 5.')
    .min(0, 'Retry attempts must be an integer from 0 to 5.')
    .max(5, 'Retry attempts must be an integer from 0 to 5.'),
  azureResponsesResourceAffinityEnabled: z.boolean(),
})

type QiqiSettingsFormValues = z.infer<typeof qiqiSettingsSchema>

type QiqiSettingsSectionProps = {
  defaultValues: {
    [qiqiContextRequestLoggingOption]: boolean
    [qiqiResponsesMissingReasoningItemRetryOption]: boolean
    [qiqiResponsesStreamErrorRetryOption]: boolean
    [qiqiResponsesStreamErrorRetryTimesOption]: number
    [qiqiAzureResponsesResourceAffinityOption]?: boolean
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
    responsesStreamErrorRetryEnabled:
      props.defaultValues[qiqiResponsesStreamErrorRetryOption],
    responsesStreamErrorRetryTimes:
      props.defaultValues[qiqiResponsesStreamErrorRetryTimesOption],
    azureResponsesResourceAffinityEnabled:
      props.defaultValues[qiqiAzureResponsesResourceAffinityOption] ?? true,
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
  const responsesStreamErrorRetryEnabled = form.watch(
    'responsesStreamErrorRetryEnabled'
  )
  const azureResponsesResourceAffinityEnabled = form.watch(
    'azureResponsesResourceAffinityEnabled'
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
      {
        key: qiqiResponsesStreamErrorRetryOption,
        value: values.responsesStreamErrorRetryEnabled,
        savedValue: props.defaultValues[qiqiResponsesStreamErrorRetryOption],
      },
      {
        key: qiqiResponsesStreamErrorRetryTimesOption,
        value: values.responsesStreamErrorRetryTimes,
        savedValue:
          props.defaultValues[qiqiResponsesStreamErrorRetryTimesOption],
      },
      {
        key: qiqiAzureResponsesResourceAffinityOption,
        value: values.azureResponsesResourceAffinityEnabled,
        savedValue:
          props.defaultValues[qiqiAzureResponsesResourceAffinityOption] ?? true,
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
              <div className='space-y-3'>
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
                        <FormDescription className='text-pretty'>
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

                <div className='bg-muted/30 space-y-4 rounded-md px-3 py-3 sm:px-4'>
                  <FormField
                    control={form.control}
                    name='responsesStreamErrorRetryEnabled'
                    render={({ field }) => (
                      <SettingsSwitchItem className='items-start p-0'>
                        <SettingsSwitchContent className='max-w-xl space-y-1'>
                          <div className='flex flex-wrap items-center gap-2'>
                            <Badge variant='outline' className='font-mono'>
                              {RESPONSES_STREAM_ERROR_RETRY_RULE.id}
                            </Badge>
                            <FormLabel>
                              {t(
                                RESPONSES_STREAM_ERROR_RETRY_RULE.shortNameKey
                              )}
                            </FormLabel>
                            <Badge variant='secondary'>
                              {t(
                                responsesStreamErrorRetryEnabled
                                  ? 'Enabled'
                                  : 'Disabled'
                              )}
                            </Badge>
                            {dirtyFields.responsesStreamErrorRetryEnabled ? (
                              <Badge variant='outline'>
                                {t('Unsaved changes')}
                              </Badge>
                            ) : null}
                          </div>
                          <FormDescription className='text-pretty'>
                            {t(
                              RESPONSES_STREAM_ERROR_RETRY_RULE.descriptionKey
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

                  <FormField
                    control={form.control}
                    name='responsesStreamErrorRetryTimes'
                    render={({ field }) => (
                      <FormItem className='max-w-xs'>
                        <div className='flex flex-wrap items-center gap-2'>
                          <FormLabel>{t('Retry attempts')}</FormLabel>
                          <Badge variant='secondary'>
                            {t('Recommended: 2')}
                          </Badge>
                          {dirtyFields.responsesStreamErrorRetryTimes ? (
                            <Badge variant='outline'>
                              {t('Unsaved changes')}
                            </Badge>
                          ) : null}
                        </div>
                        <FormControl>
                          <Input
                            type='number'
                            min={0}
                            max={5}
                            step={1}
                            {...safeNumberFieldProps(field)}
                            disabled={
                              !responsesStreamErrorRetryEnabled ||
                              updateOption.isPending ||
                              isSubmitting
                            }
                          />
                        </FormControl>
                        <FormDescription className='text-pretty'>
                          {t(
                            'Recommended: 2 retries, for up to 3 total attempts. Allowed range: 0 to 5.'
                          )}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </div>

                <FormField
                  control={form.control}
                  name='azureResponsesResourceAffinityEnabled'
                  render={({ field }) => (
                    <SettingsSwitchItem className='bg-muted/30 items-start rounded-md px-3 py-3 sm:px-4'>
                      <SettingsSwitchContent className='max-w-xl space-y-1'>
                        <div className='flex flex-wrap items-center gap-2'>
                          <Badge variant='outline' className='font-mono'>
                            {AZURE_RESPONSES_RESOURCE_AFFINITY_RULE.id}
                          </Badge>
                          <FormLabel>
                            {t(
                              AZURE_RESPONSES_RESOURCE_AFFINITY_RULE.shortNameKey
                            )}
                          </FormLabel>
                          <Badge variant='secondary'>
                            {t(
                              azureResponsesResourceAffinityEnabled
                                ? 'Enabled'
                                : 'Disabled'
                            )}
                          </Badge>
                          {dirtyFields.azureResponsesResourceAffinityEnabled ? (
                            <Badge variant='outline'>
                              {t('Unsaved changes')}
                            </Badge>
                          ) : null}
                        </div>
                        <FormDescription className='text-pretty'>
                          {t(
                            AZURE_RESPONSES_RESOURCE_AFFINITY_RULE.descriptionKey
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
              </div>
            </TabsContent>
          </Tabs>
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
