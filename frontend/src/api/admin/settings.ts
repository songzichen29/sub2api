/**
 * Admin Settings API endpoints
 * Handles system settings management for administrators
 */

import { apiClient } from "../client";
import type { DiscountRule } from "@/types/payment";
import type {
  CustomMenuItem,
  CustomEndpoint,
  LoginAgreementDocument,
  NotifyEmailEntry,
} from "@/types";

export interface DefaultSubscriptionSetting {
  group_id: number;
  validity_days?: number;
  starts_at?: string;
  expires_at?: string;
  mode?: "days" | "range";
}

export interface PaymentPaidUserRateRule {
  group_id: number;
  rate_multiplier: number;
  assigned_users?: number;
}

export interface PaymentPaidUserRateBackfillStatus {
  total_paid_users: number;
  assigned_users: number;
  rule_count: number;
  status: string;
  error?: string;
  updated_at?: string;
}

export interface AccountImportApplyTemplate {
  id: string
  name: string
  enableTags: boolean
  enableGroups: boolean
  enableProxy: boolean
  enableConcurrency: boolean
  enablePriority: boolean
  enableModelRestriction: boolean
  applyTags: string[]
  applyGroupIds: number[]
  applyProxyId: number | null
  applyConcurrency: number
  applyPriority: number
  modelRestrictionMode: 'whitelist' | 'mapping'
  allowedModels: string[]
  modelMappings: Array<{ from: string; to: string }>
}

export type AuthSourceType = "email" | "linuxdo" | "oidc" | "wechat";

export interface AuthSourceDefaultsValue {
  balance: number;
  concurrency: number;
  subscriptions: DefaultSubscriptionSetting[];
  grant_on_signup: boolean;
  grant_on_first_bind: boolean;
}

export type AuthSourceDefaultsState = Record<
  AuthSourceType,
  AuthSourceDefaultsValue
>;
export type PaymentVisibleMethod = "alipay" | "wxpay";
export type PaymentVisibleMethodSource =
  | ""
  | "official_alipay"
  | "easypay_alipay"
  | "official_wxpay"
  | "easypay_wxpay";
export type WeChatConnectMode = "open" | "mp" | "mobile";
export type PlatformType = "anthropic" | "openai" | "gemini" | "antigravity";

export interface PlatformQuotaLimits {
  daily: number | null;
  weekly: number | null;
  monthly: number | null;
}

export type DefaultPlatformQuotasMap = Partial<
  Record<PlatformType, PlatformQuotaLimits>
>;

export interface PaymentVisibleMethodSourceOption {
  value: PaymentVisibleMethodSource;
  labelZh: string;
  labelEn: string;
}

export interface WeChatConnectModeOption {
  value: WeChatConnectMode;
  labelZh: string;
  labelEn: string;
}

const AUTH_SOURCE_TYPES: AuthSourceType[] = [
  "email",
  "linuxdo",
  "oidc",
  "wechat",
];
const AUTH_SOURCE_DEFAULT_BALANCE = 0;
const AUTH_SOURCE_DEFAULT_CONCURRENCY = 5;
const PLATFORMS: PlatformType[] = [
  "anthropic",
  "openai",
  "gemini",
  "antigravity",
];
const PAYMENT_VISIBLE_METHOD_SOURCE_OPTIONS: Record<
  PaymentVisibleMethod,
  PaymentVisibleMethodSourceOption[]
> = {
  alipay: [
    { value: "", labelZh: "未配置", labelEn: "Not configured" },
    {
      value: "official_alipay",
      labelZh: "支付宝官方",
      labelEn: "Official Alipay",
    },
    {
      value: "easypay_alipay",
      labelZh: "易支付支付宝",
      labelEn: "EasyPay Alipay",
    },
  ],
  wxpay: [
    { value: "", labelZh: "未配置", labelEn: "Not configured" },
    {
      value: "official_wxpay",
      labelZh: "微信官方",
      labelEn: "Official WeChat Pay",
    },
    {
      value: "easypay_wxpay",
      labelZh: "易支付微信",
      labelEn: "EasyPay WeChat Pay",
    },
  ],
};
const PAYMENT_VISIBLE_METHOD_SOURCE_ALIASES: Record<
  PaymentVisibleMethod,
  Record<string, PaymentVisibleMethodSource>
> = {
  alipay: {
    official_alipay: "official_alipay",
    alipay: "official_alipay",
    alipay_direct: "official_alipay",
    official: "official_alipay",
    easypay_alipay: "easypay_alipay",
    easypay: "easypay_alipay",
  },
  wxpay: {
    official_wxpay: "official_wxpay",
    wxpay: "official_wxpay",
    wxpay_direct: "official_wxpay",
    wechat: "official_wxpay",
    official: "official_wxpay",
    easypay_wxpay: "easypay_wxpay",
    easypay: "easypay_wxpay",
  },
};
const WECHAT_CONNECT_MODE_OPTIONS: WeChatConnectModeOption[] = [
  { value: "open", labelZh: "PC 应用", labelEn: "PC App" },
  {
    value: "mp",
    labelZh: "公众号",
    labelEn: "Official Account",
  },
  {
    value: "mobile",
    labelZh: "移动应用",
    labelEn: "Mobile App",
  },
];
const WECHAT_CONNECT_MODE_ALIASES: Record<string, WeChatConnectMode> = {
  open: "open",
  open_platform: "open",
  official: "open",
  wx_open: "open",
  mp: "mp",
  official_account: "mp",
  wechat_mp: "mp",
  mini_program: "mp",
  mobile: "mobile",
  mobile_app: "mobile",
  native_app: "mobile",
};

export async function getAccountImportTemplates() {
  const { data } = await apiClient.get<{ templates?: AccountImportApplyTemplate[] }>('/admin/settings/account-import-templates')
  return Array.isArray(data?.templates) ? data.templates : []
}

export async function updateAccountImportTemplates(templates: AccountImportApplyTemplate[]) {
  const { data } = await apiClient.put<{ templates?: AccountImportApplyTemplate[] }>('/admin/settings/account-import-templates', { templates })
  return Array.isArray(data?.templates) ? data.templates : []
}

