# 开发环境指南 (临时)

本文档旨在帮助开发者快速在本地搭建并运行 Cyime 的开发环境。

**注意**: 当前项目仍在完善中，本文描述的是一个临时的、以功能验证为目的的开发流程。

## 1. 环境要求
- Go (版本 >= 1.22)
- pnpm (用于前端)
- `sqlite3` 命令行工具 (用于手动操作数据库)

## 2. 后端设置与启动

后端服务负责所有 API 逻辑和数据库交互。

1. **进入后端目录**:
   ```bash
   cd packages/server
   ```
2. **下载依赖**:
   如果是第一次运行，或代码更新后，请执行：
   ```bash
   go mod tidy
   ```
3. **启动后端服务**:
   ```bash
   go run ./cmd/server/main.go
   ```
   服务将启动在 `http://localhost:8080`。

## 3. 数据库手动配置 (当前阶段核心)

由于我们采用了“完全数据库驱动”的灵活设计，代码中不包含任何写死的认证提供商。因此，在开发阶段，我们需要手动向数据库中添加至少一个提供商才能进行登录测试。

**步骤 1: 创建数据库文件**

首次启动后端服务时，它会自动在您的个人主目录下创建数据库文件。

1. 运行后端服务: `go run ./cmd/server/main.go`
2. 看到日志显示 "Starting server on port 8080..." 后，立即按 `Ctrl+C` 停止服务。
3. 此时，一个空的数据库文件已经创建在 `~/.cyimewrite/cyimewrite.db`。

**步骤 2: 手动插入认证提供商**

以下以添加 **GitHub** 作为 OAuth2 提供商为例。

1. **使用 `sqlite3` 打开数据库**:
   ```bash
   sqlite3 ~/.cyimewrite/cyimewrite.db
   ```

2. **使用初始化工具写入提供商配置**:
   不要直接向 `client_secret_encrypted` 字段插入明文 Client Secret。该字段必须由服务端使用 `APP_ENCRYPTION_KEY` 加密后保存。请运行初始化工具并按提示输入 Client ID / Client Secret：

   ```bash
   cd packages/server
   go run cmd/init/main.go
   ```

   初始化工具会自动加密 Client Secret；服务启动时也会把历史明文记录迁移为加密值。

3. **仅查看数据库**: 如需排查配置，可用 `sqlite3 ~/.cyimewrite/cyimewrite.db` 查看记录，但不要手工写入明文密钥。

## 4. 前端设置与启动

1. **进入前端目录**:
    ```bash
    cd packages/web
    ```
2. **安装依赖**:
    如果是第一次运行，请执行：
    ```bash
    pnpm install
    ```
3. **启动前端开发服务**:
    ```bash
    pnpm run dev
    ```
    服务通常会启动在 `http://localhost:5173`。

## 5. 端到端测试流程

1. 确保已按步骤3配置好数据库。
2. 在一个终端中，启动后端服务。
3. 在另一个终端中，启动前端服务。
4. 打开浏览器，访问前端地址 `http://localhost:5173/login`。
5. 点击页面上动态加载出的 "使用 github 登录" 按钮，完成授权流程。
6. 最终浏览器会跳转并显示一个包含您用户信息的 JSON 响应，代表整个后端认证流程成功。
