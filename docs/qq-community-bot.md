# QQ 群机器人（NapCat / OneBot v11）

本模块集成在 Sub2API 后端与管理面板。QQ 登录由独立 NapCat 负责；不使用 QQ 官方机器人 AppID / AppSecret。默认关闭机器人、签到和聊天。

## 配置

1. 更新 Sub2API 到包含此功能的版本。启动时执行对应数据库迁移：MySQL `106_qq_community_bot.sql` / PostgreSQL `248_qq_community_bot.sql`。
2. 管理员打开 **QQ 群机器人**，填写机器人 QQ、允许的群号及随机访问令牌（至少 24 字符）。仅允许白名单群调用。
3. 在 NapCat 登录该普通 QQ 账号。在网络配置中添加 **OneBot v11 反向 WebSocket**：
   - URL：`wss://你的站点域名/api/v1/qq-bot/onebot/ws`
   - 连接类型：Universal（同一连接传事件和调用 API）。
   - 消息格式：`array`，必须保留原生 `text` / `at` 消息段。
   - Token：与后台访问令牌相同，使用 `Authorization: Bearer ...` 传递。
   - 心跳：建议 30 秒；重连间隔建议 5 秒。
   - NapCat 需发送 `X-Self-ID` 和 `X-Client-Role: Universal`。
4. 保存并启用，后台查看连接状态。保存任何配置会断开当前连接，由 NapCat 自动重连。状态只反映当前 Sub2API 实例。

同一台服务器可让 NapCat 连接 `ws://宿主机地址:8080/api/v1/qq-bot/onebot/ws`；容器中的 `127.0.0.1` 指容器自身，不能直接指向另一个容器。不要为连接机器人公开数据库、Redis 或 NapCat 管理端口。

若使用 Nginx，WebSocket 路径须转发 Upgrade/Connection，读超时应超过心跳间隔，例如在对应站点内增加：

```nginx
location = /api/v1/qq-bot/onebot/ws {
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_read_timeout 120s;
}
```

QQ 登录及实际群消息收发需要账号持有者扫码与群成员身份。自动测试用模拟 OneBot 验证协议，不代表已登录真实 QQ。

## 用户绑定与指令

1. 登录网站 → 个人资料 → QQ 账号绑定 → 生成绑定码。
2. 在白名单群发送 `/绑定 绑定码`，绑定码五分钟有效，仅能提交一次。
3. 回网站刷新，核对显示的 QQ，点击“确认绑定此 QQ”。未完成确认的 QQ 不能查余额、领签到或使用聊天。
4. 用户在网站解除绑定；管理员也可在绑定列表解除。QQ 端不能替他人绑定、解绑或设置管理员。

| 群内指令 | 功能 |
| --- | --- |
| `/帮助` | 查看指令 |
| `/余额` | 查询发送者所绑定的账号余额 |
| `/签到` | 领取当日奖励 |
| `/渠道` | 查询本人可用分组近一小时聚合状态 |
| `/聊天 问题` | 使用站点专用 Key 调用模型 |
| `@机器人 问题` | 同上 |
| `/清空对话` | 清除本人在当前群的模型历史 |

余额和签到结果在群内回复，其他群成员可见。请勿在群内发送站点密码、登录令牌或 API Key。

## 签到与模型费用

- 签到奖励以美元余额计，下限 `checkin_reward` 为 `0` 时关闭，上限 `checkin_reward_max` 为 `0` 时固定发放下限金额；否则在上下限内随机发放，允许范围 `0～100`，最多 8 位小数。例如下限 `0.05`、上限 `0.10`。金额在余额和流水的同一事务中生成并保存，重复签到回显原领取金额，不会重抽或再入账。
- 以北京时间自然日和站点 `user_id` 去重，更换 QQ、换群、重投事件均不能再次领取。
- 余额修改和签到流水在同一事务，失败同时回滚。提交后清除余额及 API Key 鉴权缓存。
- 模型聊天使用**本站专用 API Key**，由该 Key 所属账号承担费用。建议单独建立机器人运营账号和 Key，设置其消费限额；绑定成员不会因机器人使用专用 Key 而被额外扣款。
- 模型只调用固定本机网关 `/v1/chat/completions`，与其他 API 调用使用同样的鉴权、路由和计费。
- 保存前可点击“测试模型”，实际发起一次最多 32 个输出 Token 的请求，因此会消耗该 Key 的额度。请填写该 Key 所属分组支持的准确模型名。
- 每人每日次数按站点账号统计，全站次数共用；请求开始即消耗一次，失败不退还、不自动重发。另有限制最大输出、历史长度与冷却时间。
- 历史按 QQ + 群隔离；同一会话在处理中不能同时生成或清空，最长租约三分钟。解绑会删除原 QQ 的聊天历史。
- 模型无余额修改、转账或管理工具。自然语言请求不会发放奖励。