export function normalizeDefaultSubscriptionSettings(
  subscriptions: DefaultSubscriptionSetting[] | null | undefined,
): DefaultSubscriptionSetting[] {
  if (!Array.isArray(subscriptions)) return [];
  const normalized: DefaultSubscriptionSetting[] = [];
  for (const item of subscriptions) {
    const groupID = Math.floor(Number(item.group_id));
    if (groupID <= 0) continue;

    const startsAtRaw = String(item.starts_at || "").trim();
    const expiresAtRaw = String(item.expires_at || "").trim();
    if (startsAtRaw || expiresAtRaw) {
      if (!startsAtRaw || !expiresAtRaw) continue;
      const startsAt = new Date(startsAtRaw);
      const expiresAt = new Date(expiresAtRaw);
      if (
        Number.isNaN(startsAt.getTime()) ||
        Number.isNaN(expiresAt.getTime()) ||
        expiresAt.getTime() <= startsAt.getTime()
      ) {
        continue;
      }
      normalized.push({
        group_id: groupID,
        starts_at: startsAt.toISOString(),
        expires_at: expiresAt.toISOString(),
      });
      continue;
    }

    const validityDays = Math.floor(Number(item.validity_days));
    if (validityDays <= 0) continue;
    normalized.push({
      group_id: groupID,
      validity_days: Math.min(36500, Math.max(1, validityDays)),
    });
  }
  return normalized;
}

export function normalizePlatformQuotasMap(
  input?: DefaultPlatformQuotasMap | null,
): DefaultPlatformQuotasMap {
  const result: DefaultPlatformQuotasMap = {};
  for (const platform of PLATFORMS) {
    const source = input?.[platform];
    result[platform] = {
      daily:
        typeof source?.daily === "number" && Number.isFinite(source.daily)
          ? source.daily
          : null,
      weekly:
        typeof source?.weekly === "number" && Number.isFinite(source.weekly)
          ? source.weekly
          : null,
      monthly:
        typeof source?.monthly === "number" && Number.isFinite(source.monthly)
          ? source.monthly
          : null,
    };
  }
  return result;
}

export function sanitizePlatformQuotasMap(
  input?: DefaultPlatformQuotasMap | null,
): DefaultPlatformQuotasMap {
  const clean = (value: unknown): number | null =>
    typeof value === "number" && Number.isFinite(value) && value >= 0
      ? value
      : null;
  const result: DefaultPlatformQuotasMap = {};
  for (const platform of PLATFORMS) {
    const source = input?.[platform];
    result[platform] = {
      daily: clean(source?.daily),
      weekly: clean(source?.weekly),
      monthly: clean(source?.monthly),
    };
  }
  return result;
}

export function buildAuthSourceDefaultsState(
  settings: Partial<SystemSettings>,
): AuthSourceDefaultsState {
  const raw = settings as Record<string, unknown>;

  return AUTH_SOURCE_TYPES.reduce((acc, source) => {
    const subscriptions = raw[`auth_source_default_${source}_subscriptions`];
    acc[source] = {
      balance: Number(
        raw[`auth_source_default_${source}_balance`] ??
          AUTH_SOURCE_DEFAULT_BALANCE,
      ),
      concurrency: Math.max(
        1,
        Number(
          raw[`auth_source_default_${source}_concurrency`] ??
            AUTH_SOURCE_DEFAULT_CONCURRENCY,
        ),
      ),
      subscriptions: normalizeDefaultSubscriptionSettings(
        Array.isArray(subscriptions)
          ? (subscriptions as DefaultSubscriptionSetting[])
          : [],
      ),
      grant_on_signup:
        raw[`auth_source_default_${source}_grant_on_signup`] === true,
      grant_on_first_bind:
        raw[`auth_source_default_${source}_grant_on_first_bind`] === true,
    };
    return acc;
  }, {} as AuthSourceDefaultsState);
}

export function appendAuthSourceDefaultsToUpdateRequest(
  payload: UpdateSettingsRequest,
  authSourceDefaults: AuthSourceDefaultsState,
): UpdateSettingsRequest {
  const target = payload as Record<string, unknown>;

  for (const source of AUTH_SOURCE_TYPES) {
    const current = authSourceDefaults[source];
    target[`auth_source_default_${source}_balance`] =
      Number(current.balance) || 0;
    target[`auth_source_default_${source}_concurrency`] = Math.max(
      1,
      Math.floor(
        Number(current.concurrency) || AUTH_SOURCE_DEFAULT_CONCURRENCY,
      ),
    );
    target[`auth_source_default_${source}_subscriptions`] =
      normalizeDefaultSubscriptionSettings(current.subscriptions);
    target[`auth_source_default_${source}_grant_on_signup`] =
      current.grant_on_signup;
    target[`auth_source_default_${source}_grant_on_first_bind`] =
      current.grant_on_first_bind;
  }

  return payload;
}

export function getPaymentVisibleMethodSourceOptions(
  method: PaymentVisibleMethod,
): PaymentVisibleMethodSourceOption[] {
  return PAYMENT_VISIBLE_METHOD_SOURCE_OPTIONS[method];
}

export function normalizePaymentVisibleMethodSource(
  method: PaymentVisibleMethod,
  source: unknown,
): PaymentVisibleMethodSource {
  if (typeof source !== "string") return "";

  const normalized = source.trim().toLowerCase();
  if (!normalized) return "";

  return PAYMENT_VISIBLE_METHOD_SOURCE_ALIASES[method][normalized] ?? "";
}

export function getWeChatConnectModeOptions(): WeChatConnectModeOption[] {
  return WECHAT_CONNECT_MODE_OPTIONS;
}

export function normalizeWeChatConnectMode(source: unknown): WeChatConnectMode {
  if (typeof source !== "string") return "open";

  const normalized = source.trim().toLowerCase();
  if (!normalized) return "open";

  return WECHAT_CONNECT_MODE_ALIASES[normalized] ?? "open";
}

export function defaultWeChatConnectScopesForMode(mode: unknown): string {
  switch (normalizeWeChatConnectMode(mode)) {
    case "mp":
      return "snsapi_userinfo";
    case "mobile":
      return "";
    default:
      return "snsapi_login";
  }
}

