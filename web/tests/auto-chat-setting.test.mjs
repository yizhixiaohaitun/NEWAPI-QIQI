import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8')
const key = 'global.auto_chat_to_responses_enabled'

test('both settings interfaces expose and save the independent boolean option', () => {
  const modern = read('../default/src/features/system-settings/models/global-settings-card.tsx')
  const registry = read('../default/src/features/system-settings/models/section-registry.tsx')
  const defaults = read('../default/src/features/system-settings/models/index.tsx')
  const classic = read('../classic/src/pages/Setting/Model/SettingGlobalModel.jsx')
  assert.ok(defaults.includes(`'${key}': true`))
  assert.ok(registry.includes(`settings['${key}']`))
  assert.ok(modern.includes(`name='${key}'`))
  assert.match(modern, /'global\.auto_chat_to_responses_enabled':\s*values\.global\.auto_chat_to_responses_enabled/)
  assert.ok(classic.includes(`'${key}': true`))
  assert.ok(classic.includes(`field='${key}'`))
  assert.ok(classic.includes(`'${key}': value`))
})
