#!/usr/bin/env bash
# 宿主机 native 开发模式（CGO + SQLite + FTS5/sqlite-vec）
#
# 与 up-lite.sh（Docker 模式）互斥：两者共用 data/weknora.db，
# 同一个 SQLite 文件不能被两个进程同时写，所以本脚本启动前会停掉
# Docker 里的 app / frontend，只留 docreader（文档解析）继续跑。
#
# 用法：
#   ./scripts/start-native.sh            启动后端（前台，Ctrl+C 结束）
#   ./scripts/start-native.sh watch      监听 .go 变化，保存即自动重编重启（推荐日常开发）
#   ./scripts/start-native.sh bg         后端后台运行
#   ./scripts/start-native.sh web        前端 vite dev（5173，另开一个终端）
#   ./scripts/start-native.sh stop       停掉后端（含残留子进程）
#   ./scripts/start-native.sh status     查看后端 / docreader / 端口状态
#   ./scripts/start-native.sh logs       看后端日志（bg 模式下有意义）
#
# 环境变量可覆盖：MINGW_BIN / SQLITE_INC / APP_PORT / DOCREADER_PORT

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

# 工具链位置：可用 MINGW_BIN / SQLITE_INC 覆盖。
# 优先用 PATH 里的 gcc，找不到再回落到本机已知安装位置（见 docs/dev-env-windows-cgo.md）
if [ -z "${MINGW_BIN:-}" ]; then
  if command -v gcc >/dev/null 2>&1; then
    MINGW_BIN="$(dirname "$(command -v gcc)")"
  else
    MINGW_BIN="E:/xyh/mingw64/bin"
  fi
fi
SQLITE_INC="${SQLITE_INC:-E:/xyh/sqlite-include}"
APP_PORT="${APP_PORT:-8080}"
DOCREADER_PORT="${DOCREADER_PORT:-50051}"
LOG_DIR="$ROOT/logs"
LOG_FILE="$LOG_DIR/native-server.log"
GO_BIN="${GO_BIN:-$(command -v go || echo /d/xyh/go/bin/go)}"
GOPATH_BIN="$(dirname "$GO_BIN")"
COMPOSE=(docker compose -p weknora-main -f docker-compose.yml -f docker-compose.lite.yml)

say()  { printf '\033[36m[native]\033[0m %s\n' "$*"; }
warn() { printf '\033[33m[native] %s\033[0m\n' "$*" >&2; }
die()  { printf '\033[31m[native] 错误: %s\033[0m\n' "$*" >&2; exit 1; }

# 列出监听指定端口的 PID（Windows netstat 第 5 列）
pids_on_port() {
  netstat -ano 2>/dev/null \
    | awk -v p=":${1}$" '$1=="TCP" && $2 ~ p && $4=="LISTENING" {print $5}' \
    | sort -u
}

port_listening() { [ -n "$(pids_on_port "$1")" ]; }

kill_port() {
  local port=$1 pid
  for pid in $(pids_on_port "$port"); do
    [ "$pid" = "0" ] && continue
    say "结束占用 ${port} 的进程 PID=${pid}"
    taskkill //PID "$pid" //T //F >/dev/null 2>&1 || warn "PID=${pid} 未能结束（可能需要管理员权限）"
  done
}

check_toolchain() {
  [ -x "$MINGW_BIN/gcc.exe" ] || die "找不到 gcc: $MINGW_BIN/gcc.exe（见 docs/dev-env-windows-cgo.md，或用 MINGW_BIN= 指定）"
  [ -f "$SQLITE_INC/sqlite3.h" ] || die "找不到 sqlite3.h: $SQLITE_INC（见 docs/dev-env-windows-cgo.md）"
  [ -f "$ROOT/.env.lite" ] || die "缺少 .env.lite"
  command -v "$GO_BIN" >/dev/null 2>&1 || die "找不到 go 可执行文件"
  say "工具链 OK: $( "$MINGW_BIN/gcc.exe" --version 2>/dev/null | head -1 )"
}

