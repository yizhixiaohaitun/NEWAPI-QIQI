#!/usr/bin/env bash
if [ -z "${BASH_VERSION:-}" ]; then
    exec bash "$0" "$@"
fi

set -u
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR" || exit 1

# NEWAPI-QIQI one-button deploy script.
# Flow: commit/push to GitHub -> wait GitHub Actions Docker build -> deploy on na.q.srl.

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

log_info() { echo -e "${GREEN}[INFO]${NC} $*"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $*"; }
log_error() { echo -e "${RED}[ERROR]${NC} $*" >&2; }
log_step() { echo -e "${CYAN}[STEP]${NC} $*"; }

die() {
    log_error "$*"
    exit 1
}

now_epoch() {
    date +%s
}

format_duration() {
    local seconds="${1:-0}"
    local minutes remainder

    minutes=$((seconds / 60))
    remainder=$((seconds % 60))
    if [ "$minutes" -gt 0 ]; then
        printf '%dm%02ds' "$minutes" "$remainder"
    else
        printf '%ds' "$seconds"
    fi
}

strip_env_value() {
    local value="$1"
    value="${value%%#*}"
    value="${value%"${value##*[![:space:]]}"}"
    value="${value#"${value%%[![:space:]]*}"}"
    value="${value%\"}"
    value="${value#\"}"
    value="${value%\'}"
    value="${value#\'}"
    printf '%s' "$value"
}

load_dotenv_defaults() {
    local env_file="${SCRIPT_DIR}/.env"
    local key value

    [ -f "$env_file" ] || return 0
    while IFS='=' read -r key value; do
        case "$key" in
            SERVER_DOMAIN|SERVER_IP|SERVER_SSH_PORT|SERVER_SSH_USER|SERVER_ROOT_PASSWORD|FRONTEND_BASE_URL|QIQI_*)
                [ -n "${!key:-}" ] && continue
                value="$(strip_env_value "${value:-}")"
                [ -n "$value" ] && export "$key=$value"
                ;;
        esac
    done < "$env_file"
}

load_dotenv_defaults

GITHUB_REPO="${QIQI_GITHUB_REPO:-al90slj23/NEWAPI-QIQI}"
GIT_REMOTE="${QIQI_GIT_REMOTE:-qiqi}"
GIT_BRANCH="${QIQI_GIT_BRANCH:-main}"
GITHUB_WORKFLOW_FILE="${QIQI_GITHUB_WORKFLOW_FILE:-ghcr-newapi-qiqi.yml}"
GITHUB_WORKFLOW_NAME="${QIQI_GITHUB_WORKFLOW_NAME:-Build NEWAPI-QIQI image}"
GITHUB_ACTION_TIMEOUT="${QIQI_GITHUB_ACTION_TIMEOUT:-1200}"
GITHUB_ACTION_POLL_INTERVAL="${QIQI_GITHUB_ACTION_POLL_INTERVAL:-15}"

if [ -n "${QIQI_IMAGE:-}" ]; then
    IMAGE="$QIQI_IMAGE"
    IMAGE_TAG_MODE="${QIQI_IMAGE_TAG_MODE:-fixed}"
else
    IMAGE="ghcr.io/al90slj23/newapi-qiqi:main"
    IMAGE_TAG_MODE="${QIQI_IMAGE_TAG_MODE:-sha}"
fi
IMAGE_REPO="${IMAGE%:*}"
DEPLOY_HOST="${QIQI_DEPLOY_HOST:-${SERVER_DOMAIN:-na.q.srl}}"
DEPLOY_IP="${QIQI_DEPLOY_IP:-${SERVER_IP:-}}"
if [ -n "${QIQI_DEPLOY_USE_IP:-}" ]; then
    DEPLOY_USE_IP="$QIQI_DEPLOY_USE_IP"
elif [ -n "$DEPLOY_IP" ]; then
    DEPLOY_USE_IP="true"
else
    DEPLOY_USE_IP="false"
fi
DEPLOY_SSH_HOST="$DEPLOY_HOST"
[ "$DEPLOY_USE_IP" = "true" ] && [ -n "$DEPLOY_IP" ] && DEPLOY_SSH_HOST="$DEPLOY_IP"
DEPLOY_USER="${QIQI_DEPLOY_USER:-${SERVER_SSH_USER:-root}}"
DEPLOY_PORT="${QIQI_DEPLOY_PORT:-${SERVER_SSH_PORT:-22}}"
DEPLOY_DIR="${QIQI_DEPLOY_DIR:-/opt/new-api}"
COMPOSE_FILE="${QIQI_COMPOSE_FILE:-docker-compose.prod.yml}"
COMPOSE_OVERRIDE_FILE="${QIQI_COMPOSE_OVERRIDE_FILE:-docker-compose.qiqi-image.override.yml}"
COMPOSE_SERVICE="${QIQI_COMPOSE_SERVICE:-new-api}"
PUBLIC_URL="${QIQI_PUBLIC_URL:-${FRONTEND_BASE_URL:-https://na.q.srl}}"
HEALTH_PATH="${QIQI_HEALTH_PATH:-/api/status}"
HEALTH_TIMEOUT="${QIQI_HEALTH_TIMEOUT:-120}"
HEALTH_INTERVAL="${QIQI_HEALTH_INTERVAL:-5}"
if [ -n "${QIQI_SSH_KEY:-}" ]; then
    SSH_KEY="$QIQI_SSH_KEY"
elif [ -f "${HOME}/.ssh/qiqi" ]; then
    SSH_KEY="${HOME}/.ssh/qiqi"
else
    SSH_KEY="${HOME}/.ssh/al90slj23"
fi
SSH_CONFIG_FILE="${QIQI_SSH_CONFIG_FILE:-/dev/null}"
SSH_PASSWORD="${QIQI_DEPLOY_PASSWORD:-${SERVER_ROOT_PASSWORD:-}}"
SSH_USE_SSHPASS="false"
AUTO_ADD_ALL="${QIQI_DEPLOY_AUTO_ADD_ALL:-false}"

SSH_OPTS=(-F "$SSH_CONFIG_FILE" -p "$DEPLOY_PORT" -o StrictHostKeyChecking=no -o ConnectTimeout=10 -o ServerAliveInterval=30 -o ServerAliveCountMax=6)
if [ -f "$SSH_KEY" ]; then
    SSH_OPTS=(-i "$SSH_KEY" "${SSH_OPTS[@]}")
elif [ -n "$SSH_PASSWORD" ] && command -v sshpass >/dev/null 2>&1; then
    SSH_USE_SSHPASS="true"
    SSH_OPTS=(-o PreferredAuthentications=password -o PubkeyAuthentication=no "${SSH_OPTS[@]}")
elif [ -n "$SSH_PASSWORD" ]; then
    log_warn "检测到 SSH 密码但未安装 sshpass，将回退到系统 ssh 交互/默认认证。"
fi

shell_single_quote() {
    printf "'"
    printf "%s" "$1" | sed "s/'/'\\\\''/g"
    printf "'"
}

remote_exec() {
    # shellcheck disable=SC2029
    if [ "$SSH_USE_SSHPASS" = "true" ]; then
        SSHPASS="$SSH_PASSWORD" sshpass -e ssh "${SSH_OPTS[@]}" "${DEPLOY_USER}@${DEPLOY_SSH_HOST}" "$@"
    else
        ssh "${SSH_OPTS[@]}" "${DEPLOY_USER}@${DEPLOY_SSH_HOST}" "$@"
    fi
}

require_command() {
    command -v "$1" >/dev/null 2>&1 || die "缺少命令: $1"
}

show_header() {
    echo ""
    echo -e "${BLUE}═══════════════════════════════════════════════════${NC}"
    echo -e "${BLUE}  NEWAPI-QIQI deploy${NC}"
    echo -e "${BLUE}  repo: ${GITHUB_REPO}${NC}"
    echo -e "${BLUE}  image: ${IMAGE}${NC}"
    echo -e "${BLUE}  target: ${DEPLOY_USER}@${DEPLOY_SSH_HOST}:${DEPLOY_DIR}${NC}"
    echo -e "${BLUE}═══════════════════════════════════════════════════${NC}"
    echo ""
}

show_menu() {
    show_header
    echo "  1) 完整发布：git push -> GitHub Actions 构建 Docker -> 部署到 na.q.srl"
    echo "  2) 仅部署：跳过 git/GitHub，远端直接 pull 最新镜像并重建服务"
    echo "  3) 查看远端状态"
    echo "  4) 查看应用日志"
    echo "  5) 重启应用服务"
    echo "  0) 退出"
    echo ""
    echo -e "  ${YELLOW}输入操作编号（直接回车默认 1 完整发布）:${NC}"
}

git_has_changes() {
    ! git diff --quiet 2>/dev/null || \
        ! git diff --cached --quiet 2>/dev/null || \
        [ -n "$(git ls-files --others --exclude-standard 2>/dev/null)" ]
}

git_add_all_excluding_embedded_repos() {
    local embedded_roots=()
    local paths=()
    local git_dir parent rel path embedded_root skip

    while IFS= read -r git_dir; do
        parent="${git_dir%/.git}"
        rel="${parent#./}"
        [ -n "$rel" ] || continue
        [ "$rel" = "." ] && continue
        log_warn "跳过嵌套 git 仓库: ${rel}"
        embedded_roots+=("$rel")
    done < <(find . -path ./.git -prune -o -type d -name .git -print)

    while IFS= read -r -d '' path; do
        skip="false"
        for embedded_root in "${embedded_roots[@]}"; do
            if [ "$path" = "$embedded_root" ] || [[ "$path" == "$embedded_root/"* ]]; then
                skip="true"
                break
            fi
        done
        [ "$skip" = "true" ] && continue
        paths+=("$path")
    done < <(git ls-files -m -d -o --exclude-standard -z)

    [ "${#paths[@]}" -eq 0 ] && return 0
    git add -A -- "${paths[@]}"
}

confirm_git_scope_for_deploy() {
    local commit_msg="${1:-}"
    local reply

    git_has_changes || return 0

    log_warn "当前工作区有未提交改动。"
    git --no-pager status --short
    echo ""
    git --no-pager diff --stat || true
    echo ""

    if [ "$AUTO_ADD_ALL" != "true" ]; then
        echo "  1) 执行 git add -A 并提交这些改动 [默认]"
        echo "  0) 取消发布"
        read -r -p "请选择 [1]: " reply
        reply="${reply:-1}"
        case "$reply" in
            1|y|Y|yes|YES) ;;
            *) die "已取消发布。你也可以先手动 git add/commit，再重新运行脚本。" ;;
        esac
    else
        log_warn "QIQI_DEPLOY_AUTO_ADD_ALL=true：将执行 git add -A。"
    fi

    git_add_all_excluding_embedded_repos || die "git add 失败"

    if git diff --cached --quiet; then
        log_info "没有已暂存改动需要提交。"
        return 0
    fi

    if [ -z "$commit_msg" ]; then
        read -r -p "提交信息 [update]: " commit_msg
        commit_msg="${commit_msg:-update}"
    fi
    git commit -m "$commit_msg" || die "git commit 失败"
}

