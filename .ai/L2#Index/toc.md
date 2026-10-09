# NEWAPI-QIQI L2#规范索引 (Index)

> **定位**：NEWAPI-QIQI 项目规范索引  
> **架构**：ZERO v2.0.0 六层架构  
> **适用范围**：new-api 本地开发、CI 构建、线上部署

---

## 📋 快速导航

先读项目级规则，再读 ZERO 通用规则。

### 项目级最高优先级

- **NEWAPI-QIQI 开发规范** - `00.project-01.development-rules.md`
  - Docker 镜像必须默认在 GitHub 构建
  - 禁止默认在业务服务器上构建 Docker 镜像
  - 生产服务器只负责拉取镜像、重启容器、健康检查和回滚
  - 官方 new-api 基线必须清晰

### ZERO 通用理念

ZERO 是以文件夹路径为唯一主体的命名规范框架（路径为王）。本项目采用其六层 `.ai` 架构和通用规范。

### 📂 目录说明

- **L3#Standards/standards/** - 完整规范（当前唯一规范主版本）
  - 核心架构原则
  - 路径映射规范
  - API、数据库、前端、质量规范
  - 详细说明和多个例子

- **L3#Standards/references/** - 参考实现
  - 管理后台布局
  - 组件实现
  - 最佳实践示例

- **L3#Standards/inspirations/** - 灵感来源库
  - Claude-Code-Source 参考
  - 企业级项目经验

### 🗂️ 主题索引

#### 工具与开发流程

- **Hooks 与规范同步** - `09.tool-04.hooks.md`
  - Hook 配置规范（支持 hook 的 IDE）
  - 规范同步冲突检测流程（开发方法论级，适用所有环境）
  - 自动触发 vs 手动执行
  - 逐步纯化历史文档

- **脚本管理** - `09.tool-01.gogogo-sh.md`
  - gogogo.sh 统一入口脚本
  - 部署、开发、缓存管理

- **AI 记忆体系管理** - `09.tool-03.ai-gogogo.md`
  - gogogo.sh 8：IDE 映射管理（L1#项目概览 (Overview)/L2#规范索引 (Index)/L3#完整规范 (Standards) → 各 IDE）
  - gogogo.sh ai：AI 体系状态、Kiro specs 同步、L4#操作日志 (Changelog) 日志

- **AI 记忆体系架构** - `10.ai-memory-01.architecture.md`
  - 六层架构（L0#工作执行 (Execution) - L5#知识图谱 (Knowledge)）
  - 文档蒸馏与映射

- **六层架构命名规范** - `10.ai-memory-02.naming.md`
  - 六层架构文件命名规范
  - 各层命名方式和分类编号

- **六层架构官方命名对照表** - `10.ai-memory-03.six-layer-naming.md`
  - 中英文命名对照表
  - 文件夹和文档引用格式
  - 从旧七层迁移到六层的步骤

#### 工作流与集成

- **工作流集成** - `08.workflow-00.integration.md`
  - CI/CD 集成
  - Git 工作流
  - 部署流程
  - 自动化测试

- **na.q.srl 部署排障经验** - `../L4#Changelog/2026-07-09-newapi-qiqi-na-q-srl-deploy-lessons.md`
  - SSH 必须优先使用 `.env` 的 `SERVER_IP` 和 `QIQI_SSH_KEY`
  - 生产 compose 文件是 `/opt/new-api/docker-compose.prod.yml`
  - 端口冲突、模板 compose 误用、健康检查和嵌套 git 仓库处理经验

### 功能入口：节日统一消耗折扣

