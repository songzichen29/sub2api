## Why

当前支付系统仅支持套餐级别的静态折扣（原价/现价），缺乏动态的满减折扣和支付时使用的优惠券功能。这限制了运营灵活性，无法通过阶梯促销手段提升客单价，也无法通过优惠券码进行精准营销活动。

## What Changes

- 新增满减折扣功能：管理员在支付配置中设置阶梯规则（如满50打95折、满100打9折），系统在创建订单时自动匹配最高档位计算减免金额
- 新增支付优惠券功能：引入独立的 Coupon 模型（区别于现有注册优惠码 PromoCode），支持固定金额抵扣和百分比折扣两种类型，支付时可输入优惠券码抵扣
- 满减与优惠券可叠加：计算顺序为先满减后优惠券，优惠金额均记录在订单中
- 新增价格预览接口：前端可在用户确认支付前实时获取价格明细（原价、满减减免、优惠券减免、手续费、实付金额）
- PaymentOrder 新增折扣相关字段：满减减免金额、优惠券码、优惠券减免金额
- 前端支付页面增加满减提示、优惠券输入框和价格明细面板
- 管理后台新增优惠券 CRUD 管理页面和满减规则配置入口
- 退款时自动回退优惠券使用次数，退款金额基于实际支付金额

## Capabilities

### New Capabilities

- `threshold-discount`: 满减折扣规则配置与订单金额自动计算，支持阶梯式折扣比例和固定金额减免两种模式
- `payment-coupon`: 支付优惠券的创建、验证、使用和退款回退，支持固定金额和百分比折扣类型，含使用次数限制和每用户限用控制
- `price-preview`: 支付前价格预览接口，实时计算并返回满减、优惠券、手续费等完整价格明细

### Modified Capabilities

（无现有 spec 需要修改）

## Impact

- **后端核心**：`payment_order.go` 创建订单流程增加折扣计算步骤；`payment_config_service.go` 新增 DiscountRules 配置字段；`schema/payment_order.go` 新增字段
- **新增后端模块**：`discount_service.go`、`coupon_service.go`、`coupon_repository.go`、`schema/coupon.go`、`schema/coupon_usage.go`
- **新增 API**：`POST /payment/preview-price`、优惠券管理相关 admin API
- **数据库**：新增 `coupons` 和 `coupon_usages` 表；`payment_orders` 表新增 3 个字段
- **前端**：`PaymentView.vue` 增加满减提示和优惠券输入；新增 `CouponInput.vue`、`PriceBreakdown.vue` 组件；`types/payment.ts` 和 `api/payment.ts` 扩展
- **管理后台**：优惠券管理页面、支付配置页增加满减规则配置区域
- **退款流程**：退款逻辑需处理优惠券回退和基于实付金额退款
