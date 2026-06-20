## Context

当前支付系统架构：
- `PaymentConfig`（存于 settings 表）管理支付配置，包含充值倍率、手续费率、金额范围等
- `SubscriptionPlan` 有 `price` 和 `original_price` 两个字段，支持套餐级别的静态折扣展示
- `PromoCode` 为注册优惠码，用户注册时输入可获得赠送余额，与支付流程完全独立
- `PaymentOrder` 记录订单信息，金额字段有 `amount`（订单金额）和 `pay_amount`（实际支付金额，含手续费）
- 创建订单流程在 `PaymentService.CreateOrder` 中完成，金额计算路径清晰：校验 → 取基准金额 → 计算手续费 → 生成订单 → 调用支付网关
- 前端 `PaymentView.vue` 管理充值和订阅两种支付场景，价格展示简单（金额 + 手续费）

约束条件：
- 折扣计算必须在后端执行，前端仅做预览展示
- 需要与现有的手续费机制（`fee_rate`）和充值倍率（`balance_recharge_multiplier`）共存
- 优惠券使用需要事务级别的并发安全（行锁），与 PromoCode 的 ApplyPromoCode 模式一致
- 不能破坏现有订单创建流程的兼容性（无折扣/无优惠券时行为不变）

## Goals / Non-Goals

**Goals:**
- 支持管理员配置阶梯式满减折扣规则，订单创建时自动匹配最高档位
- 支持管理员创建支付优惠券，用户在支付时可输入优惠券码抵扣
- 满减和优惠券可叠加，计算顺序：先满减 → 再优惠券 → 最后手续费
- 提供实时价格预览接口，前端展示完整价格明细
- 退款时正确处理优惠券回退，退款金额基于实付金额
- 管理后台提供优惠券 CRUD 和满减规则配置能力

**Non-Goals:**
- 不改造现有注册优惠码（PromoCode）的功能或模型
- 不改变现有套餐的 `original_price` 静态折扣展示逻辑
- 不支持优惠券找零（券面值大于订单金额时不退差价）
- 不支持优惠券与其他促销活动的复杂组合规则（如互斥、优先级排序）
- 不做优惠券的自动推荐/最优组合计算（用户每次只能使用一张券）
- 不修改每日限额重置（daily_limit_reset）订单的计价逻辑

## Decisions

### Decision 1: 满减规则存储在 PaymentConfig 的 JSON 字段中

**选择**：在 `PaymentConfig` 中新增 `discount_rules` JSON 字段，存于 settings 表。

**替代方案**：
- 新建独立的 `discount_rules` 表 -- 规则数量有限（通常 < 10 条），单独建表增加复杂度但不带来明显收益
- 硬编码在配置文件中 -- 无法动态管理，每次修改需重启服务

**理由**：满减规则是全局配置的一部分，与现有的 `min_amount`、`fee_rate` 等配置项性质相同。存储在 PaymentConfig 中可以复用已有的配置读写机制，管理员可通过后台动态修改，无需数据库迁移。

**规则格式**：
```json
[
  { "threshold": 50, "type": "rate", "value": 0.95, "label": "满50打95折", "enabled": true },
  { "threshold": 100, "type": "rate", "value": 0.90, "label": "满100打9折", "enabled": true },
  { "threshold": 200, "type": "reduce", "value": 30, "label": "满200减30", "enabled": false }
]
```
支持 `rate`（折后支付比例）和 `reduce`（固定减免）两种类型混用。`enabled` 默认为 true；保存时按 `threshold` 升序规范化，计算和用户侧查询仅使用启用规则。校验只强制 `threshold` 唯一且递增、金额/比例合法，不尝试比较混合规则的“折扣力度”，避免不同金额点下结论不一致。

### Decision 2: 支付优惠券使用独立模型，不复用 PromoCode

**选择**：新建 `Coupon` 和 `CouponUsage` 实体。

**替代方案**：
- 扩展 PromoCode 模型增加支付优惠功能 -- PromoCode 的语义是"注册送余额"，字段（bonus_amount）和使用流程（注册时调用）都与支付优惠券差异很大，强行复用会导致模型混乱

**理由**：
- 职责清晰分离：PromoCode = 注册送余额，Coupon = 支付抵扣
- Coupon 需要独立的字段：优惠券类型（fixed/percent）、最低消费门槛、每用户限用次数、适用范围（balance/subscription/all）
- 独立模型便于各自演进，互不影响

### Decision 3: 折扣计算插入 CreateOrder 的金额计算链路

**选择**：在 `CreateOrder` 中，确定 `limitAmount`（基准金额）之后、调用 `calculateCreateOrderPayAmount` 之前，插入折扣计算步骤。

**计算流程**：
```
limitAmount (= plan.Price 或 req.Amount)
  → applyThresholdDiscount → after_discount, threshold_discount_amount
  → applyCoupon → after_coupon, coupon_discount_amount
  → calculateCreateOrderPayAmount(after_coupon, feeRate) → pay_amount
```

**理由**：这个位置在现有流程中是金额确定的第一个点，在此之前已经完成了订单类型校验和基准金额确定。插入折扣计算不改变上下游逻辑，只需修改 `limitAmount` → `after_coupon` 的传递。

### Decision 4: 优惠券使用与订单创建在同一事务中

**选择**：在 `createOrderInTx` 事务中完成优惠券验证、锁定、扣减和使用记录创建。

