#!/usr/bin/env python3
"""容器内的回环转发：127.0.0.1:11434 -> 宿主机 Ollama。

背景：数据库里保存的模型地址是 http://127.0.0.1:11434（与 native 模式一致）。
在容器内 127.0.0.1 指向容器自身，模型调用会失败。这个脚本在容器的回环地址上
监听 11434，把流量转发到 host.docker.internal:11434（Docker Desktop 内置 DNS
解析到宿主机），这样同一份数据库在 native 和容器里都能用，无需改模型配置。

由 docker-compose.lite.yml 的 entrypoint 覆盖在启动前拉起，随容器生命周期
自动退出，不依赖额外的侧车容器（侧车在 app 重建后会因为共享的 network
namespace 失效）。
"""

import socket
import sys
import threading

LISTEN_HOST = "127.0.0.1"
LISTEN_PORT = 11434
TARGET_HOST = "host.docker.internal"
TARGET_PORT = 11434
BUFSIZE = 65536


def _pipe(src: socket.socket, dst: socket.socket) -> None:
    try:
        while True:
            data = src.recv(BUFSIZE)
            if not data:
                break
            dst.sendall(data)
    except OSError:
        pass
    finally:
        try:
            dst.shutdown(socket.SHUT_WR)
        except OSError:
            pass


def _handle(client: socket.socket) -> None:
    try:
        upstream = socket.create_connection((TARGET_HOST, TARGET_PORT), timeout=10)
    except OSError as exc:
        print(f"[ollama-loopback] upstream unreachable: {exc}", file=sys.stderr, flush=True)
        client.close()
        return
    upstream.settimeout(None)
    threading.Thread(target=_pipe, args=(client, upstream), daemon=True).start()
    _pipe(upstream, client)
    for sock in (client, upstream):
        try:
            sock.close()
        except OSError:
            pass


def main() -> int:
    server = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    server.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    try:
        server.bind((LISTEN_HOST, LISTEN_PORT))
    except OSError as exc:
        print(f"[ollama-loopback] bind failed: {exc}", file=sys.stderr, flush=True)
        return 1
    server.listen(128)
    print(
        f"[ollama-loopback] {LISTEN_HOST}:{LISTEN_PORT} -> {TARGET_HOST}:{TARGET_PORT}",
        flush=True,
    )
    while True:
        client, _ = server.accept()
        threading.Thread(target=_handle, args=(client,), daemon=True).start()


if __name__ == "__main__":
    sys.exit(main())
