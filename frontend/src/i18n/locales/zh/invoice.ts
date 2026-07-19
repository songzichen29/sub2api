export default {
  invoice: {
    tabs: { apply: '申请开票', history: '我的发票' },
    selectOrders: {
      title: '选择开票订单',
      minimum: '本次申请订单合计需满 {amount}',
      selectOrder: '选择订单 #{id}',
      orderId: '订单 ID', orderNo: '订单号', orderType: '订单类型', amount: '可开票金额', paidAt: '支付时间',
      empty: '当前没有符合条件的可开票订单。退款、已申请或已开票订单不可再次申请。',
    },
    summary: { selected: '已选 {count} 笔订单，合计 {amount}', met: '已满足开票金额要求。', remaining: '还差 {amount} 可提交申请。' },
    confirm: {
      title: '确认发票信息', total: '共 {count} 笔订单，开票金额 {amount}',
      warning: '发票申请提交后不可自行取消，所选订单将不能再次申请开票。如信息有误或有其他问题，请联系管理员处理。',
      acknowledge: '我已确认订单及发票信息无误，并知晓申请提交后不可自行取消。', submit: '提交开票申请',
    },
    header: {
      add: '新增抬头', edit: '编辑抬头', manage: '管理抬头', empty: '还没有保存的发票抬头。', choose: '选择发票抬头', choosePlaceholder: '请选择发票抬头', required: '请先新增并选择发票抬头。',
      type: '抬头类型', personal: '个人', company: '企业', title: '发票抬头', taxNumber: '纳税人识别号', email: '邮箱', phone: '联系电话', address: '地址', default: '设为默认抬头',
      deleteConfirm: '确定删除抬头“{title}”吗？历史申请不会受影响。',
    },
    history: {
      title: '开票记录', subtitle: '查看线下开票处理进度与历史订单。', applicationNo: '申请编号', amount: '开票金额', status: '状态', createdAt: '申请时间',
      rejectionReason: '拒绝原因', invoiceNumber: '发票号码', orderNo: '订单号', orderType: '订单类型',
    },
    admin: {
      searchPlaceholder: '搜索申请编号、订单号或用户邮箱', export: '导出当前筛选结果', minimumAmount: '最低开票金额', allStatuses: '全部状态',
      startDate: '开始日期', endDate: '结束日期', user: '用户', processStatus: '处理状态', note: '管理员备注',
    },
    status: { PENDING: '待处理', PROCESSING: '处理中', INVOICED: '已开票', REJECTED: '已拒绝' },
    orderTypes: { balance: '余额充值', subscription: '订阅套餐', daily_limit_reset: '日限额重置' },
    messages: { submitted: '开票申请已提交，请等待管理员线下处理。' },
    errors: {},
  },
}
