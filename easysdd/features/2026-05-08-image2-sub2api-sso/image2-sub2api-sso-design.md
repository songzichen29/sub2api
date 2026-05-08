---
doc_type: feature-design
feature: 2026-05-08-image2-sub2api-sso
status: approved
summary: 让 gpt-image-2-webui 复用 sub2api 登录态，并按 sub2api 用户隔离 image2 生成数据
tags: [auth, sso, image2, data-isolation, cross-project]
---

## 0. 术语约定

- **sub2api 登录态**：sub2api 前端登录后保存在浏览器 `localStorage` 的 `auth_token`，请求后端时作为 `Authorization: Bearer {token}` 使用。防冲突：仓库内已有 `auth_token`、`/api/v1/auth/me`、JWT 中间件语义，当前 feature 不重命名这些概念。证据：`frontend/src/stores/auth.ts:11`、`frontend/src/stores/auth.ts:103`、`backend/internal/server/routes/auth.go:182`、`backend/internal/handler/auth_handler.go:399`。
- **image2**：本方案中指 `D:\data\gpt-image-2-webui` 这个 Next.js 图片生成工作台，不指 sub2api 内的模型定价或图片模型元数据。防冲突：在 sub2api 代码中没有现成 `image2` 路由；gpt-image 项目 README/代码中以 `generated-images`、`openaiImageHistory`、`ImageDB` 表示图片产物。
- **Image2 Session**：gpt-image 服务端基于 sub2api `auth/me` 校验结果签发的短期站内会话 Cookie，用于保护 image2 页面和 API。防冲突：不复用 sub2api JWT 名称，避免误以为 gpt-image 自己签发 sub2api 访问令牌。
- **用户隔离域**：以 sub2api `user.id` 为唯一分区键，隔离 gpt-image 的历史元数据、浏览器图片缓存、服务端文件存储和删除/读取 API。防冲突：不同于 sub2api 的用户分组、订阅或并发额度，只做 image2 数据归属隔离。
- **入口跳转**：未登录访问 image2 时跳到 sub2api `/login`；登录后通过 sub2api 中转入口把 token 传给 image2 完成会话交换。防冲突：不改 sub2api 现有 OAuth / 2FA 登录流程，只复用登录后的 `redirect` 行为。证据：`frontend/src/router/index.ts:40`、`frontend/src/views/auth/LoginView.vue:363`。

术语 grep 结论：

- `auth` / `login` / `token` 已是 sub2api 核心认证术语，本方案只新增 `image2` 与 `Image2 Session`，避免覆盖既有 JWT / OAuth / API Key 语义。
- sub2api 已有 `CustomPageView` 会对 iframe URL 追加 `user_id`、`token`、`theme`、`lang`、`ui_mode=embedded`，可作为登录后 token 交接入口的现有模式。证据：`frontend/src/views/user/CustomPageView.vue:105`、`frontend/src/utils/embedded-url.ts:16`、`frontend/src/utils/embedded-url.ts:30`。
- gpt-image 当前 `APP_PASSWORD` 是独立密码保护，不具备 sub2api 用户身份；历史和缓存 key 是全局固定 key。证据：`D:/data/gpt-image-2-webui/src/app/api/images/route.ts:304`、`D:/data/gpt-image-2-webui/src/hooks/use-home-history.ts:5`、`D:/data/gpt-image-2-webui/src/lib/db.ts:12`。

## 1. 决策与约束

### 需求摘要

做什么：

1. `gpt-image-2-webui` 未认证访问时跳转到 sub2api 登录。
2. sub2api 已登录用户访问 image2 时无需再输入 gpt-image 的 `APP_PASSWORD`，能够直接进入图片生成页面。
3. image2 中生成的历史记录、浏览器缓存图片、服务端文件图片、图片读取和删除必须按 sub2api 用户隔离。

为谁做：sub2api 的普通用户和管理员；管理员同样作为一个 sub2api 用户拥有自己的 image2 数据分区。

成功标准：

- 未登录浏览器打开 image2 首页，最终进入 sub2api `/login?redirect=...`，登录后回到 image2 并可生成图片。
- 已登录 sub2api 的浏览器从 sub2api 的 image2 入口或直接访问 image2，最终不再看到 gpt-image 密码弹窗，能直接使用。
- 用户 A 生成的 history / IndexedDB 缓存 / `generated-images` 文件，用户 B 登录后不可在列表、详情、图片 URL、删除 API 中读取或删除。
- image2 API 在缺少或无效 Image2 Session 时返回 401，不调用 OpenAI 兼容接口。

