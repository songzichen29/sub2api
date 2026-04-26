# Sub2API 项目部署分析

## 1. 项目概览

- 仓库：`songzichen29/sub2api`
- 默认分支：`main`
- 定位：AI API Gateway / 订阅额度分发平台
- 后端技术栈：Go、Gin、Ent
- 前端技术栈：Vue 3、Vite、TailwindCSS
- 依赖中间件：PostgreSQL 15+、Redis 7+
- 官方支持的部署方式：
  - 二进制 + systemd
  - Docker Compose（官方推荐）
  - 源码构建

## 2. 官方部署方式要点

### 2.1 Docker Compose Local

- 包含 `sub2api + postgres + redis`
- 使用本地目录持久化：`data/`、`postgres_data/`、`redis_data/`
- 优点：迁移、备份简单，和宿主机现有环境耦合低
- 缺点：会额外起 2 个容器

### 2.2 Docker Compose Standalone

- 只启动 `sub2api`
- 需要外部提供 PostgreSQL 和 Redis
- 优点：适合复用现有中间件
- 缺点：前提是宿主机已有可用 PostgreSQL / Redis，且连接参数明确

### 2.3 二进制安装

- 依赖宿主机已安装 PostgreSQL、Redis
- 适合不想走 Docker 的环境
- 当前目标服务器已具备 Docker / Compose，因此不优先推荐

## 3. 目标服务器现状

### 3.1 基础资源

- 系统：Ubuntu 22.04 系列内核（5.15）
- CPU：4 核
- 内存：3.6 GiB
- 可用内存：约 1.9~2.0 GiB
- 磁盘：40G，总已用约 9.8G，可用约 28G
- Swap：无

### 3.2 已安装 / 已运行能力

- Docker：已安装并运行
- Docker Compose v2：已安装
- Python 3.10：已安装
- Nginx：未安装
- Caddy：未安装
- Node / pnpm / PM2：未安装
- SSH：可用

### 3.3 现有容器

- `rbac-admin-server`
  - 暴露：宿主机 `80 -> 容器 8080`，同时也占用宿主机 `8080`
  - 内存占用：约 341 MiB
- `minio`
  - 暴露：`19000`、`19001`
  - 内存占用：约 129 MiB
- `mysql`
  - 暴露：`13306 -> 3306`
  - 内存占用：约 454 MiB
- `videocraft-redis`
  - 暴露：`16379 -> 6379`
  - 内存占用：约 9 MiB

### 3.4 关键结论

1. 当前机器资源足够再部署一个轻量 Go 服务和少量中间件。
2. 当前宿主机 **没有 PostgreSQL**，因此不能直接按 Sub2API 的要求复用数据库层。
3. 当前已有一个 Redis，但它属于其他业务，理论上可复用，实践上不建议直接混用。
4. 宿主机 **80 端口已经被现有容器占用**，不能直接再让 Sub2API 对外占用 80。
5. 443 当前未见占用，但由于没有统一反向代理，正式生产前仍需设计入口方式。

## 4. 与当前服务器最匹配的部署建议

## 4.1 最推荐：Docker Compose，复用 Docker 体系，但单独为 Sub2API 建专属 PostgreSQL / Redis

推荐方式：使用官方 `docker-compose.local.yml` 思路，但端口改为非冲突端口，例如 `18080`。

### 原因

1. **和现有环境一致**：当前服务器已经以 Docker 为主，继续用 Compose 最自然。
2. **隔离性最好**：Sub2API 的 PostgreSQL、Redis 与现有业务隔离，避免相互影响。
3. **资源可接受**：
   - Go 应用通常不会像 Java 那样吃内存；
   - 新增一个 PostgreSQL 容器 + 一个 Redis 容器，在这台机器上可承受；
   - 当前仍有约 2G 可用内存，磁盘也较充足。
4. **迁移与备份方便**：官方 local 版直接用目录挂载，后续打包迁移简单。
5. **回滚简单**：Compose 独立目录管理，风险更可控。

### 适合的对外访问方式

- 短期测试：直接通过 `http://服务器IP:18080`
- 后续正式上线：
  - 方案 A：新增统一反向代理（Caddy/Nginx）接管 80/443
  - 方案 B：在云厂商 LB / CDN / 网关层做转发到 `18080`

## 4.2 次推荐：Docker Compose Standalone，复用现有 Redis，但 PostgreSQL 仍单独补一个

### 可行性

- Sub2API 官方 standalone 版支持外部 PostgreSQL / Redis。
- 你当前没有 PostgreSQL，因此即便走 standalone，也至少要补 PostgreSQL。

### 风险