export function resolveWeChatConnectModeCapabilities(
  openEnabled: unknown,
  mpEnabled: unknown,
  mobileEnabled: unknown,
  legacyMode: unknown,
): { openEnabled: boolean; mpEnabled: boolean; mobileEnabled: boolean } {
  if (
    typeof openEnabled === "boolean" ||
    typeof mpEnabled === "boolean" ||
    typeof mobileEnabled === "boolean"
  ) {
    return {
      openEnabled: openEnabled === true,
      mpEnabled: mpEnabled === true,
      mobileEnabled: mobileEnabled === true,
    };
  }

  switch (normalizeWeChatConnectMode(legacyMode)) {
    case "mp":
      return { openEnabled: false, mpEnabled: true, mobileEnabled: false };
    case "mobile":
      return { openEnabled: false, mpEnabled: false, mobileEnabled: true };
    default:
      return { openEnabled: true, mpEnabled: false, mobileEnabled: false };
  }
}

export function deriveWeChatConnectStoredMode(
  openEnabled: boolean,
  mpEnabled: boolean,
  mobileEnabled: boolean,
  legacyMode: unknown,
): WeChatConnectMode {
  if (mpEnabled) return "mp";
  if (mobileEnabled) return "mobile";
  if (openEnabled) return "open";
  return normalizeWeChatConnectMode(legacyMode);
}

/**
 * System settings interface
 */
export interface SystemSettings {
  // Registration settings
  registration_enabled: boolean;
  email_verify_enabled: boolean;
  registration_email_suffix_whitelist: string[];
  promo_code_enabled: boolean;
  password_reset_enabled: boolean;
  frontend_url: string;
  invitation_code_enabled: boolean;
  totp_enabled: boolean; // TOTP 双因素认证
  totp_encryption_key_configured: boolean; // TOTP 加密密钥是否已配置
  login_agreement_enabled?: boolean;
  login_agreement_mode?: "modal" | "checkbox" | string;
  login_agreement_updated_at?: string;
  login_agreement_documents?: LoginAgreementDocument[];
  // Default settings
  default_balance: number;
  affiliate_recharge_enabled: boolean;
  affiliate_subscription_enabled: boolean;
  affiliate_recharge_rebate_rate: number;
  affiliate_subscription_rebate_rate: number;
  affiliate_rebate_freeze_hours: number;
  affiliate_rebate_duration_days: number;
  affiliate_rebate_per_invitee_cap: number;
  affiliate_admin_recharge_enabled: boolean;
  default_concurrency: number;
  default_user_rpm_limit: number;
  default_subscriptions: DefaultSubscriptionSetting[];
  auth_source_default_email_balance?: number;
  auth_source_default_email_concurrency?: number;
  auth_source_default_email_subscriptions?: DefaultSubscriptionSetting[];
  auth_source_default_email_grant_on_signup?: boolean;
  auth_source_default_email_grant_on_first_bind?: boolean;
  auth_source_default_linuxdo_balance?: number;
  auth_source_default_linuxdo_concurrency?: number;
  auth_source_default_linuxdo_subscriptions?: DefaultSubscriptionSetting[];
  auth_source_default_linuxdo_grant_on_signup?: boolean;
  auth_source_default_linuxdo_grant_on_first_bind?: boolean;
  auth_source_default_oidc_balance?: number;
  auth_source_default_oidc_concurrency?: number;
  auth_source_default_oidc_subscriptions?: DefaultSubscriptionSetting[];
  auth_source_default_oidc_grant_on_signup?: boolean;
  auth_source_default_oidc_grant_on_first_bind?: boolean;
  auth_source_default_wechat_balance?: number;
  auth_source_default_wechat_concurrency?: number;
  auth_source_default_wechat_subscriptions?: DefaultSubscriptionSetting[];
  auth_source_default_wechat_grant_on_signup?: boolean;
  auth_source_default_wechat_grant_on_first_bind?: boolean;
  default_platform_quotas?: DefaultPlatformQuotasMap;
  force_email_on_third_party_signup?: boolean;
  // OEM settings
  site_name: string;
  site_logo: string;
  site_subtitle: string;
  api_base_url: string;
  openai_free_image_bridge_url: string;
  openai_free_image_bridge_auth_key_configured: boolean;
  contact_info: string;
  doc_url: string;
  home_content: string;
  hide_ccs_import_button: boolean;
  table_default_page_size: number;
  table_page_size_options: number[];
  backend_mode_enabled: boolean;
  custom_menu_items: CustomMenuItem[];
  custom_endpoints: CustomEndpoint[];
  // SMTP settings
  smtp_host: string;
  smtp_port: number;
  smtp_username: string;
  smtp_password_configured: boolean;
  smtp_from_email: string;
  smtp_from_name: string;
  smtp_use_tls: boolean;
  // Cloudflare Turnstile settings
  turnstile_enabled: boolean;
  turnstile_site_key: string;
  turnstile_secret_key_configured: boolean;
  api_key_acl_trust_forwarded_ip: boolean;
  forwarded_client_ip_headers: string[];

  // LinuxDo Connect OAuth settings
  linuxdo_connect_enabled: boolean;
  linuxdo_connect_client_id: string;
  linuxdo_connect_client_secret_configured: boolean;
  linuxdo_connect_redirect_url: string;

  // WeChat Connect OAuth settings
  wechat_connect_enabled: boolean;
  wechat_connect_app_id: string;
  wechat_connect_app_secret_configured: boolean;
  wechat_connect_open_app_id?: string;
  wechat_connect_open_app_secret_configured?: boolean;
  wechat_connect_mp_app_id?: string;
  wechat_connect_mp_app_secret_configured?: boolean;
  wechat_connect_mobile_app_id?: string;
  wechat_connect_mobile_app_secret_configured?: boolean;
  wechat_connect_open_enabled?: boolean;
  wechat_connect_mp_enabled?: boolean;
  wechat_connect_mobile_enabled?: boolean;
  wechat_connect_mode: string;
  wechat_connect_scopes: string;
  wechat_connect_redirect_url: string;
  wechat_connect_frontend_redirect_url: string;

