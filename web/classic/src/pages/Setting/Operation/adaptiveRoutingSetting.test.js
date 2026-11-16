import assert from 'node:assert/strict';
import { test } from 'node:test';
import {
  adaptiveRoutingKey,
  adaptiveRoutingOption,
  adaptiveRoutingValue,
} from './adaptiveRoutingSetting.js';

test('adaptive routing option uses the canonical key and string boolean value', () => {
  assert.equal(adaptiveRoutingKey, 'adaptive_routing_setting.enabled');
  assert.deepEqual(adaptiveRoutingOption(true), {
    key: adaptiveRoutingKey,
    value: 'true',
  });
  assert.deepEqual(adaptiveRoutingOption(false), {
    key: adaptiveRoutingKey,
    value: 'false',
  });
});

test('adaptive routing values default to disabled unless explicitly true', () => {
  assert.equal(adaptiveRoutingValue(true), true);
  assert.equal(adaptiveRoutingValue('true'), true);
  assert.equal(adaptiveRoutingValue(false), false);
  assert.equal(adaptiveRoutingValue('false'), false);
  assert.equal(adaptiveRoutingValue(undefined), false);
});
