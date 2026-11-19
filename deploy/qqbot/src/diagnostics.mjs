// 只输出可枚举的错误标识，不把上游正文、聊天内容、地址或凭据写入日志。
export function errorFields(error) {
  const code = String(error.code || error.message || '');
  return { code: /^[a-zA-Z][a-zA-Z0-9_]{2,80}$/u.test(code) ? code : 'operation_failed',
    type: error.name, stage: error.stage, status: error.status, cause: error.cause?.code };
}