  // Generic OIDC OAuth settings
  oidc_connect_enabled: boolean;
  oidc_connect_provider_name: string;
  oidc_connect_client_id: string;
  oidc_connect_client_secret_configured: boolean;
  oidc_connect_issuer_url: string;
  oidc_connect_discovery_url: string;
  oidc_connect_authorize_url: string;
  oidc_connect_token_url: string;
  oidc_connect_userinfo_url: string;
  oidc_connect_jwks_url: string;
  oidc_connect_scopes: string;
  oidc_connect_redirect_url: string;
  oidc_connect_frontend_redirect_url: string;
  oidc_connect_token_auth_method: string;
  oidc_connect_use_pkce: boolean;
  oidc_connect_validate_id_token: boolean;
  oidc_connect_allowed_signing_algs: string;
  oidc_connect_clock_skew_seconds: number;
  oidc_connect_require_email_verified: boolean;
  oidc_connect_userinfo_email_path: string;
  oidc_connect_userinfo_id_path: string;
  oidc_connect_userinfo_username_path: string;

  // Model fallback configuration
  enable_model_fallback: boolean;
  fallback_model_anthropic: string;
  fallback_model_openai: string;
  fallback_model_gemini: string;
  fallback_model_antigravity: string;

  // Identity patch configuration (Claude -> Gemini)
  enable_identity_patch: boolean;
  identity_patch_prompt: string;

  // Ops Monitoring (vNext)
  ops_monitoring_enabled: boolean;
  ops_realtime_monitoring_enabled: boolean;
  ops_query_mode_default: "auto" | "raw" | "preagg" | string;
  ops_metrics_interval_seconds: number;

  // Claude Code version check
  min_claude_code_version: string;
  max_claude_code_version: string;

  // 分组隔离
  allow_ungrouped_key_scheduling: boolean;

  // Gateway forwarding behavior
  enable_fingerprint_unification: boolean;
  enable_metadata_passthrough: boolean;
  enable_cch_signing: boolean;
  enable_claude_oauth_system_prompt_injection: boolean;
  claude_oauth_system_prompt: string;
  claude_oauth_system_prompt_blocks: string;
  enable_anthropic_cache_ttl_1h_injection: boolean;
  rewrite_message_cache_control: boolean;
  enable_client_dateline_normalization: boolean;
  antigravity_user_agent_version: string;
  openai_codex_user_agent: string;
  web_search_emulation_enabled?: boolean;

  // Payment configuration
  payment_enabled: boolean;
  risk_control_enabled: boolean;

  // Cyber session block
  cyber_session_block_enabled: boolean;
  cyber_session_block_ttl_seconds: number;

  payment_min_amount: number;
  payment_max_amount: number;
  payment_daily_limit: number;
  payment_order_timeout_minutes: number;
  payment_max_pending_orders: number;
  payment_enabled_types: string[];
  payment_balance_disabled: boolean;
  payment_balance_recharge_multiplier: number;
  payment_discount_rules: DiscountRule[];
  payment_quick_amounts: number[];
  payment_paid_user_rate_enabled: boolean;
  payment_paid_user_rate_rules: PaymentPaidUserRateRule[];
  payment_paid_user_rate_backfill: PaymentPaidUserRateBackfillStatus;
  payment_recharge_fee_rate: number;
  payment_load_balance_strategy: string;
  payment_product_name_prefix: string;
  payment_product_name_suffix: string;
  payment_help_image_url: string;
  payment_help_text: string;
  payment_cancel_rate_limit_enabled: boolean;
  payment_cancel_rate_limit_max: number;
  payment_cancel_rate_limit_window: number;
  payment_cancel_rate_limit_unit: string;
  payment_cancel_rate_limit_window_mode: string;
  payment_alipay_force_qrcode?: boolean;
  payment_alipay_mobile_precreate_deep_link?: boolean;
  payment_visible_method_alipay_source?: string;
  payment_visible_method_wxpay_source?: string;
  payment_visible_method_alipay_enabled?: boolean;
  payment_visible_method_wxpay_enabled?: boolean;
  openai_low_upstream_rate_priority_enabled?: boolean;
  openai_oauth_scheduling_rate_multiplier?: number;
  openai_advanced_scheduler_enabled?: boolean;
  openai_advanced_scheduler_sticky_weighted_enabled?: boolean;
  openai_advanced_scheduler_subscription_priority_enabled?: boolean;
  openai_advanced_scheduler_lb_top_k?: string;
  openai_advanced_scheduler_weight_priority?: string;
  openai_advanced_scheduler_weight_load?: string;
  openai_advanced_scheduler_weight_queue?: string;
  openai_advanced_scheduler_weight_error_rate?: string;
  openai_advanced_scheduler_weight_ttft?: string;
  openai_advanced_scheduler_weight_reset?: string;
  openai_advanced_scheduler_weight_quota_headroom?: string;
  openai_advanced_scheduler_weight_upstream_cost?: string;
  openai_advanced_scheduler_weight_previous_response?: string;
  openai_advanced_scheduler_weight_session_sticky?: string;
  openai_advanced_scheduler_effective_lb_top_k?: string;
  openai_advanced_scheduler_effective_weight_priority?: string;
  openai_advanced_scheduler_effective_weight_load?: string;
  openai_advanced_scheduler_effective_weight_queue?: string;
  openai_advanced_scheduler_effective_weight_error_rate?: string;
  openai_advanced_scheduler_effective_weight_ttft?: string;
  openai_advanced_scheduler_effective_weight_reset?: string;
  openai_advanced_scheduler_effective_weight_quota_headroom?: string;
  openai_advanced_scheduler_effective_weight_upstream_cost?: string;
  openai_advanced_scheduler_effective_weight_previous_response?: string;
  openai_advanced_scheduler_effective_weight_session_sticky?: string;
  standalone_account_import_enabled: boolean;
  standalone_account_import_password_configured: boolean;

  // 余额、订阅到期与账号限额通知
  balance_low_notify_enabled: boolean;
  balance_low_notify_threshold: number;
  balance_low_notify_recharge_url: string;
  subscription_expiry_notify_enabled: boolean;
  account_quota_notify_enabled: boolean;
  account_quota_notify_emails: NotifyEmailEntry[];

  // Channel Monitor feature switch
  channel_monitor_enabled: boolean;
  channel_monitor_default_interval_seconds: number;

  // Available Channels feature switch
  available_channels_enabled: boolean;