明确不做：

- 不把 sub2api JWT 长期持久化到 gpt-image 的 `localStorage`；只在交换阶段短暂使用 URL/query 中的 token。
- 不在 gpt-image 中实现独立注册、密码登录、用户表或权限后台。
- 不改 sub2api 用户注册、OAuth、2FA、refresh token 签发逻辑。
- 不把 image2 生成历史同步回 sub2api 数据库；本次只做 gpt-image 项目内部隔离。
- 不处理已有未隔离历史/图片的自动归属迁移；旧数据默认视为未归属，隔离模式下不展示给任何已登录用户。

### 关键决策

1. **认证权威放在 sub2api，gpt-image 只做会话消费方。**
   gpt-image 通过调用 sub2api `GET /api/v1/auth/me` 校验用户身份，因为该接口已经挂在 JWT 中间件之后，能复用账号激活、TokenVersion 撤销等现有判断。证据：`backend/internal/server/middleware/jwt_auth.go` 校验 token 后设置用户上下文，`backend/internal/server/routes/auth.go:182` 暴露 `/auth/me`。

2. **token 交接使用一次性“交换后清 URL”流程。**
   sub2api 现有自定义 iframe 已会把 `token` 拼给嵌入页；gpt-image 首屏读取 `token` query 后立即 POST 到自己的 `/api/sub2api-session/exchange`，服务端校验 sub2api 后写入 `HttpOnly` Image2 Session Cookie，然后前端 `history.replaceState` 移除 URL 中的 `token`。这样能复用现有入口，又避免把 sub2api token 长期留在 gpt-image 存储里。

3. **数据隔离一律以 sub2api `user.id` 为分区键。**
   服务端文件路径改为 `generated-images/{userId}/{filename}`；图片读取 `/api/image/{filename}` 和删除 `/api/image-delete` 都从当前 Image2 Session 取 `userId`，不接受客户端传入的 userId 作为授权依据。

4. **gpt-image 的 `APP_PASSWORD` 被 sub2api SSO 取代。**
   进入 SSO 模式后，`APP_PASSWORD` 不再作为 image2 主认证条件；保留代码兼容非 SSO 部署，但当 `SUB2API_BASE_URL` / `SUB2API_IMAGE2_ENTRY_URL` 配置存在时优先走 sub2api SSO。

5. **直接访问 image2 的回跳地址指向 sub2api 的 image2 入口，不直接指向 image2 自己。**
   原因：如果 image2 与 sub2api 不同 origin，image2 无法读取 sub2api 的 `localStorage auth_token`；必须让 sub2api 前端在已登录上下文中重新打开/嵌入 image2，并追加 token。已确认默认入口复用 sub2api 现有自定义菜单 `/custom/image2`；本 feature 不新增 sub2api `/image2` 专用路由；同时让已登录用户访问 `/login?redirect=/custom/image2` 时优先回到 redirect，保证直接访问 image2 也能回到自定义菜单入口。

### 被拒方案

- **让 gpt-image 直接读取 sub2api localStorage。** 不可行：不同 origin 无法读取；同 origin 也会把两个项目的存储强耦合。
- **把 sub2api JWT 放进 gpt-image localStorage 后长期使用。** 被拒：扩大 token 暴露面，退出/撤销处理也更难做干净。
- **只按客户端 history key 隔离，不保护服务端 API。** 被拒：用户仍可猜测 `/api/image/{filename}` 或调用删除 API，服务端文件会泄露。
- **把所有 image2 数据搬进 sub2api 数据库。** 被拒：范围过大，且用户当前只要求隔离，不要求跨设备同步。

### 主流程概述

正常路径：

1. 用户访问 sub2api 登录页完成登录，sub2api 前端保存 `auth_token`。
2. 用户打开 sub2api 的 image2 入口，`buildEmbeddedUrl` 把 `token={auth_token}`、`user_id`、主题和语言拼到 image2 URL。
3. gpt-image 首屏检测 URL 中的 `token`，调用 `/api/sub2api-session/exchange`。
4. gpt-image 服务端用 `Authorization: Bearer {token}` 请求 `${SUB2API_BASE_URL}/api/v1/auth/me`。
5. 校验成功后写入 Image2 Session Cookie，返回 `{ authenticated: true, user }`，前端清理 URL token 并加载工作台。
6. 生成、读取、删除图片时，gpt-image API 从 Image2 Session 取 `userId`，只访问该用户分区。

异常/边界：

