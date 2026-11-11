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
import type { i18n as I18n } from 'i18next'

import i18n from './config'

// Dedicated token strings; never rewrite shared locale JSON. Register after init as well.
export const tokenRequestCompatibilityTranslations = {
  en: {
    "For paired client-only tool continuations, on HTTP 400 or 429 with the exact invalid thinking signature error, before output and cancellation, compatibility may remove all assistant thinking and redacted thinking blocks and retry once on the same channel. This is lossy best effort; text, tools and settings stay intact, and upstream acceptance is not guaranteed. Ordinary rate limits do not trigger this recovery.": "For paired client-only tool continuations, on HTTP 400 or 429 with the exact invalid thinking signature error, before output and cancellation, compatibility may remove all assistant thinking and redacted thinking blocks and retry once on the same channel. This is lossy best effort; text, tools and settings stay intact, and upstream acceptance is not guaranteed. Ordinary rate limits do not trigger this recovery.",
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
  zhCN: {
    "For paired client-only tool continuations, on HTTP 400 or 429 with the exact invalid thinking signature error, before output and cancellation, compatibility may remove all assistant thinking and redacted thinking blocks and retry once on the same channel. This is lossy best effort; text, tools and settings stay intact, and upstream acceptance is not guaranteed. Ordinary rate limits do not trigger this recovery.": "纯客户端工具调用及结果配对完整时，仅在输出前且请求未取消、遇到 HTTP 400 或 429 的精确思考签名错误后，允许移除全部助手 thinking/redacted_thinking 并在同渠道重试一次。这是有损的尽力恢复，保留正文、工具和配置，不保证上游接受；普通限流不触发。",
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
  zhTW: {
    "For paired client-only tool continuations, on HTTP 400 or 429 with the exact invalid thinking signature error, before output and cancellation, compatibility may remove all assistant thinking and redacted thinking blocks and retry once on the same channel. This is lossy best effort; text, tools and settings stay intact, and upstream acceptance is not guaranteed. Ordinary rate limits do not trigger this recovery.": "純客戶端工具呼叫及結果配對完整時，僅在輸出前且請求未取消、遇到 HTTP 400 或 429 的精確思考簽名錯誤後，允許移除全部助手 thinking/redacted_thinking 並在同渠道重試一次。這是有損的盡力恢復，保留正文、工具和設定，不保證上游接受；一般限流不觸發。",
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
} as const

export function registerTokenRequestCompatibility(instance: I18n) {
  const register = () => {
    for (const [language, strings] of Object.entries(
      tokenRequestCompatibilityTranslations
    )) {
      instance.addResourceBundle(language, 'translation', strings, true, true)
    }
  }
  if (instance.isInitialized) register()
  else instance.once('initialized', register)
}

registerTokenRequestCompatibility(i18n)