  // Affiliate (邀请返利) feature switch
  affiliate_enabled: boolean;

  // OpenAI fast/flex policy
  openai_fast_policy_settings?: OpenAIFastPolicySettings;
}

export interface UpdateSettingsRequest {
  registration_enabled?: boolean;
  email_verify_enabled?: boolean;
  registration_email_suffix_whitelist?: string[];
  promo_code_enabled?: boolean;
  password_reset_enabled?: boolean;
  frontend_url?: string;
  invitation_code_enabled?: boolean;
  totp_enabled?: boolean; // TOTP 双因素认证
  session_binding_enabled?: boolean; // 会话 IP/UA 绑定
  step_up_enabled?: boolean; // 敏感操作 step-up 2FA
  audit_log_retention_days?: number; // 审计日志保留天数
  login_agreement_enabled?: boolean;
  login_agreement_mode?: "modal" | "checkbox" | string;
  login_agreement_updated_at?: string;
  login_agreement_documents?: LoginAgreementDocument[];
  default_balance?: number;
  affiliate_recharge_enabled?: boolean;
  affiliate_subscription_enabled?: boolean;
  affiliate_recharge_rebate_rate?: number;
  affiliate_subscription_rebate_rate?: number;
  affiliate_rebate_freeze_hours?: number;
  affiliate_rebate_duration_days?: number;
  affiliate_rebate_per_invitee_cap?: number;
  affiliate_admin_recharge_enabled?: boolean;
  default_concurrency?: number;
  default_user_rpm_limit?: number;
  default_subscriptions?: DefaultSubscriptionSetting[];
  auth_source_default_email_balance?: number;
  auth_source_default_email_concurrency?: number;
  auth_source_default_email_subscriptions?: DefaultSubscriptionSetting[];
  auth_source_default_email_grant_on_signup?: boolean;
  auth_source_default_email_grant_on_first_bind?: boolean;
  auth_source_default_linuxdo_balance?: number;
  auth_source_default_linuxdo_concurrency?: number;
  auth_source_default_linuxdo_subscriptions?: DefaultSubscriptionSetting[];
  auth_source_default_linuxdo_grant_on_signup?: boolean;
  auth_source_default_linuxdo_grant_on_first_bind?: boolean;
  auth_source_default_oidc_balance?: number;
  auth_source_default_oidc_concurrency?: number;
  auth_source_default_oidc_subscriptions?: DefaultSubscriptionSetting[];
  auth_source_default_oidc_grant_on_signup?: boolean;
  auth_source_default_oidc_grant_on_first_bind?: boolean;
  auth_source_default_wechat_balance?: number;
  auth_source_default_wechat_concurrency?: number;
  auth_source_default_wechat_subscriptions?: DefaultSubscriptionSetting[];
  auth_source_default_wechat_grant_on_signup?: boolean;
  auth_source_default_wechat_grant_on_first_bind?: boolean;
  default_platform_quotas?: DefaultPlatformQuotasMap;
  force_email_on_third_party_signup?: boolean;
  site_name?: string;
  site_logo?: string;
  site_subtitle?: string;
  api_base_url?: string;
  openai_free_image_bridge_url?: string;
  openai_free_image_bridge_auth_key?: string;
  contact_info?: string;
  doc_url?: string;
  home_content?: string;
  hide_ccs_import_button?: boolean;
  table_default_page_size?: number;
  table_page_size_options?: number[];
  backend_mode_enabled?: boolean;
  custom_menu_items?: CustomMenuItem[];
  custom_endpoints?: CustomEndpoint[];
  smtp_host?: string;
  smtp_port?: number;
  smtp_username?: string;
  smtp_password?: string;
  smtp_from_email?: string;
  smtp_from_name?: string;
  smtp_use_tls?: boolean;
  turnstile_enabled?: boolean;
  turnstile_site_key?: string;
  turnstile_secret_key?: string;
  api_key_acl_trust_forwarded_ip?: boolean;
  forwarded_client_ip_headers?: string[];
  linuxdo_connect_enabled?: boolean;
  linuxdo_connect_client_id?: string;
  linuxdo_connect_client_secret?: string;
  linuxdo_connect_redirect_url?: string;
  wechat_connect_enabled?: boolean;
  wechat_connect_app_id?: string;
  wechat_connect_app_secret?: string;
  wechat_connect_open_app_id?: string;
  wechat_connect_open_app_secret?: string;
  wechat_connect_mp_app_id?: string;
  wechat_connect_mp_app_secret?: string;
  wechat_connect_mobile_app_id?: string;
  wechat_connect_mobile_app_secret?: string;
  wechat_connect_open_enabled?: boolean;
  wechat_connect_mp_enabled?: boolean;
  wechat_connect_mobile_enabled?: boolean;
  wechat_connect_mode?: string;
  wechat_connect_scopes?: string;
  wechat_connect_redirect_url?: string;
  wechat_connect_frontend_redirect_url?: string;
  oidc_connect_enabled?: boolean;
  oidc_connect_provider_name?: string;
  oidc_connect_client_id?: string;
  oidc_connect_client_secret?: string;
  oidc_connect_issuer_url?: string;
  oidc_connect_discovery_url?: string;
  oidc_connect_authorize_url?: string;
  oidc_connect_token_url?: string;
  oidc_connect_userinfo_url?: string;
  oidc_connect_jwks_url?: string;
  oidc_connect_scopes?: string;
  oidc_connect_redirect_url?: string;
  oidc_connect_frontend_redirect_url?: string;
  oidc_connect_token_auth_method?: string;
  oidc_connect_use_pkce?: boolean;
  oidc_connect_validate_id_token?: boolean;
  oidc_connect_allowed_signing_algs?: string;
  oidc_connect_clock_skew_seconds?: number;
  oidc_connect_require_email_verified?: boolean;
  oidc_connect_userinfo_email_path?: string;
  oidc_connect_userinfo_id_path?: string;
  oidc_connect_userinfo_username_path?: string;
  enable_model_fallback?: boolean;
  fallback_model_anthropic?: string;
  fallback_model_openai?: string;
  fallback_model_gemini?: string;
  fallback_model_antigravity?: string;
  enable_identity_patch?: boolean;
  identity_patch_prompt?: string;
  ops_monitoring_enabled?: boolean;
  ops_realtime_monitoring_enabled?: boolean;
  ops_query_mode_default?: "auto" | "raw" | "preagg" | string;
  ops_metrics_interval_seconds?: number;
  min_claude_code_version?: string;
  max_claude_code_version?: string;
  allow_ungrouped_key_scheduling?: boolean;
  enable_fingerprint_unification?: boolean;
  enable_metadata_passthrough?: boolean;
  enable_cch_signing?: boolean;
  enable_claude_oauth_system_prompt_injection?: boolean;
  claude_oauth_system_prompt?: string;
  claude_oauth_system_prompt_blocks?: string;
  enable_anthropic_cache_ttl_1h_injection?: boolean;
  rewrite_message_cache_control?: boolean;
  enable_client_dateline_normalization?: boolean;
  antigravity_user_agent_version?: string;
  openai_codex_user_agent?: string;
  // Payment configuration
  payment_enabled?: boolean;
  risk_control_enabled?: boolean;