## 记录与排错

后台提供绑定、签到流水和指令回复记录，不记录完整用户提问或绑定码。签到流水永久保留，避免清理后重复入账。

- `success`：业务成功且 OneBot 已确认接受发送请求。
- `failed`：业务拒绝或失败；回复中的错误经过脱敏。
- `send_failed`：写连接失败、OneBot 拒绝发送或 15 秒内未确认。已完成的签到不会因此撤销或重复发放。
- `cancelled`：发送前发现配置关闭或变更，取消回复。
- `processing`：处理中；进程意外终止时可能保留该状态，不自动重放以避免重复调用。

同一消息由机器人 QQ、群号和消息 ID 生成摘要，在数据库去重。模型请求和资金操作不依赖进程内去重。

频道状态沿用当前绑定用户的可用分组授权，仅输出分组名、平台、成功率与首字延迟；不会输出上游账号、密钥、原始报错或其他人的余额。需要开启 V2 渠道监控并有近一小时的样本。

访问令牌和模型 Key 使用站点现有加密器落库。GET 配置不回传明文；留空保存保留原凭据，清除必须显式勾选。机器人配置和测试请求的正文不写入审计记录。

单实例只接受一条 Universal 连接。多实例部署请将此 WS 路径固定到一个实例，否则后台当前实例的连接显示可能与实际接入实例不同。数据库行锁、每日奖励唯一约束、用量配额及会话租约仍跨实例生效。

## 验证与回滚

- 后端：`go test ./internal/qqbot ./internal/repository ./internal/handler ./internal/service -run TestQQBot -count=1`。
- 前端：`pnpm build`（含中英文完整性检查、TypeScript 检查与生产构建）。
- SQL 单元测试覆盖两种数据库的占位符与配置加锁、重复签到、余额更新失败回滚、绑定确认及聊天额度。`go test -tags integration ./internal/qqbot/integration -count=1 -v` 使用独立 Docker 测试容器，覆盖真实 MySQL 绑定、16 个并发签到、跨群和更换 QQ 后的每日限制；本地没有 Docker 时会跳过，不能将跳过解释为通过真实数据库测试。
- 回滚到上一镜像前先关闭机器人；保留新增表和已发放签到流水，不反向扣回已发余额。新增表不会改变现有网关调用路径。

参考了 [DeanZFC/sub2api-custom](https://github.com/DeanZFC/sub2api-custom) 的功能设计；本实现为 NapCat / OneBot 适配，未复制其 PostgreSQL 专用机器人实现。

## 开发交接

- 开发分支：`codex/add-qq-community-bot`，基于生产分支 `merge/upstream-main-into-pgsql-mysql-20260426`。
- 后台地址：`/admin/qq-bot`；用户入口：个人资料的 QQ 账号绑定卡片。
- 已通过后端 QQBot 定向测试与服务器装配/清理测试、7 项前端组件与资料页回归测试、前端 ESLint 和 TypeScript 检查；两份 SQL 已核对无 UTF-8 BOM。
- 独立 MySQL 集成测试已在新机的隔离 Docker 容器执行通过：真实迁移与双向绑定、16 路并发随机签到仅一笔入账、流水与最终余额一致、换群和更换 QQ 后不能重复领取且回显原奖励。本地无 Docker，不能在本地执行这一项。仓库原有 `-tags integration ./internal/repository` 还存在与本功能无关的旧接口编译错误，本次未扩展修改。
- 当前 Windows Go 环境未启用 CGO，不能执行 `-race`，需在具备 CGO 的环境补跑。
- 发布遵循源码 → GitHub 生产分支构建 → 新服务器拉取对应提交镜像并固定 digest 的流程。线上版本、NapCat 登录及实群验收状态以新机 `/data/ops/qq-bot/` 运维交接和发布记录为准，自动测试不代表真实群验收完成。
