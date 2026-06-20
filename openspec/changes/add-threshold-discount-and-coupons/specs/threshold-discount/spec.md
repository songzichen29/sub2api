## ADDED Requirements

### Requirement: 满减折扣规则配置
系统 SHALL 允许管理员在支付配置中设置阶梯式满减折扣规则。每条规则包含门槛金额、折扣类型（rate 折后支付比例或 reduce 固定减免）、折扣值、可选的描述标签和启用状态。规则列表 SHALL 支持启用和禁用。

#### Scenario: 管理员创建满减规则
- **WHEN** 管理员在支付配置中设置满减规则 `[{threshold: 50, type: "rate", value: 0.95}, {threshold: 100, type: "rate", value: 0.90}]`
- **THEN** 系统 SHALL 将规则保存到 PaymentConfig 的 discount_rules 字段，并按门槛金额升序排列

#### Scenario: 管理员清空满减规则
- **WHEN** 管理员将满减规则列表设置为空数组
- **THEN** 系统 SHALL 禁用满减折扣功能，所有订单不再享受满减优惠

#### Scenario: 规则门槛金额必须唯一并递增
- **WHEN** 管理员设置的规则包含重复门槛，或门槛金额小于等于 0
- **THEN** 系统 SHALL 拒绝保存并返回验证错误

#### Scenario: 禁用规则不参与计算
- **WHEN** 满减规则中某条规则 `enabled=false`
- **THEN** 系统 SHALL 保存该规则但在订单创建、价格预览和用户侧规则查询中忽略该规则

### Requirement: 订单金额自动应用满减折扣
系统 SHALL 在创建订单时，根据订单基准金额自动匹配满减规则中门槛最高的适用规则，计算减免金额。

#### Scenario: 余额充值满足最高档位
- **WHEN** 用户充值 150 元，满减规则为 `[{threshold: 50, type: "rate", value: 0.95}, {threshold: 100, type: "rate", value: 0.90}]`
- **THEN** 系统 SHALL 匹配 threshold=100 的规则，计算 discount_amount = 150 * (1 - 0.90) = 15.00 元，折后金额 = 135.00 元

#### Scenario: 金额低于最低门槛不享受折扣
- **WHEN** 用户充值 30 元，最低门槛为 50 元
- **THEN** 系统 SHALL 不应用任何折扣，discount_amount = 0

#### Scenario: 固定金额减免类型
- **WHEN** 用户充值 200 元，规则为 `{threshold: 200, type: "reduce", value: 30}`
- **THEN** 系统 SHALL 应用固定减免，discount_amount = 30.00 元，折后金额 = 170.00 元

#### Scenario: 订阅套餐同样适用满减
- **WHEN** 用户购买价格为 100 元的订阅套餐，满减规则 `{threshold: 100, type: "rate", value: 0.90}` 适用
- **THEN** 系统 SHALL 对套餐价格应用满减折扣，discount_amount = 10.00 元

#### Scenario: 每日限额重置订单不适用满减
- **WHEN** 用户支付每日限额重置费用
- **THEN** 系统 SHALL 不对该订单应用满减折扣

### Requirement: 折扣金额记录在订单中
系统 SHALL 在 PaymentOrder 中记录满减折扣金额（discount_amount 字段），同时保持 amount 字段为原始金额，pay_amount 字段为折后含手续费的最终支付金额。

#### Scenario: 订单包含满减折扣
- **WHEN** 创建一笔 100 元的充值订单，满减减免 10 元，手续费率 2%
- **THEN** 系统 SHALL 设置 amount=100.00, discount_amount=10.00, pay_amount=91.80（即 90 * 1.02）

#### Scenario: 无折扣时字段为零
- **WHEN** 创建一笔不满足满减条件的订单
- **THEN** 系统 SHALL 设置 discount_amount=0，且 pay_amount 的计算与现有行为一致

### Requirement: 手续费在折扣后计算
系统 SHALL 在满减折扣之后计算手续费，即手续费基于折后金额计算。

#### Scenario: 手续费基于折后金额
- **WHEN** 订单基准金额 100 元，满减减免 10 元，手续费率 5%
- **THEN** 系统 SHALL 计算手续费 = 90 * 5% = 4.50 元，pay_amount = 90 + 4.50 = 94.50 元

### Requirement: 折后金额不得低于零
系统 SHALL 确保满减折扣后的金额不低于零。当固定减免类型的减免值大于订单金额时，折后金额 SHALL 为零。

#### Scenario: 固定减免超过订单金额
- **WHEN** 订单金额 20 元，满减规则 `{threshold: 20, type: "reduce", value: 50}`
- **THEN** 系统 SHALL 将折后金额设为 0 元，discount_amount 设为 20.00 元