- **管理配置与存储**：`setting/ratio_setting/group_ratio.go` 注册 `group_ratio_setting.festival_discount_enabled`（默认关闭）和 `group_ratio_setting.festival_discount_factor`（默认 1）；`controller/option.go` 在写入前验证有限值且 `0 < factor <= 1`。
- **管理前端**：default 分组定价入口 `/system-settings/billing/group-pricing`，表单 `web/default/src/features/system-settings/models/group-ratio-form.tsx`，保存 `web/default/src/features/system-settings/models/ratio-settings-card.tsx`；classic 入口 `web/classic/src/pages/Setting/Ratio/GroupRatioSettings.jsx`。两套前端的可视化/JSON 模式均保留折扣控件；保存开启时先写系数、最后开活动，关闭时先关活动，避免系数保存失败却启用旧值。classic 从配置字符串还原布尔值与数字。
- **计费快照与覆盖**：`relay/helper/price.go` 在请求计价时冻结活动开关/系数到 `types.PriceData`，分层表达式另存于 `billingexpr.BillingSnapshot`；同步 token、固定价格、音频/实时及任务预扣均在原分组/特殊倍率后乘一次。
- **异步结算与退款**：`model.TaskBillingContext` 持久化任务提交时系数，`service/task_billing.go` 的 token 重算、差额结算和退款复用该快照，不读取活动新值。
- **展示与审计**：`controller/pricing.go` 返回已折扣的用户分组倍率；`service/log_info_generate.go` 与 `service/task_billing.go` 写入 `festival_discount_enabled/factor`，充值倍率不参与。
- **重试与并发读写**：`relay/common/relay_info.go` 保存首次计价快照，`relay/helper/price.go` 在同步和任务重新计价时复用；`setting/config/config.go` 的锁保护活动配置读取及单字段更新（`model/option.go`），避免同一字段并发读写；两次 option 请求不是跨字段事务，开启/关闭的安全写入顺序由前端保证。
- **异步边界**：`controller/relay.go` 标记新任务 `BillingSnapshotCaptured`，`service/task_billing.go` 对新任务持久化的零模型/分组倍率不回退当前值，旧任务无此标记时保留历史查找；`service/task_polling.go` 的 adaptor 非零返回值是最终额度，现有生产 adaptor 均返回零，不在此分支二次打折。
- **回归入口**：`setting/ratio_setting/festival_discount_test.go`、`relay/helper/price_test.go`、`relay/helper/festival_retry_test.go`、`service/festival_discount_test.go`、`service/task_billing_test.go`、`pkg/billingexpr/billingexpr_test.go`；聚焦运行 `go test ./setting/ratio_setting ./types ./pkg/billingexpr ./relay/helper ./service ./controller ./relay`。

### Relay 对外错误隐私边界

- `middleware/distributor.go`：首次渠道选择失败或无渠道时，对外仅返回模型和请求 ID；分组、选中分组及底层错误只写后台日志。
- `controller/relay.go#getChannel`：重试渠道选择采用同样边界，保留原错误码和跳过重试语义。
- `middleware/auth.go#TokenAuth`：分组权限和弃用错误不返回具体分组名，后台日志保留定位信息。
- `i18n/locales/{en,zh-CN,zh-TW}.yaml`：渠道错误翻译不得包含分组或底层错误占位符。
- `service/upstream_resource.go#SanitizeFinalRelayError`：最终 HTTP/WS 错误边界同时识别上游中英文分组权限/无渠道错误；检查包装后的 message、`RelayError`/metadata/raw body，复制并净化对外错误，保留状态码、错误类型/代码、重试标记及原对象供渠道健康和后台诊断。
- `types/group_error_privacy.go`：集中定义保守语义识别与 OpenAI/Claude/嵌套错误 envelope 净化；覆盖实际的 group 下渠道获取失败、当前分组下模型无渠道、分组停用/无权限措辞，但不按固定分组名匹配。已识别错误按最小标准信封重建，仅保守保留合法错误 type/code，丢弃 provider-controlled param、metadata/details、raw/nested/数组诊断，避免敏感名称换字段泄露。
- `relay/helper/common.go`：已提交的 OpenAI、Claude、Responses SSE 与 WebSocket `WssPublicString`/`WssPublicObject` 结构化错误写出前执行同一净化；先解析 JSON 再按解码文本识别，覆盖 unicode escapes、流式数组、bare string 与嵌套 `response.error` 并统一重建。普通 completion/正常 WS 帧/拒答内容不属于错误 envelope，不改写；原始流错误只记后台日志。`relay/channel/openai/relay_realtime.go` 仅上游到客户端调用公开写出口，客户到上游仍用原始 `WssString`，不净化客户输入。
- `controller/relay.go#respondTaskError` / `publicMidjourneyError`：任务与 Midjourney 的独立错误出口同样只修改公开副本；任务检查 message、内部 Error 和附加 Data，已识别分组错误不透传附加诊断，保留状态码、计费/重试标记与后台原件。
- `model/task.go#CopyForPublicResponse` / `model/task_detail.go#SanitizeTaskPublicErrorJSON`：持久化任务的 FailReason/Data 仅在公开副本上处理；数据库原件继续供轮询、重试、计费和诊断。只处理显式失败字段和错误信封，不改写正常 prompt/output。
- `relay/relay_task.go`：`TaskModel2Dto` 统一供列表/详情使用；视频 converter 读取公开任务副本，fetch 最终响应也检查错误信封。`controller/task.go` 复用上述 DTO 与既有凭据清理。`relay/mjproxy_handler.go#coverMidjourneyTaskDto` 在公开 DTO 净化 FailReason/Description 及失败 Result 派生字段；Midjourney submit 的原始响应直写也在持久化后执行公开净化，正常成功提交字节与原始任务均不改写。
- 任务旁路回归：`controller/task_group_privacy_test.go`、`controller/relay_retry_test.go`、`model/task_group_privacy_test.go`、`relay/relay_task_group_privacy_test.go`；验证失败字段、fetch 真实写出、附加诊断、公开副本不污染原件与普通成功输出不变。
- 回归入口：`middleware/distributor_privacy_test.go`、`controller/relay_group_privacy_test.go`、`types/group_error_privacy_test.go`、`service/upstream_resource_test.go`、`relay/helper/group_error_privacy_test.go`；覆盖本地无渠道/数据库失败、上游完整 raw body 与嵌套序列化、中英文随机分组、OpenAI/Claude/Responses 流错误、后台保留原文及非分组错误/正常模型文本不受影响。

