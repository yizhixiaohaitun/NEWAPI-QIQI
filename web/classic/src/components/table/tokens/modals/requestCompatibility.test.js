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
import { readFileSync } from 'node:fs';
import { describe, test } from 'node:test';
import { withRequestCompatibility } from './requestCompatibility.js';

describe('classic token request compatibility field', () => {
  test('new token defaults on and absent/null legacy fields load on', () => {
    assert.equal(
      withRequestCompatibility().request_compatibility_enabled,
      true,
    );
    for (const value of [undefined, null, true, false]) {
      const data = {
        name: 'test',
        request_compatibility_enabled: value,
        future: { keep: true },
      };
      const loaded = withRequestCompatibility(data);
      assert.equal(loaded.request_compatibility_enabled, value ?? true);
      assert.equal(loaded.future, data.future);
      assert.equal(data.request_compatibility_enabled, value);
      assert.equal(
        JSON.parse(JSON.stringify(loaded)).request_compatibility_enabled,
        value ?? true,
      );
    }
  });

  test('every edit/single/batch payload keeps the selected boolean', () => {
    for (const enabled of [false, true]) {
      const values = withRequestCompatibility({
        name: 'test',
        tokenCount: 3,
        request_compatibility_enabled: enabled,
      });
      for (let i = 0; i < values.tokenCount; i++) {
        const { tokenCount, ...payload } = withRequestCompatibility(values);
        assert.equal(tokenCount, 3);
        assert.equal(
          JSON.parse(JSON.stringify({ ...payload, name: `test-${i}` }))
            .request_compatibility_enabled,
          enabled,
        );
      }
      assert.equal(
        JSON.parse(
          JSON.stringify({ ...withRequestCompatibility(values), id: 1 }),
        ).request_compatibility_enabled,
        enabled,
      );
    }
  });

  test('actual modal wires defaults, API load, both save branches and existing Form.Switch', () => {
    const source = readFileSync(
      new URL('./EditTokenModal.jsx', import.meta.url),
      'utf8',
    );
    assert.match(source, /request_compatibility_enabled: true/);
    assert.match(
      source.replace(/\s+/g, ''),
      /setValues\(withRequestCompatibility\(\{\.\.\.getInitValues\(\),\.\.\.data\}\),?\)/,
    );
    assert.equal(
      source
        .replace(/\s+/g, '')
        .match(/localInputs\}=withRequestCompatibility\(values\)/g)?.length,
      2,
    );
    assert.match(source, /field='request_compatibility_enabled'/);
  });
});
