import assert from 'node:assert/strict'
import test from 'node:test'
import { DEFAULT_MODEL_REMOVAL_KEYWORDS, normalizeModelRemovalKeywords, orderModelRemovalUpdates } from './model-removal-settings.ts'

test('model removal uses a specific provider legacy message', () => {
  assert.equal(DEFAULT_MODEL_REMOVAL_KEYWORDS, 'This Model is marked by provider as Legacy')
  assert.equal(normalizeModelRemovalKeywords(' \r\n  Legacy message  \r\n\n other \n'), 'Legacy message\nother')
  assert.equal(normalizeModelRemovalKeywords(' \r\n '), '')
})
test('keyword save completes before enabling, disable completes before keyword changes', () => {
  const keys = ['AutomaticRemoveModelEnabled', 'AutomaticRemoveModelKeywords']
  assert.deepEqual(orderModelRemovalUpdates(keys, true), ['AutomaticRemoveModelKeywords', 'AutomaticRemoveModelEnabled'])
  assert.deepEqual(orderModelRemovalUpdates(keys.toReversed(), false), keys)
  assert.deepEqual(orderModelRemovalUpdates(['AutomaticRemoveModelKeywords'], true), ['AutomaticRemoveModelKeywords'])
})
