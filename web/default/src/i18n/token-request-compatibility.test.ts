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
import { test } from 'node:test'

import { createInstance } from 'i18next'

import i18n from './config'
import {
  registerTokenRequestCompatibility,
  tokenRequestCompatibilityTranslations,
} from './token-request-compatibility'

test('actual imported default instance registers Chinese token strings', async () => {
  await i18n.changeLanguage('zhCN')
  assert.equal(i18n.t('Request compatibility'), '请求兼容')
  assert.equal(i18n.t('Thinking format adaptation'), '思考格式适配')
  assert.equal(
    i18n.t('Opus 4.7 sampling parameter adaptation'),
    'Opus 4.7 采样参数适配'
  )
  assert.equal(
    i18n.t('Enabled for new tokens; you can turn it off.'),
    '新建默认开启，可手动关闭。'
  )
})

test('dedicated bundle registers before/after async init, preserving unrelated strings', async () => {
  for (const registerBefore of [false, true]) {
    const instance = createInstance()
    if (registerBefore) registerTokenRequestCompatibility(instance)
    await instance.init({
      lng: 'zhCN',
      fallbackLng: 'en',
      nsSeparator: false,
      resources: { zhCN: { translation: { Unrelated: '保留' } } },
    })
    if (!registerBefore) registerTokenRequestCompatibility(instance)
    assert.equal(instance.t('Unrelated'), '保留')
    for (const [language, strings] of Object.entries(
      tokenRequestCompatibilityTranslations
    )) {
      for (const [key, value] of Object.entries(strings)) {
        assert.equal(instance.t(key, { lng: language }), value)
      }
    }
  }
})
