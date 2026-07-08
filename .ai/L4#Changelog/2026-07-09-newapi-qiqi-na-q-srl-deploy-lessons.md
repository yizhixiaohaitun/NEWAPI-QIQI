# 2026-07-09 NEWAPI-QIQI na.q.srl 部署排障经验

## 背景

本次发布流程为：本地 `./qiqi.sh 1` 推送到 GitHub，GitHub Actions 构建 `ghcr.io/al90slj23/newapi-qiqi` 镜像，然后服务器 `na.q.srl` 拉取镜像并重建 Docker Compose 服务。

GitHub Actions 构建本身成功，实际失败点集中在 SSH 路由、SSH key、远端 compose 文件选择和端口占用。

## 固化配置

项目 `.env` 必须显式包含 qiqi 部署配置：

```env
QIQI_SSH_KEY=/Users/al90slj23/.ssh/qiqi
QIQI_DEPLOY_USE_IP=true
QIQI_DEPLOY_DIR=/opt/new-api
QIQI_COMPOSE_FILE=docker-compose.prod.yml
QIQI_COMPOSE_SERVICE=new-api
```

原因：

- 本机访问 `na.q.srl` 时可能解析到 `198.18.0.20`，不是生产服务器真实 SSH 入口。
- 生产服务器真实 IP 来自 `.env` 的 `SERVER_IP=38.148.249.92`。
- 可登录生产服务器的 key 是 `~/.ssh/qiqi`，不是 `~/.ssh/al90slj23`。

## 远端生产栈事实

生产栈不要猜，当前事实如下：

- 服务器：`root@38.148.249.92`
- 公开域名：`https://na.q.srl`
- 远端目录：`/opt/new-api`
- 生产 compose：`/opt/new-api/docker-compose.prod.yml`
- Compose project name：`newapi-qiqi`
- 应用服务名：`new-api`
- 应用容器名：`newapi-qiqi-app`
- 应用监听：`127.0.0.1:3000->3000/tcp`
- 数据挂载：
  - `/opt/new-api/data -> /data`
  - `/opt/new-api/logs -> /app/logs`

不要用 `/opt/new-api/docker-compose.yml` 部署生产。这个文件是模板/默认栈，会创建 `new-api`、`postgres`、`redis` 另一套容器，并与生产容器抢占 `127.0.0.1:3000`。

## 失败现象与判断

### 1. `Connection closed by 198.18.0.20 port 22`

说明脚本在用域名做 SSH，并且本机解析到了错误的保留地址。修复是默认 `QIQI_DEPLOY_USE_IP=true`，使用 `.env` 中的 `SERVER_IP`。

### 2. `Permission denied (publickey,password)`

说明网络已到服务器，但 key 或认证方式不对。本项目应使用：

```bash
ssh -i /Users/al90slj23/.ssh/qiqi root@38.148.249.92
```

### 3. `Bind for 127.0.0.1:3000 failed: port is already allocated`

通常是脚本误用了 `docker-compose.yml`，创建了另一套 `new-api` 容器，而生产 `newapi-qiqi-app` 已经占用 3000。

排查：

```bash
docker ps -a --format 'table {{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}'
ss -ltnp '( sport = :3000 )'
docker inspect newapi-qiqi-app --format '{{ index .Config.Labels "com.docker.compose.project.working_dir" }}|{{ index .Config.Labels "com.docker.compose.project.config_files" }}|{{ index .Config.Labels "com.docker.compose.service" }}|{{ .Config.Image }}'
```

如果误创建了模板栈，可清理：

```bash
cd /opt/new-api
docker compose -f docker-compose.yml down
```

不要对 `docker-compose.prod.yml` 执行 `down`，除非明确要停止生产服务。

## 部署命令

常用命令：

```bash
./qiqi.sh 1  # 完整发布：push -> GitHub Actions build -> deploy
./qiqi.sh 2  # 仅部署：拉取已构建镜像并重建生产服务
./qiqi.sh 3  # 只读状态检查
```

首次启动或服务依赖可能不存在时，`docker compose up` 不能带 `--no-deps`，否则 `postgres`、`postgres-log`、`redis` 不会跟随启动。

## 健康检查

健康检查使用：

```bash
curl -sS https://na.q.srl/api/status
```

不要用 `curl -I /api/status` 判断接口健康；线上 `HEAD /api/status` 可能返回 404，但 `GET /api/status` 正常返回 `success: true`。

成功部署后响应头应出现类似：

```text
x-new-api-version: newapi-qiqi-YYYYMMDD-<short_sha>
```

## Git 注意事项

`git add -A` 曾经把 `ZERO` 这个嵌套 git 仓库作为 gitlink 提交进去，Git 会提示：

```text
warning: adding embedded git repository: ZERO
```

这会导致外层仓库 clone 后不包含 `ZERO` 内容。`qiqi.sh` 已改为自动跳过嵌套 `.git` 目录。以后如果再次看到 embedded repo warning，要停止提交并检查 `git status --short`。

## 本次最终结果

本次成功部署后：

- 线上版本：`newapi-qiqi-20260708-629ee71`
- `GET https://na.q.srl/api/status` 返回 `success: true`
- `./qiqi.sh 3` 显示 `newapi-qiqi-app` 为 `healthy`
