## 1. 数据库迁移与实体模型

- [x] 1.1 创建 `ent/schema/coupon.go`，定义 Coupon 实体（code, type, value, min_amount, max_discount, scope, max_uses, used_count, per_user_limit, status, starts_at, expires_at, notes）
- [x] 1.2 创建 `ent/schema/coupon_usage.go`，定义 CouponUsage 实体（coupon_id, user_id, order_id, discount_amount, used_at, status）
- [x] 1.3 修改 `ent/schema/payment_order.go`，新增 `discount_amount`、`coupon_code`、`coupon_discount_amount` 三个字段
- [x] 1.4 运行 `go generate ./ent` 生成 ent CRUD 代码
- [x] 1.5 编写数据库迁移脚本 `migrations/xxx_add_coupons_and_discount_fields.sql`，包含新表创建和字段添加

## 2. 满减折扣核心逻辑

- [x] 2.1 修改 `payment_config_service.go`，在 PaymentConfig 和 UpdatePaymentConfigRequest 中新增 `DiscountRules` 字段（JSON 数组）
- [x] 2.2 创建 `service/discount_service.go`，实现满减规则匹配和金额计算逻辑（DiscountService.ApplyThresholdDiscount）
- [x] 2.3 为 DiscountService 编写单元测试，覆盖：无规则、金额不足门槛、匹配最高档位、固定减免类型、混合类型、折后金额归零等场景

## 3. 优惠券核心逻辑

- [x] 3.1 创建 `service/coupon_repository.go`，实现数据访问层（GetByCode, GetByCodeForUpdate, Create, Update, List, CreateUsage, GetUsageByCouponAndUser, IncrementUsedCount, DecrementUsedCount）
- [x] 3.2 创建 `service/coupon_service.go`，实现优惠券服务（Validate, Apply, Refund, CRUD 操作）
- [x] 3.3 实现优惠券验证逻辑，覆盖：存在性、状态、过期时间、未生效、使用次数、每用户限用、最低消费门槛
- [x] 3.4 实现优惠券使用逻辑，在事务中完成验证 + 行锁 + 扣减 + 创建使用记录
- [x] 3.5 实现优惠券退款回退逻辑，回退 used_count 并标记使用记录为 refunded
- [x] 3.6 为 CouponService 编写单元测试，覆盖：验证通过、各种验证失败、固定金额/百分比计算、面值超过订单金额、并发安全

## 4. 订单创建流程集成

- [x] 4.1 修改 `CreateOrderRequest` DTO，新增 `coupon_code` 可选字段
- [x] 4.2 修改 `payment_order.go` 的 `CreateOrder` 方法，在 limitAmount 确定后插入满减折扣计算步骤
- [x] 4.3 修改 `createOrderInTx` 方法，在事务中集成优惠券验证和使用逻辑
- [x] 4.4 修改 `createOrderInTx` 中的订单创建 builder，设置 discount_amount、coupon_code、coupon_discount_amount 字段
- [x] 4.5 确保满减和优惠券的计算顺序：先满减 → 再优惠券 → 最后手续费
- [x] 4.6 编写 CreateOrder 集成测试，覆盖：仅满减、仅优惠券、满减+优惠券、无折扣、每日限额重置不适用折扣

## 5. 价格预览接口

- [x] 5.1 定义 `PreviewPriceRequest` 和 `PreviewPriceResponse` DTO
- [x] 5.2 在 PaymentService 中实现 `PreviewPrice` 方法，复用 DiscountService 和 CouponService 的验证/计算逻辑
- [x] 5.3 在 `handler/payment_handler.go` 中实现 `PreviewPrice` handler
- [x] 5.4 在 `routes/payment.go` 中注册 `POST /payment/preview-price` 路由
- [x] 5.5 实现 `GET /payment/discount-rules` 接口，返回当前满减规则列表
- [x] 5.6 为预览接口编写测试，覆盖：正常预览、无效优惠券、最低消费不足

## 6. 退款流程适配

- [x] 6.1 修改退款逻辑，退款金额基于 pay_amount（实付金额）而非 amount（原价）
- [x] 6.2 退款审批通过后，如果订单使用了优惠券，调用 CouponService.Refund 回退使用次数
- [x] 6.3 编写退款相关测试，覆盖：有优惠券的退款回退、无优惠券的退款

## 7. 管理后台 API

- [x] 7.1 在 `handler/admin/` 中创建优惠券管理 handler（Create, Update, Delete, List, ListUsages）
- [x] 7.2 在 admin routes 中注册优惠券管理路由
- [x] 7.3 在 PaymentConfig 更新接口中支持 discount_rules 字段的读写
- [x] 7.4 接入 CouponRepository、CouponService、DiscountService、优惠券管理 handler 的依赖初始化和 wiring

## 8. 前端类型与 API 层

- [x] 8.1 修改 `types/payment.ts`，扩展 CreateOrderRequest 增加 coupon_code 字段，新增 PricePreviewResponse、DiscountRule、CouponInfo 类型
- [x] 8.2 修改 `api/payment.ts`，新增 previewPrice() 和 getDiscountRules() API 调用方法
- [x] 8.3 更新 PaymentOrder 类型定义，增加 discount_amount、coupon_code、coupon_discount_amount 字段

## 9. 前端支付页面改造

- [x] 9.1 创建 `CouponInput.vue` 组件，实现优惠券输入、验证请求和错误提示
- [x] 9.2 创建 `PriceBreakdown.vue` 组件，展示完整价格明细（原价 → 满减减免 → 优惠券减免 → 手续费 → 实付）
- [x] 9.3 修改 `PaymentView.vue` 充值 tab，集成满减提示、CouponInput 和 PriceBreakdown
- [x] 9.4 修改 `PaymentView.vue` 订阅确认流程，集成满减提示、CouponInput 和 PriceBreakdown
- [x] 9.5 修改 `paymentFlow.ts`，在 buildCreateOrderPayload 中传入 coupon_code 参数
- [x] 9.6 在充值金额输入后调用 getDiscountRules 展示满减提示，在优惠券输入后调用 previewPrice 更新价格明细

## 10. 前端管理页面

- [x] 10.1 创建管理后台优惠券列表页面（展示优惠券列表、状态、使用量）
- [x] 10.2 创建管理后台优惠券创建/编辑表单（支持固定金额和百分比两种类型）
- [x] 10.3 创建管理后台优惠券使用记录查看页面
- [x] 10.4 在支付配置页面中增加满减规则配置区域（动态添加/删除/排序规则条目）

## 11. 国际化

- [x] 11.1 在 `zh.ts` 和 `en.ts` 中添加满减相关文案（满X减Y、满X折、满减提示等）
- [x] 11.2 在 `zh.ts` 和 `en.ts` 中添加优惠券相关文案（优惠券码、输入优惠券、验证错误信息等）
- [x] 11.3 在 `zh.ts` 和 `en.ts` 中添加价格明细相关文案（原价、折扣、手续费、实付等）

## 12. 端到端验证

- [ ] 12.1 验证完整充值流程：输入金额 → 满减提示 → 输入优惠券 → 价格明细展示 → 创建订单 → 支付
- [ ] 12.2 验证完整订阅流程：选择套餐 → 满减折扣 → 输入优惠券 → 创建订单 → 支付
- [ ] 12.3 验证退款流程：含优惠券订单退款 → 优惠券回退 → 可再次使用
- [ ] 12.4 验证边界场景：无效优惠券、过期优惠券、不满足最低消费、scope 不匹配、daily_limit_reset 不适用、折后金额为零