  // Cyber session block
  cyber_session_block_enabled?: boolean;
  cyber_session_block_ttl_seconds?: number;

  payment_min_amount?: number;
  payment_max_amount?: number;
  payment_daily_limit?: number;
  payment_order_timeout_minutes?: number;
  payment_max_pending_orders?: number;
  payment_enabled_types?: string[];
  payment_balance_disabled?: boolean;
  payment_balance_recharge_multiplier?: number;
  payment_discount_rules?: DiscountRule[];
  payment_quick_amounts?: number[];
  payment_paid_user_rate_enabled?: boolean;
  payment_paid_user_rate_rules?: PaymentPaidUserRateRule[];
  payment_recharge_fee_rate?: number;
  payment_load_balance_strategy?: string;
  payment_product_name_prefix?: string;
  payment_product_name_suffix?: string;
  payment_help_image_url?: string;
  payment_help_text?: string;
  payment_cancel_rate_limit_enabled?: boolean;
  payment_cancel_rate_limit_max?: number;
  payment_cancel_rate_limit_window?: number;
  payment_cancel_rate_limit_unit?: string;
  payment_cancel_rate_limit_window_mode?: string;
  payment_alipay_force_qrcode?: boolean;
  payment_alipay_mobile_precreate_deep_link?: boolean;
  payment_visible_method_alipay_source?: string;
  payment_visible_method_wxpay_source?: string;
  payment_visible_method_alipay_enabled?: boolean;
  payment_visible_method_wxpay_enabled?: boolean;
  openai_low_upstream_rate_priority_enabled?: boolean;
  openai_oauth_scheduling_rate_multiplier?: number;
  openai_advanced_scheduler_enabled?: boolean;
  openai_advanced_scheduler_sticky_weighted_enabled?: boolean;
  openai_advanced_scheduler_subscription_priority_enabled?: boolean;
  openai_advanced_scheduler_lb_top_k?: string;
  openai_advanced_scheduler_weight_priority?: string;
  openai_advanced_scheduler_weight_load?: string;
  openai_advanced_scheduler_weight_queue?: string;
  openai_advanced_scheduler_weight_error_rate?: string;
  openai_advanced_scheduler_weight_ttft?: string;
  openai_advanced_scheduler_weight_reset?: string;
  openai_advanced_scheduler_weight_quota_headroom?: string;
  openai_advanced_scheduler_weight_upstream_cost?: string;
  openai_advanced_scheduler_weight_previous_response?: string;
  openai_advanced_scheduler_weight_session_sticky?: string;
  standalone_account_import_enabled?: boolean;
  standalone_account_import_password?: string;
  // 余额、订阅到期与账号限额通知
  balance_low_notify_enabled?: boolean;
  balance_low_notify_threshold?: number;
  balance_low_notify_recharge_url?: string;
  subscription_expiry_notify_enabled?: boolean;
  account_quota_notify_enabled?: boolean;
  account_quota_notify_emails?: NotifyEmailEntry[];

  // Channel Monitor feature switch
  channel_monitor_enabled?: boolean;
  channel_monitor_default_interval_seconds?: number;

  // Available Channels feature switch
  available_channels_enabled?: boolean;

  // Affiliate (邀请返利) feature switch
  affiliate_enabled?: boolean;

  // OpenAI fast/flex policy
  openai_fast_policy_settings?: OpenAIFastPolicySettings;
}

/**
 * Get all system settings
 * @returns System settings
 */
export async function getSettings(): Promise<SystemSettings> {
  const { data } = await apiClient.get<SystemSettings>("/admin/settings");
  return data;
}

/**
 * Update system settings
 * @param settings - Partial settings to update
 * @returns Updated settings
 */
export async function updateSettings(
  settings: UpdateSettingsRequest,
): Promise<SystemSettings> {
  const { data } = await apiClient.put<SystemSettings>(
    "/admin/settings",
    settings,
  );
  return data;
}

/**
 * Test SMTP connection request
 */
export interface TestSmtpRequest {
  smtp_host: string;
  smtp_port: number;
  smtp_username: string;
  smtp_password: string;
  smtp_use_tls: boolean;
}

/**
 * Test SMTP connection with provided config
 * @param config - SMTP configuration to test
 * @returns Test result message
 */
export async function testSmtpConnection(
  config: TestSmtpRequest,
): Promise<{ message: string }> {
  const { data } = await apiClient.post<{ message: string }>(
    "/admin/settings/test-smtp",
    config,
  );
  return data;
}

/**
 * Send test email request
 */
export interface SendTestEmailRequest {
  email: string;
  smtp_host: string;
  smtp_port: number;
  smtp_username: string;
  smtp_password: string;
  smtp_from_email: string;
  smtp_from_name: string;
  smtp_use_tls: boolean;
}

/**
 * Send test email with provided SMTP config
 * @param request - Email address and SMTP config
 * @returns Test result message
 */
export async function sendTestEmail(
  request: SendTestEmailRequest,
): Promise<{ message: string }> {
  const { data } = await apiClient.post<{ message: string }>(
    "/admin/settings/send-test-email",
    request,
  );
  return data;
}

// ==================== Email Template Settings ====================

