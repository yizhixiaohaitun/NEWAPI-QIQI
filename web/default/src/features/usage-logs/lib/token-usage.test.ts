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

import type { LogOtherData } from '../types'
import { getTokenUsageSummary } from './token-usage.ts'

const cacheOnly = getTokenUsageSummary(0, 0, {
  cache_tokens: 1901,
  cache_creation_tokens: 2909,
} as LogOtherData)
assert.deepEqual(
  {
    promptTokens: cacheOnly.promptTokens,
    completionTokens: cacheOnly.completionTokens,
    cacheReadTokens: cacheOnly.cacheReadTokens,
    cacheWriteTokens: cacheOnly.cacheWriteTokens,
    totalInputTokens: cacheOnly.totalInputTokens,
    hasTokens: cacheOnly.hasTokens,
  },
  {
    promptTokens: 0,
    completionTokens: 0,
    cacheReadTokens: 1901,
    cacheWriteTokens: 2909,
    totalInputTokens: 4810,
    hasTokens: true,
  }
)

const splitCache = getTokenUsageSummary(22, 0, {
  cache_tokens: 1901,
  cache_creation_tokens: 2909,
  cache_creation_tokens_5m: 1000,
  cache_creation_tokens_1h: 1909,
  usage_semantic: 'anthropic',
} as LogOtherData)
assert.equal(splitCache.cacheWriteTokens, 2909)
assert.equal(splitCache.totalInputTokens, 4832)

const explicitTotal = getTokenUsageSummary(22, 0, {
  cache_tokens: 1901,
  cache_write_tokens: 2909,
  input_tokens_total: 4900,
  usage_semantic: 'anthropic',
} as LogOtherData)
assert.equal(explicitTotal.cacheWriteTokens, 2909)
assert.equal(explicitTotal.totalInputTokens, 4900)

const legacyOpenAI = getTokenUsageSummary(100, 20, {
  cache_tokens: 30,
  cache_creation_tokens: 50,
} as LogOtherData)
assert.equal(legacyOpenAI.totalInputTokens, 100)
assert.equal(getTokenUsageSummary(0, 0, null).hasTokens, false)
