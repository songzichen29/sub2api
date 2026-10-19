import { describe, expect, it } from "vitest";

import {
  appendAuthSourceDefaultsToUpdateRequest,
  buildAuthSourceDefaultsState,
  normalizePlatformQuotasMap,
  sanitizePlatformQuotasMap,
  type AuthSourceDefaultsState,
  type AuthSourceDefaultsValue,
  type AuthSourceType,
  type DefaultPlatformQuotasMap,
  type UpdateSettingsRequest,
} from "@/api/admin/settings";
import { listPlatformIds } from "@/constants/platformCatalog";

const allNullQuotas = normalizePlatformQuotasMap();

function completeAuthSourceDefaults(
  overrides: Partial<Record<AuthSourceType, Partial<AuthSourceDefaultsValue>>>,
): AuthSourceDefaultsState {
  const value = (source: AuthSourceType): AuthSourceDefaultsValue => ({
    balance: 0,
    concurrency: 5,
    subscriptions: [],
    grant_on_signup: false,
    grant_on_first_bind: false,
    platform_quotas: {},
    ...overrides[source],
  });

  return {
    email: value("email"),
    linuxdo: value("linuxdo"),
    oidc: value("oidc"),
    wechat: value("wechat"),
    github: value("github"),
    google: value("google"),
    dingtalk: value("dingtalk"),
  };
}
/** 与后端 AllowedQuotaPlatforms 一致的全部具体平台（平台清单）。 */
const quotaPlatforms = listPlatformIds();

/** 全部平台全 null 的 map，用于断言归一化默认值 */
const expectedNullQuotas: DefaultPlatformQuotasMap = Object.fromEntries(
  quotaPlatforms.map((platform) => [platform, { daily: null, weekly: null, monthly: null }]),
)

