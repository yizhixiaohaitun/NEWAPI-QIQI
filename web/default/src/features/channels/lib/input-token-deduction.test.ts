import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  buildChannelSettingWithInputTokenDeduction,
  isNonnegativeIntegerInput,
  parseInputTokenDeductionFormValues,
} from './input-token-deduction.ts'

describe('input token deduction channel setting', () => {
  test('keeps blank group values on the channel default and preserves unrelated settings', () => {
    const result = JSON.parse(
      buildChannelSettingWithInputTokenDeduction(
        '{"proxy":"socks5://example","future_setting":{"enabled":true}}',
        { force_format: true },
        '490',
        { default: '', vip: '0', partner: '490' }
      )
    )

    assert.deepEqual(result, {
      proxy: 'socks5://example',
      future_setting: { enabled: true },
      force_format: true,
      input_token_deduction: 490,
      input_token_deduction_by_group: { vip: 0, partner: 490 },
    })
  })

  test('normalizes an empty channel default to zero and round-trips explicit group zero', () => {
    const setting = buildChannelSettingWithInputTokenDeduction('{}', {}, '', {
      default: 0,
    })
    assert.deepEqual(parseInputTokenDeductionFormValues(setting), {
      input_token_deduction: '0',
      input_token_deduction_by_group: { default: '0' },
    })
  })

  test('round-trips a 500 token channel default', () => {
    const setting = buildChannelSettingWithInputTokenDeduction(
      '{}',
      {},
      500,
      {}
    )
    assert.equal(
      parseInputTokenDeductionFormValues(setting).input_token_deduction,
      '500'
    )
  })

  test('rejects negative and fractional values', () => {
    for (const invalidValue of [-1, '-1', 1.5, '1.5']) {
      assert.equal(isNonnegativeIntegerInput(invalidValue), false)
      assert.throws(() =>
        buildChannelSettingWithInputTokenDeduction('{}', {}, invalidValue, {})
      )
      assert.throws(() =>
        buildChannelSettingWithInputTokenDeduction('{}', {}, 0, {
          default: invalidValue,
        })
      )
    }
  })
})
