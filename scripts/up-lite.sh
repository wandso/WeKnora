#!/usr/bin/env bash
# WeKnora —— SQLite(lite) + 容器化启动脚本
#
# 两件事这个脚本负责：
#
# 1. 前端 nginx 的 proxy_pass 写的是 ${APP_HOST}:${APP_PORT}，envsubst 在容器
#    启动时把它替换成字面量 "app:8080"，nginx 只在启动时解析一次 DNS 并缓存
#    IP。app 容器一旦被重建（换镜像、改配置都会换 IP），nginx 仍指向旧 IP，
#    所有 /api 请求返回 502，页面看起来像"数据没了"。所以拉起服务后固定重启
#    一次 frontend，让它重新解析。
#
# 2. `docker compose up -d` 永远不会重建镜像——镜像几号构建的就永远跑哪天的
#    代码。所以这里提供 --update：比对源码指纹，只重建真正变了的镜像。
#    commit 了代码不等于镜像更新了，这一点只能靠显式 build 解决。
#
# 用法：
#   ./scripts/up-lite.sh                  # 启动，用现有镜像（秒级）
#   ./scripts/up-lite.sh --update         # 起前按需重建（源码变了才 build）
#   ./scripts/up-lite.sh --frontend       # 只重建前端再起（改了 UI，几分钟）
#   ./scripts/up-lite.sh --app            # 只重建后端再起（改了 Go，增量较快）
#   ./scripts/up-lite.sh --rebuild        # 强制重建全部
#   ./scripts/up-lite.sh stop             # 停止（数据保留在 ./data）
#   ./scripts/up-lite.sh logs app         # 跟踪日志
#
# --update 的指纹 = git HEAD + 工作区改动状态。只比 HEAD 会漏掉未提交的改动，
# 只比文件 mtime 会被 checkout / 时钟漂移误判，两者组合才是"当前源码"。
# 指纹存在 .docker-stamps/ 下（已 gitignore），删掉即强制下次重建。

set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

PROJECT=weknora-main
COMPOSE=(docker compose -p "$PROJECT" -f docker-compose.yml -f docker-compose.lite.yml)
SERVICES=(redis docreader app frontend)
STAMP_DIR=.docker-stamps

APP_IMAGE=wechatopenai/weknora-app:latest
UI_IMAGE=wechatopenai/weknora-ui:latest

# 进入 app 镜像的东西：Go 源码、迁移、配置、Dockerfile 与容器脚本。
APP_PATHS=(cmd internal go.mod go.sum migrations config docker)
# 进入 frontend 镜像的东西：build context 是 ./frontend。
UI_PATHS=(frontend)

fingerprint() {
  {
    git rev-parse HEAD 2>/dev/null || echo nogit
    git status --porcelain -- "$@" 2>/dev/null || true
  } | sha1sum | cut -d' ' -f1
}

# 源码比上次构建新，或镜像根本不存在 -> 需要重建
needs_build() {
  local name=$1 image=$2
  shift 2
  local current stored
  current=$(fingerprint "$@")
  stored=$(cat "${STAMP_DIR}/${name}" 2>/dev/null || echo "")
  if [ "$current" != "$stored" ]; then
    return 0
  fi
  # 指纹一致但镜像被删了，也要重建
  docker image inspect "$image" >/dev/null 2>&1 && return 1
  return 0
}

record_stamp() {
  local name=$1
  shift
  mkdir -p "$STAMP_DIR"
  fingerprint "$@" >"${STAMP_DIR}/${name}"
}

# 构建时间取"上次完成构建的时刻"而不是 now：BUILD_TIME_ARG 会写进 Dockerfile
# 的 ENV 层，值一变后面所有层（包括运行镜像里那 400 多个 apt 包）全部重装。
# 用固定的值，同一份源码重复构建才能真正命中缓存。
build_time_stamp() {
  local name=$1
  if [ -f "${STAMP_DIR}/${name}" ]; then
    date -u -r "${STAMP_DIR}/${name}" '+%Y-%m-%d %H:%M:%S UTC'
  else
    date -u '+%Y-%m-%d %H:%M:%S UTC'
  fi
}

