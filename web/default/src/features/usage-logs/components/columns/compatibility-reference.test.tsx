// @ts-expect-error Bun provides its test runner at runtime.
import { test } from 'bun:test'
import assert from 'node:assert/strict'
import { renderToStaticMarkup } from 'react-dom/server'
import { CompatibilityReference } from './compatibility-reference'
import { findEnhancedCompatibilityRule, ENHANCED_COMPATIBILITY_RULES } from '../../../qiq/enhanced-compatibility-rules'

test('tool signature retry is a lossy attempt, not a success', () => {
  const event = { rule_id: 'QIQI-EC-006', key: 'claude_tool_thinking_signature_retry', event_type: 'applied', action: 'strip_tool_turn_thinking_and_retry', outcome: 'retry_started', retried: true }
  for (const type of [2, 5]) {
    const html = renderToStaticMarkup(<CompatibilityReference log={{ type } as never} event={event} t={(key) => key} />)
    assert.match(html, /QIQI-EC-006/)
    assert.match(html, /有损/)
    assert.match(html, /不保证上游接受/)
    assert.doesNotMatch(html, /Recovered|历史思考签名兼容/)
  }
  assert.equal(findEnhancedCompatibilityRule({ key: event.key })?.id, event.rule_id)
  assert.equal(ENHANCED_COMPATIBILITY_RULES.some((rule) => rule.id === event.rule_id), false)
})

test('signature retry is an attempt, never a recovery', () => {
  const event = { rule_id: 'QIQI-EC-005', key: 'claude_historical_thinking_signature_retry', outcome: 'retry_started', count: 3 }
  const html = renderToStaticMarkup(<CompatibilityReference log={{ type: 2 } as never} event={event} t={(key, options) => String(options?.defaultValue ?? key).replace('{{referenceCount}}', String(options?.referenceCount))} />)
  assert.match(html, /历史思考签名兼容/)
  assert.match(html, /已清理 3 个历史思考块/)
  assert.match(html, /同渠道重试 1 次/)
  assert.doesNotMatch(html, /失败/)
  const withoutCount = renderToStaticMarkup(<CompatibilityReference log={{ type: 2 } as never} event={{ ...event, count: undefined }} t={(key) => key} />)
  assert.match(withoutCount, /已清理历史思考块/)
  assert.doesNotMatch(withoutCount, /已清理 1 个/)
  assert.doesNotMatch(html, /Recovered/)
  assert.equal(findEnhancedCompatibilityRule(event)?.settingKey, 'request_compatibility_enabled')
  assert.equal(ENHANCED_COMPATIBILITY_RULES.some((rule) => rule.id === event.rule_id), false)
})