export interface EmailTemplateOption {
  value: string;
  label?: string;
  description?: string;
  category?: string;
  optional?: boolean;
}

export type EmailTemplateEventOption = string | EmailTemplateOption;

export interface EmailTemplateSummary {
  event: string;
  locale: string;
  subject: string;
  is_custom?: boolean;
  updated_at?: string;
}

export interface EmailTemplateListResponse {
  events: EmailTemplateEventOption[];
  locales: string[];
  templates?: EmailTemplateSummary[];
  placeholders?: string[];
}

export interface EmailTemplateDetail {
  event: string;
  locale: string;
  subject: string;
  html: string;
  is_custom?: boolean;
  updated_at?: string;
  placeholders?: string[];
}

export interface UpdateEmailTemplateRequest {
  subject: string;
  html: string;
}

export interface PreviewEmailTemplateRequest extends UpdateEmailTemplateRequest {
  event: string;
  locale: string;
}

export interface EmailTemplatePreviewResponse {
  subject: string;
  html: string;
}

export async function getEmailTemplates(): Promise<EmailTemplateListResponse> {
  const { data } = await apiClient.get<EmailTemplateListResponse>(
    "/admin/settings/email-templates",
  );
  return data;
}

export async function getEmailTemplate(
  event: string,
  locale: string,
): Promise<EmailTemplateDetail> {
  const { data } = await apiClient.get<EmailTemplateDetail>(
    `/admin/settings/email-templates/${encodeURIComponent(event)}/${encodeURIComponent(locale)}`,
  );
  return data;
}

export async function updateEmailTemplate(
  event: string,
  locale: string,
  request: UpdateEmailTemplateRequest,
): Promise<EmailTemplateDetail> {
  const { data } = await apiClient.put<EmailTemplateDetail>(
    `/admin/settings/email-templates/${encodeURIComponent(event)}/${encodeURIComponent(locale)}`,
    request,
  );
  return data;
}

export async function restoreOfficialEmailTemplate(
  event: string,
  locale: string,
): Promise<EmailTemplateDetail> {
  const { data } = await apiClient.post<EmailTemplateDetail>(
    `/admin/settings/email-templates/${encodeURIComponent(event)}/${encodeURIComponent(locale)}/restore-official`,
  );
  return data;
}

export async function previewEmailTemplate(
  request: PreviewEmailTemplateRequest,
): Promise<EmailTemplatePreviewResponse> {
  const { data } = await apiClient.post<EmailTemplatePreviewResponse>(
    "/admin/settings/email-template-preview",
    request,
  );
  return data;
}

/**
 * Admin API Key status response
 */
export interface AdminApiKeyStatus {
  exists: boolean;
  masked_key: string;
}

/**
 * Get admin API key status
 * @returns Status indicating if key exists and masked version
 */
export async function getAdminApiKey(): Promise<AdminApiKeyStatus> {
  const { data } = await apiClient.get<AdminApiKeyStatus>(
    "/admin/settings/admin-api-key",
  );
  return data;
}

/**
 * Regenerate admin API key
 * @returns The new full API key (only shown once)
 */
export async function regenerateAdminApiKey(): Promise<{ key: string }> {
  const { data } = await apiClient.post<{ key: string }>(
    "/admin/settings/admin-api-key/regenerate",
  );
  return data;
}

/**
 * Delete admin API key
 * @returns Success message
 */
export async function deleteAdminApiKey(): Promise<{ message: string }> {
  const { data } = await apiClient.delete<{ message: string }>(
    "/admin/settings/admin-api-key",
  );
  return data;
}

// ==================== Overload Cooldown Settings ====================

/**
 * Overload cooldown settings interface (529 handling)
 */
export interface OverloadCooldownSettings {
  enabled: boolean;
  cooldown_minutes: number;
}

export async function getOverloadCooldownSettings(): Promise<OverloadCooldownSettings> {
  const { data } = await apiClient.get<OverloadCooldownSettings>(
    "/admin/settings/overload-cooldown",
  );
  return data;
}

export async function updateOverloadCooldownSettings(
  settings: OverloadCooldownSettings,
): Promise<OverloadCooldownSettings> {
  const { data } = await apiClient.put<OverloadCooldownSettings>(
    "/admin/settings/overload-cooldown",
    settings,
  );
  return data;
}

// ==================== 429 Rate Limit Cooldown Settings ====================

export interface RateLimit429CooldownSettings {
  enabled: boolean;
  cooldown_seconds: number;
}

export async function getRateLimit429CooldownSettings(): Promise<RateLimit429CooldownSettings> {
  const { data } = await apiClient.get<RateLimit429CooldownSettings>(
    "/admin/settings/rate-limit-429-cooldown",
  );
  return data;
}

export async function updateRateLimit429CooldownSettings(
  settings: RateLimit429CooldownSettings,
): Promise<RateLimit429CooldownSettings> {
  const { data } = await apiClient.put<RateLimit429CooldownSettings>(
    "/admin/settings/rate-limit-429-cooldown",
    settings,
  );
  return data;
}

// ==================== Panel Rate Limit Settings ====================

/**
 * Panel API rate limit settings.
 * Authenticated panel endpoints are limited per user account (reverse-proxy
 * safe); public endpoints are limited per publicly routable client IP.
 */
export interface PanelRateLimitSettings {
  enabled: boolean;
  user_rpm: number;
  heavy_rpm: number;
  exempt_admin: boolean;
  public_ip_rpm: number;
}

export async function getPanelRateLimitSettings(): Promise<PanelRateLimitSettings> {
  const { data } = await apiClient.get<PanelRateLimitSettings>(
    "/admin/settings/panel-rate-limit",
  );
  return data;
}

export async function updatePanelRateLimitSettings(
  settings: PanelRateLimitSettings,
): Promise<PanelRateLimitSettings> {
  const { data } = await apiClient.put<PanelRateLimitSettings>(
    "/admin/settings/panel-rate-limit",
    settings,
  );
  return data;
}

// ==================== Stream Timeout Settings ====================

/**
 * Stream timeout settings interface
 */
export interface StreamTimeoutSettings {
  enabled: boolean;
  action: "temp_unsched" | "error" | "none";
  temp_unsched_minutes: number;
  threshold_count: number;
  threshold_window_minutes: number;
}

