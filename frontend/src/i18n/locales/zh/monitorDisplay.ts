export default {
  monitorDisplay: {
    title: 'V1 用户页展示配置',
    description: '使用上下箭头调整分类和分类内渠道顺序，保存后用户渠道状态页刷新即生效。关联计费分组后，卡片显示用户可用分组的当前 Token 倍率；此处不会修改计费价格。',
    moveUp: '上移 {name}',
    moveDown: '下移 {name}',
    billingGroup: '关联计费分组',
    unlinked: '未关联（不展示倍率）',
    missingGroup: '原分组 #{id} 已不存在，请重新选择',
    disabled: '已停用',
    noMonitors: '此分类暂无监控项，用户页不展示空分类',
    loadFailed: '展示配置加载失败',
    saveFailed: '展示配置保存失败',
    saved: '展示配置已保存，用户渠道状态页刷新后生效',
    currentRate: '当前 Token 倍率 {rate}×',
    rateHint: '按关联计费分组计算，包含你的专属倍率与当前高峰因子；图片、视频等独立计价以计费配置为准。',
  },
}
