import i18n from './i18n';

export const adaptiveRoutingTranslations = {
  en: {
    自动分流: 'Adaptive routing',
    '同优先级渠道根据近期上游故障减少分流，保留少量探测；恢复后逐步回流。手动优先级和权重不变。短期状态仅保存在当前进程内，重启后重置。':
      'For channels with the same priority, recent upstream failures reduce traffic while a small amount remains for probing; traffic gradually returns after recovery. Manual priorities and weights are unchanged. Short-term state is kept in this process and resets after restart.',
  },
  'zh-TW': {
    自动分流: '自動分流',
    '同优先级渠道根据近期上游故障减少分流，保留少量探测；恢复后逐步回流。手动优先级和权重不变。短期状态仅保存在当前进程内，重启后重置。':
      '同優先級渠道依近期上游故障減少流量，保留少量探測；恢復後逐步回流。手動優先級與權重不變。短期狀態僅保存在目前程序內，重啟後重置。',
  },
  fr: {
    自动分流: 'Routage adaptatif',
    '同优先级渠道根据近期上游故障减少分流，保留少量探测；恢复后逐步回流。手动优先级和权重不变。短期状态仅保存在当前进程内，重启后重置。':
      "Pour les canaux de même priorité, les erreurs récentes de l'amont réduisent le trafic tout en conservant une faible part de sondage ; le trafic revient progressivement après rétablissement. Les priorités et poids manuels restent inchangés. L'état temporaire reste dans ce processus et est réinitialisé au redémarrage.",
  },
  ru: {
    自动分流: 'Адаптивная маршрутизация',
    '同优先级渠道根据近期上游故障减少分流，保留少量探测；恢复后逐步回流。手动优先级和权重不变。短期状态仅保存在当前进程内，重启后重置。':
      'Для каналов с одинаковым приоритетом недавние ошибки upstream уменьшают трафик, сохраняя небольшой объём проверок; после восстановления трафик постепенно возвращается. Ручные приоритеты и веса не меняются. Кратковременное состояние хранится только в этом процессе и сбрасывается после перезапуска.',
  },
  ja: {
    自动分流: '適応型ルーティング',
    '同优先级渠道根据近期上游故障减少分流，保留少量探测；恢复后逐步回流。手动优先级和权重不变。短期状态仅保存在当前进程内，重启后重置。':
      '同じ優先度のチャネルでは、最近の上流障害に応じてトラフィックを減らし、少量の探査を維持します。復旧後は段階的に戻します。手動の優先度と重みは変更しません。短期状態はこのプロセス内だけに保持され、再起動でリセットされます。',
  },
  vi: {
    自动分流: 'Định tuyến thích ứng',
    '同优先级渠道根据近期上游故障减少分流，保留少量探测；恢复后逐步回流。手动优先级和权重不变。短期状态仅保存在当前进程内，重启后重置。':
      'Với các kênh cùng mức ưu tiên, lỗi upstream gần đây sẽ giảm lưu lượng nhưng vẫn giữ một phần nhỏ để thăm dò; lưu lượng tăng dần trở lại sau khi khôi phục. Ưu tiên và trọng số thủ công không thay đổi. Trạng thái ngắn hạn chỉ tồn tại trong tiến trình này và được đặt lại khi khởi động lại.',
  },
};

export function registerAdaptiveRouting(instance = i18n) {
  for (const [language, strings] of Object.entries(
    adaptiveRoutingTranslations,
  )) {
    instance.addResourceBundle(language, 'translation', strings, true, true);
  }
}

registerAdaptiveRouting();
