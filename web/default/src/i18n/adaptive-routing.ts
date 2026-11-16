import type { i18n as I18n } from 'i18next'

import i18n from './config'

export const adaptiveRoutingTranslations = {
  en: {
    'Automatic traffic routing': 'Automatic traffic routing',
    'Among channels with the same priority, recent upstream failures reduce traffic while retaining a small amount for probing. Traffic returns gradually after recovery. Manual priorities and weights are unchanged. Short-term state is local to this process and resets on restart.':
      'Among channels with the same priority, recent upstream failures reduce traffic while retaining a small amount for probing. Traffic returns gradually after recovery. Manual priorities and weights are unchanged. Short-term state is local to this process and resets on restart.',
  },
  zhCN: {
    'Automatic traffic routing': '自动分流',
    'Among channels with the same priority, recent upstream failures reduce traffic while retaining a small amount for probing. Traffic returns gradually after recovery. Manual priorities and weights are unchanged. Short-term state is local to this process and resets on restart.':
      '仅对同优先级渠道生效：根据近期上游故障减少异常渠道流量，同时保留少量请求用于探测；上游恢复后逐步回流。手动设置的优先级和权重不会改变。短期状态仅保存在当前进程内，重启后重置，也不会在多节点间共享。',
  },
  zhTW: {
    'Automatic traffic routing': '自動分流',
    'Among channels with the same priority, recent upstream failures reduce traffic while retaining a small amount for probing. Traffic returns gradually after recovery. Manual priorities and weights are unchanged. Short-term state is local to this process and resets on restart.':
      '僅對同優先級渠道生效：依近期上游故障減少異常渠道流量，同時保留少量請求用於探測；上游恢復後逐步回流。手動設定的優先級和權重不會改變。短期狀態僅保存在目前進程內，重啟後重置，也不會在多節點間共享。',
  },
  fr: {
    'Automatic traffic routing': 'Routage automatique du trafic',
  },
  ja: {
    'Automatic traffic routing': '自動トラフィックルーティング',
  },
  ru: {
    'Automatic traffic routing': 'Автоматическая маршрутизация трафика',
  },
  vi: {
    'Automatic traffic routing': 'Định tuyến lưu lượng tự động',
  },
} as const

export function registerAdaptiveRoutingTranslations(instance: I18n) {
  const register = () => {
    for (const [language, strings] of Object.entries(
      adaptiveRoutingTranslations
    )) {
      instance.addResourceBundle(language, 'translation', strings, true, true)
    }
  }
  if (instance.isInitialized) register()
  else instance.once('initialized', register)
}

registerAdaptiveRoutingTranslations(i18n)