# native 与 Docker 共用同一个 SQLite 文件，必须二选一
stop_docker_app() {
  local running
  running="$(docker ps --filter name=WeKnora-app --filter status=running -q 2>/dev/null || true)"
  if [ -n "$running" ]; then
    warn "Docker 里的 app 正在跑，且与 native 共用 data/weknora.db，先停掉它"
    "${COMPOSE[@]}" stop app frontend >/dev/null 2>&1 || warn "停容器失败，请手动执行: ${COMPOSE[*]} stop app frontend"
    say "已停止 WeKnora-app / WeKnora-frontend（数据在 ./data，不受影响）"
  fi
}

ensure_docreader() {
  if port_listening "$DOCREADER_PORT"; then
    say "docreader 已在 127.0.0.1:${DOCREADER_PORT}"
    return 0
  fi
  say "docreader 未监听 ${DOCREADER_PORT}，从 compose 拉起（只起 docreader，不动 app）"
  "${COMPOSE[@]}" up -d docreader >/dev/null 2>&1 || die "docreader 启动失败"
  local i
  for i in $(seq 1 30); do
    port_listening "$DOCREADER_PORT" && { say "docreader 就绪"; return 0; }
    sleep 1
  done
  die "docreader 30 秒内未监听 ${DOCREADER_PORT}，请查看: ${COMPOSE[*]} logs docreader"
}

export_native_env() {
  export CGO_ENABLED=1
  export CC="$MINGW_BIN/gcc.exe"
  export CXX="$MINGW_BIN/g++.exe"
  export CGO_CFLAGS="-I$SQLITE_INC"
  export PATH="$MINGW_BIN:$GOPATH_BIN:$PATH"
  export ENV_FILE=.env.lite
  export GO_BUILD_TAGS=sqlite_fts5
}

prepare() {
  check_toolchain
  stop_docker_app
  ensure_docreader
  if port_listening "$APP_PORT"; then
    warn "端口 ${APP_PORT} 已被占用，清理残留进程"
    kill_port "$APP_PORT"
    sleep 1
  fi
  mkdir -p "$LOG_DIR"
}

cmd_start() {
  prepare
  export_native_env
  say "后端启动于 http://localhost:${APP_PORT}（前台运行，Ctrl+C 结束）"
  say "日志同时写入 $LOG_FILE"
  exec go run -tags sqlite_fts5 ./cmd/server 2>&1 | tee -a "$LOG_FILE"
}

cmd_bg() {
  prepare
  export_native_env
  say "后端后台启动，日志: $LOG_FILE"
  nohup go run -tags sqlite_fts5 ./cmd/server >"$LOG_FILE" 2>&1 &
  local i
  for i in $(seq 1 60); do
    if curl -fsS --max-time 2 --noproxy "*" "http://localhost:${APP_PORT}/health" >/dev/null 2>&1; then
      say "就绪: http://localhost:${APP_PORT}/health -> $(curl -fsS --noproxy '*' "http://localhost:${APP_PORT}/health")"
      say "停止: ./scripts/start-native.sh stop"
      return 0
    fi
    sleep 1
  done
  warn "60 秒内未就绪，最后 20 行日志："
  tail -20 "$LOG_FILE" >&2 || true
  return 1
}

