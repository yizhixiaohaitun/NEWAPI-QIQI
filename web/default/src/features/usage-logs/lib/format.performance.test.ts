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

import type { UsageLog } from '../data/schema'
import {
  parseUsageLogOther,
  shouldMountUsageLogDetails,
} from './parse-log-other.ts'

const originalParse = JSON.parse
let parseCount = 0
JSON.parse = ((...args: Parameters<typeof JSON.parse>) => {
  parseCount += 1
  return originalParse(...args)
}) as typeof JSON.parse

try {
  for (const rowCount of [10, 100]) {
    parseCount = 0
    const logs = Array.from(
      { length: rowCount },
      (_, index) =>
        ({
          id: index,
          other: JSON.stringify({
            cache_tokens: 1901,
            cache_creation_tokens: 2909,
            reject_reason: 'claude_stop_reason=refusal',
          }),
        }) as UsageLog
    )
    const fixtureParseCount = parseCount

    for (const log of logs) {
      // Mirrors the current admin desktop hot path: eight cells plus row tint.
      for (let consumer = 0; consumer < 9; consumer += 1) {
        const other = parseUsageLogOther(log)
        assert.equal(other?.cache_tokens, 1901)
      }
    }

    const renderParseCount = parseCount - fixtureParseCount
    assert.equal(renderParseCount, rowCount)
    process.stdout.write(
      `${rowCount} rows: ${renderParseCount} JSON.parse calls\n`
    )
  }

  const mutableLog = {
    id: 101,
    other: '{"cache_tokens":1}',
  } as UsageLog
  assert.equal(parseUsageLogOther(mutableLog)?.cache_tokens, 1)
  mutableLog.other = '{"cache_tokens":999}'
  assert.equal(parseUsageLogOther(mutableLog)?.cache_tokens, 999)

  let closedDetailsMountCount = 0
  for (let row = 0; row < 100; row += 1) {
    if (shouldMountUsageLogDetails(false)) closedDetailsMountCount += 1
  }
  assert.equal(closedDetailsMountCount, 0)
  assert.equal(shouldMountUsageLogDetails(true), true)
  process.stdout.write(
    `100 closed rows: ${closedDetailsMountCount} details dialog mounts\n`
  )
} finally {
  JSON.parse = originalParse
}
