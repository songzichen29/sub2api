export default {
  monitorDisplay: {
    title: 'V1 用户页展示配置',
    description: '使用上下箭头调整分类和分类内渠道顺序，保存后用户渠道状态页刷新即生效。关联计费分组后，卡片在平台标签旁显示当前 Token 倍率的一半；此设置不会修改实际计费倍率。',
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
    userRate: '用户倍率 {rate}x',
    rateHint: '此处显示关联计费分组当前 Token 倍率的一半，实际计费倍率不变。图片、视频等独立计价以计费配置为准。',
  },
}
