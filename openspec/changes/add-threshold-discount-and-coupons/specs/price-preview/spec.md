## ADDED Requirements

### Requirement: 价格预览接口
系统 SHALL 提供 `POST /api/v1/payment/preview-price` 接口，接受订单参数和可选的优惠券码，返回完整的价格明细。该接口 SHALL 为只读操作，不创建订单，不产生副作用。

#### Scenario: 预览余额充值价格
- **WHEN** 前端请求预览 `{order_type: "balance", amount: 100}`，满减规则存在 `{threshold: 100, type: "rate", value: 0.90}`，无优惠券
- **THEN** 系统 SHALL 返回 `{base_amount: 100, threshold_discount: 10, coupon_discount: 0, after_discount: 90, fee: 1.80, pay_amount: 91.80, fee_rate: 2}`（假设 fee_rate=2%）

#### Scenario: 预览含优惠券的价格
- **WHEN** 前端请求预览 `{order_type: "balance", amount: 100, coupon_code: "SAVE5"}`，满减减免 10 元，优惠券面值 5 元
- **THEN** 系统 SHALL 返回 `{base_amount: 100, threshold_discount: 10, coupon_discount: 5, after_discount: 85, fee: 1.70, pay_amount: 86.70}`

#### Scenario: 预览订阅套餐价格
- **WHEN** 前端请求预览 `{order_type: "subscription", plan_id: 1}`，套餐价格 100 元，满减减免 10 元
- **THEN** 系统 SHALL 基于套餐价格计算满减折扣并返回价格明细

#### Scenario: 预览无效优惠券返回错误
- **WHEN** 前端请求预览时携带无效的优惠券码
- **THEN** 系统 SHALL 返回优惠券验证错误（如 COUPON_INVALID、COUPON_EXPIRED 等），不返回价格明细

#### Scenario: 预览不满足优惠券最低消费
- **WHEN** 前端请求预览时携带的优惠券要求最低消费 100 元，但折后金额仅为 80 元
- **THEN** 系统 SHALL 返回 "COUPON_MIN_AMOUNT_NOT_MET" 错误，并在响应中携带 min_amount 和 current_amount 信息

### Requirement: 价格明细包含完整折扣信息
预览响应 SHALL 包含所有价格组成部分，使前端能够展示完整的价格明细面板。

#### Scenario: 响应字段完整
- **WHEN** 预览请求成功
- **THEN** 响应 SHALL 包含以下字段：base_amount（原始金额）、threshold_discount（满减减免）、coupon_discount（优惠券减免）、after_discount（折后金额）、fee（手续费）、pay_amount（最终支付金额）、fee_rate（手续费率）、applied_threshold_rule（应用的满减规则信息）、coupon_info（优惠券信息，如适用）

#### Scenario: 无折扣时明细为零
- **WHEN** 订单不满足任何满减条件且未使用优惠券
- **THEN** 系统 SHALL 返回 threshold_discount=0, coupon_discount=0，其他字段正常计算

### Requirement: 满减规则查询接口
系统 SHALL 提供 `GET /api/v1/payment/discount-rules` 接口，返回当前生效的满减规则列表，供前端展示提示信息。

#### Scenario: 获取满减规则
- **WHEN** 前端请求获取满减规则
- **THEN** 系统 SHALL 返回当前启用的满减规则列表，按门槛金额升序排列，包含 threshold、type、value、label、enabled 字段

#### Scenario: 无满减规则时返回空列表
- **WHEN** 管理员未配置满减规则
- **THEN** 系统 SHALL 返回空数组 `[]`

### Requirement: 预览与实际支付一致性
价格预览接口与订单创建 SHALL 使用相同的折扣计算逻辑，确保预览结果与实际支付金额一致。

#### Scenario: 预览金额与实际一致
- **WHEN** 用户通过预览接口获得 pay_amount = 86.70 元，随后立即使用该参数创建订单
- **THEN** 系统 SHALL 创建的订单 pay_amount = 86.70 元（假设规则和优惠券状态未变化）

#### Scenario: 规则变化导致不一致
- **WHEN** 用户预览后、创建订单前，管理员修改了满减规则或停用了优惠券
- **THEN** 系统 SHALL 在创建订单时重新计算折扣，若与预览结果不一致，以实际计算为准并在创建订单响应中返回实际金额
