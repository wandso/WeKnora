#!/usr/bin/env bash
# WeKnora —— SQLite(lite) + 容器化启动脚本
#
# 为什么需要这个脚本：前端 nginx 的 proxy_pass 里写的是 ${APP_HOST}:${APP_PORT}，
# envsubst 在容器启动时把它替换成字面量 "app:8080"，nginx 只在启动时解析一次 DNS
# 并缓存 IP。一旦 app 容器被重建（换镜像、改配置都会换 IP），nginx 仍指向旧 IP，
# 所有 /api 请求返回 502，页面会显示成"没有任何数据"。
# 所以这里在拉起服务后固定重启一次 frontend，让它重新解析 app 的地址。
#
# 用法：
#   ./scripts/up-lite.sh            # 启动/更新全部服务
#   ./scripts/up-lite.sh stop       # 停止（数据保留在 ./data）
#   ./scripts/up-lite.sh logs app   # 跟踪日志

set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

PROJECT=weknora-main
COMPOSE=(docker compose -p "$PROJECT" -f docker-compose.yml -f docker-compose.lite.yml)
SERVICES=(redis docreader app frontend)

case "${1:-up}" in
up)
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
  ;;
stop)
  "${COMPOSE[@]}" stop
  ;;
logs)
  "${COMPOSE[@]}" logs -f "${2:-app}"
  ;;
*)
  echo "用法: $0 [up|stop|logs <service>]" >&2
  exit 2
  ;;
esac