- 复用现有 `videocraft-redis` 虽然技术上可行，但不建议：
  - 和现有业务耦合；
  - Redis 配置、淘汰策略、持久化策略可能互相影响；
  - 故障排查边界变模糊。

### 结论

- 如果你特别想少起一个容器，可以：
  - 新起 `postgres` 容器
  - 复用现有 Redis 的独立 DB index
- 但这不是最佳长期方案。

## 4.3 不推荐：源码构建 / 宿主机二进制直跑

### 原因

- 服务器没有 Node / pnpm 环境；源码构建要补依赖。
- 官方已经提供成熟 Compose 方案，没必要在当前机器上增加宿主机依赖复杂度。
- 直跑会让后续升级、迁移、回滚都不如容器方案简单。

## 5. 端口与入口建议

### 当前冲突情况

- `80`：已占用
- `8080`：已占用

### 建议端口

- Sub2API：`18080:8080`
- PostgreSQL：不对外暴露，容器内联通即可
- Redis：不对外暴露，容器内联通即可

### 为什么先不抢 80/443

- 你现在机器上已有一个对外 Web 服务占用 80。
- 在没有明确“是否愿意重构现有入口”的前提下，最安全做法是先让 Sub2API 跑在独立高位端口。

## 6. 资源角度的部署评估

### 结论

这台机器 **可以部署 Sub2API**，用于轻中等规模自用/小范围共享没有问题。

### 主要依据

- CPU 负载很低（load average 接近 0）
- 内存虽不算富余，但当前仍有约 2G available
- Docker 当前容器占用总体可控
- 磁盘余量 28G，对新增 1 个应用 + 1 个 PG + 1 个 Redis 足够

### 风险点

1. **没有 Swap**：如果后续并发增大、PostgreSQL 缓存上升、再叠加其他服务，可能触发 OOM。
2. **MySQL + Java + MinIO 已常驻**：后续如果现有业务流量上升，Sub2API 可用余量会下降。
3. **若做公开网关**：请求量、长连接、账单统计、上游重试都会增加资源波动。

### 建议补强

- 增加 2G Swap（至少降低 OOM 风险）
- PostgreSQL 适度收紧内存参数
- Redis 保持轻量即可
- 生产流量上来前，优先补统一反向代理与 HTTPS 入口

## 7. 实际部署建议（推荐落地方案）

### 推荐拓扑

- `sub2api` 容器
- `sub2api-postgres` 容器
- `sub2api-redis` 容器
- 宿主机对外暴露：`18080`
- 数据目录：独立目录持久化

### 推荐目录

```text
/opt/sub2api/
├── docker-compose.yml
├── .env
├── data/
├── postgres_data/
└── redis_data/
```

### 推荐策略

1. 先按 `docker-compose.local.yml` 部署
2. 将 `SERVER_PORT` 改成 `18080`
3. 首先用 IP + 端口验证功能
4. 验证没问题后，再决定是否接入域名、HTTPS、反向代理

## 8. 如果要正式对外提供服务

建议分两步：

### 第一步：先稳定跑起来

- 直接走 `18080`
- 完成初始化、管理员账号、数据库连接、Redis 连接验证

### 第二步：再接入正式入口

可选方案：

1. **新增 Caddy / Nginx 统一接管 80/443**
   - 适合你愿意顺手整理这台机器上的多个 Web 服务入口
2. **云负载均衡 / CDN / 反向代理转发到 18080**
   - 适合不想改动当前 80 端口占用的情况
3. **仅内网 / 小范围使用**
   - 直接端口访问即可

注意：如果后续使用 Nginx 反向代理，仓库 README 特别提到要开启：

```nginx
underscores_in_headers on;
```

否则某些带下划线的头会被丢弃，影响 sticky session。

## 9. 最终建议

### 推荐排序

1. **首选**：官方 Docker Compose 本地目录版，独立 PostgreSQL + 独立 Redis，映射到 `18080`
2. **备选**：Standalone + 独立 PostgreSQL + 复用现有 Redis
3. **不建议**：源码构建 / 宿主机直跑

### 一句话结论

对你这台服务器来说，**最合适的是继续沿用现有 Docker 体系，把 Sub2API 当成一套独立业务栈部署，先走 `18080` 非冲突端口，数据库和 Redis 尽量独立，不要直接混入现有 MySQL/Redis 业务。**

## 10. 下一步可执行动作

如果需要继续推进，可直接进入以下任一动作：

1. 生成适合你这台服务器的 `docker-compose.yml` 和 `.env` 模板
2. 直接在服务器上创建部署目录并落地部署
3. 继续分析如何接入你现有域名 / HTTPS / 反向代理
