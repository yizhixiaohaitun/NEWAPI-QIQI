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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import type { TFunction } from 'i18next'

import { apiKeySchema } from '../types'
import {
  getApiKeyFormDefaultValues,
  getApiKeyFormSchema,
  transformApiKeyToFormDefaults,
  transformFormDataToPayload,
} from './api-key-form'

const token = {
  id: 1,
  name: 'frontend-test',
  key: 'synthetic',
  status: 1,
  remain_quota: 0,
  used_quota: 0,
  unlimited_quota: true,
  expired_time: -1,
  created_time: 1,
  accessed_time: 1,
  model_limits_enabled: false,
}
const t = ((key: string) => key) as TFunction

// Check the JSON wire contract, not an in-memory deep clone.
function jsonWire<T>(value: T): T {
  const serialized = JSON.stringify(value)
  return JSON.parse(serialized) as T
}

describe('token request compatibility form boundary', () => {
  test('create and batch defaults serialize enabled for either group default', () => {
    for (const auto of [false, true]) {
      const defaults = getApiKeyFormDefaultValues(auto)
      for (const tokenCount of [1, 3]) {
        const form = getApiKeyFormSchema(t).parse({
          ...defaults,
          name: 'test',
          tokenCount,
        })
        const payload = transformFormDataToPayload(form)
        for (let i = 0; i < tokenCount; i++) {
          assert.equal(
            jsonWire({ ...payload, name: `test-${i}` })
              .request_compatibility_enabled,
            true
          )
        }
      }
    }
  })

  test('loads and saves true/false and legacy absent/null without falsy fallback', () => {
    for (const value of [true, false, undefined, null]) {
      const raw =
        value === undefined
          ? token
          : { ...token, request_compatibility_enabled: value }
      const parsed = apiKeySchema.parse(raw)
      const form = transformApiKeyToFormDefaults(parsed)
      assert.equal(form.request_compatibility_enabled, value ?? true)
      const payload = transformFormDataToPayload(
        getApiKeyFormSchema(t).parse(form)
      )
      assert.equal(
        jsonWire(payload).request_compatibility_enabled,
        value ?? true
      )
      assert.equal(payload.unlimited_quota, true)
    }
  })

  test('explicit disable also survives new batch payloads and schema strips no field', () => {
    const form = getApiKeyFormSchema(t).parse({
      ...getApiKeyFormDefaultValues(false),
      name: 'batch-off',
      tokenCount: 3,
      request_compatibility_enabled: false,
    })
    const payload = transformFormDataToPayload(form)
    for (let i = 0; i < (form.tokenCount ?? 1); i++) {
      assert.equal(
        jsonWire({ ...payload, name: `batch-off-${i}` })
          .request_compatibility_enabled,
        false
      )
    }
    assert.equal(
      getApiKeyFormSchema(t).safeParse({
        ...form,
        request_compatibility_enabled: 'false',
      }).success,
      false
    )
    assert.equal(
      apiKeySchema.safeParse({
        ...token,
        request_compatibility_enabled: 'false',
      }).success,
      false
    )
  })
})
