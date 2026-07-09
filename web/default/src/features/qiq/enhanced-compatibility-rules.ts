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

export interface EnhancedCompatibilityRuleDefinition {
  id: string
  key: string
  settingKey: `qiqi_setting.${string}`
  shortNameKey: string
  descriptionKey: string
  referenceKeys: {
    recovered: string
    recommended: string
    attempted: string
  }
}

export const RESPONSES_MISSING_REASONING_ITEM_RULE = {
  id: 'QIQI-EC-001',
  key: 'responses_missing_reasoning_item_retry',
  settingKey: 'qiqi_setting.responses_missing_reasoning_item_retry_enabled',
  shortNameKey: 'Invalid reasoning reference recovery',
  descriptionKey:
    'Detects the exact OpenAI Responses 400 error for an invalid empty rs_ reasoning reference. It removes only that empty reference and retries once on the same channel; reasoning content, encrypted state, messages, and tool calls are preserved.',
  referenceKeys: {
    recovered:
      'Recovered after removing invalid reasoning references ({{referenceCount}}) and retrying once on the same channel.',
    recommended:
      'This error matches the rule. Enable it to remove the invalid reasoning reference and retry once on the same channel.',
    attempted: 'The rule was applied, but the retry still failed.',
  },
} as const satisfies EnhancedCompatibilityRuleDefinition

export const ENHANCED_COMPATIBILITY_RULES = [
  RESPONSES_MISSING_REASONING_ITEM_RULE,
] as const satisfies readonly EnhancedCompatibilityRuleDefinition[]

export function findEnhancedCompatibilityRule(event: {
  rule_id?: string
  key?: string
}): EnhancedCompatibilityRuleDefinition | undefined {
  return ENHANCED_COMPATIBILITY_RULES.find(
    (rule) => rule.id === event.rule_id || rule.key === event.key
  )
}
