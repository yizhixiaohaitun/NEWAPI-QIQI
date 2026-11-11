export function signatureRetryReferences(isAdmin, other, t) {
  const events = other?.admin_info?.compatibility_events;
  if (!isAdmin || !Array.isArray(events)) return [];

  return events.flatMap((event) => {
    if (event?.outcome !== 'retry_started') return [];
    if (event.rule_id === 'QIQI-EC-005' && event.key === 'claude_historical_thinking_signature_retry') {
      return [{
        key: t('兼容处理'),
        value: `${event.rule_id} · ${t('历史思考签名兼容')}：${t('已清理历史思考块，并在同渠道重试 1 次。')}`,
      }];
    }
    if (event.rule_id === 'QIQI-EC-006' && event.key === 'claude_tool_thinking_signature_retry') {
      return [{
        key: t('兼容处理'),
        value: `${event.rule_id} · ${t('工具轮次思考签名有损兼容')}：${t('已移除全部助手思考块并尝试在同渠道重试 1 次；不保证上游接受。')}`,
      }];
    }
    return [];
  });
}
