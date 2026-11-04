import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { signatureRetryReferences } from './compatibility-reference.js';

const event = { rule_id: 'QIQI-EC-005', key: 'claude_historical_thinking_signature_retry', outcome: 'retry_started', count: 3 };
const t = (text) => text;

test('日志详情入口使用受权限控制的事件筛选', () => {
  const source = readFileSync(new URL('./useUsageLogsData.jsx', import.meta.url), 'utf8');
  assert.match(source, /expandDataLocal\.push\(\.\.\.signatureRetryReferences\(isAdminUser, other, t\)\)/);
});

test('管理员只看到实际记录的签名重试，不宣称恢复或猜测数量', () => {
  assert.deepEqual(signatureRetryReferences(true, { admin_info: { compatibility_events: [event] } }, t), [{
    key: '兼容处理',
    value: 'QIQI-EC-005 · 历史思考签名兼容：已清理历史思考块，并在同渠道重试 1 次。',
  }]);
  assert.deepEqual(signatureRetryReferences(false, { admin_info: { compatibility_events: [event] } }, t), []);
  assert.deepEqual(signatureRetryReferences(true, {}, t), []);
  assert.deepEqual(signatureRetryReferences(true, { admin_info: { compatibility_events: [{ ...event, outcome: 'accepted' }, { key: 'other', outcome: 'retry_started' }] } }, t), []);
});
