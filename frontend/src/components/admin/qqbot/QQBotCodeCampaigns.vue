<template>
  <div class="space-y-5">
    <section class="card space-y-4 p-5">
      <div class="flex flex-wrap items-center justify-between gap-3"><h2 class="text-lg font-semibold">{{ t('qqbot.codeCampaigns.title') }}</h2><div class="flex gap-2"><button class="btn btn-secondary" :disabled="busy" @click="load">{{ t('common.refresh') }}</button><button class="btn btn-primary" :disabled="busy || !config" @click="create">{{ t('qqbot.codeCampaigns.create') }}</button></div></div>
      <p class="text-sm">{{ t('qqbot.codeCampaigns.help') }} <code>/领码</code></p>
      <p v-if="error" role="alert" class="text-sm text-red-600">{{ error }}</p>
      <p v-if="notice" role="status" class="text-sm">{{ notice }}</p>
      <div class="overflow-x-auto"><table class="w-full whitespace-nowrap text-left text-sm"><thead class="border-b dark:border-dark-700"><tr><th class="py-2 pr-4">{{ t('qqbot.codeCampaigns.name') }}</th><th class="pr-4">{{ t('qqbot.codeCampaigns.content') }}</th><th class="pr-4">{{ t('qqbot.codeCampaigns.groups') }}</th><th class="pr-4">{{ t('qqbot.codeCampaigns.inventory') }}</th><th class="pr-4">{{ t('qqbot.codeCampaigns.sentRedeemed') }}</th><th>{{ t('qqBot.actions') }}</th></tr></thead><tbody>
        <tr v-for="c in campaigns" :key="c.id" class="border-b dark:border-dark-700"><td class="py-3 pr-4">{{ c.name }}<span class="block text-xs text-gray-500">{{ t(`qqbot.codeCampaignStates.${availability(c)}`) }}</span></td><td class="pr-4">{{ reward(c) }}</td><td class="max-w-72 whitespace-normal break-words pr-4">{{ c.groups.map(groupName).join('、') || '—' }}</td><td class="pr-4">{{ c.claimed }} / {{ c.stock_limit }}</td><td class="pr-4">{{ c.sent }} / {{ c.redeemed }}</td><td><div class="flex gap-2"><button class="btn btn-secondary" :disabled="busy" @click="edit(c)">{{ t('qqbot.codeCampaigns.edit') }}</button><button class="btn btn-secondary" :disabled="busy" @click="showClaims(c.id)">{{ t('qqbot.codeCampaigns.records') }}</button></div></td></tr>
      </tbody></table><p v-if="!campaigns.length && !busy" class="py-4 text-gray-500">{{ t('common.noData') }}</p></div>
      <form v-if="form" class="space-y-4 border-t pt-4 dark:border-dark-700" @submit.prevent="save">
        <fieldset :disabled="busy" class="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          <label class="text-sm">{{ t('qqbot.codeCampaigns.name') }}<input v-model.trim="form.name" class="input mt-1" required maxlength="60" /></label>
          <label class="text-sm">{{ t('qqbot.codeCampaigns.type') }}<select v-model="form.type" class="input mt-1" :disabled="form.claimed > 0"><option v-for="type in types" :key="type" :value="type">{{ t(`qqbot.codeCampaigns.types.${type}`) }}</option></select></label>
          <label v-if="form.type !== 'subscription'" class="text-sm">{{ t(form.type === 'balance' ? 'qqbot.codeCampaigns.balanceValue' : 'qqbot.codeCampaigns.countValue') }}<input v-model.number="form.value" class="input mt-1" type="number" :min="form.type === 'balance' ? 0.00000001 : 1" max="1000" :step="form.type === 'balance' ? '0.00000001' : '1'" required :disabled="form.claimed > 0" /></label>
          <template v-else><label class="text-sm">{{ t('qqbot.codeCampaigns.subscriptionGroup') }}<select v-model="form.subscription_group_id" class="input mt-1" required :disabled="form.claimed > 0"><option :value="null" disabled>{{ t('qqbot.codeCampaigns.chooseGroup') }}</option><option v-for="g in subscriptionGroups" :key="g.id" :value="g.id">{{ g.name }}</option></select></label><label class="text-sm">{{ t('qqbot.codeCampaigns.validityDays') }}<input v-model.number="form.validity_days" class="input mt-1" type="number" min="1" max="3650" required :disabled="form.claimed > 0" /></label></template>
          <label class="text-sm">{{ t('qqbot.codeCampaigns.frequency') }}<select v-model="form.claim_mode" class="input mt-1" :disabled="form.claimed > 0"><option value="once">{{ t('qqbot.codeCampaigns.once') }}</option><option value="daily">{{ t('qqbot.codeCampaigns.daily') }}</option></select></label>
          <label class="text-sm">{{ t('qqbot.codeCampaigns.stock') }}<input v-model.number="form.stock_limit" class="input mt-1" type="number" :min="Math.max(1, form.claimed)" max="100000" required /></label>
          <label class="text-sm">{{ t('qqbot.codeCampaigns.codeHours') }}<input v-model.number="form.code_valid_hours" class="input mt-1" type="number" min="1" max="8760" required /></label>
          <label class="text-sm">{{ t('qqbot.codeCampaigns.start') }}<input v-model="start" type="datetime-local" class="input mt-1" /></label>
          <label class="text-sm">{{ t('qqbot.codeCampaigns.end') }}<input v-model="end" type="datetime-local" class="input mt-1" /></label>
          <label class="flex items-center gap-2 text-sm"><input v-model="form.enabled" type="checkbox" />{{ t('qqbot.codeCampaigns.enabled') }}</label>
          <QQGroupPicker v-model="form.groups" :groups="availableGroups" :policies="config?.group_policies" :label="t('qqbot.codeCampaigns.groups')" class="md:col-span-2 xl:col-span-3" />
        </fieldset>
        <p class="text-sm text-gray-500">{{ t('qqbot.codeCampaigns.formHelp') }}</p>
        <p v-if="form.claimed" class="text-sm text-gray-500">{{ t('qqbot.codeCampaigns.lockedHelp') }}</p>
        <div class="flex gap-2"><button class="btn btn-primary" :disabled="busy">{{ t('common.save') }}</button><button type="button" class="btn btn-secondary" :disabled="busy" @click="form = undefined">{{ t('common.cancel') }}</button></div>
      </form>
    </section>
    <section ref="recordsSection" class="card space-y-4 p-5" :aria-busy="recordsLoading">
      <h2 class="text-lg font-semibold">{{ t('qqbot.codeCampaigns.records') }}</h2>
      <form class="flex flex-wrap items-end gap-3" @submit.prevent="page = 1; loadClaims()">
        <label class="text-sm">{{ t('qqbot.codeCampaigns.name') }}<select v-model="campaignID" class="input mt-1"><option value="">{{ t('qqbot.codeCampaigns.all') }}</option><option v-for="c in campaigns" :key="c.id" :value="c.id">{{ c.name }}</option></select></label>
        <label class="text-sm">{{ t('qqbot.activityStats.search') }}<input v-model="search" class="input mt-1" maxlength="254" /></label>
        <button class="btn btn-secondary" :disabled="recordsLoading">{{ t('qqbot.activityStats.query') }}</button><button type="button" class="btn btn-secondary" :disabled="exporting" @click="exportRows">{{ t('qqbot.activityStats.export') }}</button>
      </form>
      <p v-if="recordsError" role="alert" class="text-sm text-red-600">{{ recordsError }}</p>
      <div v-if="claims && !recordsError" class="overflow-x-auto"><table class="w-full whitespace-nowrap text-left text-sm"><thead class="border-b dark:border-dark-700"><tr><th class="py-2 pr-4">{{ t('qqbot.activityStats.account') }}</th><th class="pr-4">{{ t('qqbot.codeCampaigns.name') }}</th><th class="pr-4">{{ t('qqbot.codeCampaigns.content') }}</th><th class="pr-4">{{ t('qqbot.codeCampaigns.delivery') }}</th><th class="pr-4">{{ t('qqbot.codeCampaigns.redemption') }}</th><th class="pr-4">{{ t('qqbot.codeCampaigns.time') }}</th><th>{{ t('qqBot.actions') }}</th></tr></thead><tbody>
        <tr v-for="r in claims.items" :key="r.id" class="border-b dark:border-dark-700"><td class="py-3 pr-4">{{ r.email }}<span class="block text-xs text-gray-500">#{{ r.user_id }} · QQ {{ r.member_id }}</span></td><td class="pr-4">{{ r.campaign_name }}<span class="block text-xs text-gray-500">{{ r.group_id }}</span></td><td class="pr-4">{{ reward(r) }}<code class="block text-xs text-gray-500">{{ r.code_hint }}</code></td><td class="pr-4">{{ t(`qqbot.codeCampaigns.deliveryStates.${r.delivery_status}`) }}<span class="block text-xs text-gray-500">{{ r.attempts }} {{ t('qqbot.codeCampaigns.attempts') }}</span></td><td class="pr-4">{{ t(`qqbot.codeCampaigns.codeStates.${r.status}`) }}</td><td class="pr-4 text-xs">{{ formatTime(r.created_at) }}<span class="block text-gray-500">{{ t('qqbot.codeCampaigns.expires') }} {{ formatTime(r.expires_at) }}</span></td><td><div class="flex gap-2"><button class="btn btn-secondary" :disabled="!!actionID || r.status !== 'unused'" @click="act(r, 'resend')">{{ t('qqbot.codeCampaigns.resend') }}</button><button class="btn btn-secondary" :disabled="!!actionID || r.status !== 'unused'" @click="act(r, 'disable')">{{ t('qqbot.codeCampaigns.disable') }}</button></div></td></tr>
      </tbody></table><p v-if="!claims.items.length" class="py-4 text-gray-500">{{ t('common.noData') }}</p></div>
      <div v-if="claims" class="flex items-center justify-between text-sm"><span>{{ t('qqBot.page', { page: claims.page, total: claims.total }) }}</span><div class="flex gap-2"><button class="btn btn-secondary" :disabled="recordsLoading || claims.page <= 1" @click="page--; loadClaims()">{{ t('qqBot.previous') }}</button><button class="btn btn-secondary" :disabled="recordsLoading || claims.page * claims.page_size >= claims.total" @click="page++; loadClaims()">{{ t('qqBot.next') }}</button></div></div>
    </section>
  </div>
