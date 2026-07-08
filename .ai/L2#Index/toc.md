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
