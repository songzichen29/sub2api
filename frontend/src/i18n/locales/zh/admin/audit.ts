export default {
  audit: {
    title: '操作日志',
    description: '记录管理员与用户的管理面操作，请求头凭证仅保留首尾、请求体已脱敏。日志无法单条删除，全量清理需二次验证。',
    clearAll: '全部清理',
    empty: '暂无操作日志',
    loadFailed: '加载操作日志失败',
    filters: {
      all: '全部',
      q: '关键字',
      qPlaceholder: '路径 / 动作 / 操作者邮箱',
      actorEmail: '操作者邮箱',
      action: '动作',
      clientIp: '客户端 IP',
      method: '请求方法',
      authMethod: '认证方式',
      result: '结果',
      resultSuccess: '成功',
      resultFailure: '失败',
      startTime: '开始时间',
      endTime: '结束时间'
    },
    columns: {
      time: '时间',
      actor: '操作者',
      action: '动作',
      method: '方法',
      result: '结果',
      clientIp: '客户端 IP',
      detail: '详情'
    },
    actions: {
      accounts: {
        delete: '删除账号',
        export: '导出账号',
        import: '导入账号',
        schedulable: {
          create: '设置账号可调度状态'
        },
        test: {
          create: '测试账号'
        },
        today_stats: {
          batch: {
            create: '批量刷新账号今日统计'
          }
        },
        update: '更新账号',
        upstream_billing_probe: {
          create: '探测账号上游计费'
        }
      },
      admin_api_key: {
        read: '查看管理 API 密钥',
        regenerate: '重新生成管理 API 密钥',
        delete: '删除管理 API 密钥'
      },
      audit_log: {
        clear: '清理全部操作日志'
      },
      backups: {
        create: '创建备份',
        delete: '删除备份',
        restore: '恢复备份',
        download: '下载备份',
        s3_config: {
          read: '查看备份 S3 配置',
          update: '更新备份 S3 配置'
        }
      },
      channel_monitors: {
        run: {
          create: '执行渠道监控'
        },
        update: '更新渠道监控'
      },
      channels: {
        update: '更新渠道'
      },
      groups: {
        create: '创建分组',
        duplicate: '复制分组',
        api_keys: {
          read: '查看分组 API 密钥'
        }
      },
      prompt_audit: {
        config: {
          update: '更新提示词审计配置'
        },
        endpoint: {
          probe: '探测提示词审计端点'
        },
        event: {
          delete: '删除提示词审计事件'
        },
        events: {
          batch_delete: '批量删除提示词审计事件',
          delete_preview: '预览提示词审计事件删除',
          filter_delete: '按筛选条件删除提示词审计事件'
        }
      },
      risk_control: {
        config: {
          update: '更新风险控制配置'
        }
      },
      settings: {
        email_template_preview: {
          create: '预览邮件模板'
        }
      },
      users: {
        update: '更新用户',
        api_keys: {
          read: '查看用户 API 密钥'
        }
      },
      auth: {
        login: {
          _label: '登录',
          twoFactor: '两步验证登录'
        },
        logout: {
          create: '退出登录'
        },
        register: '注册',
        token: {
          refresh: '刷新登录令牌'
        },
        session_binding: {
          mismatch: '会话绑定不匹配'
        },
        step_up: {
          verify: '验证二次认证'
        }
      },
      keys: {
        create: '创建 API 密钥',
        update: '更新 API 密钥'
      },
      usage: {
        dashboard: {
          api_keys_usage: {
            create: '刷新 API 密钥用量看板'
          }
        }
      },
      user: {
        aff: {
          transfer: {
            create: '将推广额度转入余额'
          }
        }
      }
    },
    actionSegments: {
      accounts: '账号',
      admin_api_key: '管理 API 密钥',
      api_key: 'API 密钥',
      api_keys: 'API 密钥',
      audit_log: '操作日志',
      auth: '认证',
      backups: '备份',
      batch: '批量',
      channel_monitors: '渠道监控',
      channels: '渠道',
      config: '配置',
      create: '新建',
      data_management: '数据管理',
      delete: '删除',
      download: '下载',
      duplicate: '复制',
      export: '导出',
      groups: '分组',
      import: '导入',
      keys: 'API 密钥',
      login: '登录',
      prompt_audit: '提示词审计',
      read: '查看',
      regenerate: '重新生成',
      restore: '恢复',
      risk_control: '风险控制',
      s3_config: 'S3 配置',
      settings: '系统设置',
      update: '更新',
      usage: '用量',
      user: '用户',
      users: '用户',
      twoFactor: '两步验证'
    },
    detail: {
      title: '操作日志详情',
      actorRole: '角色',
      methodPath: '方法 / 路径',
      latency: '耗时',
      requestId: '请求 ID',
      credential: '凭证（掩码）',
      userAgent: 'User-Agent',
      requestBody: '请求体（已脱敏）',
      extra: '附加信息'
    },
    clearConfirm: {
      title: '清理全部操作日志',
      message: '此操作将永久删除所有操作日志，且不可恢复。清理动作本身会被留痕记录。确定继续吗？',
      totpTitle: '输入二次验证码',
      totpHint: '清理操作日志需要现场验证 TOTP 验证码。',
      success: '已清理 {count} 条操作日志',
      failed: '清理操作日志失败'
    }
  }
}