### Claude 非流式 refusal 解释兼容

- `dto/claude.go#ClaudeResponse` / `ClaudeStopDetails`：解析上游非流式 `stop_reason` 与可选 `stop_details.explanation`；缺少解释时不生成替代文案。
- `service/relayconvert/internal/claude_messages/to_oai_chat_resp.go#ResponseClaude2OpenAI`：仅在明确 `refusal`、没有正常正文且解释非空时，将上游解释逐字写入 OpenAI `message.refusal`，并按既有流式兼容语义同步给只读取 `message.content` 的客户端；已有正文、普通 `end_turn` 不受影响。
- `relay/channel/claude/relay-claude.go#HandleClaudeResponseData`：OpenAI 格式调用上述转换；Claude 原生格式继续直接透传上游响应字节。
- 回归入口：`service/relayconvert/internal/claude_messages/to_oai_chat_resp_test.go` 覆盖精确空拒绝、解释逐字保留且不重复、已有正文和普通结束；`relay/channel/claude/relay_claude_test.go` 从真实非流式 handler 覆盖 OpenAI 可见性与 Claude 原生字节不变。

### 分组与渠道重试选择链路

- `middleware/distributor.go#Distribute`：首次随机选择或渠道亲和命中时，在请求上下文记录实际 auto 分组；亲和命中同时记录配置索引及已访问分组。
- `service/channel_select.go#CacheGetRandomSatisfiedChannel`：跨分组开启后，重试按配置顺序选择尚未访问的可用分组，每个新分组从其首选优先级开始；无渠道分组直接跳过，耗尽后不回绕。请求内单 key 渠道一旦失败，后续选择会排除该渠道，即使它同时属于多个 Auto 分组；固定组及关闭跨组时同样不回打已失败渠道，但多 key 渠道保留既有 key 轮换。
- `model/channel_cache.go#GetRandomSatisfiedChannelExcluding` / `model/ability.go#GetChannelExcluding`：缓存与数据库选择路径都先排除请求内失败渠道，再从最高尚可用优先级选择，避免重试索引跳过未尝试的优先级。
- `controller/relay.go` / `controller/seedance_asset.go`：Relay、RelayTask 与 Seedance 共用 `RetryParam` 和请求级失败渠道集合；仅在错误满足重试策略后标记失败。`shouldRetry` 先执行显式 SkipRetry、剩余预算和指定渠道等停止条件，再处理 `channel:*` 与资源不足错误，避免指定渠道切换到组内其他渠道；客户端取消、亲和禁止重试及请求总超时同样停止。
- 回归入口：`service/channel_select_cross_group_test.go` 覆盖分组顺序与边界；`service/channel_select_failed_channel_test.go`、`model/channel_excluding_test.go` 和 `model/channel_excluding_path_test.go` 覆盖固定组、缓存/数据库优先级一致性及高级路由过滤后选择最高兼容层级；`controller/relay_auto_group_http_test.go` 从真实 `/v1/chat/completions` 入口连接 A=500、B=200 mock 上游，覆盖同一渠道横跨两个 Auto 分组时 `use_channel=461,464`；`controller/relay_fixed_group_http_test.go` 先建立真实 `/v1/messages` 请求头亲和，再验证固定组 A=503（上游分组无可用渠道）后立即走 B=200 且 `use_channel=461,464`；`controller/relay_auto_group_retry_test.go` 覆盖控制器逐次选择及耗尽。

### 渠道固定输入 Token 减免

