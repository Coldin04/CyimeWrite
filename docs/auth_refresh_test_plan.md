# 认证刷新测试流程

这份流程用于验证编辑文章时 Access Token 过期、Refresh Token 轮转、路由守卫跳转之间不会互相误伤。

## 本地短周期配置

在后端运行环境中临时设置：

```bash
ACCESS_TOKEN_LIFETIME_SECONDS=20
REFRESH_TOKEN_LIFETIME_HOURS=1
```

`ACCESS_TOKEN_LIFETIME_SECONDS` 只建议本地测试使用；它会覆盖 `ACCESS_TOKEN_LIFETIME_MINUTES`。测试完成后删除该配置，恢复默认 15 分钟或生产配置。

## 单标签页编辑续期

1. 启动后端和前端。
2. 登录并打开任意文章编辑页。
3. 在文章中连续输入内容，至少等待 60 秒。
4. 打开浏览器 Network，观察 `/api/v1/auth/refresh` 大约每 17 秒成功一次。
5. 预期结果：页面始终停留在编辑页，不跳 `/login`；自动保存和普通 API 请求继续成功。

## 401 被动刷新

1. 登录并打开文章编辑页。
2. 等待当前 Access Token 过期后，再触发一次保存、图片上传或其他需要鉴权的请求。
3. 预期结果：首次业务请求如果返回 401，前端会调用 `/api/v1/auth/refresh`，随后重试原请求；页面不跳 `/login`。

## 多标签页并发刷新

1. 同一账号打开两个文章编辑页标签。
2. 两个标签页都保持激活或交替激活，等待 60 秒以上。
3. 观察两个标签页的 Network。
4. 预期结果：同一时刻只有一个标签页执行 refresh 关键区；两个标签页都不会因为 Refresh Token 轮转竞态而跳 `/login`。

## 临时网络失败

1. 登录并打开文章编辑页。
2. 在 refresh 即将触发前，通过 DevTools Network 切到 Offline，保持 5-10 秒后恢复 Online。
3. 预期结果：临时失败不会立刻清空登录态；恢复网络后下一次刷新重试成功，页面仍在编辑页。

## 真正失效场景

1. 登录并打开文章编辑页。
2. 在后端或用户安全页撤销当前会话，或清除 `cyime_refresh_token`。
3. 等待下一次 refresh。
4. 预期结果：后端明确返回 401/403 后，前端清空登录态并由路由守卫跳转到 `/login`。
