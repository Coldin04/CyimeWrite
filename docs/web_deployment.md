# Web 部署说明

当前前端只部署 `packages/web`，后端与 realtime 独立部署。

## 通用设置

- 仓库根目录：`/`
- 前端项目目录：`packages/web`
- Node 版本：`22`（建议固定为 `22.17.1`）
- Pages / EdgeOne 部署时都建议先保存配置，再重新触发一次完整部署

## 环境变量约定

当前 `web` 端统一使用运行时公开变量：

- `PUBLIC_API_BASE_URL`
- `PUBLIC_AVATAR_MAX_BYTES`
- `PUBLIC_AVATAR_OUTPUT_SIZE`

这些变量用于 SSR 运行时与浏览器端公开配置读取，不要求写进仓库，也不要求用户修改 `wrangler.toml`。

`PUBLIC_API_BASE_URL` 也会写入前端公开的 `/skill.md`、`/manifest.json` 和 `/openapi.json`，用于声明 MCP、REST Open API 和 Skill OAuth 地址。后端部署应同步配置 `API_BASE_URL` / `PUBLIC_API_BASE_URL` 指向同一个后端公网 origin；如果启用 Skill OAuth，还需要在后端配置 `PUBLIC_BASE_URL` 和按需配置 `CYIME_SKILL_OAUTH_REDIRECT_URIS`。

## Cloudflare Pages

- Root directory：`packages/web`
- Build command：`pnpm install --frozen-lockfile && pnpm run build`
- Build output directory：`.svelte-kit/cloudflare`
- 仓库内已提供 `packages/web/wrangler.toml`，默认包含：
  - `name = "cyimewrite-web"`
  - `pages_build_output_dir = ".svelte-kit/cloudflare"`
  - `compatibility_date = "2026-04-04"`
  - `compatibility_flags = ["nodejs_compat"]`

### Cloudflare 操作步骤

1. 选择仓库后，手动填写 `Root directory = packages/web`
2. 手动填写 `Build command = pnpm install --frozen-lockfile && pnpm run build`
3. `Build output directory` 填 `.svelte-kit/cloudflare`
4. 在项目环境变量里填写：

- `PUBLIC_API_BASE_URL=https://你的后端域名`
- `PUBLIC_AVATAR_MAX_BYTES=2097152`
- `PUBLIC_AVATAR_OUTPUT_SIZE=512`

5. 如果 Dashboard 提示当前项目由 `wrangler.toml` 管理，普通变量不可直接编辑：
   - 可以将上述变量以相同名字作为加密变量填写
   - 对当前场景也能正常工作，因为这些值本身就是前端公开配置
6. 保存后重新部署

### Cloudflare 说明

- `pnpm run build` 会自动识别 `CF_PAGES=1`，并切到 Cloudflare 适配构建
- `wrangler.toml` 只保留固定兼容配置，不存放每个用户自己的后端域名
- 如果修改了环境变量，请重新触发部署，不要只依赖历史部署缓存

## EdgeOne Pages

- Root directory：`packages/web`
- Install command：`pnpm install --frozen-lockfile --config.node-linker=hoisted`
- Build command：`pnpm run build:edgeone`
- Output directory：`.edgeone/assets`
- 仓库内已提供 `packages/web/edgeone.json`，默认固定：
  - `installCommand = pnpm install --frozen-lockfile --config.node-linker=hoisted`
  - `buildCommand = pnpm run build:edgeone`
  - `outputDirectory = .edgeone/assets`
  - `nodeVersion = 22.17.1`

### EdgeOne 操作步骤

1. 选择仓库后，手动填写 `Root directory = packages/web`
2. 如果控制台未自动读取 `edgeone.json`，安装命令填写 `pnpm install --frozen-lockfile --config.node-linker=hoisted`
3. 如果控制台未自动读取 `edgeone.json`，构建命令填写 `pnpm run build:edgeone`
4. 如果控制台未自动读取 `edgeone.json`，输出目录填写 `.edgeone/assets`
5. 在环境变量里填写：

- `PUBLIC_API_BASE_URL=https://你的后端域名`
- `PUBLIC_AVATAR_MAX_BYTES=2097152`
- `PUBLIC_AVATAR_OUTPUT_SIZE=512`

6. 保存后重新部署

### EdgeOne 说明

- EdgeOne 的 SSR 函数运行时对 `pnpm` 默认依赖链接布局可能不够稳定，因此这里显式使用 `node-linker=hoisted`
- 如果后续仍出现 `ERR_MODULE_NOT_FOUND`，优先检查控制台实际使用的安装命令是否与 `edgeone.json` 一致

## 本地验证

在 `packages/web` 目录下执行：

```bash
pnpm run build
pnpm run build:cloudflare
pnpm run build:edgeone
```
