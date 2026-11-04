/*
Copyright (C) 2025 QuantumNous

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
import i18n from './i18n';

// Dedicated token strings; register once initialization completes, without shared locale edits.
export const tokenRequestCompatibilityTranslations = {
  en: {
    "历史思考签名兼容": "Historical thinking signature compatibility",
    "已清理历史思考块，并在同渠道重试 1 次。": "Historical thinking blocks removed; retried once on the same channel.",
    "兼容处理": "Compatibility action",
    'Only on the exact historical thinking signature error before output, removes thinking blocks from completed turns and retries once on the same channel; thinking stays enabled and success is not guaranteed.':
      'Only on the exact historical thinking signature error before output, removes thinking blocks from completed turns and retries once on the same channel; thinking stays enabled and success is not guaranteed.',
    'Request compatibility': 'Request compatibility',
    'Thinking format adaptation': 'Thinking format adaptation',
    'Opus 4.7 sampling parameter adaptation':
      'Opus 4.7 sampling parameter adaptation',
    'Enabled for new tokens; you can turn it off.':
      'Enabled for new tokens; you can turn it off.',
    'Turning this off stops the adjustments above; normal protocol conversion remains.':
      'Turning this off stops the adjustments above; normal protocol conversion remains.',
    'Raw passthrough keeps the original request unchanged.':
      'Raw passthrough keeps the original request unchanged.',
    'Adapts Claude thinking only for proven model capabilities.':
      'Adapts Claude thinking only for proven model capabilities.',
    'Omits non-default temperature for gpt-6-luna; keeps 1.':
      'Omits non-default temperature for gpt-6-luna; keeps 1.',
    'Removes temperature/top_p/top_k for Opus 4.7.':
      'Removes temperature/top_p/top_k for Opus 4.7.',
  },
  'zh-CN': {
    '历史思考签名兼容': '历史思考签名兼容',
    '已清理历史思考块，并在同渠道重试 1 次。': '已清理历史思考块，并在同渠道重试 1 次。',
    '兼容处理': '兼容处理',
    'Only on the exact historical thinking signature error before output, removes thinking blocks from completed turns and retries once on the same channel; thinking stays enabled and success is not guaranteed.':
      '仅在输出前遇到精确的历史思考签名错误时，移除已结束轮次的历史思考块并在同渠道重试一次；不关闭思考，也不保证成功。',
    'Request compatibility': '请求兼容',
    'Thinking format adaptation': '思考格式适配',
    'Opus 4.7 sampling parameter adaptation': 'Opus 4.7 采样参数适配',
    'Enabled for new tokens; you can turn it off.':
      '新建默认开启，可手动关闭。',
    'Turning this off stops the adjustments above; normal protocol conversion remains.':
      '关闭后不再自动调整上述参数；普通协议转换仍保留。',
    'Raw passthrough keeps the original request unchanged.':
      '原始透传保持请求原样，不修改字节。',
    'Adapts Claude thinking only for proven model capabilities.':
      '仅针对已证实的模型能力适配 Claude 思考格式。',
    'Omits non-default temperature for gpt-6-luna; keeps 1.':
      '省略 gpt-6-luna 的非默认 temperature；保留 1。',
    'Removes temperature/top_p/top_k for Opus 4.7.':
      '移除 Opus 4.7 的 temperature/top_p/top_k 参数。',
  },
  'zh-TW': {
    '历史思考签名兼容': '歷史思考簽名相容',
    '已清理历史思考块，并在同渠道重试 1 次。': '已清理歷史思考區塊，並在同渠道重試 1 次。',
    '兼容处理': '相容處理',
    'Only on the exact historical thinking signature error before output, removes thinking blocks from completed turns and retries once on the same channel; thinking stays enabled and success is not guaranteed.':
      '僅在輸出前遇到精確的歷史思考簽名錯誤時，移除已結束輪次的歷史思考區塊並在同渠道重試一次；不關閉思考，也不保證成功。',
    'Request compatibility': '請求相容',
    'Thinking format adaptation': '思考格式適配',
    'Opus 4.7 sampling parameter adaptation': 'Opus 4.7 取樣參數適配',
    'Enabled for new tokens; you can turn it off.':
      '新建預設開啟，可手動關閉。',
    'Turning this off stops the adjustments above; normal protocol conversion remains.':
      '關閉後不再自動調整上述參數；一般協定轉換仍保留。',
    'Raw passthrough keeps the original request unchanged.':
      '原始透傳保持請求原樣，不修改位元組。',
    'Adapts Claude thinking only for proven model capabilities.':
      '僅針對已證實的模型能力適配 Claude 思考格式。',
    'Omits non-default temperature for gpt-6-luna; keeps 1.':
      '省略 gpt-6-luna 的非預設 temperature；保留 1。',
    'Removes temperature/top_p/top_k for Opus 4.7.':
      '移除 Opus 4.7 的 temperature/top_p/top_k 參數。',
  },
  fr: {
    "历史思考签名兼容": "Compatibilité des signatures de réflexion historiques",
    "已清理历史思考块，并在同渠道重试 1 次。": "Blocs de réflexion historiques supprimés ; une nouvelle tentative sur le même canal.",
    "兼容处理": "Action de compatibilité",
    'Request compatibility': 'Compatibilité des requêtes',
    'Thinking format adaptation': 'Adaptation du format de réflexion',
    'Opus 4.7 sampling parameter adaptation':
      'Adaptation des paramètres Opus 4.7',
    'Enabled for new tokens; you can turn it off.':
      'Activé pour les nouveaux jetons ; désactivable.',
    'Turning this off stops the adjustments above; normal protocol conversion remains.':
      'Désactiver arrête ces ajustements ; la conversion normale des protocoles reste active.',
    'Raw passthrough keeps the original request unchanged.':
      'Le passthrough brut conserve la requête originale sans modification.',
    'Adapts Claude thinking only for proven model capabilities.':
      'Adapte Claude thinking uniquement aux capacités confirmées du modèle.',
    'Omits non-default temperature for gpt-6-luna; keeps 1.':
      'Omet temperature non standard pour gpt-6-luna ; conserve 1.',
    'Removes temperature/top_p/top_k for Opus 4.7.':
      'Supprime temperature/top_p/top_k pour Opus 4.7.',
  },
  ru: {
    "历史思考签名兼容": "Совместимость исторических подписей размышлений",
    "已清理历史思考块，并在同渠道重试 1 次。": "Исторические блоки размышлений удалены; повторено один раз по тому же каналу.",
    "兼容处理": "Действие совместимости",
    'Request compatibility': 'Совместимость запросов',
    'Thinking format adaptation': 'Адаптация формата размышлений',
    'Opus 4.7 sampling parameter adaptation': 'Адаптация параметров Opus 4.7',
    'Enabled for new tokens; you can turn it off.':
      'Для новых токенов включено; можно отключить.',
    'Turning this off stops the adjustments above; normal protocol conversion remains.':
      'Отключение прекращает эти изменения; обычное преобразование протоколов сохраняется.',
    'Raw passthrough keeps the original request unchanged.':
      'Исходный passthrough не меняет запрос и его байты.',
    'Adapts Claude thinking only for proven model capabilities.':
      'Адаптирует Claude thinking только для подтверждённых возможностей модели.',
    'Omits non-default temperature for gpt-6-luna; keeps 1.':
      'Для gpt-6-luna удаляет нестандартную temperature; сохраняет 1.',
    'Removes temperature/top_p/top_k for Opus 4.7.':
      'Удаляет temperature/top_p/top_k для Opus 4.7.',
  },
  ja: {
    "历史思考签名兼容": "過去の思考署名の互換性",
    "已清理历史思考块，并在同渠道重试 1 次。": "過去の思考ブロックを除去し、同じチャネルで一度再試行しました。",
    "兼容处理": "互換処理",
    'Request compatibility': 'リクエスト互換性',
    'Thinking format adaptation': '思考形式の適応',
    'Opus 4.7 sampling parameter adaptation': 'Opus 4.7 サンプリング設定の適応',
    'Enabled for new tokens; you can turn it off.':
      '新規トークンは既定で有効。手動でオフにできます。',
    'Turning this off stops the adjustments above; normal protocol conversion remains.':
      'オフにすると上記の調整を停止します。通常のプロトコル変換は維持します。',
    'Raw passthrough keeps the original request unchanged.':
      '生のパススルーでは元のリクエストとバイト列を変更しません。',
    'Adapts Claude thinking only for proven model capabilities.':
      '確認済みのモデル能力にのみ Claude thinking を適応します。',
    'Omits non-default temperature for gpt-6-luna; keeps 1.':
      'gpt-6-luna の既定値以外の temperature を省略し、1 は保持します。',
    'Removes temperature/top_p/top_k for Opus 4.7.':
      'Opus 4.7 の temperature/top_p/top_k を除去します。',
  },
  vi: {
    "历史思考签名兼容": "Tương thích chữ ký suy nghĩ cũ",
    "已清理历史思考块，并在同渠道重试 1 次。": "Đã xóa các khối suy nghĩ cũ và thử lại một lần trên cùng kênh.",
    "兼容处理": "Xử lý tương thích",
    'Request compatibility': 'Tương thích yêu cầu',
    'Thinking format adaptation': 'Điều chỉnh định dạng suy nghĩ',
    'Opus 4.7 sampling parameter adaptation':
      'Điều chỉnh tham số lấy mẫu Opus 4.7',
    'Enabled for new tokens; you can turn it off.':
      'Mặc định bật cho token mới; có thể tắt thủ công.',
    'Turning this off stops the adjustments above; normal protocol conversion remains.':
      'Tắt sẽ ngừng các điều chỉnh trên; chuyển đổi giao thức thông thường vẫn được giữ.',
    'Raw passthrough keeps the original request unchanged.':
      'Truyền thẳng thô giữ nguyên yêu cầu và byte gốc.',
    'Adapts Claude thinking only for proven model capabilities.':
      'Chỉ điều chỉnh Claude thinking cho khả năng mô hình đã được xác nhận.',
    'Omits non-default temperature for gpt-6-luna; keeps 1.':
      'Bỏ temperature khác mặc định cho gpt-6-luna; giữ nguyên 1.',
    'Removes temperature/top_p/top_k for Opus 4.7.':
      'Loại bỏ temperature/top_p/top_k cho Opus 4.7.',
  },
};

export function registerTokenRequestCompatibility(instance) {
  const register = () => {
    for (const [language, strings] of Object.entries(
      tokenRequestCompatibilityTranslations,
    )) {
      instance.addResourceBundle(language, 'translation', strings, true, true);
    }
  };
  if (instance.isInitialized) register();
  else {
    // The classic theme's i18next 23 supports on/off, not once.
    const onInitialized = () => {
      instance.off('initialized', onInitialized);
      register();
    };
    instance.on('initialized', onInitialized);
  }
}

registerTokenRequestCompatibility(i18n);
