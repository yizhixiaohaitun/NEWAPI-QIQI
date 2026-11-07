import assert from 'node:assert/strict';
import test from 'node:test';
import { normalizeModelRemovalKeywords, orderModelRemovalUpdates } from './modelRemovalSettings.js';

test('classic keyword normalization preserves explicit empty value', () => {
  assert.equal(normalizeModelRemovalKeywords(' \r\n Legacy message \r\n other \n'), 'Legacy message\nother');
  assert.equal(normalizeModelRemovalKeywords(' \r\n'), '');
});
test('classic option writes enable last and disable first', () => {
  const keys = [{ key: 'AutomaticRemoveModelEnabled' }, { key: 'AutomaticRemoveModelKeywords' }];
  assert.deepEqual(orderModelRemovalUpdates(keys, true).map((x) => x.key), keys.toReversed().map((x) => x.key));
  assert.deepEqual(orderModelRemovalUpdates(keys.toReversed(), false), keys);
});
