import i18n from './i18n'

const en = {
  "报错自动移除渠道模型": "Remove a channel model on error",
  "按关键词自动移除模型": "Automatically remove models by error keywords",
  "自动移除模型的报错关键词": "Model removal error keywords",
  "仅从出错渠道移除对应模型，不删除渠道，不影响其他模型；此规则优先于整渠道自动禁用。": "Only removes the affected model from the failing channel, not the channel or other models. This rule takes precedence over disabling the entire channel.",
  "上游错误包含任意非空行时触发。留空不匹配；建议使用明确的模型停用错误，不要填写 404 或 Access denied 等宽泛内容。移除后不会自动恢复。": "Matches any non-empty line in an upstream error, case-insensitively. Empty rules match nothing. Use specific model retirement errors, not broad text such as 404 or Access denied. Removed models are not restored automatically.",
  "一行一个，不区分大小写": "One keyword per line, case-insensitive"
}
const zh = Object.fromEntries(Object.keys(en).map((key) => [key, key]))
const register = () => {
  i18n.addResourceBundle('en', 'translation', en, true, true)
  i18n.addResourceBundle('zh-CN', 'translation', zh, true, true)
}
if (i18n.isInitialized) register()
else i18n.on('initialized', function onInitialized() {
  i18n.off('initialized', onInitialized)
  register()
})
