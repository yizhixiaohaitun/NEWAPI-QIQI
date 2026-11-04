import i18n from './config'

const translations = {
  en: {
    '兼容处理': 'Compatibility handling',
    '历史思考签名兼容': 'Historical thinking signature compatibility',
    '遇到特定历史思考签名错误时，清理历史思考块并在同渠道重试一次。':
      'On the specific historical thinking signature error, removes historical thinking blocks and retries once on the same channel.',
    '已清理历史思考块，并在同渠道重试 1 次。':
      'Removed historical thinking blocks and retried once on the same channel.',
    '已清理 {{referenceCount}} 个历史思考块，并在同渠道重试 1 次。':
      'Removed {{referenceCount}} historical thinking blocks and retried once on the same channel.',
  },
  zhCN: {
    '兼容处理': '兼容处理',
    '历史思考签名兼容': '历史思考签名兼容',
    '遇到特定历史思考签名错误时，清理历史思考块并在同渠道重试一次。': '遇到特定历史思考签名错误时，清理历史思考块并在同渠道重试一次。',
    '已清理历史思考块，并在同渠道重试 1 次。': '已清理历史思考块，并在同渠道重试 1 次。',
    '已清理 {{referenceCount}} 个历史思考块，并在同渠道重试 1 次。': '已清理 {{referenceCount}} 个历史思考块，并在同渠道重试 1 次。',
  },
  zhTW: {
    '兼容处理': '相容處理',
    '历史思考签名兼容': '歷史思考簽名相容',
    '遇到特定历史思考签名错误时，清理历史思考块并在同渠道重试一次。': '遇到特定歷史思考簽名錯誤時，清理歷史思考區塊並在同渠道重試一次。',
    '已清理历史思考块，并在同渠道重试 1 次。': '已清理歷史思考區塊，並在同渠道重試 1 次。',
    '已清理 {{referenceCount}} 个历史思考块，并在同渠道重试 1 次。': '已清理 {{referenceCount}} 個歷史思考區塊，並在同渠道重試 1 次。',
  },
}

const register = () => {
  for (const [language, strings] of Object.entries(translations)) {
    i18n.addResourceBundle(language, 'translation', strings, true, true)
  }
}
if (i18n.isInitialized) register()
else i18n.once('initialized', register)