- `dto/channel_settings.go#ChannelSettings`：渠道设置保存 `input_token_deduction` 与可选 `input_token_deduction_by_group`；分组条目（包括显式 `0`）按成功请求最终 `RelayInfo.UsingGroup` 优先于渠道默认值。
- `model/channel.go#ValidateSettings`：拒绝默认值或分组值中的负数；设置沿现有渠道缓存和请求上下文传递，不新增请求级数据库查询。
- `service/input_token_deduction.go`：从归一化计费用量创建独立副本，仅减文本输入；顺序为普通文本、cache read、cache write（5m 后 1h），下限为零，保持缓存明细一致，不改客户端原始 usage、输出、图像或音频。
- `service/text_quota.go#PostTextConsumeQuota`：普通倍率、tiered 参数、最终结算和消费日志共用扣减后的计费用量；固定按次价格不因 token 减免消失。管理员日志 `admin_info` 保存原文本输入和实际减免，普通日志仅显示扣后输入。
- 默认主题入口：`web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx` 在高级设置中编辑渠道默认和当前分组减免，已有减免配置自动展开；`web/default/src/features/channels/lib/channel-form.ts` 负责完整加载、校验及保存，`web/default/src/features/channels/lib/input-token-deduction.ts` 保留未知 setting 字段并区分留空继承与显式 `0`。
- 经典主题入口：`web/classic/src/components/table/channels/modals/EditChannelModal.jsx` 与同目录 `inputTokenDeduction.js` 提供同等编辑及加载保存语义，不更改已有配置结构。
- 回归入口：`service/input_token_deduction_test.go`（含 SQLite 余额/消费日志、缓存与协议语义、最终分组、tiered/按次边界和 benchmark）、`model/channel_input_token_deduction_test.go`（配置验证）、`web/default/src/features/channels/lib/input-token-deduction.test.ts`（留空/零/未知配置保留和整数校验）。

### 系统徽标本地上传

- 默认主题：`web/default/src/features/system-settings/general/system-info-section.tsx` 使用本地文件选择、即时预览、更换、移除及设置表单保存/重置；读取代际与卸载清理防止 reset/unmount 后晚完成的读取覆盖表单，保存失败不更新已保存基线。`logo-image.ts` 按文件内容识别并用浏览器解码 PNG/JPEG/WebP/GIF，限制 256 KiB、4096×4096 像素，并转换为持久化 data URL。已有 HTTP(S) Logo URL 继续兼容。
- 经典主题：`web/classic/src/components/settings/OtherSetting.jsx` 与同目录 `logoImage.js` 提供相同的内容识别、完整解码、尺寸限制、读取竞态保护、预览、移除和保存语义；成功后刷新 `/api/status`、localStorage 与 Header。
- 后端边界：`controller/logo_option.go` 在通用 Option 更新入口执行严格 base64、`image.DecodeConfig` 格式/尺寸检查和完整 `image.Decode`，拒绝截断图片及 SVG/HTML 等主动内容。值仍存入既有 `Logo` option，不新增文件目录、请求级查询或部署卷；`web/default/src/features/system-settings/hooks/use-update-option.ts` 将 Logo 纳入 status 刷新路径。
- 回归入口：`controller/logo_option_test.go` 覆盖完整解码、截断、256 KiB 与尺寸边界；`model/logo_option_storage_test.go` 覆盖 SQLite data URL 往返和 MySQL longtext/PostgreSQL text 类型；`web/default/src/features/system-settings/general/logo-image.test.ts` 覆盖内容识别、浏览器解码和尺寸/大小拒绝。两个主题均需生产构建验证。

### 🎯 核心概念

#### 路径为王 (Path is King)

文件夹路径是唯一主体，数据库表、API路由、后端类、权限点全部从路径自动推导：
开始
```
文件夹路径（唯一主体）
pages/admin/users/config/levels/
        ↓ 自动映射
数据库表：admin_users_config_levels
API路径：/api/admin/users/config/levels
后端类名：AdminUsersConfigLevels
权限标识：admin.users.config.levels
```

#### AI 友好命名

- **命名哲学**：`04.quality-00.naming-philosophy.md`
  - 完整描述 > 缩写简写
  - AI 友好 > 人类打字方便
  - 行业通用 > 项目特定
  - 精准语义 > 模糊简称

- **AI 友好命名扩展规范**：`04.quality-02.ai-friendly-naming.md`
  - 长度不是成本，40-60 字符的完整命名是默认
  - 结构要素不允许省略（主体 + 子能力 + 角色层 + 扩展名）
  - 平铺优先，结构靠命名而非目录
  - 抽象化模糊词禁用（`util`、`helper`、`manager` 等单独使用）
  - 缩写白名单收敛到行业标准

### 🤖 对 AI 的友好性

- ✅ 简洁的目录结构便于 LLM 理解
- ✅ L2 先定位，L3 按需读取
- ✅ 清晰的规范便于 Agent 执行
- ✅ 支持所有主流 AI IDE

### 📚 了解更多

- [L1 项目指南](../L1#Overview/guide.md)
- [AI 记忆体系架构](../L3#Standards/standards/10.ai-memory-01.architecture.md)
- [六层架构命名规范](../L3#Standards/standards/10.ai-memory-03.six-layer-naming.md)

---

**最后更新**：2026-04-26