- 无 token 且无 Image2 Session：页面跳转到 `${SUB2API_LOGIN_URL}?redirect=${encodeURIComponent(SUB2API_IMAGE2_ENTRY_URL)}`；API 返回 401。
- token 无效、过期、用户禁用：exchange 返回 401，清掉 Image2 Session Cookie 并跳登录。
- sub2api `/auth/me` 网络失败：exchange 返回 502，不创建会话，前端显示可重试错误。
- 用户切换账号：新 token exchange 后覆盖旧 Image2 Session；客户端 history/DB 使用新的 userId namespace，不混用旧账号数据。
- iframe 场景 Cookie：如果跨站 iframe 被浏览器阻断第三方 Cookie，需要部署为同站点路径/子域，或配置 `IMAGE2_COOKIE_SAMESITE=None` + HTTPS；否则推荐从 sub2api “新窗口打开”进入 image2。

## 2. 接口契约

### 2.1 gpt-image 新增：交换 sub2api token

`POST /api/sub2api-session/exchange`

请求：

```json
{
  "token": "sub2api-access-token",
  "source": "embedded"
}
```

成功响应：

```json
{
  "authenticated": true,
  "user": {
    "id": 31,
    "email": "me@example.com",
    "username": "linuxdo-handle",
    "role": "user"
  }
}
```

主要错误：

```json
// 401: token 缺失、无效、过期、用户不可用
{
  "authenticated": false,
  "error": "SUB2API_TOKEN_INVALID",
  "loginUrl": "https://sub2api.example/login?redirect=..."
}
```

```json
// 502: sub2api auth/me 不可达或响应异常
{
  "authenticated": false,
  "error": "SUB2API_AUTH_UNAVAILABLE"
}
```

来源：新增于 `D:/data/gpt-image-2-webui/src/app/api/sub2api-session/exchange/route.ts`；校验目标复用 `backend/internal/server/routes/auth.go:182` 与 `backend/internal/handler/auth_handler.go:399`。

### 2.2 gpt-image 变更：认证状态

`GET /api/auth-status`

SSO 模式成功响应：

```json
{
  "authMode": "sub2api",
  "authenticated": true,
  "user": {
    "id": 31,
    "email": "me@example.com",
    "username": "linuxdo-handle",
    "role": "user"
  },
  "configuredBaseUrl": "https://api.example/v1",
  "keysUrl": "https://api.example/keys"
}
```

未认证响应：

```json
{
  "authMode": "sub2api",
  "authenticated": false,
  "loginUrl": "https://sub2api.example/login?redirect=...",
  "configuredBaseUrl": "https://api.example/v1",
  "keysUrl": "https://api.example/keys"
}
```

兼容响应：未配置 SSO 时保留现有 `passwordRequired` / `APP_PASSWORD` 行为。

来源：变更 `D:/data/gpt-image-2-webui/src/app/api/auth-status/route.ts:3`，替换/扩展 `D:/data/gpt-image-2-webui/src/hooks/use-home-auth.ts:20` 消费逻辑。

### 2.3 gpt-image 变更：图片生成 API

`POST /api/images`

新增约束：

- SSO 模式下必须存在有效 Image2 Session Cookie。
- 服务端忽略客户端传来的 `user_id`，只使用 session 中的 `user.id`。
- fs 模式保存路径从 `generated-images/{filename}` 改为 `generated-images/{userId}/{filename}`。

成功响应示例不改变字段形状：

```json
{
  "images": [
    {
      "filename": "1777474614523-0.png",
      "path": "/api/image/1777474614523-0.png",
      "output_format": "png"
    }
  ],
  "usage": { "total_tokens": 123 }
}
```

主要错误：

```json
// 401
{
  "error": "Unauthorized: image2 session required.",
  "loginUrl": "https://sub2api.example/login?redirect=..."
}
```

来源：变更 `D:/data/gpt-image-2-webui/src/app/api/images/route.ts:277`，现有 APP_PASSWORD 校验在 `D:/data/gpt-image-2-webui/src/app/api/images/route.ts:304`。

### 2.4 gpt-image 变更：图片读取与删除

`GET /api/image/{filename}`：

