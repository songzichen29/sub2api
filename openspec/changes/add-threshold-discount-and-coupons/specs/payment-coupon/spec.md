## ADDED Requirements

### Requirement: 优惠券创建与管理
系统 SHALL 支持管理员创建、编辑、停用支付优惠券。优惠券包含码值、类型（fixed 固定金额/percent 减免比例）、面值、最低消费门槛、最大折扣金额（百分比类型）、适用范围、总使用次数限制、每用户限用次数、状态、生效时间和过期时间。

#### Scenario: 创建固定金额优惠券
- **WHEN** 管理员创建优惠券 `{code: "SAVE10", type: "fixed", value: 10, min_amount: 50, max_uses: 100, per_user_limit: 1}`
- **THEN** 系统 SHALL 创建一张面值为 10 元的固定金额优惠券，最低消费 50 元，总量 100 张，每用户限用 1 次

#### Scenario: 创建百分比折扣优惠券
- **WHEN** 管理员创建优惠券 `{code: "OFF20", type: "percent", value: 0.20, max_discount: 50, min_amount: 100}`
- **THEN** 系统 SHALL 创建一张 8 折优惠券，最大减免 50 元，最低消费 100 元

#### Scenario: 优惠券码自动生成
- **WHEN** 管理员创建优惠券时未指定 code
- **THEN** 系统 SHALL 自动生成一个 8 位大写字母数字组合的唯一优惠券码

#### Scenario: 优惠券码唯一性
- **WHEN** 管理员创建的优惠券码与已有记录重复
- **THEN** 系统 SHALL 拒绝创建并返回唯一性冲突错误

#### Scenario: 管理员停用优惠券
- **WHEN** 管理员将优惠券状态设为 disabled
- **THEN** 系统 SHALL 立即阻止该优惠券被新订单使用，已使用的订单不受影响

### Requirement: 优惠券验证
系统 SHALL 在支付前验证优惠券码的有效性，包括存在性、状态、过期时间、使用次数、每用户限用次数和最低消费门槛。

#### Scenario: 有效优惠券通过验证
- **WHEN** 用户输入有效的优惠券码，订单金额满足最低消费门槛，且用户未超过该券的每用户限用次数
- **THEN** 系统 SHALL 返回验证通过，并计算该券的预计折扣金额

#### Scenario: 优惠券不存在
- **WHEN** 用户输入不存在的优惠券码
- **THEN** 系统 SHALL 返回错误 "COUPON_INVALID"

#### Scenario: 优惠券已过期
- **WHEN** 用户输入已过期的优惠券码
- **THEN** 系统 SHALL 返回错误 "COUPON_EXPIRED"

#### Scenario: 优惠券已停用
- **WHEN** 用户输入状态为 disabled 的优惠券码
- **THEN** 系统 SHALL 返回错误 "COUPON_DISABLED"

#### Scenario: 优惠券已达最大使用次数
- **WHEN** 用户输入已达到 max_uses 上限的优惠券码
- **THEN** 系统 SHALL 返回错误 "COUPON_EXHAUSTED"

#### Scenario: 用户已达该券的限用次数
- **WHEN** 用户已使用该券达到 per_user_limit 次
- **THEN** 系统 SHALL 返回错误 "COUPON_USER_LIMIT_REACHED"

#### Scenario: 订单金额不满足最低消费
- **WHEN** 用户输入的优惠券要求最低消费 100 元，但折后订单金额仅为 80 元
- **THEN** 系统 SHALL 返回错误 "COUPON_MIN_AMOUNT_NOT_MET"，并携带 min_amount 和 current_amount 信息

#### Scenario: 优惠券未生效
- **WHEN** 用户输入的优惠券 starts_at 时间在未来
- **THEN** 系统 SHALL 返回错误 "COUPON_NOT_STARTED"

#### Scenario: 优惠券适用范围不匹配
- **WHEN** 用户在余额充值订单中使用 scope=subscription 的优惠券，或在订阅订单中使用 scope=balance 的优惠券
- **THEN** 系统 SHALL 返回错误 "COUPON_SCOPE_MISMATCH"