push_to_github() {
    require_command git
    git remote get-url "$GIT_REMOTE" >/dev/null 2>&1 || die "找不到 git remote: ${GIT_REMOTE}"

    log_info "推送到 GitHub: ${GIT_REMOTE} HEAD:${GIT_BRANCH}"
    git push "$GIT_REMOTE" "HEAD:${GIT_BRANCH}" || die "git push 失败，停止部署"
}

wait_ghcr_image_tag() {
    local sha="$1"
    local short_sha="${sha:0:7}"
    local tag_sha="${IMAGE_REPO}:sha-${short_sha}"
    local waited=0
    local output

    log_warn "改用镜像 tag 轮询等待构建完成: ${tag_sha}"
    while [ "$waited" -lt "$GITHUB_ACTION_TIMEOUT" ]; do
        if output="$(remote_exec "docker manifest inspect $(shell_single_quote "$tag_sha") >/dev/null 2>&1" 2>&1)"; then
            log_info "GHCR 镜像已可拉取: ${tag_sha}"
            return 0
        fi
        sleep "$GITHUB_ACTION_POLL_INTERVAL"
        waited=$((waited + GITHUB_ACTION_POLL_INTERVAL))
        log_info "镜像还未就绪（${waited}s/${GITHUB_ACTION_TIMEOUT}s）..."
    done

    [ -n "$output" ] && log_warn "$output"
    return 1
}