- SSO 模式下必须有 Image2 Session。
- 实际读取 `generated-images/{session.userId}/{filename}`。
- `filename` 仍禁止 `..`、`/`、`\`。

`POST /api/image-delete`：

请求仍为：

```json
{
  "filenames": ["1777474614523-0.png"]
}
```

删除结果只作用于当前用户目录：

```json
{
  "message": "All files deleted successfully.",
  "results": [
    { "filename": "1777474614523-0.png", "success": true }
  ]
}
```

来源：变更 `D:/data/gpt-image-2-webui/src/app/api/image/[filename]/route.ts:9` 与 `D:/data/gpt-image-2-webui/src/app/api/image-delete/route.ts:23`。

### 2.5 gpt-image 前端状态与存储契约

新增 `Image2User` 状态：

```ts
type Image2User = {
  id: number;
  email?: string;
  username?: string;
  role?: 'admin' | 'user' | string;
};
```

历史 key：

```ts
// 旧：openaiImageHistory
// 新：openaiImageHistory:${user.id}
```

Dexie schema：

```ts
interface ImageRecord {
  userId: number;
  filename: string;
  blob: Blob;
}

// 建议 version(2).stores({ images: '&[userId+filename], userId, filename' })
```

来源：变更 `D:/data/gpt-image-2-webui/src/hooks/use-home-history.ts:5`、`D:/data/gpt-image-2-webui/src/lib/db.ts:8`、`D:/data/gpt-image-2-webui/src/app/page.tsx:425`。

### 2.6 sub2api 入口契约

最小实现不新增 sub2api API，优先复用当前自定义菜单/iframe 入口：

```json
{
  "id": "image2",
  "label": "Image2",
  "url": "https://image2.example/",
  "visibility": "user",
  "sort_order": 10
}
```

sub2api 渲染 `/custom/image2` 时，实际 iframe URL 会变成：

```text
https://image2.example/?user_id=31&token={sub2api_access_token}&theme=dark&lang=zh&ui_mode=embedded&src_host=https%3A%2F%2Fsub2api.example&src_url=...
```

来源：`frontend/src/views/user/CustomPageView.vue:105`、`frontend/src/utils/embedded-url.ts:16`、`frontend/src/utils/embedded-url.ts:30`。

可选增强：如果用户希望地址固定为 sub2api `/image2`，再新增 Vue Router 路由 `/image2` 作为 `/custom/image2` 的专用别名；本方案默认先不做，避免引入新的设置项。

## 3. 实现提示

### 目标文件状况评估

- `D:/data/gpt-image-2-webui/src/app/api/images/route.ts` 约 847 行，已经包含 OpenAI 参数组装、流式响应、文件落盘、APP_PASSWORD 校验等多种职责；认证和用户目录逻辑不应继续堆进这个文件。建议先新增 `src/lib/server/sub2api-auth.ts` 与 `src/lib/server/image-storage.ts`，再在 route 中少量接入。
- `D:/data/gpt-image-2-webui/src/app/page.tsx` 约 1514 行，已经承担首页全部状态和交互；用户会话 bootstrap 建议放进独立 hook（如 `src/hooks/use-sub2api-session.ts`），页面只接收 `image2User` / `authReady` / `loginUrl`。
- sub2api 的 `CustomPageView.vue` 和 `embedded-url.ts` 职责清晰，最小方案无需改；只有选择 `/image2` 专用别名时才小改 router。

### 改动计划

1. 在 gpt-image 新增 server auth 模块：读取配置、验证 sub2api token、签发/校验 Image2 Session Cookie、构造 loginUrl。
2. 新增 `/api/sub2api-session/exchange`，完成 token -> Image2 Session 的交换，并清理无效 session。
3. 改造 `/api/auth-status` 与 `use-home-auth`：识别 SSO 模式、返回当前 user/loginUrl，前端负责无会话跳转。
4. 改造图片相关 API：`/api/images`、`/api/image/{filename}`、`/api/image-delete`、`/api/models` 在 SSO 模式下统一要求 Image2 Session。
5. 改造 gpt-image 客户端数据隔离：history localStorage key、Dexie schema、图片缓存读写、清空历史/删除历史均携带当前 userId。
6. 补齐 sub2api 入口说明；如确认需要固定 `/image2` 路由，再在 sub2api frontend 增加路由别名和导航入口。
7. 更新 gpt-image README / docker-compose 环境变量说明：`SUB2API_BASE_URL`、`SUB2API_LOGIN_URL`、`SUB2API_IMAGE2_ENTRY_URL`、`IMAGE2_SESSION_SECRET`、Cookie SameSite/Secure 配置。

### 实现风险与约束

- URL 中的 sub2api token 必须在交换成功或失败后从地址栏移除，不能写入 gpt-image localStorage。
- gpt-image 所有服务端读取/删除文件的地方必须从 session userId 拼目录，不能相信请求体/query 的 userId。
- Cookie 签名密钥 `IMAGE2_SESSION_SECRET` 必须来自环境变量；开发模式可 fallback 到随机密钥但要明确日志提示“重启后会话失效”，不能硬编码固定密钥。
- 跨站 iframe Cookie 受浏览器策略影响；已确认 sub2api 与 image2 是同一服务器上的不同二级域名，因此实现默认按跨子域 Cookie/iframe 场景处理：Image2 Session Cookie 支持 `SameSite=None; Secure` 配置，同时保留 top-level 打开 image2 的可靠 fallback。
- 旧未隔离数据不迁移，避免把旧匿名数据错误归到第一个登录用户。

### 推进顺序

1. **认证基础模块**：新增 gpt-image server auth helper 和单元测试，能用伪 sub2api `/auth/me` 响应完成 token 校验与 cookie 签名校验。退出信号：auth helper 测试覆盖成功、401、502、cookie 篡改。
2. **会话交换闭环**：新增 exchange API 和前端 bootstrap hook，访问带 `?token=` 的 image2 能换到 cookie 并清理 URL。退出信号：手工或测试验证 URL token 被移除，`/api/auth-status` 显示 authenticated。
3. **页面未登录跳转**：无 token/无 cookie 访问 image2 时跳 sub2api 登录；登录后通过 sub2api image2 入口回到 image2。退出信号：无登录态访问会进入 `/login?redirect=...`，已登录态入口能直达工作台。
4. **API 保护与 fs 用户目录**：图片生成、读取、删除、模型列表都接入 `requireImage2Session`；fs 保存到用户目录。退出信号：用户 A 的 `/api/image/{filename}` 在用户 B session 下返回 404/403，删除也只删 B 自己目录。
5. **客户端数据隔离**：history key 与 Dexie 图片表按 userId 分区，页面所有读写使用当前 userId。退出信号：同浏览器切换 A/B 账号，history 列表和 IndexedDB 缓存互不可见。
6. **入口和文档收尾**：补 README/docker-compose，明确不同二级域名部署时的环境变量、Cookie SameSite/Secure 和 sub2api custom menu 配置。退出信号：文档中能按 env + custom menu 配出完整链路。

### 测试设计

- 认证交换：
  - token 有效时 exchange 设置 HttpOnly Cookie，并返回 user。
  - token 无效/过期时返回 401 且清理 cookie。
  - sub2api 不可达时返回 502，不能创建 cookie。
- 页面跳转：
  - 无 session 打开 image2，生成的 loginUrl 带 `redirect=SUB2API_IMAGE2_ENTRY_URL`。
  - 带 `?token=` 打开 image2，交换成功后地址栏不再包含 token。
- API 保护：
  - SSO 模式缺 cookie 调 `/api/images`、`/api/models`、`/api/image-delete`、`/api/image/{filename}` 均不进入 OpenAI/文件逻辑。
  - 非 SSO 模式保留现有 APP_PASSWORD 行为。
- 数据隔离：
  - fs 模式：A 生成文件落在 `generated-images/{A}/`，B 读取同 filename 失败。
  - IndexedDB 模式：A/B 同浏览器同 origin 切换账号后，history 与 cached blobs 分区独立。
  - 删除：A 删除自己的 history 只删除 `generated-images/{A}/filename`，不影响 B 同名文件。
- 回归：
  - gpt-image `npm run lint` / `npm run build`。
  - sub2api 如果只配置 custom menu 不改代码，则无需跑前端构建；若新增 `/image2` 路由，则跑 `pnpm --dir frontend test:run` 中相关 router/custom page 测试或 `pnpm --dir frontend typecheck`。

## 4. 与项目级架构文档的关系

- sub2api 当前只有 `easysdd/architecture/frontend-model-marketplace.md`，没有总入口 `DESIGN.md`；本 feature 会引用但不强行补全 sub2api 架构中心。
- gpt-image 已有 `D:/data/gpt-image-2-webui/easysdd/architecture/DESIGN.md`，其中第 4 节写着“历史元数据保存在浏览器 localStorage、图片二进制优先缓存到 IndexedDB、服务端可选文件系统保存”。实现完成后需要补充：
  - sub2api 是 image2 的认证权威；
  - SSO 模式下 gpt-image 用 Image2 Session Cookie 表示已验证用户；
  - history / IndexedDB / generated-images 均按 sub2api userId 分区。
- 已确认本次复用 sub2api 自定义菜单入口，不新增 sub2api `/image2` 专用路由；sub2api 架构文档暂不需要新增路由说明。