</template>
<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { saveAs } from 'file-saver'
import { qqbotAPI, type QQBotConfig, type QQCodeCampaign, type QQCodeClaim, type QQCodeClaimsPage } from '@/api/qqcommunity'
import { getAll } from '@/api/admin/groups'
import type { AdminGroup } from '@/types'
import QQGroupPicker from './QQGroupPicker.vue'
const { t } = useI18n()
const campaigns = ref<QQCodeCampaign[]>([]), config = ref<QQBotConfig>(), subscriptionGroups = ref<AdminGroup[]>([])
const form = ref<QQCodeCampaign>(), start = ref(''), end = ref(''), busy = ref(false), error = ref(''), notice = ref('')
const claims = ref<QQCodeClaimsPage>(), campaignID = ref(''), search = ref(''), page = ref(1), recordsLoading = ref(false), recordsError = ref(''), actionID = ref(''), exporting = ref(false)
const recordsSection = ref<HTMLElement>()
const now = ref(Date.now())
let statusTimer: number | undefined
const types = ['lottery_chances', 'balance', 'subscription', 'concurrency'] as const
let request = 0
const availableGroups = computed(() => [...new Set([...(config.value?.allowed_groups || []), ...(config.value?.group_policies?.map(g => g.group_id) || [])])])
const groupName = (id: string) => config.value?.group_policies?.find(g => g.group_id === id)?.label || id
const params = () => ({ campaign_id: campaignID.value, search: search.value, page: page.value, page_size: 20 })
const toInput = (time: string | null) => time ? new Date(new Date(time).getTime() + 8 * 3600000).toISOString().slice(0, 16) : ''
const toISO = (value: string) => value ? new Date(`${value}:00+08:00`).toISOString() : null
const formatTime = (time: string | null) => time ? new Date(time).toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai', hour12: false }) : '—'
function availability(c: QQCodeCampaign) {
 if (!c.enabled) return 'paused'
 if (c.ends_at && Date.parse(c.ends_at) <= now.value) return 'ended'
 if (c.starts_at && Date.parse(c.starts_at) > now.value) return 'scheduled'
 if (c.claimed >= c.stock_limit) return 'exhausted'
 if (!c.groups.length) return 'unassigned'
 if (config.value?.enabled === false) return 'botPaused'
 return 'open'
}
function reward(c: { type: string; value: number; validity_days: number }) { return `${t(`qqbot.codeCampaigns.types.${c.type}`)} · ${c.type === 'subscription' ? `${c.validity_days} ${t('qqbot.codeCampaigns.days')}` : c.type === 'balance' ? `$${c.value}` : c.value}` }
function create() { form.value = { id: crypto.randomUUID().replace(/-/g, ''), name: '', type: 'lottery_chances', value: 1, subscription_group_id: null, validity_days: 30, enabled: false, groups: [], claim_mode: 'once', stock_limit: 100, code_valid_hours: 168, starts_at: null, ends_at: null, version: 0, claimed: 0, sent: 0, redeemed: 0 }; start.value = ''; end.value = ''; notice.value = '' }
function edit(c: QQCodeCampaign) { form.value = { ...c, groups: [...c.groups] }; start.value = toInput(c.starts_at); end.value = toInput(c.ends_at); notice.value = '' }
watch(() => form.value?.type, () => { if (form.value && !form.value.claimed && form.value.type !== 'balance') form.value.value = Math.max(1, Math.trunc(form.value.value)) })
async function load() { busy.value = true; error.value = ''; try { const [list, cfg, groups] = await Promise.all([qqbotAPI.codeCampaigns(), qqbotAPI.config(), getAll()]); campaigns.value = list; config.value = cfg; subscriptionGroups.value = groups.filter(g => g.subscription_type === 'subscription' && g.status === 'active') } catch (e) { error.value = (e as Error).message } finally { busy.value = false } }
async function save() {
 if (!form.value || busy.value) return
 busy.value = true; error.value = ''; notice.value = ''
 try { const saved = await qqbotAPI.saveCodeCampaign({ ...form.value, starts_at: toISO(start.value), ends_at: toISO(end.value) }); form.value = undefined; notice.value = t('qqbot.codeCampaigns.saved'); campaigns.value = await qqbotAPI.codeCampaigns(); campaignID.value = saved.id; page.value = 1; await loadClaims() }
 catch (e) { error.value = (e as Error).message; if ((e as {status?: number}).status === 409) { campaigns.value = await qqbotAPI.codeCampaigns() } }
 finally { busy.value = false }
}
async function loadClaims() { const seq = ++request; recordsLoading.value = true; recordsError.value = ''; try { const result = await qqbotAPI.codeClaims(params()); if (seq === request) claims.value = result } catch (e) { if (seq === request) recordsError.value = (e as Error).message } finally { if (seq === request) recordsLoading.value = false } }
function showClaims(id: string) { campaignID.value = id; page.value = 1; void loadClaims(); recordsSection.value?.scrollIntoView({ block: 'start' }) }
async function act(r: QQCodeClaim, action: 'resend' | 'disable') {
 if (actionID.value || !window.confirm(t(`qqbot.codeCampaigns.${action}Confirm`, { user: r.email || r.user_id }))) return
 actionID.value = r.id; recordsError.value = ''; notice.value = ''
 try { if (action === 'resend') notice.value = (await qqbotAPI.resendCodeClaim(r.id)).message; else { await qqbotAPI.disableCodeClaim(r.id); notice.value = t('qqbot.codeCampaigns.disabled') }; await loadClaims(); campaigns.value = await qqbotAPI.codeCampaigns() }
 catch (e) { recordsError.value = (e as Error).message } finally { actionID.value = '' }
}
async function exportRows() { exporting.value = true; recordsError.value = ''; try { saveAs(await qqbotAPI.exportCodeClaims(params()), 'code-claims.csv') } catch (e) { recordsError.value = (e as Error).message } finally { exporting.value = false } }
onMounted(() => { void load(); void loadClaims(); statusTimer = window.setInterval(() => { now.value = Date.now() }, 30000) })
onBeforeUnmount(() => { ++request; window.clearInterval(statusTimer) })
</script>
