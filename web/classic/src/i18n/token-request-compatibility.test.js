/*
Copyright (C) 2025 QuantumNous

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
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { createInstance } from 'i18next';

test('actual classic instance registers dedicated Chinese token strings', async () => {
  // Browser bootstrap needs window.__i18n; no application APIs or browser data are used.
  globalThis.window = { location: { search: '', hash: '', pathname: '/' } };
  const { default: i18n } = await import('./i18n.js');
  await import('./token-request-compatibility.js');
  await i18n.changeLanguage('zh-CN');
  assert.equal(i18n.t('Request compatibility'), '请求兼容');
  assert.equal(i18n.t('Thinking format adaptation'), '思考格式适配');
  assert.equal(
    i18n.t('Opus 4.7 sampling parameter adaptation'),
    'Opus 4.7 采样参数适配',
  );
  assert.equal(
    i18n.t('Enabled for new tokens; you can turn it off.'),
    '新建默认开启，可手动关闭。',
  );
});

test('registers before/after init without replacing unrelated translations', async () => {
  if (!globalThis.window)
    globalThis.window = { location: { search: '', hash: '', pathname: '/' } };
  const {
    registerTokenRequestCompatibility,
    tokenRequestCompatibilityTranslations,
  } = await import('./token-request-compatibility.js');
  for (const registerBefore of [false, true]) {
    const instance = createInstance();
    if (registerBefore) registerTokenRequestCompatibility(instance);
    await instance.init({
      lng: 'zh-CN',
      fallbackLng: 'en',
      nsSeparator: false,
      resources: { 'zh-CN': { translation: { Unrelated: '保留' } } },
    });
    if (!registerBefore) registerTokenRequestCompatibility(instance);
    assert.equal(instance.t('Unrelated'), '保留');
    for (const [language, strings] of Object.entries(
      tokenRequestCompatibilityTranslations,
    )) {
      for (const [key, value] of Object.entries(strings)) {
        assert.equal(instance.t(key, { lng: language }), value);
      }
    }
  }
});