wait_github_actions_build() {
    local sha="$1"
    local run_id run_url waited gh_output gh_status final_state

    if ! command -v gh >/dev/null 2>&1; then
        log_warn "未找到 gh CLI，无法直接监控 GitHub Actions。"
        wait_ghcr_image_tag "$sha" || die "等待 GHCR 镜像超时"
        return 0
    fi

    waited=0
    log_info "等待 GitHub Actions 接收提交: ${sha}"
    log_info "Actions 页面: https://github.com/${GITHUB_REPO}/actions/workflows/${GITHUB_WORKFLOW_FILE}"

    while [ "$waited" -lt "$GITHUB_ACTION_TIMEOUT" ]; do
        gh_output="$(gh run list \
            --repo "$GITHUB_REPO" \
            --workflow "$GITHUB_WORKFLOW_FILE" \
            --commit "$sha" \
            --limit 10 \
            --json databaseId,url \
            --jq 'sort_by(.databaseId) | reverse | .[0].databaseId // empty' 2>&1)"
        gh_status=$?
        if [ "$gh_status" -ne 0 ]; then
            log_warn "查询 GitHub Actions 失败：${gh_output}"
            wait_ghcr_image_tag "$sha" || die "等待 GHCR 镜像超时"
            return 0
        fi

        run_id="$gh_output"
        [ -n "$run_id" ] && break

        sleep "$GITHUB_ACTION_POLL_INTERVAL"
        waited=$((waited + GITHUB_ACTION_POLL_INTERVAL))
        log_info "GitHub Actions 尚未出现本次构建任务（${waited}s）..."
    done

    if [ -z "${run_id:-}" ]; then
        log_warn "没有找到 workflow ${GITHUB_WORKFLOW_FILE} 对应的 run，尝试按镜像 tag 等待。"
        wait_ghcr_image_tag "$sha" || die "等待 GitHub Actions/GHCR 镜像超时"
        return 0
    fi

    run_url="$(gh run view "$run_id" --repo "$GITHUB_REPO" --json url --jq '.url' 2>/dev/null || true)"
    log_info "已找到构建任务: ${GITHUB_WORKFLOW_NAME} #${run_id}"
    [ -n "$run_url" ] && log_info "任务地址: ${run_url}"

    if ! gh run watch "$run_id" --repo "$GITHUB_REPO" --exit-status --interval "$GITHUB_ACTION_POLL_INTERVAL"; then
        final_state="$(gh run view "$run_id" --repo "$GITHUB_REPO" --json status,conclusion --jq '"status=\(.status), conclusion=\(.conclusion)"' 2>/dev/null || true)"
        [ -n "$final_state" ] && log_error "GitHub Actions 最终状态: ${final_state}"
        gh run view "$run_id" --repo "$GITHUB_REPO" --log-failed 2>/dev/null || true
        die "GitHub Actions 构建失败或被取消"
    fi

    log_info "GitHub Actions 构建完成并成功。"
}

