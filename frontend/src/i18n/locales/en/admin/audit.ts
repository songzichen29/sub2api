export default {
  audit: {
    title: 'Audit Logs',
    description: 'Records management-plane operations by admins and users. Header credentials keep only their first/last characters and request bodies are redacted. Entries cannot be deleted individually; clearing all requires two-factor verification.',
    clearAll: 'Clear All',
    empty: 'No audit logs yet',
    loadFailed: 'Failed to load audit logs',
    filters: {
      all: 'All',
      q: 'Keyword',
      qPlaceholder: 'Path / action / actor email',
      actorEmail: 'Actor Email',
      action: 'Action',
      clientIp: 'Client IP',
      method: 'Method',
      authMethod: 'Auth Method',
      result: 'Result',
      resultSuccess: 'Success',
      resultFailure: 'Failure',
      startTime: 'Start Time',
      endTime: 'End Time'
    },
    columns: {
      time: 'Time',
      actor: 'Actor',
      action: 'Action',
      method: 'Method',
      result: 'Result',
      clientIp: 'Client IP',
      detail: 'Detail'
    },
    actions: {
      accounts: {
        delete: 'Delete account',
        export: 'Export accounts',
        import: 'Import accounts',
        schedulable: {
          create: 'Set account schedulable status'
        },
        test: {
          create: 'Test account'
        },
        today_stats: {
          batch: {
            create: 'Refresh account daily stats in batch'
          }
        },
        update: 'Update account',
        upstream_billing_probe: {
          create: 'Probe account upstream billing'
        }
      },
      admin_api_key: {
        read: 'View admin API key',
        regenerate: 'Regenerate admin API key',
        delete: 'Delete admin API key'
      },
      audit_log: {
        clear: 'Clear all audit logs'
      },
      backups: {
        create: 'Create backup',
        delete: 'Delete backup',
        restore: 'Restore backup',
        download: 'Download backup',
        s3_config: {
          read: 'View backup S3 configuration',
          update: 'Update backup S3 configuration'
        }
      },
      channel_monitors: {
        run: {
          create: 'Run channel monitor'
        },
        update: 'Update channel monitor'
      },
      channels: {
        update: 'Update channel'
      },
      groups: {
        create: 'Create group',
        duplicate: 'Duplicate group',
        api_keys: {
          read: 'View group API keys'
        }
      },
      prompt_audit: {
        config: {
          update: 'Update prompt audit configuration'
        },
        endpoint: {
          probe: 'Probe prompt audit endpoint'
        },
        event: {
          delete: 'Delete prompt audit event'
        },
        events: {
          batch_delete: 'Batch delete prompt audit events',
          delete_preview: 'Preview prompt audit event deletion',
          filter_delete: 'Delete prompt audit events by filter'
        }
      },
      risk_control: {
        config: {
          update: 'Update risk control configuration'
        }
      },
      settings: {
        email_template_preview: {
          create: 'Preview email template'
        }
      },
      users: {
        update: 'Update user',
        api_keys: {
          read: 'View user API keys'
        }
      },
      auth: {
        login: {
          _label: 'Sign in',
          twoFactor: 'Sign in with two-factor authentication'
        },
        logout: {
          create: 'Sign out'
        },
        register: 'Register account',
        token: {
          refresh: 'Refresh sign-in token'
        },
        session_binding: {
          mismatch: 'Session binding mismatch'
        },
        step_up: {
          verify: 'Verify step-up authentication'
        }
      },
      keys: {
        create: 'Create API key',
        update: 'Update API key'
      },
      usage: {
        dashboard: {
          api_keys_usage: {
            create: 'Refresh API key usage dashboard'
          }
        }
      },
      user: {
        aff: {
          transfer: {
            create: 'Transfer affiliate quota to balance'
          }
        }
      }
    },
    actionSegments: {
      accounts: 'Accounts',
      admin_api_key: 'Admin API key',
      api_key: 'API key',
      api_keys: 'API keys',
      audit_log: 'Audit logs',
      auth: 'Authentication',
      backups: 'Backups',
      batch: 'Batch',
      channel_monitors: 'Channel monitors',
      channels: 'Channels',
      config: 'Configuration',
      create: 'Create',
      data_management: 'Data management',
      delete: 'Delete',
      download: 'Download',
      duplicate: 'Duplicate',
      export: 'Export',
      groups: 'Groups',
      import: 'Import',
      keys: 'API keys',
      login: 'Sign in',
      prompt_audit: 'Prompt audit',
      read: 'View',
      regenerate: 'Regenerate',
      restore: 'Restore',
      risk_control: 'Risk control',
      s3_config: 'S3 configuration',
      settings: 'Settings',
      update: 'Update',
      usage: 'Usage',
      user: 'User',
      users: 'Users',
      twoFactor: 'Two-factor authentication'
    },
    detail: {
      title: 'Audit Log Detail',
      actorRole: 'Role',
      methodPath: 'Method / Path',
      latency: 'Latency',
      requestId: 'Request ID',
      credential: 'Credential (masked)',
      userAgent: 'User-Agent',
      requestBody: 'Request Body (redacted)',
      extra: 'Extra'
    },
    clearConfirm: {
      title: 'Clear All Audit Logs',
      message: 'This permanently deletes all audit logs and cannot be undone. The clear action itself is recorded. Continue?',
      totpTitle: 'Enter Two-Factor Code',
      totpHint: 'Clearing audit logs requires a fresh TOTP verification.',
      success: 'Cleared {count} audit log(s)',
      failed: 'Failed to clear audit logs'
    }
  }
}