describe("admin settings auth source defaults helpers", () => {
  it("builds auth source defaults state from flat settings fields", () => {
    const state = buildAuthSourceDefaultsState({
      auth_source_default_email_balance: 9.5,
      auth_source_default_email_concurrency: 3,
      auth_source_default_email_subscriptions: [
        { group_id: 1, validity_days: 30 },
      ],
      auth_source_default_email_grant_on_signup: false,
      auth_source_default_email_grant_on_first_bind: true,
      auth_source_default_linuxdo_balance: 6,
      auth_source_default_linuxdo_concurrency: 8,
      auth_source_default_linuxdo_subscriptions: [
        { group_id: 2, validity_days: 60 },
      ],
      auth_source_default_linuxdo_grant_on_signup: true,
      auth_source_default_linuxdo_grant_on_first_bind: false,
    });

    expect(state.email).toEqual({
      balance: 9.5,
      concurrency: 3,
      subscriptions: [{ group_id: 1, validity_days: 30, mode: "days" }],
      grant_on_signup: false,
      grant_on_first_bind: true,
      platform_quotas: allNullQuotas,
    });
    expect(state.linuxdo).toEqual({
      balance: 6,
      concurrency: 8,
      subscriptions: [{ group_id: 2, validity_days: 60, mode: "days" }],
      grant_on_signup: true,
      grant_on_first_bind: false,
      platform_quotas: allNullQuotas,
    });
    expect(state.oidc).toEqual({
      balance: 0,
      concurrency: 5,
      subscriptions: [],
      grant_on_signup: false,
      grant_on_first_bind: false,
      platform_quotas: allNullQuotas,
    });
    expect(state.wechat).toEqual({
      balance: 0,
      concurrency: 5,
      subscriptions: [],
      grant_on_signup: false,
      grant_on_first_bind: false,
      platform_quotas: allNullQuotas,
    });
  });

  it("defaults grant-on-signup to disabled when settings are missing", () => {
    const state = buildAuthSourceDefaultsState({});

    expect(state.email.grant_on_signup).toBe(false);
    expect(state.linuxdo.grant_on_signup).toBe(false);
    expect(state.oidc.grant_on_signup).toBe(false);
    expect(state.wechat.grant_on_signup).toBe(false);
  });

  it("appends auth source defaults back onto update payload", () => {
    const payload: UpdateSettingsRequest = {
      site_name: "Sub2API",
    };

    appendAuthSourceDefaultsToUpdateRequest(payload, completeAuthSourceDefaults({
      email: {
        balance: 1.25,
        concurrency: 2,
        subscriptions: [{ group_id: 3, validity_days: 7 }],
        grant_on_signup: true,
        grant_on_first_bind: false,
      },
      linuxdo: {
        balance: 0,
        concurrency: 6,
        subscriptions: [],
        grant_on_signup: false,
        grant_on_first_bind: true,
      },
      oidc: {
        balance: 4,
        concurrency: 9,
        subscriptions: [{ group_id: 9, validity_days: 90 }],
        grant_on_signup: true,
        grant_on_first_bind: true,
      },
      wechat: {
        balance: 2,
        concurrency: 5,
        subscriptions: [],
        grant_on_signup: false,
        grant_on_first_bind: false,
      },
    }));

    expect(payload).toMatchObject({
      site_name: "Sub2API",
      auth_source_default_email_balance: 1.25,
      auth_source_default_email_concurrency: 2,
      auth_source_default_email_subscriptions: [
        { group_id: 3, validity_days: 7 },
      ],
      auth_source_default_email_grant_on_signup: true,
      auth_source_default_email_grant_on_first_bind: false,
      auth_source_default_linuxdo_balance: 0,
      auth_source_default_linuxdo_concurrency: 6,
      auth_source_default_linuxdo_subscriptions: [],
      auth_source_default_linuxdo_grant_on_signup: false,
      auth_source_default_linuxdo_grant_on_first_bind: true,
      auth_source_default_oidc_balance: 4,
      auth_source_default_oidc_concurrency: 9,
      auth_source_default_oidc_subscriptions: [
        { group_id: 9, validity_days: 90 },
      ],
      auth_source_default_oidc_grant_on_signup: true,
      auth_source_default_oidc_grant_on_first_bind: true,
      auth_source_default_wechat_balance: 2,
      auth_source_default_wechat_concurrency: 5,
      auth_source_default_wechat_subscriptions: [],
      auth_source_default_wechat_grant_on_signup: false,
      auth_source_default_wechat_grant_on_first_bind: false,
    });
  });

  it("keeps explicit time range subscriptions when appending payload", () => {
    const payload: UpdateSettingsRequest = {};

    appendAuthSourceDefaultsToUpdateRequest(payload, completeAuthSourceDefaults({
      email: {
        balance: 0,
        concurrency: 5,
        subscriptions: [
          {
            group_id: 10,
            starts_at: "2030-01-01T00:00:00Z",
            expires_at: "2030-02-01T00:00:00Z",
            mode: "range",
          },
        ],
        grant_on_signup: true,
        grant_on_first_bind: false,
      },
      linuxdo: {
        balance: 0,
        concurrency: 5,
        subscriptions: [],
        grant_on_signup: false,
        grant_on_first_bind: false,
      },
      oidc: {
        balance: 0,
        concurrency: 5,
        subscriptions: [],
        grant_on_signup: false,
        grant_on_first_bind: false,
      },
      wechat: {
        balance: 0,
        concurrency: 5,
        subscriptions: [],
        grant_on_signup: false,
        grant_on_first_bind: false,
      },
    }));

    expect(payload.auth_source_default_email_subscriptions).toEqual([
      {
        group_id: 10,
        starts_at: "2030-01-01T00:00:00.000Z",
        expires_at: "2030-02-01T00:00:00.000Z",
        mode: "range",
      },
    ]);
  });
});

