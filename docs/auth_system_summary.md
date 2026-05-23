# 统一认证系统总结

本文档总结了 Cyime 应用中已实现的、基于 OAuth2/OIDC 的统一认证系统的架构与核心流程。

## 1. 核心概念

本系统采用业界推荐的最佳实践，以确保安全性、健壮性和良好的用户体验。

### 1.1. 双令牌模式 (Dual-Token Model)

-   **Access Token (访问令牌)**:
    -   **类型**: 短时效 (默认为15分钟，可配置) 的 JWT。
    -   **用途**: 用于访问受保护的后端 API (如 `/api/v1/user/me`)。
    -   **存储**: 仅存于前端的**内存**中，安全性较高，随页面刷新而消失。

-   **Refresh Token (刷新令牌)**:
    -   **类型**: 长时效 (默认为30天，可配置) 的高熵随机字符串。
    -   **用途**: 专门用于安全地换取新的 Access Token。
    -   **存储**: 仅存于一个**安全的、`HttpOnly` 的 Cookie** 中，JavaScript 无法读取，能有效防止 XSS 攻击。

### 1.2. 混合式令牌刷新策略 (Hybrid Token Refresh)

为了兼顾用户体验与系统可靠性，我们采用了主动与被动相结合的混合刷新策略。

-   **主动刷新 (Proactive)**: 前端通过定时器，在 Access Token 过期前就自动请求新的令牌，为活跃用户提供无缝体验。
-   **被动刷新 (Reactive)**: 前端通过一个全局的 `apiFetch` 工具，在 API 请求因令牌过期而返回 401 错误时，自动触发刷新流程并重试请求。此策略作为安全网，处理电脑休眠、网络断开等边缘情况。

### 1.3. 刷新令牌旋转 (Refresh Token Rotation)

为了最大化安全，后端在每次使用 Refresh Token 后，都会使其失效并签发一个新的 Refresh Token，有效降低了令牌被盗用后的风险。

## 2. 核心认证流程

1.  **触发登录**: 前端从 `/api/v1/auth/config` 获取提供商列表，用户点击后被重定向至 `/api/v1/auth/login/:provider`，然后跳转到第三方认证页面。
2.  **后端回调**: 用户授权后，浏览器被重定向回 `/api/v1/auth/callback/:provider`。
3.  **令牌签发**: 后端用 `code` 交换令牌，获取用户信息，执行“查找或创建用户”的数据库操作，然后签发新的 Access Token 和 Refresh Token。
4.  **安全交付**: 后端将 Refresh Token 放入 `HttpOnly` Cookie，然后重定向回前端 `.../auth/callback` 页面，并将 Access Token 作为 URL 片段 (`#token=...`) 传递。
5.  **前端处理**:
    -   `auth/callback` 页面获取 Access Token。
    -   调用 `auth.loginAndFetchUser` 方法，该方法使用此 Token 请求 `/api/v1/user/me` 接口获取用户信息。
    -   获取成功后，将 Token 和用户信息存入全局 `auth` Store，并安排下一次的主动刷新。
    -   最后，页面跳转至 `/workspace`。
6.  **路由保护**: `workspace/+layout.svelte` 路由守卫检查 `auth` Store 的状态，确认用户已登录，允许页面渲染。
7.  **退出登录**: 用户点击“退出”按钮，调用 `auth.logout()`，清空 Store 状态，路由守卫自动将用户重定向回登录页。

## 3. API 端点总结

### /api/v1/auth

#### GET /api/v1/auth/config
- **描述**: 获取所有已激活且可用的认证提供商列表，用于在前端登录页面动态渲染登录按钮。
- **方法**: `GET`
- **成功响应 (Code 200)**:
    - **内容**: `{"providers": [{"name": "github", "icon": "...", "ssoUrl": "..."}]}`

#### GET /api/v1/auth/login/:provider
- **描述**: 重定向用户到指定的认证提供商的登录页面，开始授权流程。
- **方法**: `GET`
- **URL 参数**: `provider` (string) - 认证提供商的名称。
- **成功响应**: `307 Temporary Redirect`

#### GET /api/v1/auth/callback/:provider
- **描述**: 处理第三方认证成功后的回调。交换授权码，签发并交付双令牌。
- **方法**: `GET`
- **查询参数**: `code`, `state`
- **成功响应**: 重定向至前端回调URL，并在 Cookie 中设置 `cyime_refresh_token`。

#### POST /api/v1/auth/refresh
- **描述**: 使用有效的 `cyime_refresh_token` Cookie 来获取一个新的 Access Token（实现了令牌旋转）。
- **方法**: `POST`
- **成功响应 (Code 200)**: `{"accessToken": "<新JWT>"}`
- **错误响应**: `401 Unauthorized`

### /api/v1/user

#### GET /api/v1/user/me
- **描述**: 获取当前已登录用户的个人资料。
- **方法**: `GET`
- **请求**: **Headers**: `Authorization: Bearer <accessToken>`
- **成功响应 (Code 200)**: `{"id": "...", "email": "...", "displayName": "...", "avatarUrl": "..."}`
- **错误响应**: `401 Unauthorized`

## 4. 前端核心实现

-   **`src/lib/stores/auth.ts`**: 核心状态管理器，封装了登录、退出、令牌刷新（主动+被动）、状态存储等所有认证逻辑。
-   **`src/lib/api.ts`**: 全局 `apiFetch` 工具，封装了原生 `fetch`，自动处理 401 错误和请求重试。
-   **`src/routes/auth/callback/+page.svelte`**: 负责处理回调、调用 Store 的登录方法。
-   **`src/routes/workspace/+layout.svelte`**: 路由守卫，保护工作区页面。

## 5. 配置

系统通过以下环境变量进行配置：
- `JWT_SECRET_KEY`
- `REFRESH_TOKEN_LIFETIME_HOURS`
- `ACCESS_TOKEN_LIFETIME_MINUTES`
- `FRONTEND_CALLBACK_URL`
- `PUBLIC_BASE_URL` / `FRONTEND_BASE_URL`
- `API_BASE_URL` / `PUBLIC_API_BASE_URL`
- `CORS_ALLOWED_ORIGINS`
- `CYIME_SKILL_OAUTH_REDIRECT_URIS`

Skill OAuth 需要前端与后端公网地址都配置正确：未登录授权请求会跳转到 `${PUBLIC_BASE_URL}/login`，登录后会进入前端渲染的授权确认页；只有用户确认后，后端才会生成 authorization code。授权回调和 return target 会使用后端公网地址。生产环境中的 HTTPS `redirect_uri` 必须写入 `CYIME_SKILL_OAUTH_REDIRECT_URIS`；本地 loopback 与 custom scheme 默认允许。
