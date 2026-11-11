# Claude 长工具续轮 signature 兼容（仅本地）

## 问题和选择

- 基线：e7fc2c35221f82e1d0995b583ecfdf83fb4cdc91；沿用仓库官方 new-api 基线，不混入其他工作区资料。
- 用户报告 89:3000 返回 HTTP 429、`Invalid `signature` in `thinking` block`，明确禁止更新服务器。
- 完整 UTF-8 附件 265539 bytes、SHA256 75f6c814a0ceea3ec6a1e06ae321c17d2940eb9391463c0cb2a9773df2e67ebf；57 条消息、19 个 thinking，末尾为工具结果，没有新的真实用户轮次。附件正文、签名、工具资料不入仓。
- 用户明确选择“优先恢复续聊”：精确拒绝后允许删除无法继续使用的思考内容。有损最佳努力，不承诺真实上游接受，不本地验签或重新生成签名，不把错误 content.44 强配客户端索引。

## 实现边界

- `ClaudeHelper` 状态映射前识别 400/429 的完整精确签名错误；普通限流及其他状态不触发。请求兼容开启、非 raw、未写出/Flush、请求及上游上下文仍有效时才考虑恢复。
- 最后真实输入后的纯客户端工具链必须 ID 唯一且全部配对；活动 thinking 存在时，删除所有 assistant thinking/redacted_thinking，不保留依赖旧前缀的签名。消息数、正文、工具调用/结果、顺序、thinking/output_config 配置保持。
- 未知内容、server tools、混合用户输入、重复/缺失 ID、缺结果、助手预填、删除后空消息均不处理。字符串形式的已结束历史回复保留。无活动 thinking 时继续旧历史策略；旧策略遇任何思考删除后空消息整次原子放弃，不能部分清理后留下依赖签名。
- Gin 入站上下文按渠道 ID 保存恢复额度，控制器同渠道换 key 重新进入 helper 也最多一次兼容重发。不同渠道各一次；外层普通重试策略不修改。二次失败保留真实错误。
- EC006 独立于 EC005，管理员日志只写 `retry_started`，两主题说明“有损、尝试、不保证接受”，不写原始签名、不伪造恢复成功/数量，普通用户 admin_info 隔离保持。令牌能力说明同步。
- 数值/未知字段保真仅承诺首次实际出站到重发；已有 DTO/AWS SDK 首次 any 序列化边界不在本轮修复范围。

## 验证证据

全部测试仅本机 localhost/mock，不访问或更新服务器，不调用生产付费模型。

- 旧 handler overlay 的 HTTP429 与工具续轮反例均失败，证明新回归可抓到原缺陷；不将已修复版本的通过称为旧版失败。
- `go test ./relay -run "TestClaude(Signature|ToolSignature)|TestStripHistoricalClaudeThinking" -count=1 -v`，设置 `NEWAPI_CLAUDE_TOOL_SIGNATURE_ATTACHMENT` 为本机附件：187 个 PASS 行，1 个 SKIP（旧 54 消息附件可选入口，本次未提供），exit 0。
- 精确附件四路径 Claude/AWS、流/非流：首发 57 消息及签名保留；二次只删除 19 thinking，独立 oracle 比较正文/工具/配置不改。实际 AWS SDK 传输被 loopback transport 强制改到本地。
- 覆盖成功不变、SK/raw/已输出/Flush/取消/总期限、400/429/其他错误、映射前状态、仅一次、二次不同错误、原子历史、工具配对边界、helper 同渠道重入、首发到重发大数工具 input 保真。
- `go test ./dto ./relay ./relay/channel/claude ./relay/channel/aws ./relay/channel/vertex ./service ./service/relayconvert/... -count=1` exit 0。
- 默认主题 6 项测试、经典主题 6 项测试均通过；Bun 使用 1.3.14。
- 两主题 production build 通过；本轮默认前端 8 文件 oxlint 0 错误/警告。
- 默认全量 typecheck 被已有 `model-mutate-drawer.tsx` 的 `ModelSettings.global.auto_chat_to_responses_enabled` 缺失阻塞；全量 lint 有其他未改文件的历史错误，未在本轮扩大修复。
- 实际默认日志组件配生产 CSS 的 280px 本地隔离预览已截屏目视，明确显示 EC006 有损及不保证接受。截图位于本地临时证据目录，不是线上日志。
- 独立审查发现并复核关闭：F1 历史纯思考消息残留签名；F2 helper 重入刷新兼容预算。最终未发现明显新缺陷。未实跑完整控制器调度或真实上游，不能宣称线上原症状消失。

## 交付和回退

- 仅提交本地仓库；未 push、未构建发布镜像、未更新/访问生产服务器（含 89:3000/3001）。
- 回退：反向撤销本次本地提交即可；无需数据库/配置迁移。关闭既有 SK 请求兼容或 raw 透传时本次恢复不运行。
- 本机临时证据：`tmp/claude-signature-acceptance.log`、`claude-signature-full-regression.log`、`claude-signature-{default,classic}-tests.log`、`claude-signature-{default,classic}-build.log`、`claude-signature-{typecheck,lint,changed-lint}.log`、`signature-baseline-negative.log`、`signature-ui/preview.png`。这些不是仓库交付文件，用户正文不复制进报告。
