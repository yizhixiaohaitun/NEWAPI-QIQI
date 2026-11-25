# 渠道重试规则（JSON）

在默认主题的「系统设置 → 模型与路由 → 路由可靠性」找到「渠道重试规则（JSON）」，粘贴数组并保存。以下示例适用于尚无报错规则的情况：

```json
[
  {
    "stream_hedge": {
      "delay_ms": 15000,
      "retry_same_channel_first": true,
      "models": ["claude-fable-5-2", "claude-opus-5-5"]
    }
  }
]
```

已有 `stream_hedge` 时直接编辑该条；否则把它作为独立的一项追加进原数组，不覆盖其他报错规则。先更新到支持 `models` 的网关版本，之后保存设置无需重启。本进程的后续请求立即使用，其他进程通过原有配置同步刷新。

## 流式请求未出有效内容

新版可用 `parallel_backup_scope` 选择双备用范围，不用逐条填写模型名。以下示例选中全部合格的 Claude 原生流式请求（包括已授权的新型号和别名）：

```json
[{"stream_hedge":{"delay_ms":15000,"retry_same_channel_first":true,"parallel_backup_scope":"claude"}}]
```

将范围值改为 `all`，可覆盖符合条件的 Claude Messages、OpenAI Chat Completions、OpenAI Responses 和 Gemini `streamGenerateContent`，按原生流式协议和渠道适配器判断，不按品牌名或前缀判断。非流式、跨协议转换和有状态请求仍沿用普通路径。例如，Responses 必须显式发送 `store: false`，不接受挂接已有会话或由上游执行的内置工具；不满足条件的渠道或请求不重复发出。

```json
[{"stream_hedge":{"delay_ms":15000,"retry_same_channel_first":true,"parallel_backup_scope":"all"}}]
```

`parallel_backup_scope` 与旧的 `parallel_backup_models` 二选一，且要求 `retry_same_channel_first: true`。现有八九站版本尚不识别新字段；本地新版升级前仍用精确名单，回滚前先移除新字段。

- `models` 可选，按请求中的模型名（含别名）精确匹配。留空数组 `[]` 不对任何模型启用；省略则不限制名字，仍受协议和安全条件限制。旧 `parallel_backup_models` 只支持 Claude 原生流且在同时填写 `models` 时须为其子集。
- 开启范围或精确并行名单后，第一段十五秒内没有有效内容，就保留原请求并同时启动同渠道和另一个同组兼容渠道的备用。仅有一次重试额度或没有合格第二渠道时，只发同渠道备用。备用只在本次实际分组内选路（包括 `auto` 令牌），不扩大路由权限。若不填范围且并行名单未配置或为空，同渠道优先模式仍先开一路，再等一个间隔才寻找另一路。回滚到不识别名单的网关前须移除 `parallel_backup_models`，不能只置为空数组。
- 心跳、空文本、只有开头元数据不算有效内容。重试共用原请求总时限，每次备用占用一次原有重试额度。平台只结算获胜的一路；上游可能对已接收的并行请求各自计费。
- `delay_ms` 必填，为 0 到 300000 的整数；0 显式关闭，包括覆盖启动参数。`retry_same_channel_first` 缺省为 `false`，表示一次延迟后直接尝试不同渠道。移除 `stream_hedge` 后恢复原 `STREAM_HEDGE_DELAY_MS` 行为（未设时默认关闭）。指定渠道、禁止换渠道的亲和配置和已提交响应仍不参加竞速。
- 同一数组只允许一个 `stream_hedge` 项，不能与报错匹配字段混写在同一对象中。
- `max_body_mb` 可选，为 1 到 128 的整数，表示允许启动备用的请求体上限（MB），省略时为 16。请求体超过上限或已落到磁盘缓存的请求不开备用，按普通单路处理，失败后仍走原有串行重试规则。每一路备用都会各自生成一份上游请求体，上限越大，并发时占用内存越多。旧版网关不认识这个字段，会整段读不进渠道重试规则；回滚到旧版前先删掉 `max_body_mb`。

```json
[{"stream_hedge":{"delay_ms":5000,"retry_same_channel_first":true,"parallel_backup_scope":"claude","max_body_mb":16}}]
```

## 上游报错换渠道

- 可填 `channel_id`（报错渠道的正整数编号）、`status`（400 到 599）、`error_code`（忽略大小写精确匹配）和 `message_contains`（忽略大小写的子串匹配）。至少填 `status`、`error_code` 或 `message_contains` 之一；同一条规则的条件必须同时满足，从上往下取第一条命中。
- `action` 必填：`retry` 交给原有串行重试和选路逻辑，`stop` 不再尝试。规则不指定备用渠道，不扩大原有重试预算；未命中继续用原有策略。普通报错规则不覆盖取消、指定渠道、禁止换渠道的亲和配置、已提交响应或既有优先的专用重试规则。
- 默认 `[]`。无效配置不会覆盖已保存的配置。上限 128 条报错规则，另可选一个流式配置，总大小 64 KiB。

也可以使用超级管理员登录态，通过 `PUT /api/option/` 保存，再通过 `GET /api/option/` 读回。`value` 是 JSON 数组的文本，例如：

```json
{
  "key": "ChannelRetryPolicy",
  "value": "[{\"channel_id\":123,\"status\":429,\"error_code\":\"upstream_busy\",\"action\":\"retry\"},{\"stream_hedge\":{\"delay_ms\":15000,\"retry_same_channel_first\":true}}]"
}
```
