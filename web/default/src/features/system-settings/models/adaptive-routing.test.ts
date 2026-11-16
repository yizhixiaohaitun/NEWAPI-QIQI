// @ts-expect-error Bun supplies this test module at runtime.
import { expect, test } from 'bun:test'

test('schema and option parsing preserve both switch states', async () => {
  const { getOptionValue } = await import('../hooks/use-system-options')
  const { buildFormDefaults, routingReliabilitySchema, normalizeFormValues } =
    await import('./routing-reliability-form')
  const defaults = {
    RetryTimes: 0,
    ChannelDisableThreshold: '',
    AutomaticDisableChannelEnabled: false,
    AutomaticEnableChannelEnabled: false,
    AutomaticDisableKeywords: '',
    AutomaticDisableStatusCodes: '401',
    AutomaticRetryStatusCodes: '500',
    AutomaticRetryKeywords: '',
    'monitor_setting.auto_test_channel_enabled': false,
    'monitor_setting.auto_test_channel_minutes': 10,
    'monitor_setting.channel_test_mode': 'scheduled_all' as const,
    'adaptive_routing_setting.enabled': false,
  }
  for (const enabled of [false, true]) {
    const loaded = getOptionValue(
      [{ key: 'adaptive_routing_setting.enabled', value: String(enabled) }],
      defaults
    )
    const form = buildFormDefaults(loaded)
    const parsed = routingReliabilitySchema.parse(form)
    expect(parsed.adaptive_routing_setting.enabled).toBe(enabled)
    expect(
      normalizeFormValues(parsed)['adaptive_routing_setting.enabled']
    ).toBe(enabled)
  }
  expect(getOptionValue([], defaults)['adaptive_routing_setting.enabled']).toBe(
    false
  )
})