**理由**：确保优惠券使用和订单创建的原子性。如果分开事务，可能出现优惠券已扣减但订单创建失败的不一致状态。这与 PromoCode 的 `ApplyPromoCode` 使用事务 + 行锁（FOR UPDATE）的模式一致。

### Decision 5: 价格预览使用独立的只读接口

**选择**：新增 `POST /api/v1/payment/preview-price` 接口，接受金额、套餐ID、优惠券码等参数，返回完整价格明细。

**理由**：
- 前端需要在用户输入金额/优惠券码后实时展示价格变化，但不希望每次预览都创建订单
- 预览接口与创建订单使用相同的折扣计算逻辑（共享 `DiscountService`），确保预览结果与实际支付一致
- 预览接口是只读的，不产生副作用，不需要事务

### Decision 6: PaymentOrder 新增折扣字段而非复用现有字段

**选择**：`payment_orders` 表新增 `discount_amount`、`coupon_code`、`coupon_discount_amount` 三个字段。

**替代方案**：
- 将折扣信息存入 JSON 字段 -- 查询和统计不方便
- 修改 `amount` 字段的语义为折后金额 -- 破坏现有语义，影响退款和对账逻辑

**理由**：保留 `amount`（原始订单金额）和 `pay_amount`（实际支付金额含手续费）的现有语义不变，新增字段记录折扣明细，便于审计、统计和退款处理。退款时以 `pay_amount` 为基准。

### Decision 7: 退款时优惠券回退策略

**选择**：退款时将优惠券使用次数回退（`used_count--`），标记对应的 `coupon_usage` 记录为已回退。退款金额 = `pay_amount`（实付金额），而非 `amount`（原价）。

**理由**：用户实际支付的金额是折后金额，退款也应基于实际支付金额。优惠券回退后用户可再次使用该券。

### Decision 8: 百分比字段语义固定

**选择**：满减 `rate.value` 表示折后支付比例（如 `0.90` = 9 折，减免 10%）；优惠券 `percent.value` 表示减免比例（如 `0.20` = 减免 20%，即 8 折效果）。

**理由**：该语义与现有 proposal/spec 示例保持一致。实现和前端文案必须明确区分，避免将优惠券百分比误解为支付比例。

### Decision 9: 优惠券适用范围与 daily_limit_reset

**选择**：优惠券 `scope` 支持 `balance`、`subscription`、`all`。余额充值订单只允许 `balance/all`，订阅订单只允许 `subscription/all`，每日限额重置订单不适用满减和优惠券。

**错误码**：范围不匹配返回 `COUPON_SCOPE_MISMATCH`；每日限额重置订单携带优惠券返回 `COUPON_NOT_APPLICABLE`。

### Decision 10: 预览与创建不一致时以创建时重新计算为准

**选择**：价格预览只提供即时参考，不生成锁定价格或有效期 token。创建订单时必须重新读取满减规则和优惠券状态并重新计算；如果期间规则变化，订单按最新计算结果创建，响应返回实际金额。

**理由**：避免引入价格快照 token、缓存和过期校验复杂度。前端应在创建订单响应中展示最终金额。

### Decision 11: 多币种与精度

**选择**：满减门槛和优惠券面值均以订单业务金额所在货币解释；当前配置为全局规则，不为不同 provider currency 维护独立规则。折扣计算先用 decimal 按业务金额计算，折后金额再交给现有 `calculateCreateOrderPayAmount` 按支付货币精度处理手续费和最终支付金额。零小数货币仍由现有货币精度校验决定是否允许该金额。

### Decision 12: 优惠券删除策略

**选择**：管理端删除优惠券采用“停用/归档式删除”（设置 status=disabled 或 archived），不硬删除已使用记录。历史订单通过 `coupon_code` 和 `coupon_usage` 保持审计可追溯。

### Decision 13: 退款金额口径

**选择**：新增折扣后，普通余额订单的最大可退金额以 `pay_amount` 为上限；网关退款金额直接使用本次退款实付金额。余额扣减仍按实际退给用户的金额执行。订阅按比例退款以折后实付口径作为上限，避免超过用户实际支付金额。

**兼容说明**：无折扣订单保持现有行为等价：当 `amount` 与 `pay_amount` 仅因手续费不同而不一致时，退款上限改为实付金额会把手续费纳入可退范围，这是本变更的明确选择。

## Risks / Trade-offs

- **[金额精度风险]** 折扣计算涉及浮点运算，可能产生精度问题 → 所有金额字段使用 DECIMAL(20,2) 或 DECIMAL(20,8)，计算过程中间结果保留足够精度，最终结果四舍五入到分
- **[并发超发风险]** 限量优惠券在高并发下可能被超额使用 → 事务中使用 SELECT FOR UPDATE 锁定优惠券记录，与 PromoCode 验证模式一致
- **[满减与套餐原价冲突]** 套餐已有 `original_price` 展示静态折扣，满减在此基础上再折扣可能让管理员困惑 → 前端价格明细中清晰展示每一层折扣的来源和金额
- **[退款复杂化]** 退款逻辑需要区分原价和实付金额，还需要处理优惠券回退 → 退款流程增加专门的折扣处理分支，确保退款金额基于实付
- **[预览与实际不一致]** 用户预览后到实际支付期间，满减规则或优惠券状态可能变化 → 创建订单时重新计算并以最新结果为准，响应返回最终金额
- **[向后兼容]** 新增字段需要数据库迁移 → 所有新字段有默认值（0 或空字符串），现有数据无需回填，迁移可在线执行
