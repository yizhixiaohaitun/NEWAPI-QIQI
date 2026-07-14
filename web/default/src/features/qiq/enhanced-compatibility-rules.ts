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
  retryTimesSettingKey?: `qiqi_setting.${string}`
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
    'Detects the exact OpenAI Responses 400 error for invalid empty rs_ reasoning references. After verifying the reported item, it removes all reference-only empty rs_ reasoning items and retries once on the same channel; reasoning content, encrypted state, messages, and tool calls are preserved.',
  referenceKeys: {
    recovered:
      'Recovered after removing invalid reasoning references ({{referenceCount}}) and retrying once on the same channel.',
    recommended:
      'This error matches the rule and contains {{referenceCount}} invalid empty reasoning references. Enable it to remove them all and retry once on the same channel.',
    attempted:
      'Removed {{referenceCount}} invalid reasoning references, but the retry still failed.',
  },
} as const satisfies EnhancedCompatibilityRuleDefinition

export const RESPONSES_STREAM_ERROR_RETRY_RULE = {
  id: 'QIQI-EC-002',
  key: 'responses_stream_error_retry',
  settingKey: 'qiqi_setting.responses_stream_error_retry_enabled',
  retryTimesSettingKey: 'qiqi_setting.responses_stream_error_retry_times',
  shortNameKey: 'Early Responses stream error recovery',
  descriptionKey:
    'Buffers only initial OpenAI Responses control events. If an upstream error or premature EOF arrives before output, it is converted into a retryable error and retried through the normal channel strategy. Once output starts, streaming continues without retry to prevent duplicate content.',
  referenceKeys: {
    recovered:
      'Recovered after an upstream Responses error arrived before output and the request was retried transparently.',
    recommended:
      'This stream failed before producing output. Enable the rule to retry early upstream errors automatically.',
    attempted:
      'The early Responses stream retry rule was applied, but all configured attempts failed.',
  },
} as const satisfies EnhancedCompatibilityRuleDefinition

export const AZURE_RESPONSES_RESOURCE_AFFINITY_RULE = {
  id: 'QIQI-EC-003',
  key: 'azure_responses_resource_affinity',
  settingKey: 'qiqi_setting.azure_responses_resource_affinity_enabled',
  shortNameKey: 'Azure Responses resource affinity protection',
  descriptionKey:
    'Prioritizes routing requests with previous_response_id or item_reference back to the Azure resource that created that state, preventing 400 errors caused by a different Azure OpenAI resource. If the original resource is unavailable, the request fails safely instead of being routed randomly across resources.',
  referenceKeys: {
    recovered:
      'Prioritizes routing requests with previous_response_id or item_reference back to the Azure resource that created that state, preventing 400 errors caused by a different Azure OpenAI resource. If the original resource is unavailable, the request fails safely instead of being routed randomly across resources.',
    recommended:
      'Prioritizes routing requests with previous_response_id or item_reference back to the Azure resource that created that state, preventing 400 errors caused by a different Azure OpenAI resource. If the original resource is unavailable, the request fails safely instead of being routed randomly across resources.',
    attempted:
      'Prioritizes routing requests with previous_response_id or item_reference back to the Azure resource that created that state, preventing 400 errors caused by a different Azure OpenAI resource. If the original resource is unavailable, the request fails safely instead of being routed randomly across resources.',
  },
} as const satisfies EnhancedCompatibilityRuleDefinition

export const ENHANCED_COMPATIBILITY_RULES = [
  RESPONSES_MISSING_REASONING_ITEM_RULE,
  RESPONSES_STREAM_ERROR_RETRY_RULE,
  AZURE_RESPONSES_RESOURCE_AFFINITY_RULE,
] as const satisfies readonly EnhancedCompatibilityRuleDefinition[]

export function findEnhancedCompatibilityRule(event: {
  rule_id?: string
  key?: string
}): EnhancedCompatibilityRuleDefinition | undefined {
  return ENHANCED_COMPATIBILITY_RULES.find(
    (rule) => rule.id === event.rule_id || rule.key === event.key
  )
}
