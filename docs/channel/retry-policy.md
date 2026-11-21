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

`parallel_backup_scope` 与旧的 `parallel_backup_models` 二选一，且要求 `retry_same_channel_first: true`。已有 `models` 仍是有效的精确名字限制；希望免维护名单则一并移除 `models`。未设置范围或名单时不会同时发两路备用。旧版网关不读这个新字段；升级前仍用精确名单，回滚前先移除新字段。

- `models` 按请求中的模型名精确匹配，包含别名时须各自列出。空数组 `[]` 对任何模型都不启用；省略 `models` 则适用于所有符合条件的 Claude 原生流式请求，不再限制 Opus。升级前如果已有不带 `models` 的流式规则、但希望维持指定模型范围，请在升级后补上对应名单。非流式及其他协议不受这个选项控制。
- 按示例开启后，匹配的请求在十五秒仍无有效内容时，保留原请求，并行重试同一渠道一次。再等十五秒仍无有效内容，并有第二次重试额度和同组可用的兼容备用渠道时，才启动下一渠道，不会跨分组。显式报错仍受既有重试限制；一旦有一路输出有效内容，就只转发这一路并取消其余请求。
- 如要第一个间隔结束时同时启动两条备用，设置 `retry_same_channel_first: true`，并在 `parallel_backup_models` 中明确列出适用模型，例如 `"parallel_backup_models": ["claude-fable-5-2"]`。省略或传空数组保留原时序；如果同时设置了 `models`，并行名单必须是它的子集。仅有一次重试额度或没有合格第二渠道时，只发同渠道备用。额外备用只在本次实际分组内选路（包括 `auto` 令牌），不改变普通报错重试的权限。回滚到不识别新字段的网关前，必须移除 `parallel_backup_models`（不能只设为空数组）。
- 心跳、空文本、只有开头元数据不算有效内容。重试共用原请求总时限，每次备用占用一次原有重试额度。平台只结算获胜的一路；上游可能对已接收的并行请求各自计费。
- `delay_ms` 必填，为 0 到 300000 的整数；0 显式关闭，包括覆盖启动参数。`retry_same_channel_first` 缺省为 `false`，表示一次延迟后直接尝试不同渠道。移除 `stream_hedge` 后恢复原 `STREAM_HEDGE_DELAY_MS` 行为（未设时默认关闭）。指定渠道、禁止换渠道的亲和配置和已提交响应仍不参加竞速。
- 同一数组只允许一个 `stream_hedge` 项，不能与报错匹配字段混写在同一对象中。

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