describe("platform quota helpers", () => {
  it("normalizes and sanitizes every supported platform", () => {
    const normalized = normalizePlatformQuotasMap({ anthropic: { daily: 5, weekly: null, monthly: null } });
    expect(normalized.anthropic).toEqual({ daily: 5, weekly: null, monthly: null });
    expect(normalized.openai).toEqual({ daily: null, weekly: null, monthly: null });
    expect(Object.keys(normalized)).toEqual(quotaPlatforms);
    expect(normalizePlatformQuotasMap()).toEqual(expectedNullQuotas);

    const sanitized = sanitizePlatformQuotasMap({
      anthropic: { daily: 10.5, weekly: 0, monthly: null },
      openai: { daily: -1, weekly: Number.NaN, monthly: Number.POSITIVE_INFINITY },
    });
    expect(sanitized.anthropic).toEqual({ daily: 10.5, weekly: 0, monthly: null });
    expect(sanitized.openai).toEqual({ daily: null, weekly: null, monthly: null });
    expect(Object.keys(sanitized)).toEqual(quotaPlatforms);
  });

  it("appends normalized platform quotas to every auth source", () => {
    const payload: UpdateSettingsRequest = {};
    const state = completeAuthSourceDefaults({
      email: { platform_quotas: { anthropic: { daily: 10, weekly: 50, monthly: 200 }, openai: { daily: 0, weekly: null, monthly: null } } },
    });
    appendAuthSourceDefaultsToUpdateRequest(payload, state);
    const quotas = (payload as Record<string, unknown>)["auth_source_default_email_platform_quotas"] as DefaultPlatformQuotasMap;
    expect(quotas.anthropic).toEqual({ daily: 10, weekly: 50, monthly: 200 });
    expect(quotas.openai?.daily).toBe(0);
    expect(quotas.gemini).toEqual({ daily: null, weekly: null, monthly: null });
  });
});

describe("normalizePlatformQuotasMap", () => {
  it("填充缺失的平台为全 null 三档", () => {
    const result = normalizePlatformQuotasMap({ anthropic: { daily: 5, weekly: null, monthly: null } });
    expect(result.anthropic).toEqual({ daily: 5, weekly: null, monthly: null });
    expect(result.openai).toEqual({ daily: null, weekly: null, monthly: null });
    expect(result.gemini).toEqual({ daily: null, weekly: null, monthly: null });
    expect(result.antigravity).toEqual({ daily: null, weekly: null, monthly: null });
    expect(result.grok).toEqual({ daily: null, weekly: null, monthly: null });
    expect(result.kimi).toEqual({ daily: null, weekly: null, monthly: null });
    expect(result.opencode_go).toEqual({ daily: null, weekly: null, monthly: null });
  });

  it("无参数时返回平台清单中的全部平台全 null", () => {
    const result = normalizePlatformQuotasMap();
    expect(listPlatformIds()).toEqual(quotaPlatforms);
    expect(Object.keys(result)).toEqual(quotaPlatforms);
    for (const v of Object.values(result)) {
      expect(v).toEqual({ daily: null, weekly: null, monthly: null });
    }
  });

  it("非 number 类型的值归一化为 null", () => {
    const result = normalizePlatformQuotasMap({
      anthropic: { daily: "50" as unknown as number, weekly: undefined as unknown as number, monthly: null },
    });
    expect(result.anthropic).toEqual({ daily: null, weekly: null, monthly: null });
  });
});

describe("sanitizePlatformQuotasMap", () => {
  it("保留合法的正数和零值", () => {
    const result = sanitizePlatformQuotasMap({
      anthropic: { daily: 10.5, weekly: 0, monthly: null },
    });
    expect(result.anthropic?.daily).toBe(10.5);
    expect(result.anthropic?.weekly).toBe(0);
    expect(result.anthropic?.monthly).toBe(null);
  });

  it("空字符串（v-model.number 空输入）清洗为 null", () => {
    const result = sanitizePlatformQuotasMap({
      anthropic: { daily: "" as unknown as number, weekly: null, monthly: null },
    });
    expect(result.anthropic?.daily).toBe(null);
  });

  it("负数清洗为 null", () => {
    const result = sanitizePlatformQuotasMap({
      openai: { daily: -1, weekly: null, monthly: null },
    });
    expect(result.openai?.daily).toBe(null);
  });

  it("NaN/Infinity 清洗为 null", () => {
    const result = sanitizePlatformQuotasMap({
      gemini: { daily: NaN, weekly: Infinity, monthly: null },
    });
    expect(result.gemini?.daily).toBe(null);
    expect(result.gemini?.weekly).toBe(null);
  });

  it("缺失平台填充为全 null", () => {
    const result = sanitizePlatformQuotasMap({});
    expect(Object.keys(result)).toEqual(quotaPlatforms);
    for (const v of Object.values(result)) {
      expect(v).toEqual({ daily: null, weekly: null, monthly: null });
    }
  });
});