build_image() {
  local name=$1
  # 构建期元数据：系统信息页会读取。不传就显示 unknown。
  export VERSION_ARG="${VERSION_ARG:-$(cat VERSION 2>/dev/null || echo dev)}"
  export COMMIT_ID_ARG="${COMMIT_ID_ARG:-$(git rev-parse --short HEAD 2>/dev/null || echo unknown)}"
  export BUILD_TIME_ARG="${BUILD_TIME_ARG:-$(build_time_stamp "$name")}"
  echo "==> 重建 ${name} 镜像（源码已变化）"
  "${COMPOSE[@]}" build "$name"
  echo "==> ${name} 镜像已更新"
}

# native 模式（scripts/start-native.sh）与容器共用 data/weknora.db，
# 同一个 SQLite 文件不能被两个进程同时写，所以起容器前先把宿主机上的后端停掉。
stop_native() {
  local pids
  pids=$(netstat -ano 2>/dev/null \
    | awk '$1=="TCP" && $2 ~ /:8080$/ && $4=="LISTENING" {print $5}' \
    | sort -u)
  [ -z "$pids" ] && return 0
  echo "==> 宿主机 native 后端占用 8080（与容器共用 data/weknora.db），先停掉"
  local pid
  for pid in $pids; do
    [ "$pid" = "0" ] && continue
    taskkill //PID "$pid" //T //F >/dev/null 2>&1 || true
  done
}

start_services() {
  local update_mode=$1

  case "$update_mode" in
  all)
    build_image app && record_stamp app "${APP_PATHS[@]}"
    build_image frontend && record_stamp frontend "${UI_PATHS[@]}"
    ;;
  app)
    build_image app && record_stamp app "${APP_PATHS[@]}"
    ;;
  frontend)
    build_image frontend && record_stamp frontend "${UI_PATHS[@]}"
    ;;
  auto)
    if needs_build app "$APP_IMAGE" "${APP_PATHS[@]}"; then
      build_image app && record_stamp app "${APP_PATHS[@]}"
    else
      echo "==> app 源码无变化，跳过构建"
    fi
    if needs_build frontend "$UI_IMAGE" "${UI_PATHS[@]}"; then
      build_image frontend && record_stamp frontend "${UI_PATHS[@]}"
    else
      echo "==> frontend 源码无变化，跳过构建"
    fi
    ;;
  none)
    ;;
  esac

  stop_native
  "${COMPOSE[@]}" up -d "${SERVICES[@]}"
  # 无条件重启前端：app 若被重建则 IP 已变，必须重新解析（几秒，幂等）
  docker restart WeKnora-frontend >/dev/null
  echo "等待服务就绪..."
  for _ in $(seq 1 30); do
    if curl -fsS -m 2 http://localhost:"${APP_PORT:-8081}"/health >/dev/null 2>&1; then
      echo "app 已就绪：http://localhost:${FRONTEND_PORT:-8888}"
      exit 0
    fi
    sleep 2
  done
  echo "app 尚未就绪，请查看日志：./scripts/up-lite.sh logs app" >&2
  exit 1
}

case "${1:-up}" in
stop)
  "${COMPOSE[@]}" stop
  ;;
logs)
  "${COMPOSE[@]}" logs -f "${2:-app}"
  ;;
--update | update)
  start_services auto
  ;;
--rebuild | rebuild)
  start_services all
  ;;
--app | app)
  start_services app
  ;;
--frontend | frontend)
  start_services frontend
  ;;
up | start)
  start_services none
  ;;
-h | --help | help)
  sed -n '2,32p' "$0" | sed 's/^# \{0,1\}//'
  ;;
*)
  echo "用法: $0 [up|--update|--rebuild|--app|--frontend|stop|logs <service>]" >&2
  exit 2
  ;;
esac
