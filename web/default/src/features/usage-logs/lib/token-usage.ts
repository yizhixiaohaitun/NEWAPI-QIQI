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
import type { LogOtherData } from '../types'

function tokenCount(value: number | null | undefined): number {
  return Number.isFinite(value) && (value || 0) > 0 ? Number(value) : 0
}

export interface TokenUsageSummary {
  promptTokens: number
  completionTokens: number
  cacheReadTokens: number
  cacheWriteTokens: number
  cacheWrite5mTokens: number
  cacheWrite1hTokens: number
  totalInputTokens: number
  hasTokens: boolean
}

export function getTokenUsageSummary(
  promptTokensValue: number | null | undefined,
  completionTokensValue: number | null | undefined,
  other: LogOtherData | null | undefined
): TokenUsageSummary {
  const promptTokens = tokenCount(promptTokensValue)
  const completionTokens = tokenCount(completionTokensValue)
  const cacheReadTokens = tokenCount(other?.cache_tokens)
  const cacheWrite5mTokens = tokenCount(other?.cache_creation_tokens_5m)
  const cacheWrite1hTokens = tokenCount(other?.cache_creation_tokens_1h)
  const splitCacheWriteTokens = cacheWrite5mTokens + cacheWrite1hTokens
  const cacheWriteTokens =
    splitCacheWriteTokens > 0
      ? splitCacheWriteTokens
      : tokenCount(other?.cache_write_tokens) ||
        tokenCount(other?.cache_creation_tokens)
  const explicitTotalInputTokens = tokenCount(other?.input_tokens_total)
  const hasAnthropicInputSemantics =
    other?.usage_semantic === 'anthropic' || other?.claude === true
  const totalInputTokens =
    explicitTotalInputTokens > 0
      ? explicitTotalInputTokens
      : hasAnthropicInputSemantics || promptTokens === 0
        ? promptTokens + cacheReadTokens + cacheWriteTokens
        : promptTokens

  return {
    promptTokens,
    completionTokens,
    cacheReadTokens,
    cacheWriteTokens,
    cacheWrite5mTokens,
    cacheWrite1hTokens,
    totalInputTokens,
    hasTokens: totalInputTokens > 0 || completionTokens > 0,
  }
}