select_deploy_image_for_sha() {
    local sha="$1"
    local short_sha="${sha:0:7}"

    case "$IMAGE_TAG_MODE" in
        sha)
            IMAGE="${IMAGE_REPO}:sha-${short_sha}"
            ;;
        main|fixed)
            ;;
        *)
            die "未知 QIQI_IMAGE_TAG_MODE=${IMAGE_TAG_MODE}（可选: sha / main / fixed）"
            ;;
    esac
    log_info "本次部署镜像: ${IMAGE}"
}

resolve_remote_deploy_dir() {
    local q_dir remote_script resolved

    q_dir="$(shell_single_quote "$DEPLOY_DIR")"
    remote_script="
set -eu
for d in ${q_dir} /root/NEWAPI-QIQI /root/newapi-qiqi /root/new-api /opt/NEWAPI-QIQI /opt/newapi-qiqi /opt/new-api; do
  if [ -f \"\$d/${COMPOSE_FILE}\" ]; then
    printf '%s' \"\$d\"
    exit 0
  fi
done
echo \"missing compose file ${COMPOSE_FILE}; checked ${DEPLOY_DIR}, /root/NEWAPI-QIQI, /root/newapi-qiqi, /root/new-api, /opt/NEWAPI-QIQI, /opt/newapi-qiqi, /opt/new-api\" >&2
exit 1
"
    resolved="$(remote_exec "$remote_script")" || die "远端部署目录不可用：${DEPLOY_USER}@${DEPLOY_SSH_HOST}:${DEPLOY_DIR}"
    [ -n "$resolved" ] || die "远端部署目录解析为空"
    DEPLOY_DIR="$resolved"
    log_info "远端部署目录: ${DEPLOY_DIR}"
}

remote_compose_cmd() {
    local command="$1"
    local q_dir q_file q_override_file q_service q_image

    q_dir="$(shell_single_quote "$DEPLOY_DIR")"
    q_file="$(shell_single_quote "$COMPOSE_FILE")"
    q_override_file="$(shell_single_quote "$COMPOSE_OVERRIDE_FILE")"
    q_service="$(shell_single_quote "$COMPOSE_SERVICE")"
    q_image="$(shell_single_quote "$IMAGE")"

    remote_exec "
set -eu
cd ${q_dir}
compose_file=${q_file}
override_file=${q_override_file}
service=${q_service}
image=${q_image}
docker compose -f \"\$compose_file\" config --services | grep -qx \"\$service\"
echo 'compose services:'
docker compose -f \"\$compose_file\" config --services
case $(shell_single_quote "$command") in
  deploy)
    cat > \"\$override_file\" <<EOF
services:
  \"\$service\":
    image: \$image
EOF
    docker pull \"\$image\"
    docker compose -f \"\$compose_file\" -f \"\$override_file\" pull \"\$service\" || true
    docker compose -f \"\$compose_file\" -f \"\$override_file\" up -d --force-recreate \"\$service\"
    docker image prune -f >/dev/null 2>&1 || true
    ;;
  restart)
    docker compose -f \"\$compose_file\" restart \"\$service\"
    ;;
  status)
    docker compose -f \"\$compose_file\" ps
    ;;
  logs)
    docker compose -f \"\$compose_file\" logs --tail=200 \"\$service\"
    ;;
