# Cyime

🍋 Cyime —— 青柠写 轻快写作，随云流动，支持多方案图片上传的云文档

![Cyime Homepage Preview Light](packages/web/src/lib/assets/home_preview_light.png)
![Cyime Homepage Preview Dark](packages/web/src/lib/assets/home_preview_dark.png)

[![deepwiki](https://img.shields.io/badge/Ask_DeepWiki-_.svg?style=for-the-badge&color=00b0aa&labelColor=000000&logoColor=ffffff)](https://deepwiki.com/Coldin04/Cyime)
[![zread](https://img.shields.io/badge/Ask_Zread-_.svg?style=for-the-badge&color=00b0aa&labelColor=000000&logo=data%3Aimage%2Fsvg%2Bxml%3Bbase64%2CPHN2ZyB3aWR0aD0iMTYiIGhlaWdodD0iMTYiIHZpZXdCb3g9IjAgMCAxNiAxNiIgZmlsbD0ibm9uZSIgeG1sbnM9Imh0dHA6Ly93d3cudzMub3JnLzIwMDAvc3ZnIj4KPHBhdGggZD0iTTQuOTYxNTYgMS42MDAxSDIuMjQxNTZDMS44ODgxIDEuNjAwMSAxLjYwMTU2IDEuODg2NjQgMS42MDE1NiAyLjI0MDFWNC45NjAxQzEuNjAxNTYgNS4zMTM1NiAxLjg4ODEgNS42MDAxIDIuMjQxNTYgNS42MDAxSDQuOTYxNTZDNS4zMTUwMiA1LjYwMDEgNS42MDE1NiA1LjMxMzU2IDUuNjAxNTYgNC45NjAxVjIuMjQwMUM1LjYwMTU2IDEuODg2NjQgNS4zMTUwMiAxLjYwMDEgNC45NjE1NiAxLjYwMDFaIiBmaWxsPSIjZmZmIi8%2BCjxwYXRoIGQ9Ik00Ljk2MTU2IDEwLjM5OTlIMi4yNDE1NkMxLjg4ODEgMTAuMzk5OSAxLjYwMTU2IDEwLjY4NjQgMS42MDE1NiAxMS4wMzk5VjEzLjc1OTlDMS42MDE1NiAxNC4xMTM0IDEuODg4MSAxNC4zOTk5IDIuMjQxNTYgMTQuMzk5OUg0Ljk2MTU2QzUuMzE1MDIgMTQuMzk5OSA1LjYwMTU2IDE0LjExMzQgNS42MDE1NiAxMy43NTk5VjExLjAzOTlDNS42MDE1NiAxMC42ODY0IDUuMzE1MDIgMTAuMzk5OSA0Ljk2MTU2IDEwLjM5OTlaIiBmaWxsPSIjZmZmIi8%2BCjxwYXRoIGQ9Ik0xMy43NTg0IDEuNjAwMUgxMS4wMzg0QzEwLjY4NSAxLjYwMDEgMTAuMzk4NCAxLjg4NjY0IDEwLjM5ODQgMi4yNDAxVjQuOTYwMUMxMC4zOTg0IDUuMzEzNTYgMTAuNjg1IDUuNjAwMSAxMS4wMzg0IDUuNjAwMUgxMy43NTg0QzE0LjExMTkgNS42MDAxIDE0LjM5ODQgNS4zMTM1NiAxNC4zOTg0IDQuOTYwMVYyLjI0MDFDMTQuMzk4NCAxLjg4NjY0IDE0LjExMTkgMS42MDAxIDEzLjc1ODQgMS42MDAxWiIgZmlsbD0iI2ZmZiIvPgo8cGF0aCBkPSJNNCAxMkwxMiA0TDQgMTJaIiBmaWxsPSIjZmZmIi8%2BCjxwYXRoIGQ9Ik00IDEyTDEyIDQiIHN0cm9rZT0iI2ZmZiIgc3Ryb2tlLXdpZHRoPSIxLjUiIHN0cm9rZS1saW5lY2FwPSJyb3VuZCIvPgo8L3N2Zz4K&logoColor=ffffff)](https://zread.ai/Coldin04/Cyime)

---

## 功能亮点

- 轻量云端写作：在线编辑、云端同步、公开分享和多端访问。
- 媒体库与图床工作流：集中管理图片与附件，支持从媒体反向定位引用文档。
- 实时协作可选：按部署需要启用或关闭协作、共享、presence 与持久化链路。
- AI 融合支持：通过 Skill、MCP 与 REST Open API 接入 AI 客户端，让授权 AI 可以搜索、读取、整理和写入工作区文档，协助管理文章资产。

## 如何部署

当前推荐的部署方式是前后端分离：

- `packages/web` 单独部署到 Cloudflare Pages 或 EdgeOne Pages，保留 SvelteKit SSR。
- `packages/server` 与 `packages/realtime` 独立部署，适合后续统一放进 Docker Compose。

### Web 部署

当前已支持以下前端 SSR 目标：

- Cloudflare Pages
- EdgeOne Pages

前端部署时只需要导入当前仓库，并将项目根目录设置为 `packages/web`。

最小必填环境变量：

- `PUBLIC_API_BASE_URL=https://你的后端域名`

这是前端访问 Go API 的基础地址，前端所有 `/api/v1/...` 请求都会拼接到这个 origin。生产部署时必须填写浏览器可访问的后端公网地址，不要填容器内网地址或仅服务器本机可访问的地址。

可选公开环境变量：

- `PUBLIC_AVATAR_MAX_BYTES=2097152`
- `PUBLIC_AVATAR_OUTPUT_SIZE=512`

服务端媒体相关环境变量里，文档图片上传上限可通过 `MEDIA_DOCUMENT_IMAGE_MAX_BYTES` 配置；当前默认值为 `5242880`（5MB）。

常用构建命令：

- Cloudflare Pages：`pnpm install --frozen-lockfile && pnpm run build`
- EdgeOne Pages 安装：`pnpm install --frozen-lockfile --config.node-linker=hoisted`
- EdgeOne Pages：`pnpm run build:edgeone`

Cloudflare Pages 构建如果涉及 Node 内建模块兼容，仓库内已经提供 [packages/web/wrangler.toml](packages/web/wrangler.toml)，默认启用 `nodejs_compat`，并且 `pnpm run build` 会自动识别 `CF_PAGES=1` 切到 Cloudflare 构建。

这些 `PUBLIC_*` 变量统一按运行时公开变量处理，适合在 Pages / EdgeOne 的项目环境变量里配置。

更完整的平台配置说明请参阅 [Web 部署说明](docs/web_deployment.md)。

### 前后端通信配置

前后端分离部署时，需要同时配置浏览器访问链路、后端跨域白名单和登录回调地址：

- `packages/web`：`PUBLIC_API_BASE_URL=https://你的后端域名`
- `packages/server`：`CORS_ALLOWED_ORIGINS=https://你的前端域名`
- `packages/server`：`FRONTEND_CALLBACK_URL=https://你的前端域名/auth/callback`
- `packages/server`：`API_BASE_URL=https://你的后端域名`

这四个地址分别解决“前端请求后端”“后端允许前端跨域请求”“登录后回到前端”“后端生成自己的公网 URL”。如果使用 AI 融合能力里的 Markdown 转换服务，还需要额外配置后端到前端的内部调用：

- `MARKDOWN_CONVERTER_URL=https://你的前端域名`
- `MARKDOWN_CONVERTER_TOKEN=同一把内部共享密钥`

`MARKDOWN_CONVERTER_TOKEN` 需要同时配置到 `packages/server` 与 `packages/web` 的运行环境，用于保护 `/markdown/convert` 内部路由。它不是用户中心创建的 API Token，也不应提供给 AI 客户端。

### Server 与 Realtime

- `packages/server` 是 Go API 服务。
- `packages/realtime` 是独立的实时协作服务。
- 当前建议两者保持独立部署，避免把 WebSocket 与前端平台运行时耦合在一起。
- 后续可以统一收敛到 Docker Compose 做一键启动。
- 如果你不使用实时协作，建议直接不启动 `packages/realtime`。

实时协作总开关：

- 使用 `COLLABORATION_ENABLED=true|false`
- 这是服务端配置，不是前端配置
- `packages/server` 会把该值作为 `collaborationEnabled` 通过 `/api/v1/config` 下发给前端
- `packages/realtime` 会用同名变量决定是否接受 websocket、presence 和强制持久化请求
- 如果要完全关闭协作，必须同时在 `packages/server/.env` 和 `packages/realtime/.env` 中设置为 `false`
- 关闭后，前端编辑页会退回单人保存链路；非 owner 访问会按“文档不存在或无权访问”处理
- 如果还希望某个前端部署本地就完全不加载协作能力，可额外设置 `packages/web/.env` 中的 `PUBLIC_COLLABORATION_ENABLED=false`
- 前端最终生效值为：`PUBLIC_COLLABORATION_ENABLED && 后端下发的 collaborationEnabled`
- 以上两个变量统一记录在根目录 [`.env.example`](.env.example)
- 对于单人部署，推荐做法是：
  - 不启动 `packages/realtime`
  - `packages/server/.env` 设 `COLLABORATION_ENABLED=false`
  - `packages/web/.env` 设 `PUBLIC_COLLABORATION_ENABLED=false`

### Skill / MCP / Open API

Cyime 提供面向 AI 客户端和外部工具的工作区集成能力：

- Skill Markdown：`https://你的前端域名/skill.md`
- Skill Manifest：`https://你的前端域名/manifest.json`
- OpenAPI：`https://你的前端域名/openapi.json`
- MCP endpoint：`https://你的后端域名/api/v1/mcp`
- REST Open API root：`https://你的后端域名/api/v1/open`

推荐优先使用 MCP；当客户端不支持 MCP 时，再使用 REST Open API。MCP 当前通过 HTTP JSON-RPC 暴露，客户端应以 `POST /api/v1/mcp` 调用。

用户中心可以创建 Cyime API Token，用于支持 HTTP MCP 的客户端、脚本或其他外部工具访问 MCP/Open API。这个 token 与站点登录会话无关，应按需要授予最小权限。

常用权限范围：

- `workspace:read`：读取文件/文件夹列表和搜索结果。
- `workspace:write`：创建文件夹、重命名、创建文档所需的工作区写权限。
- `document:read`：读取文档 Markdown 内容。
- `document:write`：创建或更新文档 Markdown 内容。
- `file:move`：移动文件或文件夹。
- `file:copy`：复制文件或文件夹。
- `file:delete`：移动文件或文件夹到回收站。该权限具有破坏性，应仅在明确需要时开启。

支持 HTTP MCP 的客户端可使用类似配置导入：

```json
{
  "mcpServers": {
    "cyime-workspace": {
      "type": "http",
      "url": "https://你的后端域名/api/v1/mcp",
      "headers": {
        "Authorization": "Bearer <CYIME_API_TOKEN>"
      }
    }
  }
}
```

不要把真实 API Token 写进仓库、README、issue、日志或公开提示词。前端用户中心里的“创建并复制 MCP”会在本地剪贴板生成导入 JSON；测试完成后可以吊销 token。

#### Skill 导入

支持 Skill 的客户端可以导入前端暴露的 `skill.md`：

```text
https://你的前端域名/skill.md
```

Skill 会声明 MCP 优先、REST Open API 兜底，并要求客户端把 API Token 配置为 `CYIME_API_TOKEN` 这类 secret 变量。不要把 token 明文写入 `skill.md`、`manifest.json`、`openapi.json` 或聊天消息。

MCP 当前包含 `cyime_search_files`、`cyime_list_files`、创建/读取/更新/增量更新 Markdown 文档、重命名、移动、复制和软删除工具。`tools/list` 会返回 `readOnlyHint`、`destructiveHint` 等 MCP annotations，方便客户端区分只读工具和需要确认的破坏性工具。

#### Markdown 转换服务

MCP/Open API 读写文档时对外使用 Markdown，内部存储使用 Tiptap JSON。为了让 AI 写入 Markdown 和前端编辑器粘贴 Markdown 的行为一致，推荐让 Go 后端调用 `packages/web` 提供的内部转换服务：

- 转换路由：`POST /markdown/convert`
- 只供 Go 后端内部访问
- 使用 `MARKDOWN_CONVERTER_TOKEN` 做内部共享密钥
- 不要将该路由作为公开 API 文档给第三方使用

后端可以只配置 web origin，服务端会自动补 `/markdown/convert`：

```env
MARKDOWN_CONVERTER_URL=https://你的前端域名
MARKDOWN_CONVERTER_TOKEN=
MARKDOWN_CONVERTER_TIMEOUT=5s
MARKDOWN_CONVERTER_FALLBACK=false
```

`MARKDOWN_CONVERTER_TOKEN` 由部署者自行生成并分别配置到 `packages/server` 和 `packages/web` 的运行环境中。它不是用户中心创建的 `cyime_sk_...` API Token，也不能给 AI 客户端使用。

本地生成内部转换 token 的示例：

```bash
openssl rand -hex 32
```

本地验证转换服务时，先在前端进程和测试 shell 中配置同一个 `MARKDOWN_CONVERTER_TOKEN`，然后请求：

```bash
curl -sS http://127.0.0.1:5173/markdown/convert \
  -H "Authorization: Bearer ${MARKDOWN_CONVERTER_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{"direction":"markdown-to-json","markdown":"# Hello\n\n- one\n- two"}'
```

如果转换服务不可用，且 `MARKDOWN_CONVERTER_FALLBACK=false`，MCP/Open API 写入会失败并明确提示文档未更改；只有显式设置 `MARKDOWN_CONVERTER_FALLBACK=true` 时才会回退到 Go 内置的简化 Markdown 转换器。


## 仓库说明

关于仓库迁移、提交历史整理以及部署仓库说明，请参阅：

- [仓库迁移与提交历史说明](docs/repository_migration_note.md)

---

## 开发指南 (Development Guide)

### 项目架构 (Project Architecture)

以下是本项目核心功能（特别是认证系统）相关的主要文件架构。

#### **后端 (`/packages/server`)**

-   `cmd/server/main.go`: **应用入口**。负责注册所有 API 路由并应用中间件。
-   `internal/auth/handler.go`: **认证处理器**。处理 `/login`, `/callback`, `/refresh` 等路由的主逻辑。
-   `internal/auth/token.go`: **令牌服务核心**。封装了所有关于令牌的创建、持久化和刷新逻辑。
-   `internal/middleware/auth.go`: **JWT 认证中间件**。提供 `Protected()` 中间件来保护需要认证的接口。
-   `internal/user/handler.go`: **用户处理器**。处理与用户数据相关的请求 (`/user/me`)。
-   `internal/database/database.go`: **数据库**。初始化 GORM 连接并执行自动迁移。
-   `internal/models/*.go`: **数据库模型**。定义了 `users`, `auth_providers` 等数据表结构。

#### **前端 (`/packages/web`)**

-   `src/lib/stores/auth.ts`: **前端认证的大脑**。通过 Svelte Store 集中管理认证状态和所有刷新逻辑。
-   `src/lib/api.ts`: **API 请求工具**。导出的 `apiFetch` 函数封装了原生 `fetch`，自动处理认证头和 401 错误重试。
-   `src/routes/auth/callback/+page.svelte`: **登录回调页**。处理从第三方登录成功后的跳转。
-   `src/routes/workspace/+layout.svelte`: **工作区路由守卫**。保护 `/workspace` 目录下的所有页面。

### 本地开发 (Local Development)

1.  **环境准备**:
    -   确保您已安装 Go (1.22+)。
    -   确保您已安装 Node.js (20+) 和 `pnpm`，建议使用 `22+`。

2.  **启动后端服务**:
    ```bash
    # 进入后端目录
    cd packages/server

    # 对数据服务进行初始化操作,并注入 Oauth 服务以便接下来的操作
    ENV=DEVELOPMENT go run ./cmd/init/main.go

    # 运行后端
    ENV=DEVELOPMENT go run ./cmd/server/main.go
    ```
    后端服务将运行在 `http://localhost:8080`。
    说明：后端启动会自动读取 `packages/server/.env`。

3.  **启动前端服务**:
    ```bash
    # 进入前端目录
    cd packages/web

    # 安装依赖
    pnpm install

    # 运行前端开发服务器
    pnpm run dev
    ```
    前端服务将运行在 `http://localhost:5173`。

### 关键环境变量

完整示例见根目录 [`.env.example`](.env.example)。下面只列当前最常用的配置项。

- 认证与会话
  - `JWT_SECRET_KEY`：JWT 与部分签名逻辑依赖的密钥，生产环境必须配置。
  - `ACCESS_TOKEN_LIFETIME_MINUTES`：Access Token 生命周期，默认 `15`。
  - `ACCESS_TOKEN_LIFETIME_SECONDS`：Access Token 秒级生命周期覆盖项，优先级高于分钟配置，主要用于本地刷新链路测试。
  - `REFRESH_TOKEN_LIFETIME_HOURS`：Refresh Token 生命周期，默认 `720`（30 天）。
  - `FRONTEND_CALLBACK_URL`：登录成功后回跳到前端的地址。

- 文档数量限制
  - `DEFAULT_DOCUMENT_QUOTA`：全局默认文档上限。
  - 留空表示不限制。
  - 用户如果单独配置了 `document_quota`，会优先使用用户自己的值。
  - 后端会在创建文档时校验这个上限。

- 实时协作开关
  - `COLLABORATION_ENABLED`：是否启用实时协作，默认 `true`
  - 需要在 `packages/server/.env` 与 `packages/realtime/.env` 保持一致
  - 前端不会直接读取本地 env，而是读取后端 `/api/v1/config` 下发的 `collaborationEnabled`
  - 可选前端附加开关：`PUBLIC_COLLABORATION_ENABLED`，默认 `true`
  - 适用于你想让某个前端部署固定保持单人模式，不去初始化协作 UI / provider
  - 这两个变量都已统一写在根目录 [`.env.example`](.env.example)

- Skill / MCP Markdown 转换
  - `MARKDOWN_CONVERTER_URL`：内部 Markdown 转换服务地址。可填写前端 origin，例如 `https://你的前端域名`，Go 后端会自动补 `/markdown/convert`。
  - `MARKDOWN_CONVERTER_TOKEN`：Go 后端与 Web 内部转换服务之间的共享密钥。需要同时配置到 `packages/server` 与 `packages/web` 的运行环境，不要写入仓库。
  - `MARKDOWN_CONVERTER_TIMEOUT`：Go 后端调用转换服务的超时时间，默认 `5s`。
  - `MARKDOWN_CONVERTER_FALLBACK`：转换服务失败时是否回退到 Go 内置简化转换器，默认 `false`。保持 `false` 时，写入失败会明确提示文档未更改。
  - `MARKDOWN_CONVERTER_MAX_BYTES`：Web 内部转换路由接受的最大请求体字节数，默认 `2097152`。

- 媒体与图床
  - `MEDIA_STORAGE_PROVIDER`：可选 `local | r2 | s3 | cos`，默认 `local`。
  - `MEDIA_TOKEN_SECRET`：私有媒体短期签名密钥；留空时回退到 `JWT_SECRET_KEY`。
  - `MEDIA_SIGN_TTL_SECONDS`：私有媒体签名有效期，默认 `120` 秒。
  - `MEDIA_AVATAR_SIGN_TTL_SECONDS`：私有头像签名有效期，默认 `300` 秒。
  - `MEDIA_AVATAR_MAX_BYTES`：头像上传大小限制，默认 `2MB`。

- 本地媒体存储
  - `MEDIA_LOCAL_ROOT_DIR`：本地文件落盘目录。
  - `MEDIA_LOCAL_BASE_URL`：本地媒体对外访问前缀。

- S3 / R2 / COS 兼容存储
  - `MEDIA_S3_ENDPOINT`
  - `MEDIA_S3_BUCKET`
  - `MEDIA_S3_REGION`
  - `MEDIA_S3_ACCESS_KEY_ID`
  - `MEDIA_S3_SECRET_ACCESS_KEY`
  - `MEDIA_S3_PUBLIC_BASE_URL`
  - 兼容旧变量：`R2_ENDPOINT`、`R2_BUCKET`、`R2_REGION`、`R2_ACCESS_KEY_ID`、`R2_SECRET_ACCESS_KEY`、`R2_PUBLIC_BASE_URL`

- 媒体 GC / 引用回收
  - `MEDIA_ASSET_GC_ENABLED`
  - `MEDIA_ASSET_GC_INTERVAL`
  - `MEDIA_ASSET_GC_BATCH_SIZE`
  - `MEDIA_ASSET_GC_MAX_ATTEMPTS`
  - `MEDIA_ASSET_GC_RETRY_GAP`
  - `MEDIA_ASSET_RECONCILE_ENABLED`
  - `MEDIA_ASSET_RECONCILE_BATCH_SIZE`

- 其他
  - `CORS_ALLOWED_ORIGINS`：允许的前端来源，多个值用英文逗号分隔。
  - `RESET_WORKSPACE_TABLES_ON_BOOT`：启动时重置业务表，仅调试时使用。

### 详细文档

-   关于此系统架构与数据流的详细技术概览，请参阅 **[统一认证系统总结](docs/auth_system_summary.md)**。
-   关于所有 API 端点的快速参考，请参阅 **[API 文档](blueprints/api_documentation.md)**。
