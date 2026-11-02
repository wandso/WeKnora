# Windows 开发环境：启用 CGO

WeKnora 的后端有四个依赖必须用 CGO 编译，`CGO_ENABLED=0` 时整个仓库无法构建：

| 依赖 | 用途 | 缺 CGO 时的报错 |
| --- | --- | --- |
| `pg_query_go/v6` | SQL 注入检测（`internal/utils/inject.go`） | `undefined: pg_query.Parse` |
| `sqlite-vec-go-bindings/cgo` | SQLite 向量检索 | `build constraints exclude all Go files` |
| `duckdb-go-bindings` | DuckDB 数据源 | 同上 |
| `mattn/go-sqlite3` | Lite 模式数据库 | 同上 |

本文记录在本机（Windows + Go 1.27.2）上跑通 `go build ./...` 的完整步骤。

## 1. 安装 MinGW-w64 工具链

推荐 [winlibs](https://github.com/brechtsanders/winlibs_mingw) 的 **POSIX 线程**版本。不要选 win32 线程版本：`duckdb-go-bindings` 附带的静态库是按 posix 线程模型编译的，win32 版本会在链接阶段失败。

```bash
# 目录：E:/xyh/mingw64
https://github.com/brechtsanders/winlibs_mingw/releases/download/14.2.0posix-19.1.1-12.0.0-ucrt-r2/winlibs-x86_64-posix-seh-gcc-14.2.0-mingw-w64ucrt-12.0.0-r2.zip
```

从国内网络直连 GitHub 只有 ~14 KB/s（233 MB 要 4 小时）。实际做法是走 `gh-proxy.com` 并**分段并行下载**，16 个连接可以跑到 ~1.5 MB/s，两分钟下完：

```bash
# 单连接测速对比（同一 URL）
curl -s --max-time 20 -r 0-8388608 -o t.bin -w "%{speed_download}\n" "$URL"
# ghproxy.net  ≈ 7 KB/s   （不可用）
# gh-proxy.com ≈ 230 KB/s （单连接）
# 16 连接并行  ≈ 1.5 MB/s （可用）
```

解压后得到 `E:/xyh/mingw64/bin/gcc.exe`：

```bash
unzip -q mingw64.zip -d /e/xyh
/e/xyh/mingw64/bin/gcc.exe --version
# gcc.exe (MinGW-W64 x86_64-ucrt-posix-seh, built by Brecht Sanders, r2) 14.2.0
```

## 2. 补 sqlite3.h

`sqlite-vec-go-bindings` 在 `SQLITE_CORE` 模式下 `#include "sqlite3.h"`，而 Windows 上没有 `libsqlite3-dev` 等价物。链接符号由 `mattn/go-sqlite3` 自带 amalgamation 提供，**只缺头文件**，因此不要另外编译 `sqlite3.c`（会重复定义符号）。

直接复制同版本的头文件，避免 ABI 不一致：

```bash
mkdir -p /e/xyh/sqlite-include
cp ~/go/pkg/mod/github.com/mattn/go-sqlite3@v1.14.24/sqlite3-binding.h /e/xyh/sqlite-include/sqlite3.h
cp ~/go/pkg/mod/github.com/mattn/go-sqlite3@v1.14.24/sqlite3ext.h     /e/xyh/sqlite-include/
```

`go-sqlite3@v1.14.24` 内置的是 SQLite 3.46.1，头文件版本与链接期实现一致。

## 3. 环境变量

```bash
export CGO_ENABLED=1
export CC="E:/xyh/mingw64/bin/gcc.exe"
export CGO_CFLAGS="-IE:/xyh/sqlite-include"
export PATH="/e/xyh/mingw64/bin:$PATH"
```

注意两点：

- `CC` 必须写 **Windows 路径**（`E:/...`，正斜杠可以），写成 `/e/...` 这种 MSYS 路径时 Go 找不到编译器。
- `GOPROXY` 默认的 `proxy.golang.org` 在本机返回 502，`cmd/desktop` 依赖 `wails/v2` 会下载失败。已改为 `go env -w GOPROXY=https://goproxy.cn,direct`。

## 4. 验证

```bash
go build ./...                      # 首次约 4 分钟（pg_query 的 C 代码很大），之后走缓存
go vet ./internal/handler/ ./internal/router/ ./internal/application/service/
go test ./internal/container/ -run TestRouterWiring
go test ./internal/application/service/ -run 'TestNormalizePage|TestSlugPattern'
```

前端：

```bash
cd frontend
npm install            # 首次约 22 分钟（npm 源已指向 registry.npmmirror.com）
npm run type-check     # vue-tsc --build，约 2 分钟
npm run build          # vite build，约 3 分钟
```

## 5. Windows 上的已知失败用例

以下用例在 Windows 下失败，与代码逻辑无关，改动前就已如此：

| 用例 | 原因 |
| --- | --- |
| `TestDataSourceScheduleAPIRejectsInvalidCron` | `t.TempDir()` 清理时 sqlite 连接仍持有句柄，`unlinkat` 报 "file is being used by another process" |
| `TestBatchDownloadKnowledgeProducesCompleteZIP` | 同上：临时 zip 在 Windows 上无法立即删除 |
| `TestBatchDownloadKnowledgeFailureDoesNotReturnPartialZIP` | 同上 |
| `TestListSkillsReturnsUsableInstalledSkillsForTheSelectedConfig` | 断言过时：期望 `Category:""`/`RiskLevel:""`，实际返回 `"unclassified"`/`"medium"` |

前三个是 Windows 文件锁语义，在 Linux/CI 上通过。最后一个是测试没有跟上 skill 元数据默认值的变化。

## 6. 本地跑起来（Lite 模式，不需要 PostgreSQL / Redis）

后端支持纯单机模式：SQLite 存数据、sqlite-vec 做向量检索、内存做流管理。适合本地验证页面与接口。

### 6.1 配置

标准构建**不会**自动读 `.env`（`internal/envfile` 只认显式 `ENV_FILE`，或在 Lite 构建下读 `.env.lite`），所以 docker 用的 `.env`（postgres + redis）不会被误用。另建一份：

```dotenv
# .env.lite
DB_DRIVER=sqlite
DB_PATH=./data/weknora.db
RETRIEVE_DRIVER=sqlite
STORAGE_TYPE=local
LOCAL_STORAGE_BASE_DIR=./data/files
STREAM_MANAGER_TYPE=memory
OLLAMA_BASE_URL=http://127.0.0.1:11434
DOCREADER_ADDR=127.0.0.1:50051
DOCREADER_TRANSPORT=grpc
SYSTEM_AES_KEY=<32 hex chars>   # openssl rand -hex 16
JWT_SECRET=<64 hex chars>       # openssl rand -hex 32
NEO4J_ENABLE=false
WEKNORA_SANDBOX_MODE=disabled
ENABLE_GRAPH_RAG=false
```

### 6.2 启动

```bash
# 后端（8080）
export CGO_ENABLED=1 CC="E:/xyh/mingw64/bin/gcc.exe" CGO_CFLAGS="-IE:/xyh/sqlite-include"
export ENV_FILE=.env.lite
go run -tags sqlite_fts5 ./cmd/server

# 前端（5173，/api 自动代理到 8080）
cd frontend && npm run dev
```

打开 <http://localhost:5173>。没有预置管理员，直接在登录页注册账号即可。

### 6.3 必须带 `-tags sqlite_fts5`

不加这个标签时 sqlite3 驱动没有 FTS5 模块，启动日志会出现：

```
[SQLite] Keyword index disabled, driver has no fts5 module: sqlite driver built without the fts5 module
```

后果是关键词检索整条链路被禁用，只剩向量召回，检索质量明显下降。Makefile 的 Lite 目标（`make build-lite`，Makefile:316）就是带这个 tag 构建的。

### 6.4 首次启动会做什么

- 自动执行迁移（`migrations/sqlite`，当前到 54）
- 注册约 562 条路由
- 播种 4 个内置智能体模板（`seeded 4 builtin agent templates`）
- 打印 `Server is running at 0.0.0.0:8080`

首次启动从 `BuildContainer` 到监听端口大约需要 30 秒（含 FTS5 表回填），不要以为卡死。

### 6.5 docreader（文档解析服务）

Lite 后端默认 `DOCREADER_ADDR=127.0.0.1:50051`。docreader 不在这个仓库进程里，需要单独提供。

**本机现状：整套 Docker 栈一直在跑**，docreader 已随 compose 启动并 healthy：

```bash
docker ps --format 'table {{.Names}}\t{{.Ports}}\t{{.Status}}'
# WeKnora-frontend    0.0.0.0:8888->80/tcp      Up (healthy)
# WeKnora-app         0.0.0.0:8081->8080/tcp    Up (healthy)
# WeKnora-docreader   50051/tcp                 Up (healthy)
# WeKnora-postgres    5432/tcp                  Up (healthy)
# WeKnora-redis       6379/tcp                  Up
```

注意 compose 里 docreader 只有 `expose: 50051`，**没有映射到宿主机**，所以本地 Lite 后端连不上。WSL2 下宿主机也访问不到容器 IP（`Test-NetConnection 172.19.0.x` 为 False），必须做端口转发。

不改动现有容器、也不改 compose 的做法是加一个 socat relay：

```bash
docker run -d --name weknora-docreader-relay --restart unless-stopped \
  --network weknora-main_WeKnora-network \
  -p 127.0.0.1:50051:50051 \
  alpine/socat tcp-listen:50051,fork,reuseaddr tcp-connect:WeKnora-docreader:50051
```

验证端到端可用（不经过 UI）：

```bash
go run ./cmd/docreader-probe 127.0.0.1:50051 docreader/testdata/test.md
```

实测结果：

| 文件 | 耗时 | 结果 |
| --- | --- | --- |
| `test.md` | 422 ms | 730 字节 markdown，标题/图片/链接/代码块/表格均正确保留 |
| `test.html` | 264 ms | 697 字节 markdown |
| `test_text.png` | 37 ms | 返回 `image_refs=1`（图片按资源保留，OCR 走预处理/VLM 路径） |
| `test.txt` | — | 服务端返回 `Unsupported file type: txt`，属预期 |

`cmd/docreader-probe` 是为此写的诊断工具，保留下来方便以后排查。

**端到端验证（后端 → relay → docreader）**

注册账号 → 建知识库 → 上传 `docreader/testdata/test.md`。第一次跑的时候日志停在：

```
[convert] docreader call ok in 0s for "test.md"                      ← 解析成功
processChunks get embedding model failed: model ID cannot be empty   ← 卡在这里
```

`parse_status=failed` 的原因不是 docreader，而是 Lite 模式**一个模型都没配**，chunk 建不了索引。按 6.8 配好模型后重跑，整条链路打通（详见 6.8 的验证结果）。

### 6.6 两套部署的关系

| | Docker 栈 | 本地 Lite |
| --- | --- | --- |
| 前端 | http://localhost:8888 | http://localhost:5173 |
| 后端 | http://localhost:8081 | http://localhost:8080 |
| 数据库 | PostgreSQL（paradedb）+ Redis | SQLite + 内存 |
| docreader | 有（容器内网） | 通过 relay 走 127.0.0.1:50051 |
| 代码版本 | 镜像构建时的版本 | **当前工作区代码** |

改代码调试请用本地 Lite（5173/8080），Docker 栈的镜像是旧的，不含新改的功能。

### 6.7 本机没有的部分

| 组件 | 影响 |
| --- | --- |
| PostgreSQL / Redis | Lite 模式不需要，已用 SQLite + 内存替代 |
| Ollama（11434） | 已在用，见 6.8。CPU 推理，7B 模型响应偏慢 |

日志写在 `/e/xyh/weknora-server.log` 与 `/e/xyh/weknora-frontend.log`。

### 6.8 配置 Ollama 模型（让文档真正入库）

Lite 模式开箱**没有任何模型**，所以上传文档必然卡在 `model ID cannot be empty`。本机 Ollama 已有的模型：

| 模型 | 维度/规模 | 用途 |
| --- | --- | --- |
| `qwen3-embedding:0.6b` | 1024 维，Q8_0，639 MB | embedding |
| `qwen2.5:7b` | 4.7 GB | 对话 / 摘要 |
| `qwen3.5:4b` | 3.3 GB | 对话备选（CPU 更快） |
| `nomic-embed-text` | 768 维 | embedding 备选 |

**坑 1：SSRF 白名单。** 模型 BaseURL 填 `http://127.0.0.1:11434` 会被拒：

```
Base URL 未通过安全校验：SSRF validation failed: hostname 127.0.0.1 is restricted
```

因为模型 BaseURL 是用户可控输入，服务端默认禁止指向回环/内网。解决：在 `.env.lite` 里加

```
SSRF_WHITELIST_EXTRA=127.0.0.1,localhost
```

然后**重启服务**。启动日志会打 `[system_settings] SSRF whitelist applied (0 primary entries, extra=true)`，`extra=true` 就说明生效了。

> 这个白名单也可以运行时改（系统设置 `ssrf.whitelist`，`PUT /api/v1/system/admin/settings/ssrf.whitelist`），但那个接口要系统管理员身份；本地开发直接改环境变量重启更省事。

**坑 2：`source` 必须是 `local`。** `local` 才会走 Ollama 客户端（`internal/models/embedding/embedder.go` 的 `newEmbedder` 分支），`remote` 走的是 OpenAI 兼容协议。Ollama 虽然也提供 OpenAI 兼容端点，但走 `local` 更直接，不用管 `/v1` 后缀。

**坑 3：embedding 维度必须显式给。** `embedding_parameters.dimension` 不填就是 0，建向量表时没有维度，索引写不进去。`qwen3-embedding:0.6b` 填 `1024`。

**创建模型（API）**

```bash
TOKEN=$(curl -s --noproxy '*' -X POST http://127.0.0.1:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"probe@example.com","password":"Test@12345"}' | jq -r .token)

# embedding
curl -s --noproxy '*' -X POST http://127.0.0.1:8080/api/v1/models \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{
  "name": "qwen3-embedding:0.6b", "type": "Embedding", "source": "local",
  "parameters": { "base_url": "http://127.0.0.1:11434", "provider": "ollama",
                  "embedding_parameters": { "dimension": 1024 } }
}'

# 对话
curl -s --noproxy '*' -X POST http://127.0.0.1:8080/api/v1/models \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{
  "name": "qwen2.5:7b", "type": "KnowledgeQA", "source": "local",
  "parameters": { "base_url": "http://127.0.0.1:11434", "provider": "ollama" }
}'
```

建完状态先是 `downloading`，后台会向 Ollama 发一次 pull（模型已存在时是空操作），1 秒内转 `active`。

**为什么 `name` 要和 Ollama 里的模型名逐字一致**：`source=local` 时 `name` 会直接当 Ollama 的 `model` 参数用，写错就是 `model not found`。

**只配一个对话模型。** 内置智能体（`builtin-quick-answer` 等）的 `model_id` 是空的，运行时回退到「列表里第一个 `KnowledgeQA` 模型」；而模型列表查询没有 `ORDER BY`。配两个对话模型，默认用哪个就取决于数据库返回顺序，属于隐性不确定性。要换模型就在智能体配置里显式指定 `model_id`。

> `is_default` 字段在模型上目前是死的：`ClearDefaultByType` 只在接口里声明、没有生产调用方，`PATCH /models/:id` 也只接受 `display_name` / `description`。别指望靠它定默认模型。

**验证结果**

| 环节 | 证据 |
| --- | --- |
| docreader 解析 | `[convert] docreader call ok in 0s for "test.md"` |
| 分块 | `chunks` 表 2 行 |
| embedding 入库 | `lite_embeddings` 2 行；`vec_embeddings_1024` 表被创建（1024 维来自模型配置） |
| 摘要生成 | `parse_status=completed`、`summary_status=completed`，描述由 qwen2.5:7b 真实生成 |
| 混合检索 | `POST /knowledge-bases/:id/hybrid-search` 命中 2 条，score 0.9999 / 0.9839 |
| 真实问答 | `POST /knowledge-chat/:session_id` 流式返回 23 个 answer 分片，`complete` 事件 `final_content=这个文档测试了Markdown解析功能，包含图片、链接、代码块、表格和分块内容。`，`references` 引用了 summary 分块与正文分块 |

注意本机 Ollama 是 **CPU 推理**（`/api/ps` 里 `size_vram=0`），7B 生成一段几百字摘要要 60 秒左右，属于算力问题不是故障。嫌慢就换 `qwen3.5:4b`（但记得按上面的理由只留一个 KnowledgeQA 模型）。

`vec_embeddings_1024` 是 sqlite-vec 的 `vec0` 虚拟表，用普通 Python `sqlite3` 打不开（`no such module: vec0`），要验证数量看 `lite_embeddings` 即可。
