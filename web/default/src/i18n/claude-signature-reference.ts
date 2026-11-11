import i18n from './config'

const translations = {
  en: {
    "工具续聊思考签名有损恢复": "Tool continuation thinking signature lossy recovery",
    "为优先恢复工具续聊，移除全部助手思考块并尝试同渠道重试一次；这是有损处理，不保证上游接受。": "To prioritize tool continuation, removes all assistant thinking blocks and attempts one retry on the same channel; this is lossy and upstream acceptance is not guaranteed.",
    "已进行有损处理：移除全部助手思考块并尝试同渠道重试一次，不保证上游接受。": "Lossy handling applied: removed all assistant thinking blocks and attempted one retry on the same channel; upstream acceptance is not guaranteed.",
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
    "工具续聊思考签名有损恢复": "工具续聊思考签名有损恢复",
    "为优先恢复工具续聊，移除全部助手思考块并尝试同渠道重试一次；这是有损处理，不保证上游接受。": "为优先恢复工具续聊，移除全部助手思考块并尝试同渠道重试一次；这是有损处理，不保证上游接受。",
    "已进行有损处理：移除全部助手思考块并尝试同渠道重试一次，不保证上游接受。": "已进行有损处理：移除全部助手思考块并尝试同渠道重试一次，不保证上游接受。",
    '兼容处理': '兼容处理',
    '历史思考签名兼容': '历史思考签名兼容',
    '遇到特定历史思考签名错误时，清理历史思考块并在同渠道重试一次。': '遇到特定历史思考签名错误时，清理历史思考块并在同渠道重试一次。',
    '已清理历史思考块，并在同渠道重试 1 次。': '已清理历史思考块，并在同渠道重试 1 次。',
    '已清理 {{referenceCount}} 个历史思考块，并在同渠道重试 1 次。': '已清理 {{referenceCount}} 个历史思考块，并在同渠道重试 1 次。',
  },
  zhTW: {
    "工具续聊思考签名有损恢复": "工具續聊思考簽名有損恢復",
    "为优先恢复工具续聊，移除全部助手思考块并尝试同渠道重试一次；这是有损处理，不保证上游接受。": "為優先恢復工具續聊，移除全部助手思考區塊並嘗試同渠道重試一次；這是有損處理，不保證上游接受。",
    "已进行有损处理：移除全部助手思考块并尝试同渠道重试一次，不保证上游接受。": "已進行有損處理：移除全部助手思考區塊並嘗試同渠道重試一次，不保證上游接受。",
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