#### Scenario: 每日限额重置订单不可使用优惠券
- **WHEN** 用户创建每日限额重置订单时携带 coupon_code
- **THEN** 系统 SHALL 返回错误 "COUPON_NOT_APPLICABLE"

### Requirement: 优惠券在订单创建时使用
系统 SHALL 在创建订单时，接受可选的 coupon_code 参数。验证通过后，在事务中完成优惠券扣减和使用记录创建。

#### Scenario: 使用固定金额优惠券创建订单
- **WHEN** 用户创建 100 元订单，使用 `{code: "SAVE10", type: "fixed", value: 10}` 优惠券
- **THEN** 系统 SHALL 在事务中锁定优惠券、创建使用记录、增加 used_count，设置 coupon_discount_amount = 10.00

#### Scenario: 使用百分比优惠券创建订单
- **WHEN** 用户创建 200 元订单，使用 `{code: "OFF20", type: "percent", value: 0.20, max_discount: 50}` 优惠券
- **THEN** 系统 SHALL 计算 coupon_discount_amount = min(200 * 0.20, 50) = 40.00 元

#### Scenario: 优惠券面值超过订单金额
- **WHEN** 用户创建 5 元订单，使用面值为 10 元的固定金额优惠券
- **THEN** 系统 SHALL 将 coupon_discount_amount 限制为 5.00 元（不超过订单金额），不找零

#### Scenario: 满减和优惠券同时使用
- **WHEN** 用户创建 100 元订单，满减减免 10 元，同时使用面值 5 元的优惠券
- **THEN** 系统 SHALL 先计算满减折扣（折后 90 元），再计算优惠券抵扣（最终 85 元），discount_amount = 10.00，coupon_discount_amount = 5.00

#### Scenario: 优惠券使用次数并发安全
- **WHEN** 多个请求同时使用同一张限量优惠券的最后一个使用名额
- **THEN** 系统 SHALL 通过事务行锁（FOR UPDATE）确保仅一个请求成功扣减，其余请求返回 "COUPON_EXHAUSTED"

### Requirement: 优惠券信息记录在订单中
系统 SHALL 在 PaymentOrder 中记录使用的优惠券码（coupon_code）和优惠券减免金额（coupon_discount_amount）。

#### Scenario: 订单记录优惠券信息
- **WHEN** 用户使用优惠券 "SAVE10" 创建订单，抵扣 10 元
- **THEN** 系统 SHALL 设置 coupon_code = "SAVE10"，coupon_discount_amount = 10.00

#### Scenario: 未使用优惠券时字段为空
- **WHEN** 用户未使用优惠券创建订单
- **THEN** 系统 SHALL 设置 coupon_code = ""，coupon_discount_amount = 0

### Requirement: 退款时优惠券回退
系统 SHALL 在订单退款时回退优惠券使用次数，并标记使用记录为已回退。

#### Scenario: 退款回退优惠券
- **WHEN** 管理员审批通过一笔使用了优惠券的订单退款
- **THEN** 系统 SHALL 将对应优惠券的 used_count 减 1，并将 coupon_usage 记录标记为 refunded

#### Scenario: 回退后优惠券可再次使用
- **WHEN** 优惠券因退款回退了使用次数
- **THEN** 该优惠券 SHALL 重新对符合条件的用户可用

### Requirement: 优惠券使用记录查询
系统 SHALL 支持管理员按优惠券查看使用记录列表，包含用户信息、订单信息、抵扣金额和使用时间。

#### Scenario: 查看优惠券使用记录
- **WHEN** 管理员查看某优惠券的使用记录
- **THEN** 系统 SHALL 返回分页的使用记录列表，包含 user_id、order_id、discount_amount、used_at 等信息

### Requirement: 优惠券删除保持审计
系统 SHALL 不硬删除已有使用记录的优惠券。管理员删除优惠券时，系统 SHALL 将其状态改为 archived 或 disabled，阻止新订单继续使用，但保留历史订单和使用记录关联。

#### Scenario: 删除已使用优惠券
- **WHEN** 管理员删除已有使用记录的优惠券
- **THEN** 系统 SHALL 将优惠券状态改为 archived 或 disabled，后续新订单不可使用该券，历史订单和 coupon_usage 记录保持可查询