/**
 * Get stream timeout settings
 * @returns Stream timeout settings
 */
export async function getStreamTimeoutSettings(): Promise<StreamTimeoutSettings> {
  const { data } = await apiClient.get<StreamTimeoutSettings>(
    "/admin/settings/stream-timeout",
  );
  return data;
}

/**
 * Update stream timeout settings
 * @param settings - Stream timeout settings to update
 * @returns Updated settings
 */
export async function updateStreamTimeoutSettings(
  settings: StreamTimeoutSettings,
): Promise<StreamTimeoutSettings> {
  const { data } = await apiClient.put<StreamTimeoutSettings>(
    "/admin/settings/stream-timeout",
    settings,
  );
  return data;
}

// ==================== Rectifier Settings ====================

/**
 * Rectifier settings interface
 */
export interface RectifierSettings {
  enabled: boolean;
  thinking_signature_enabled: boolean;
  thinking_budget_enabled: boolean;
  apikey_signature_enabled: boolean;
  apikey_signature_patterns: string[];
}

/**
 * Get rectifier settings
 * @returns Rectifier settings
 */
export async function getRectifierSettings(): Promise<RectifierSettings> {
  const { data } = await apiClient.get<RectifierSettings>(
    "/admin/settings/rectifier",
  );
  return data;
}

/**
 * Update rectifier settings
 * @param settings - Rectifier settings to update
 * @returns Updated settings
 */
export async function updateRectifierSettings(
  settings: RectifierSettings,
): Promise<RectifierSettings> {
  const { data } = await apiClient.put<RectifierSettings>(
    "/admin/settings/rectifier",
    settings,
  );
  return data;
}

// ==================== OpenAI Fast Policy Settings ====================

/**
 * OpenAI fast/flex policy rule interface.
 * Matches backend dto.OpenAIFastPolicyRule.
 */
export interface OpenAIFastPolicyRule {
  service_tier: "all" | "priority" | "flex";
  action: "pass" | "filter" | "block" | "force_priority";
  scope: "all" | "oauth" | "apikey" | "bedrock";
  user_ids?: number[];
  error_message?: string;
  model_whitelist?: string[];
  fallback_action?: "pass" | "filter" | "block" | "force_priority";
  fallback_error_message?: string;
}

/**
 * OpenAI fast/flex policy settings interface.
 */
export interface OpenAIFastPolicySettings {
  rules: OpenAIFastPolicyRule[];
}

// ==================== Beta Policy Settings ====================

/**
 * Beta policy rule interface
 */
export interface BetaPolicyRule {
  beta_token: string;
  action: "pass" | "filter" | "block";
  scope: "all" | "oauth" | "apikey" | "bedrock";
  error_message?: string;
  model_whitelist?: string[];
  fallback_action?: "pass" | "filter" | "block";
  fallback_error_message?: string;
}

/**
 * Beta policy settings interface
 */
export interface BetaPolicySettings {
  rules: BetaPolicyRule[];
}

/**
 * Get beta policy settings
 * @returns Beta policy settings
 */
export async function getBetaPolicySettings(): Promise<BetaPolicySettings> {
  const { data } = await apiClient.get<BetaPolicySettings>(
    "/admin/settings/beta-policy",
  );
  return data;
}

/**
 * Update beta policy settings
 * @param settings - Beta policy settings to update
 * @returns Updated settings
 */
export async function updateBetaPolicySettings(
  settings: BetaPolicySettings,
): Promise<BetaPolicySettings> {
  const { data } = await apiClient.put<BetaPolicySettings>(
    "/admin/settings/beta-policy",
    settings,
  );
  return data;
}

// --- Web Search Emulation Config ---

export interface WebSearchProviderConfig {
  type: "brave" | "tavily";
  api_key: string;
  api_key_configured: boolean;
  quota_limit: number | null;
  subscribed_at: number | null;
  quota_used?: number;
  proxy_id: number | null;
  expires_at: number | null;
}

export interface WebSearchEmulationConfig {
  enabled: boolean;
  providers: WebSearchProviderConfig[];
}

export interface WebSearchTestResult {
  provider: string;
  results: { url: string; title: string; snippet: string; page_age?: string }[];
  query: string;
}

export async function getWebSearchEmulationConfig(): Promise<WebSearchEmulationConfig> {
  const { data } = await apiClient.get<WebSearchEmulationConfig>(
    "/admin/settings/web-search-emulation",
  );
  return data;
}

export async function updateWebSearchEmulationConfig(
  config: WebSearchEmulationConfig,
): Promise<WebSearchEmulationConfig> {
  const { data } = await apiClient.put<WebSearchEmulationConfig>(
    "/admin/settings/web-search-emulation",
    config,
  );
  return data;
}

export async function testWebSearchEmulation(
  query: string,
): Promise<WebSearchTestResult> {
  const { data } = await apiClient.post<WebSearchTestResult>(
    "/admin/settings/web-search-emulation/test",
    { query },
  );
  return data;
}

export async function resetWebSearchUsage(payload: {
  provider_type: string;
}): Promise<void> {
  await apiClient.post(
    "/admin/settings/web-search-emulation/reset-usage",
    payload,
  );
}

export const settingsAPI = {
  getSettings,
  updateSettings,
  getAccountImportTemplates,
  updateAccountImportTemplates,
  testSmtpConnection,
  sendTestEmail,
  getEmailTemplates,
  getEmailTemplate,
  updateEmailTemplate,
  restoreOfficialEmailTemplate,
  previewEmailTemplate,
  getAdminApiKey,
  regenerateAdminApiKey,
  deleteAdminApiKey,
  getOverloadCooldownSettings,
  updateOverloadCooldownSettings,
  getRateLimit429CooldownSettings,
  updateRateLimit429CooldownSettings,
  getPanelRateLimitSettings,
  updatePanelRateLimitSettings,
  getStreamTimeoutSettings,
  updateStreamTimeoutSettings,
  getRectifierSettings,
  updateRectifierSettings,
  getBetaPolicySettings,
  updateBetaPolicySettings,
  getWebSearchEmulationConfig,
  updateWebSearchEmulationConfig,
  testWebSearchEmulation,
  resetWebSearchUsage,
};

export default settingsAPI;