esac
"
}

wait_health() {
    local waited=0
    local status_code
    local url="${PUBLIC_URL%/}${HEALTH_PATH}"
    local q_dir q_file q_service

    q_dir="$(shell_single_quote "$DEPLOY_DIR")"
    q_file="$(shell_single_quote "$COMPOSE_FILE")"
    q_service="$(shell_single_quote "$COMPOSE_SERVICE")"
    log_info "等待远端服务健康: ${url}"

    while [ "$waited" -le "$HEALTH_TIMEOUT" ]; do
        status_code="$(curl -k -L -sS -o /dev/null -w '%{http_code}' --connect-timeout 8 --max-time 20 "$url" 2>/dev/null || true)"
        if printf '%s' "$status_code" | grep -Eq '^(2|3)[0-9][0-9]$'; then
            log_info "HTTP 健康检查通过: ${status_code}"
            return 0
        fi

        if [ "$waited" -eq 0 ] || [ $((waited % 15)) -eq 0 ]; then
            log_warn "HTTP 尚未就绪（${waited}s/${HEALTH_TIMEOUT}s）: ${status_code:-no_response}"
            remote_exec "cd ${q_dir} && docker compose -f ${q_file} ps ${q_service}" || true
        fi
        sleep "$HEALTH_INTERVAL"
        waited=$((waited + HEALTH_INTERVAL))
    done

    log_error "HTTP 健康检查失败: ${status_code:-no_response}"
    remote_exec "cd ${q_dir} && docker compose -f ${q_file} logs --tail=120 ${q_service}" || true
    return 1
}

deploy_remote() {
    resolve_remote_deploy_dir
    log_step "远端拉取镜像并重建服务"
    remote_compose_cmd deploy || die "远端 docker compose 部署失败"
    wait_health || die "部署后健康检查失败"
    log_info "部署完成: ${PUBLIC_URL}"
}

full_deploy() {
    local commit_msg="${1:-}"
    local deploy_sha
    local started_at

    started_at="$(now_epoch)"
    show_header
    require_command git
    require_command ssh
    require_command curl

    log_step "1/3 提交并推送代码"
    confirm_git_scope_for_deploy "$commit_msg"
    deploy_sha="$(git rev-parse HEAD)"
    push_to_github
    log_info "部署提交: ${deploy_sha}"

    log_step "2/3 等待 GitHub Actions 构建 Docker 镜像"
    wait_github_actions_build "$deploy_sha"
    select_deploy_image_for_sha "$deploy_sha"

    log_step "3/3 部署到 ${DEPLOY_HOST}"
    deploy_remote

    log_info "总耗时: $(format_duration $(( $(now_epoch) - started_at )))"
}

update_only() {
    show_header
    require_command ssh
    require_command curl
    deploy_remote
}

remote_status() {
    show_header
    resolve_remote_deploy_dir
    remote_compose_cmd status || die "读取远端状态失败"
    echo ""
    remote_exec "df -h / | tail -1; free -h | head -2" || true
}

remote_logs() {
    show_header
    resolve_remote_deploy_dir
    remote_compose_cmd logs || die "读取远端日志失败"
}

remote_restart() {
    show_header
    resolve_remote_deploy_dir
    remote_compose_cmd restart || die "远端重启失败"
    wait_health || die "重启后健康检查失败"
}

main() {
    local choice="${1:-}"
    local commit_msg="${2:-}"

    if [ -z "$choice" ]; then
        show_menu
        printf "  > "
        read -r choice
        [ -n "$choice" ] || choice=1
    fi

    case "$choice" in
        0|exit|quit) return 0 ;;
        1|deploy|release) full_deploy "$commit_msg" ;;
        2|update) update_only ;;
        3|status) remote_status ;;
        4|logs) remote_logs ;;
        5|restart) remote_restart ;;
        *)
            show_menu
            die "未知选项: ${choice}"
            ;;
    esac
}

main "$@"