cmd_watch() {
  prepare
  export_native_env
  local bin="$ROOT/tmp/native-server.exe"
  local stamp="$ROOT/tmp/.watch-stamp"
  local server_pid=""  # 仅用于提示，实际按端口管理进程
  mkdir -p "$ROOT/tmp"

  build_and_run() {
    # 先盖戳再编译：编译失败时不会每 2 秒重试一次
    touch "$stamp"
    say "编译中..."
    local t0
    t0=$(date +%s)
    if ! go build -tags sqlite_fts5 -o "$bin" ./cmd/server; then
      warn "编译失败，保持上一个进程继续跑；修好后保存文件会自动重试"
      return 1
    fi
    # 必须按端口杀：Windows 上 $! 是 bash 的包装进程，杀不到真正监听的 exe
    kill_port "$APP_PORT"
    local i
    for i in $(seq 1 10); do
      port_listening "$APP_PORT" || break
      sleep 1
    done
    "$bin" >>"$LOG_FILE" 2>&1 &
    echo "[native $(date '+%H:%M:%S')] 重新拉起服务" >>"$LOG_FILE"
    for i in $(seq 1 30); do
      if curl -fsS --max-time 1 --noproxy '*' "http://localhost:${APP_PORT}/health" >/dev/null 2>&1; then
        say "就绪，编译+重启共 $(( $(date +%s) - t0 ))s -> http://localhost:${APP_PORT}"
        return 0
      fi
      sleep 1
    done
    warn "启动后 30 秒健康检查未通过，看日志: $LOG_FILE"
    return 1
  }

  cleanup() {
    kill_port "$APP_PORT"
    say "已退出"
    exit 0
  }
  trap cleanup INT TERM

  build_and_run || true
  say "监听 .go 变化中（Ctrl+C 结束）；日志: $LOG_FILE"
  while true; do
    sleep 2
    if [ -n "$(find cmd internal third_party -type f -name '*.go' -not -name '*_test.go' -newer "$stamp" -print -quit 2>/dev/null)" ]; then
      build_and_run || true
    fi
  done
}

cmd_stop() {
  if port_listening "$APP_PORT"; then
    kill_port "$APP_PORT"
    say "后端已停止"
  else
    say "端口 ${APP_PORT} 没有进程在监听，无需停止"
  fi
}

cmd_status() {
  printf '后端 %s: ' "$APP_PORT"
  if port_listening "$APP_PORT"; then
    printf 'LISTENING (PID %s)  ' "$(pids_on_port "$APP_PORT" | tr '\n' ' ')"
    curl -fsS --max-time 2 --noproxy "*" "http://localhost:${APP_PORT}/health" 2>/dev/null || printf '(健康检查未通过)'
    printf '\n'
  else
    printf '未运行\n'
  fi
  printf 'docreader %s: ' "$DOCREADER_PORT"
  port_listening "$DOCREADER_PORT" && printf '可连接\n' || printf '未监听\n'
  printf '前端 5173: '
  port_listening 5173 && printf '运行中\n' || printf '未运行\n'
  printf 'Docker app: '
  if [ -n "$(docker ps --filter name=WeKnora-app --filter status=running -q 2>/dev/null || true)" ]; then
    printf '运行中（与 native 互斥，需先停掉）\n'
  else
    printf '已停止\n'
  fi
}

cmd_web() {
  command -v npm >/dev/null 2>&1 || die "找不到 npm"
  [ -d "$ROOT/frontend/node_modules" ] || die "依赖未安装，先执行: cd frontend && npm install"
  mkdir -p "$LOG_DIR"
  say "前端 vite dev 启动于 http://localhost:5173（API 代理到 :${APP_PORT}）"
  cd "$ROOT/frontend"
  export VITE_DEV_PROXY_TARGET="http://localhost:${APP_PORT}"
  exec npm run dev
}

case "${1:-start}" in
  start|"") cmd_start ;;
  bg)       cmd_bg ;;
  --air|air|watch|--watch) cmd_watch ;;
  web)      cmd_web ;;
  stop)     cmd_stop ;;
  status)   cmd_status ;;
  logs)     mkdir -p "$LOG_DIR"; tail -n 60 -f "$LOG_FILE" ;;
  -h|--help|help)
    sed -n '2,24p' "$0" | sed 's/^# \{0,1\}//'
    ;;
  *) die "未知参数: $1（可用: start | --air | bg | web | stop | status | logs）" ;;
esac
