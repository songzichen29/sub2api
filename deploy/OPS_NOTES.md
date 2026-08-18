# 运维 / 反检测运维笔记 (Ops Notes)

记录与反检测相关的运维决策与 IP 信誉状态，供后续运维参考。

## 出口 IP 信誉（192.220.50.182）

最近一次核查（2026-06-30）：

| 项目 | 结果 |
|------|------|
| 地理位置 | Los Angeles, US ✅ |
| ISP / ASN | NTT America / AS2914 ✅ |
| hosting 标记（数据中心） | false ✅ |
| proxy 标记 | false ✅ |

**结论**：当前出口 IP 信誉良好，未被标记为数据中心/代理，**暂不需要住宅代理**。

### 后续建议
- 定期复检 IP 信誉（建议每月或被 Anthropic 风控时）。
- 若该 IP 后续被标记为 hosting/proxy，或出现 `Third-party apps now draw from your extra usage` 类降级：
  - 优先为**每个 OAuth 账号配置独立的住宅代理**（账户编辑页 → 代理），避免多账号共用一个被标记的出口 IP 导致关联。
  - 启用账号级 TLS 指纹 + 会话 ID 伪装（账户编辑页「反检测」分组）。

## 时区

- 容器默认使用 `Asia/Shanghai`（见 `backend/Dockerfile` 的 `ENV TZ` 与 `deploy/docker-compose*.yml` 的 `TZ`），运行时可通过 `TZ` 覆盖。
- 遥测 `client_timestamp` 始终用 UTC（`time.Now().UTC()`），与真实 CC 一致；`deployment_environment` 不再泄露真实 hostname。

## 反检测开关（per-account，账户编辑页「反检测」分组）

| 开关 | extra 字段 | 默认 | 说明 |
|------|-----------|------|------|
| TLS 指纹选择 | `enable_tls_fingerprint` / `tls_fingerprint_profile_id` | 关 | uTLS Node.js 24.x 指纹（API 转发路径走 h2） |
| 会话 ID 伪装 | `session_id_masking_enabled` | 关 | 15 分钟内固定 `metadata.user_id` 的 session |
| CCH 签名 | `enable_cch_signing` | 开 | billing header cch 字段 xxHash64 签名 |
| 遥测模拟 | `enable_telemetry` | 开 | 向 `/api/event_logging/v2/batch` 上报 tengu_* 事件 |
| GrowthBook 实验代理 | `enable_growthbook_proxy` | 开 | 主动拉取实验并上报 GrowthbookExperimentEvent |

> cch 签名与遥测的全局默认亦可在系统设置中调整；账户级字段会覆盖全局。
