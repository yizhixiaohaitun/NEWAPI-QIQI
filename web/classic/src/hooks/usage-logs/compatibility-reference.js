export function signatureRetryReferences(isAdmin, other, t) {
  const events = other?.admin_info?.compatibility_events;
  if (!isAdmin || !Array.isArray(events)) return [];

  return events
    .filter(
      (event) =>
        event?.rule_id === 'QIQI-EC-005' &&
        event?.key === 'claude_historical_thinking_signature_retry' &&
        event?.outcome === 'retry_started',
    )
    .map((event) => ({
      key: t('兼容处理'),
      value: `${event.rule_id} · ${t('历史思考签名兼容')}：${t('已清理历史思考块，并在同渠道重试 1 次。')}`,
    }));
}
