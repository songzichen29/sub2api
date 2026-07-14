<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <!-- Top Toolbar: Left (search + filters) / Right (actions) -->
        <div class="flex flex-wrap items-start justify-between gap-4">
          <!-- Left: Fuzzy user search + filters (wrap to multiple lines) -->
          <div class="flex flex-1 flex-wrap items-center gap-3">
            <!-- User Search -->
            <div
              class="relative w-full sm:w-64"
              data-filter-user-search
            >
              <Icon
                name="search"
                size="md"
                class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400"
              />
              <input
                v-model="filterUserKeyword"
                type="text"
                :placeholder="t('admin.users.searchUsers')"
                class="input pl-10 pr-8"
                @input="debounceSearchFilterUsers"
                @focus="showFilterUserDropdown = true"
              />
              <button
                v-if="selectedFilterUser"
                @click="clearFilterUser"
                type="button"
                class="absolute right-2 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
                :title="t('common.clear')"
              >
                <Icon name="x" size="sm" :stroke-width="2" />
              </button>

              <!-- User Dropdown -->
              <div
                v-if="showFilterUserDropdown && (filterUserResults.length > 0 || filterUserKeyword)"
                class="absolute z-50 mt-1 max-h-60 w-full overflow-auto rounded-lg border border-gray-200 bg-white shadow-lg dark:border-gray-700 dark:bg-gray-800"
              >
                <div
                  v-if="filterUserLoading"
                  class="px-4 py-3 text-sm text-gray-500 dark:text-gray-400"
                >
                  {{ t('common.loading') }}
                </div>
                <div
                  v-else-if="filterUserResults.length === 0 && filterUserKeyword"
                  class="px-4 py-3 text-sm text-gray-500 dark:text-gray-400"
                >
                  {{ t('common.noOptionsFound') }}
                </div>
                <button
                  v-for="user in filterUserResults"
                  :key="user.id"
                  type="button"
                  @click="selectFilterUser(user)"
                  class="w-full px-4 py-2 text-left text-sm hover:bg-gray-100 dark:hover:bg-gray-700"
                >
                  <span class="font-medium text-gray-900 dark:text-white">{{ user.email }}</span>
                  <span class="ml-2 text-gray-500 dark:text-gray-400">#{{ user.id }}</span>
                </button>
              </div>
            </div>

            <!-- Filters -->
            <div class="w-full sm:w-40">
              <Select
                v-model="filters.status"
                :options="statusOptions"
                :placeholder="t('admin.subscriptions.allStatus')"
                @change="applyFilters"
              />
            </div>
            <div class="w-full sm:w-48">
              <Select
                v-model="filters.group_id"
                :options="groupOptions"
                :placeholder="t('admin.subscriptions.allGroups')"
                @change="applyFilters"
              />
            </div>
            <div class="w-full sm:w-40">
              <Select
                v-model="filters.platform"
                :options="platformFilterOptions"
                :placeholder="t('admin.subscriptions.allPlatforms')"
                @change="applyFilters"
              />
            </div>
          </div>

          <!-- Right: Actions -->
          <div class="ml-auto flex flex-wrap items-center justify-end gap-3">
            <button
              @click="loadSubscriptions"
              :disabled="loading"
              class="btn btn-secondary"
              :title="t('common.refresh')"
            >
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
            <!-- Column Settings Dropdown -->
            <div class="relative" ref="columnDropdownRef">
              <button
                @click="showColumnDropdown = !showColumnDropdown"
                class="btn btn-secondary px-2 md:px-3"
                :title="t('admin.users.columnSettings')"
              >
                <svg class="h-4 w-4 md:mr-1.5" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M9 4.5v15m6-15v15m-10.875 0h15.75c.621 0 1.125-.504 1.125-1.125V5.625c0-.621-.504-1.125-1.125-1.125H4.125C3.504 4.5 3 5.004 3 5.625v12.75c0 .621.504 1.125 1.125 1.125z" />
                </svg>
                <span class="hidden md:inline">{{ t('admin.users.columnSettings') }}</span>
              </button>
              <!-- Dropdown menu -->
              <div
                v-if="showColumnDropdown"
                class="absolute right-0 z-50 mt-2 w-48 origin-top-right rounded-lg border border-gray-200 bg-white shadow-lg dark:border-gray-700 dark:bg-gray-800"
              >
                <div class="p-2">
                  <!-- User column mode selection -->
                  <div class="mb-2 border-b border-gray-200 pb-2 dark:border-gray-700">
                    <div class="px-3 py-1 text-xs font-medium text-gray-500 dark:text-gray-400">
                      {{ t('admin.subscriptions.columns.user') }}
                    </div>
                    <button
                      @click="setUserColumnMode('email')"
                      class="flex w-full items-center justify-between rounded-md px-3 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-200 dark:hover:bg-gray-700"
                    >
                      <span>{{ t('admin.users.columns.email') }}</span>
                      <Icon v-if="userColumnMode === 'email'" name="check" size="sm" class="text-primary-500" />
                    </button>
                    <button
                      @click="setUserColumnMode('username')"
                      class="flex w-full items-center justify-between rounded-md px-3 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-200 dark:hover:bg-gray-700"
                    >
                      <span>{{ t('admin.users.columns.username') }}</span>
                      <Icon v-if="userColumnMode === 'username'" name="check" size="sm" class="text-primary-500" />
                    </button>
                  </div>
                  <!-- Other columns toggle -->
                  <button
                    v-for="col in toggleableColumns"
                    :key="col.key"
                    @click="toggleColumn(col.key)"
                    class="flex w-full items-center justify-between rounded-md px-3 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-200 dark:hover:bg-gray-700"
                  >
                    <span>{{ col.label }}</span>
                    <Icon v-if="isColumnVisible(col.key)" name="check" size="sm" class="text-primary-500" />
                  </button>
                </div>
              </div>
            </div>
            <button
              @click="showGuideModal = true"
              class="btn btn-secondary"
              :title="t('admin.subscriptions.guide.showGuide')"
            >
              <Icon name="questionCircle" size="md" />
            </button>
            <button @click="showAssignModal = true" class="btn btn-primary">
              <Icon name="plus" size="md" class="mr-2" />
              {{ t('admin.subscriptions.assignSubscription') }}
            </button>
          </div>
        </div>
      </template>

      <!-- Subscriptions Table -->
      <template #table>
        <DataTable
          :columns="columns"
          :data="subscriptions"
          :loading="loading"
          :server-side-sort="true"
          default-sort-key="created_at"
          default-sort-order="desc"
          @sort="handleSort"
        >
          <template #cell-user="{ row }">
            <div class="flex items-center gap-2">
              <div
                class="flex h-8 w-8 items-center justify-center rounded-full bg-primary-100 dark:bg-primary-900/30"
              >
                <span class="text-sm font-medium text-primary-700 dark:text-primary-300">
                  {{ userColumnMode === 'email'
                    ? (row.user?.email?.charAt(0).toUpperCase() || '?')
                    : (row.user?.username?.charAt(0).toUpperCase() || '?')
                  }}
                </span>
              </div>
              <span class="font-medium text-gray-900 dark:text-white">
                {{ userColumnMode === 'email'
                  ? (row.user?.email || t('admin.redeem.userPrefix', { id: row.user_id }))
                  : (row.user?.username || '-')
                }}
              </span>
            </div>
          </template>

          <template #cell-group="{ row }">
            <GroupBadge
              v-if="row.group"
              :name="row.group.name"
              :platform="row.group.platform"
              :subscription-type="row.group.subscription_type"
              :rate-multiplier="row.group.rate_multiplier"
              :show-rate="false"
            />
            <span v-else class="text-sm text-gray-400 dark:text-dark-500">-</span>
          </template>

          <template #cell-starts_at="{ value }">
            <span v-if="value" class="text-sm text-gray-700 dark:text-gray-300">
              {{ formatDateOnly(value) }}
            </span>
            <span v-else class="text-sm text-gray-500">-</span>
          </template>

          <template #cell-last_used_at="{ value }">
            <span v-if="value" class="text-sm text-gray-700 dark:text-gray-300">
              {{ formatDateTime(value) }}
            </span>
            <span v-else class="text-sm text-gray-400 dark:text-dark-500">-</span>
          </template>

          <template #cell-usage="{ row }">
            <div class="min-w-[280px] space-y-2">
              <!-- Total Quota -->
              <div v-if="hasTotalQuota(row)" class="usage-row">
                <div class="flex items-center gap-2">
                  <span class="usage-label">{{ t('admin.subscriptions.totalQuota') }}</span>
                  <div class="h-1.5 flex-1 rounded-full bg-gray-200 dark:bg-dark-600">
                    <div
                      class="h-1.5 rounded-full transition-all"
                      :class="getProgressClass(getTotalQuotaUsed(row), getTotalQuotaLimit(row))"
                      :style="{
                        width: getProgressWidth(getTotalQuotaUsed(row), getTotalQuotaLimit(row))
                      }"
                    ></div>
                  </div>
                  <span class="usage-amount">
                    ${{ getTotalQuotaUsed(row).toFixed(2) }}
                    <span class="text-gray-400">/</span>
                    ${{ getTotalQuotaLimit(row).toFixed(2) }}
                  </span>
                </div>
                <div class="reset-info">
                  <svg
                    class="h-3 w-3"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                    stroke-width="2"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      d="M12 8c-1.657 0-3 .895-3 2s1.343 2 3 2 3 .895 3 2-1.343 2-3 2m0-8V6m0 10v2"
                    />
                  </svg>
                  <span>
                    {{
                      t('admin.subscriptions.totalQuotaRemaining', {
                        amount: getTotalQuotaRemaining(row).toFixed(2),
                      })
                    }}
                  </span>
                </div>
              </div>

              <!-- Daily Usage -->
              <div v-if="row.group?.daily_limit_usd" class="usage-row">
                <div class="flex items-center gap-2">
                  <span class="usage-label">{{ t('admin.subscriptions.daily') }}</span>
                  <div class="h-1.5 flex-1 rounded-full bg-gray-200 dark:bg-dark-600">
                    <div
                      class="h-1.5 rounded-full transition-all"
                      :class="getProgressClass(row.daily_usage_usd, row.group?.daily_limit_usd)"
                      :style="{
                        width: getProgressWidth(row.daily_usage_usd, row.group?.daily_limit_usd)
                      }"
                    ></div>
                  </div>
                  <span class="usage-amount">
                    ${{ row.daily_usage_usd?.toFixed(2) || '0.00' }}
                    <span class="text-gray-400">/</span>
                    ${{ row.group?.daily_limit_usd?.toFixed(2) }}
                  </span>
                </div>
                <div class="reset-info" v-if="row.daily_window_start">
                  <svg
                    class="h-3 w-3"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                    stroke-width="2"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"
                    />
                  </svg>
                  <span>{{ formatResetTime(row.daily_window_start, 'daily') }}</span>
                </div>
              </div>

              <!-- Overdraft / total pool usage -->
              <div v-if="getOverdraftLimit(row)" class="usage-row">
                <div class="flex items-center gap-2">
                  <span class="usage-label">{{ t('admin.subscriptions.overdraftTotal') }}</span>
                  <div class="h-1.5 flex-1 rounded-full bg-gray-200 dark:bg-dark-600">
                    <div
                      class="h-1.5 rounded-full transition-all"
                      :class="getProgressClass(getOverdraftDisplayUsed(row) ?? 0, getOverdraftLimit(row))"
                      :style="{
                        width: getProgressWidth(getOverdraftDisplayUsed(row) ?? 0, getOverdraftLimit(row))
                      }"
                    ></div>
                  </div>
                  <span class="usage-amount">
                    ${{ ((getOverdraftDisplayUsed(row) ?? 0) || 0).toFixed(2) }}
                    <span class="text-gray-400">/</span>
                    ${{ getOverdraftLimit(row)?.toFixed(2) }}
                  </span>
                </div>
                <div v-if="row.allow_daily_overdraft" class="reset-info">
                  <svg
                    class="h-3 w-3"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                    stroke-width="2"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z"
                    />
                  </svg>
                  <span>
                    {{
                      t('admin.subscriptions.todayOverdraftAmount', {
                        amount: getTodayOverdraftAmount(row).toFixed(2),
                      })
                    }}
                  </span>
                </div>
                <div v-if="getOverdraftLimit(row)" class="reset-info">
                  <svg
                    class="h-3 w-3"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                    stroke-width="2"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      d="M12 8c-1.657 0-3 .895-3 2s1.343 2 3 2 3 .895 3 2-1.343 2-3 2m0-8V6m0 10v2"
                    />
                  </svg>
                  <span>
                    {{ t('admin.subscriptions.overdraftRemaining') }}:
                    ${{ getOverdraftRemaining(row).toFixed(2) }}
                  </span>
                </div>
              </div>

              <!-- No Limits - Unlimited badge -->
              <div
                v-if="
                  !row.group?.daily_limit_usd &&
                  !row.group?.weekly_limit_usd &&
                  !row.group?.monthly_limit_usd &&
                  !row.allow_daily_overdraft &&
                  !hasTotalQuota(row)
                "
                class="flex items-center gap-2 rounded-lg bg-gradient-to-r from-emerald-50 to-teal-50 px-3 py-2 dark:from-emerald-900/20 dark:to-teal-900/20"
              >
                <span class="text-lg text-emerald-600 dark:text-emerald-400">∞</span>
                <span class="text-xs font-medium text-emerald-700 dark:text-emerald-300">
                  {{ t('admin.subscriptions.unlimited') }}
                </span>
              </div>
            </div>
          </template>

          <template #cell-expires_at="{ value }">
            <div v-if="value">
              <span
                class="text-sm"
                :class="
                  isExpiringSoon(value)
                    ? 'text-orange-600 dark:text-orange-400'
                    : 'text-gray-700 dark:text-gray-300'
                "
              >
                {{ formatDateOnly(value) }}
              </span>
              <div v-if="getRemainingText(value)" class="text-xs text-gray-500">
                {{ getRemainingText(value) }}
              </div>
            </div>
            <span v-else class="text-sm text-gray-500">{{
              t('admin.subscriptions.noExpiration')
            }}</span>
          </template>

          <template #cell-status="{ row, value }">
            <span
              :class="[
                'badge',
                isSubscriptionNotStarted(row)
                  ? 'badge-warning'
                  : value === 'active'
                  ? 'badge-success'
                  : value === 'expired' || value === 'quota_exhausted'
                    ? 'badge-warning'
                    : 'badge-danger'
              ]"
            >
              {{ isSubscriptionNotStarted(row) ? t('admin.subscriptions.status.not_started') : t(`admin.subscriptions.status.${value}`) }}
            </span>
          </template>

          <template #cell-actions="{ row }">
            <div class="flex items-center gap-1">
              <button
                v-if="row.status === 'active' || row.status === 'expired' || row.status === 'quota_exhausted'"
                @click="handleExtend(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-blue-50 hover:text-blue-600 dark:hover:bg-blue-900/20 dark:hover:text-blue-400"
              >
                <Icon name="calendar" size="sm" />
                <span class="text-xs">{{ t('admin.subscriptions.adjust') }}</span>
              </button>
              <button
                @click="handleOrderUsage(row)"
                :disabled="orderUsageLoading && orderUsageSubscription?.id === row.id"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-indigo-50 hover:text-indigo-600 disabled:cursor-not-allowed disabled:opacity-50 dark:hover:bg-indigo-900/20 dark:hover:text-indigo-400"
              >
                <Icon name="document" size="sm" />
                <span class="text-xs">订单用量</span>
              </button>
              <button
                v-if="row.status === 'active'"
                @click="handleResetQuota(row)"
                :disabled="resettingQuota && resettingSubscription?.id === row.id"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-orange-50 hover:text-orange-600 dark:hover:bg-orange-900/20 dark:hover:text-orange-400 disabled:cursor-not-allowed disabled:opacity-50"
              >
                <Icon name="refresh" size="sm" />
                <span class="text-xs">{{ t('admin.subscriptions.resetQuota') }}</span>
              </button>
              <button
                v-if="row.status === 'active'"
                @click="toggleWeekendSkip(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-50 hover:text-gray-700 dark:hover:bg-dark-700 dark:hover:text-gray-200"
              >
                <Icon name="calendar" size="sm" />
                <span class="text-xs">{{ row.skip_weekends ? '关闭周末' : '跳过周末' }}</span>
              </button>
              <button
                v-if="row.weekend_skip_user_changed_at"
                @click="resetWeekendSkipChange(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-50 hover:text-gray-700 dark:hover:bg-dark-700 dark:hover:text-gray-200"
              >
                <Icon name="refresh" size="sm" />
                <span class="text-xs">重置机会</span>
              </button>
              <button
                v-if="row.status === 'active' || row.status === 'quota_exhausted'"
                @click="handleRevoke(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400"
              >
                <Icon name="ban" size="sm" />
                <span class="text-xs">{{ t('admin.subscriptions.revoke') }}</span>
              </button>
            </div>
          </template>

          <template #empty>
            <EmptyState
              :title="t('admin.subscriptions.noSubscriptionsYet')"
              :description="t('admin.subscriptions.assignFirstSubscription')"
              :action-text="t('admin.subscriptions.assignSubscription')"
              @action="showAssignModal = true"
            />
          </template>
        </DataTable>
      </template>

      <!-- Pagination -->
      <template #pagination>
      <Pagination
        v-if="pagination.total > 0"
        :page="pagination.page"
        :total="pagination.total"
        :page-size="pagination.page_size"
        @update:page="handlePageChange"
        @update:pageSize="handlePageSizeChange"
      />
      </template>
    </TablePageLayout>

    <!-- Assign Subscription Modal -->
    <BaseDialog
      :show="showAssignModal"
      :title="t('admin.subscriptions.assignSubscription')"
      width="normal"
      @close="closeAssignModal"
    >
      <form
        id="assign-subscription-form"
        @submit.prevent="handleAssignSubscription"
        class="space-y-5"
      >
        <div>
          <label class="input-label">{{ t('admin.subscriptions.form.user') }}</label>
          <div class="relative" data-assign-user-search>
            <input
              v-model="userSearchKeyword"
              type="text"
              class="input pr-8"
              :placeholder="t('admin.usage.searchUserPlaceholder')"
              @input="debounceSearchUsers"
              @focus="showUserDropdown = true"
            />
            <button
              v-if="selectedUser"
              @click="clearUserSelection"
              type="button"
              class="absolute right-2 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
            >
              <Icon name="x" size="sm" :stroke-width="2" />
            </button>
            <!-- User Dropdown -->
            <div
              v-if="showUserDropdown && (userSearchResults.length > 0 || userSearchKeyword)"
              class="absolute z-50 mt-1 max-h-60 w-full overflow-auto rounded-lg border border-gray-200 bg-white shadow-lg dark:border-gray-700 dark:bg-gray-800"
            >
              <div
                v-if="userSearchLoading"
                class="px-4 py-3 text-sm text-gray-500 dark:text-gray-400"
              >
                {{ t('common.loading') }}
              </div>
              <div
                v-else-if="userSearchResults.length === 0 && userSearchKeyword"
                class="px-4 py-3 text-sm text-gray-500 dark:text-gray-400"
              >
                {{ t('common.noOptionsFound') }}
              </div>
              <button
                v-for="user in userSearchResults"
                :key="user.id"
                type="button"
                @click="selectUser(user)"
                class="w-full px-4 py-2 text-left text-sm hover:bg-gray-100 dark:hover:bg-gray-700"
              >
                <span class="font-medium text-gray-900 dark:text-white">{{ user.email }}</span>
                <span class="ml-2 text-gray-500 dark:text-gray-400">#{{ user.id }}</span>
              </button>
            </div>
          </div>
        </div>
        <div>
          <label class="input-label">{{ t('admin.subscriptions.form.group') }}</label>
          <Select
            v-model="assignForm.group_id"
            :options="subscriptionGroupOptions"
            :placeholder="t('admin.subscriptions.selectGroup')"
          >
            <template #selected="{ option }">
              <GroupBadge
                v-if="option"
                :name="(option as unknown as GroupOption).label"
                :platform="(option as unknown as GroupOption).platform"
                :subscription-type="(option as unknown as GroupOption).subscriptionType"
                :rate-multiplier="(option as unknown as GroupOption).rate"
              />
              <span v-else class="text-gray-400">{{ t('admin.subscriptions.selectGroup') }}</span>
            </template>
            <template #option="{ option, selected }">
              <GroupOptionItem
                :name="(option as unknown as GroupOption).label"
                :platform="(option as unknown as GroupOption).platform"
                :subscription-type="(option as unknown as GroupOption).subscriptionType"
                :rate-multiplier="(option as unknown as GroupOption).rate"
                :description="(option as unknown as GroupOption).description"
                :selected="selected"
              />
            </template>
          </Select>
          <p class="input-hint">{{ t('admin.subscriptions.groupHint') }}</p>
        </div>
        <div>
          <label class="input-label">{{ t('admin.subscriptions.form.assignMode') }}</label>
          <Select
            v-model="assignForm.mode"
            :options="assignModeOptions"
          />
          <p class="input-hint">{{ t('admin.subscriptions.assignModeHint') }}</p>
        </div>
        <div v-if="assignForm.mode === 'days'">
          <label class="input-label">{{ t('admin.subscriptions.form.validityDays') }}</label>
          <input v-model.number="assignForm.validity_days" type="number" min="1" class="input" />
          <p class="input-hint">{{ t('admin.subscriptions.validityHint') }}</p>
        </div>
        <div v-else class="space-y-4">
          <div>
            <label class="input-label">{{ t('admin.subscriptions.form.startsAt') }}</label>
            <input
              v-model="assignForm.starts_at_local"
              type="datetime-local"
              class="input"
            />
          </div>
          <div>
            <label class="input-label">{{ t('admin.subscriptions.form.expiresAt') }}</label>
            <input
              v-model="assignForm.expires_at_local"
              type="datetime-local"
              class="input"
            />
          </div>
          <p class="input-hint">{{ t('admin.subscriptions.timeRangeHint') }}</p>
        </div>
      </form>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button @click="closeAssignModal" type="button" class="btn btn-secondary">
            {{ t('common.cancel') }}
          </button>
          <button
            type="submit"
            form="assign-subscription-form"
            :disabled="submitting"
            class="btn btn-primary"
          >
            <svg
              v-if="submitting"
              class="-ml-1 mr-2 h-4 w-4 animate-spin"
              fill="none"
              viewBox="0 0 24 24"
            >
              <circle
                class="opacity-25"
                cx="12"
                cy="12"
                r="10"
                stroke="currentColor"
                stroke-width="4"
              ></circle>
              <path
                class="opacity-75"
                fill="currentColor"
                d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
              ></path>
            </svg>
            {{ submitting ? t('admin.subscriptions.assigning') : t('admin.subscriptions.assign') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- Adjust Subscription Modal -->
    <BaseDialog
      :show="showExtendModal"
      :title="t('admin.subscriptions.adjustSubscription')"
      width="narrow"
      @close="closeExtendModal"
    >
      <form
        v-if="extendingSubscription"
        id="extend-subscription-form"
        @submit.prevent="handleExtendSubscription"
        class="space-y-5"
      >
        <div class="rounded-lg bg-gray-50 p-4 dark:bg-dark-700">
          <p class="text-sm text-gray-600 dark:text-gray-400">
            {{ t('admin.subscriptions.adjustingFor') }}
            <span class="font-medium text-gray-900 dark:text-white">{{
              extendingSubscription.user?.email
            }}</span>
          </p>
          <p class="mt-1 text-sm text-gray-600 dark:text-gray-400">
            {{ t('admin.subscriptions.currentExpiration') }}:
            <span class="font-medium text-gray-900 dark:text-white">
              {{
                extendingSubscription.expires_at
                  ? formatDateOnly(extendingSubscription.expires_at)
                  : t('admin.subscriptions.noExpiration')
              }}
            </span>
          </p>
          <p v-if="extendingSubscription.starts_at" class="mt-1 text-sm text-gray-600 dark:text-gray-400">
            {{ t('admin.subscriptions.currentStartTime') }}:
            <span class="font-medium text-gray-900 dark:text-white">
              {{ formatDateOnly(extendingSubscription.starts_at) }}
            </span>
          </p>
          <p v-if="extendingSubscription.expires_at" class="mt-1 text-sm text-gray-600 dark:text-gray-400">
            {{ t('admin.subscriptions.remainingDays') }}:
            <span class="font-medium text-gray-900 dark:text-white">
              {{ getDaysRemaining(extendingSubscription.expires_at) ?? 0 }}
            </span>
          </p>
        </div>
        <div>
          <label class="input-label">{{ t('admin.subscriptions.form.assignMode') }}</label>
          <div class="input flex items-center bg-gray-50 text-sm text-gray-700 dark:bg-dark-700 dark:text-gray-300">
            {{ extendModeLabel }}
          </div>
          <p class="input-hint">{{ t('admin.subscriptions.adjustModeHint') }}</p>
        </div>
        <div>
          <label class="input-label">{{ adjustDaysLabel }}</label>
          <div class="flex items-center gap-2">
            <input
              v-model.number="extendForm.days"
              type="number"
              required
              class="input text-center"
              :placeholder="t('admin.subscriptions.adjustDaysPlaceholder')"
            />
          </div>
          <p class="input-hint">{{ adjustDaysHint }}</p>
        </div>
        <div v-if="extendForm.mode === 'range'" class="grid grid-cols-1 gap-3 md:grid-cols-2">
          <div>
            <label class="input-label">{{ t('admin.subscriptions.form.startsAt') }}</label>
            <input :value="extendForm.range_starts_at_local" type="datetime-local" class="input" readonly />
          </div>
          <div>
            <label class="input-label">{{ t('admin.subscriptions.form.expiresAt') }}</label>
            <input :value="rangeAdjustedExpiresAtLocal" type="datetime-local" class="input" readonly />
          </div>
          <p class="input-hint md:col-span-2">{{ t('admin.subscriptions.rangeAdjustHint') }}</p>
        </div>
      </form>
      <template #footer>
        <div v-if="extendingSubscription" class="flex justify-end gap-3">
          <button @click="closeExtendModal" type="button" class="btn btn-secondary">
            {{ t('common.cancel') }}
          </button>
          <button
            type="submit"
            form="extend-subscription-form"
            :disabled="submitting"
            class="btn btn-primary"
          >
            {{ submitting ? t('admin.subscriptions.adjusting') : t('admin.subscriptions.adjust') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- Revoke Confirmation Dialog -->
    <ConfirmDialog
      :show="showRevokeDialog"
      :title="t('admin.subscriptions.revokeSubscription')"
      :message="t('admin.subscriptions.revokeConfirm', { user: revokingSubscription?.user?.email })"
      :confirm-text="t('admin.subscriptions.revoke')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      @confirm="confirmRevoke"
      @cancel="showRevokeDialog = false"
    />

    <BaseDialog
      :show="showWeekendSkipConfirm"
      :title="weekendSkipTarget?.enabled ? '开启跳过非工作日' : '关闭跳过非工作日'"
      width="narrow"
      @close="closeWeekendSkipConfirm"
    >
      <div v-if="weekendSkipTarget" class="space-y-4 text-sm text-gray-700 dark:text-gray-300">
        <p>
          {{ weekendSkipTarget.enabled ? '开启后周六、周日不可使用，系统会把周末时间补偿到到期时间。' : '关闭后周六、周日恢复可用，系统会把剩余工作日时长换算回自然日到期时间。' }}
        </p>
        <div class="rounded-lg border border-gray-200 p-3 dark:border-dark-600">
          <div class="flex justify-between gap-3 py-1">
            <span class="text-gray-500 dark:text-dark-400">当前到期时间</span>
            <span class="font-medium">{{ formatDateTime(weekendSkipTarget.preview.current_expires_at) }}</span>
          </div>
          <div class="flex justify-between gap-3 py-1">
            <span class="text-gray-500 dark:text-dark-400">调整后到期时间</span>
            <span class="font-medium">{{ formatDateTime(weekendSkipTarget.preview.preview_expires_at) }}</span>
          </div>
          <div class="flex justify-between gap-3 py-1">
            <span class="text-gray-500 dark:text-dark-400">本次时间变化</span>
            <span :class="weekendSkipTarget.preview.delta_seconds >= 0 ? 'text-green-600 dark:text-green-400' : 'text-orange-600 dark:text-orange-400'">
              {{ formatDeltaSeconds(weekendSkipTarget.preview.delta_seconds) }}
            </span>
          </div>
        </div>
        <p class="text-xs text-gray-500 dark:text-dark-400">
          请确认到期时间变化无误后再继续。
        </p>
      </div>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" @click="closeWeekendSkipConfirm">{{ t('common.cancel') }}</button>
          <button type="button" class="btn btn-primary" :disabled="weekendSkipSubmitting" @click="confirmWeekendSkipToggle">
            {{ weekendSkipSubmitting ? t('common.processing') : t('common.confirm') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- Reset Quota Multi-Select Dialog -->
    <BaseDialog
      :show="showResetQuotaConfirm"
      :title="t('admin.subscriptions.resetQuotaTitle')"
      width="narrow"
      @close="closeResetQuotaDialog"
    >
      <div v-if="resettingSubscription" class="space-y-4">
        <p class="text-sm text-gray-600 dark:text-gray-300">
          {{ t('admin.subscriptions.resetQuotaConfirm', { user: resettingSubscription.user?.email }) }}
        </p>

        <!-- Paid lock notice -->
        <div
          v-if="isPaidSubscription(resettingSubscription)"
          class="rounded-lg border border-amber-200 bg-amber-50 p-3 text-xs text-amber-800 dark:border-amber-700/40 dark:bg-amber-900/20 dark:text-amber-300"
        >
          {{ t('admin.subscriptions.resetQuotaPaidLocked') }}
        </div>

        <!-- No-limits notice -->
        <div
          v-else-if="!hasAnyConfiguredWindow(resettingSubscription)"
          class="rounded-lg border border-gray-200 bg-gray-50 p-3 text-xs text-gray-600 dark:border-dark-600 dark:bg-dark-700 dark:text-gray-300"
        >
          {{ t('admin.subscriptions.resetQuotaNoLimits') }}
        </div>

        <!-- Window selectors -->
        <div v-else class="space-y-2">
          <p class="text-xs text-gray-500 dark:text-dark-400">
            {{ t('admin.subscriptions.resetQuotaSelectorHint') }}
          </p>
          <label
            v-for="opt in resetWindowOptions"
            :key="opt.key"
            :class="[
              'flex cursor-pointer items-start gap-2 rounded-lg border p-3 text-sm transition-colors',
              opt.disabled
                ? 'cursor-not-allowed border-gray-200 bg-gray-50 text-gray-400 dark:border-dark-600 dark:bg-dark-700/50 dark:text-dark-500'
                : 'border-gray-200 bg-white hover:border-primary-400 dark:border-dark-600 dark:bg-dark-800 dark:hover:border-primary-500'
            ]"
          >
            <input
              type="checkbox"
              class="mt-0.5 h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500 disabled:opacity-50"
              :checked="resetSelection[opt.key]"
              :disabled="opt.disabled"
              @change="toggleResetSelection(opt.key)"
            />
            <div class="min-w-0 flex-1">
              <div class="flex items-center justify-between gap-2">
                <span class="font-medium" :class="opt.disabled ? '' : 'text-gray-900 dark:text-white'">
                  {{ opt.label }}
                </span>
                <span v-if="opt.usageText" class="text-xs tabular-nums">{{ opt.usageText }}</span>
              </div>
              <p v-if="opt.disabledReason" class="mt-0.5 text-[11px] text-gray-500 dark:text-dark-400">
                {{ opt.disabledReason }}
              </p>
            </div>
          </label>
        </div>
      </div>
      <template #footer>
        <div v-if="resettingSubscription" class="flex justify-end gap-3">
          <button @click="closeResetQuotaDialog" type="button" class="btn btn-secondary">
            {{ t('common.cancel') }}
          </button>
          <button
            type="button"
            class="btn btn-primary"
            :disabled="!canSubmitResetQuota || resettingQuota"
            @click="confirmResetQuota"
          >
            {{ resettingQuota ? t('common.processing') : t('admin.subscriptions.resetQuota') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <BaseDialog
      :show="showOrderUsageModal"
      title="订单用量明细"
      width="wide"
      @close="closeOrderUsageModal"
    >
      <div class="space-y-4">
        <div class="rounded-lg border border-gray-200 bg-gray-50 p-3 text-sm dark:border-dark-600 dark:bg-dark-700">
          <div class="flex flex-wrap items-center justify-between gap-3">
            <div>
              <div class="font-medium text-gray-900 dark:text-white">
                {{ orderUsageSubscription?.user?.email || `用户 #${orderUsageSubscription?.user_id || '-'}` }}
              </div>
              <div class="mt-1 text-xs text-gray-500 dark:text-dark-400">
                订阅 #{{ orderUsageSubscription?.id || '-' }} · {{ orderUsageSubscription?.group?.name || `分组 #${orderUsageSubscription?.group_id || '-'}` }}
              </div>
            </div>
            <div v-if="orderUsageData" class="grid grid-cols-5 gap-3 text-right text-xs">
              <div>
                <div class="text-gray-500 dark:text-dark-400">订单数</div>
                <div class="mt-1 font-semibold text-gray-900 dark:text-white">{{ orderUsageData.orders.length }}</div>
              </div>
              <div>
                <div class="text-gray-500 dark:text-dark-400">订单已用</div>
                <div class="mt-1 font-semibold text-gray-900 dark:text-white">${{ orderUsageData.total_used_actual_cost.toFixed(2) }}</div>
              </div>
              <div>
                <div class="text-gray-500 dark:text-dark-400">剩余</div>
                <div class="mt-1 font-semibold" :class="(orderUsageData.total_remaining_usd ?? 0) > 0 ? 'text-green-600 dark:text-green-400' : 'text-gray-900 dark:text-white'">
                  {{ formatOptionalMoney(orderUsageData.total_remaining_usd) }}
                </div>
              </div>
              <div>
                <div class="text-gray-500 dark:text-dark-400">窗口订阅</div>
                <div class="mt-1 font-semibold text-gray-900 dark:text-white">${{ orderUsageData.total_window_subscription_used_usd.toFixed(2) }}</div>
              </div>
              <div>
                <div class="text-gray-500 dark:text-dark-400">窗口余额</div>
                <div class="mt-1 font-semibold" :class="orderUsageData.total_window_balance_used_usd > 0 ? 'text-orange-600 dark:text-orange-400' : 'text-gray-900 dark:text-white'">
                  ${{ orderUsageData.total_window_balance_used_usd.toFixed(2) }}
                </div>
              </div>
            </div>
          </div>
          <div class="mt-3 rounded-md bg-amber-50 px-3 py-2 text-xs text-amber-800 dark:bg-amber-900/20 dark:text-amber-300">
            订单已用/剩余按已支付订单额度顺序分摊；窗口订阅/余额表示该窗口内实际发生的消费。现有使用日志没有直接保存 order_id。
          </div>
        </div>

        <div v-if="orderUsageLoading" class="py-8 text-center text-sm text-gray-500 dark:text-dark-400">
          {{ t('common.loading') }}
        </div>
        <div v-else-if="!orderUsageData || orderUsageData.orders.length === 0" class="py-8 text-center text-sm text-gray-500 dark:text-dark-400">
          暂无可归因的已支付订阅订单
        </div>
        <div v-else class="overflow-x-auto rounded-lg border border-gray-200 dark:border-dark-600">
          <table class="min-w-[1420px] w-full text-left text-xs">
            <thead class="bg-gray-50 text-gray-500 dark:bg-dark-700 dark:text-dark-300">
              <tr>
                <th class="px-3 py-2 font-medium">订单</th>
                <th class="px-3 py-2 font-medium">类型</th>
                <th class="px-3 py-2 font-medium">支付时间</th>
                <th class="px-3 py-2 font-medium">窗口</th>
                <th class="px-3 py-2 text-right font-medium">额度</th>
                <th class="px-3 py-2 text-right font-medium">订单已用</th>
                <th class="px-3 py-2 text-right font-medium">剩余</th>
                <th class="px-3 py-2 text-right font-medium">窗口订阅</th>
                <th class="px-3 py-2 text-right font-medium">窗口余额</th>
                <th class="px-3 py-2 text-right font-medium">订阅请求</th>
                <th class="px-3 py-2 text-right font-medium">Tokens</th>
                <th class="px-3 py-2 font-medium">首末使用</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="order in orderUsageData.orders" :key="order.order_id" class="bg-white dark:bg-dark-800">
                <td class="px-3 py-2 align-top">
                  <div class="font-medium text-gray-900 dark:text-white">#{{ order.order_id }}</div>
                  <div class="mt-1 text-[11px] text-gray-500 dark:text-dark-400">{{ order.order_status }}</div>
                </td>
                <td class="px-3 py-2 align-top">
                  <span class="rounded-md bg-gray-100 px-2 py-0.5 text-[11px] text-gray-700 dark:bg-dark-700 dark:text-dark-200">
                    {{ formatOrderWindowKind(order) }}
                  </span>
                  <div class="mt-1 text-[11px] text-gray-500 dark:text-dark-400">
                    {{ order.subscription_days }} 天 · {{ order.validity_unit || 'day' }}
                  </div>
                </td>
                <td class="px-3 py-2 align-top text-gray-700 dark:text-gray-300">
                  {{ formatDateTime(order.paid_at) }}
                </td>
                <td class="px-3 py-2 align-top text-gray-700 dark:text-gray-300">
                  <div>{{ formatDateTime(order.window_start) }}</div>
                  <div class="mt-1 text-gray-500 dark:text-dark-400">{{ formatDateTime(order.window_end) }}</div>
                </td>
                <td class="px-3 py-2 text-right align-top tabular-nums text-gray-700 dark:text-gray-300">
                  {{ formatOptionalMoney(order.quota_usd) }}
                </td>
                <td class="px-3 py-2 text-right align-top tabular-nums" :class="isOrderOverQuota(order) ? 'text-red-600 dark:text-red-400' : 'text-gray-700 dark:text-gray-300'">
                  ${{ order.used_actual_cost_usd.toFixed(2) }}
                  <div v-if="order.over_quota_usd" class="mt-1 text-[11px] text-red-600 dark:text-red-400">
                    超 ${{ order.over_quota_usd.toFixed(2) }}
                  </div>
                </td>
                <td class="px-3 py-2 text-right align-top tabular-nums" :class="(order.remaining_usd ?? 0) > 0 ? 'text-green-600 dark:text-green-400' : 'text-gray-700 dark:text-gray-300'">
                  {{ formatOptionalMoney(order.remaining_usd) }}
                </td>
                <td class="px-3 py-2 text-right align-top tabular-nums text-gray-700 dark:text-gray-300">
                  ${{ order.window_subscription_used_usd.toFixed(2) }}
                </td>
                <td class="px-3 py-2 text-right align-top tabular-nums" :class="order.window_balance_used_usd > 0 ? 'text-orange-600 dark:text-orange-400' : 'text-gray-700 dark:text-gray-300'">
                  ${{ order.window_balance_used_usd.toFixed(2) }}
                  <div v-if="order.balance_request_count" class="mt-1 text-[11px] text-gray-500 dark:text-dark-400">
                    {{ order.balance_request_count }} 次
                  </div>
                </td>
                <td class="px-3 py-2 text-right align-top tabular-nums text-gray-700 dark:text-gray-300">
                  {{ order.request_count }}
                </td>
                <td class="px-3 py-2 text-right align-top tabular-nums text-gray-700 dark:text-gray-300">
                  {{ formatCompactNumber(order.input_tokens + order.output_tokens) }}
                </td>
                <td class="px-3 py-2 align-top text-gray-700 dark:text-gray-300">
                  <div>{{ order.first_usage_at ? formatDateTime(order.first_usage_at) : '-' }}</div>
                  <div class="mt-1 text-gray-500 dark:text-dark-400">{{ order.last_usage_at ? formatDateTime(order.last_usage_at) : '-' }}</div>
                  <div v-if="order.exhausted_at" class="mt-1 text-[11px] text-red-600 dark:text-red-400">
                    用完 {{ formatDateTime(order.exhausted_at) }}
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
      <template #footer>
        <div class="flex justify-end">
          <button type="button" class="btn btn-secondary" @click="closeOrderUsageModal">{{ t('common.close') }}</button>
        </div>
      </template>
    </BaseDialog>
    <!-- Subscription Guide Modal -->
    <teleport to="body">
      <transition name="modal">
        <div v-if="showGuideModal" class="fixed inset-0 z-50 flex items-center justify-center p-4" @mousedown.self="showGuideModal = false">
          <div class="fixed inset-0 bg-black/50" @click="showGuideModal = false"></div>
          <div class="relative max-h-[85vh] w-full max-w-2xl overflow-y-auto rounded-xl bg-white p-6 shadow-2xl dark:bg-dark-800">
            <button type="button" class="absolute right-4 top-4 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200" @click="showGuideModal = false">
              <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" /></svg>
            </button>

            <h2 class="mb-4 text-lg font-bold text-gray-900 dark:text-white">{{ t('admin.subscriptions.guide.title') }}</h2>
            <p class="mb-5 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.subscriptions.guide.subtitle') }}</p>

            <!-- Step 1 -->
            <div class="mb-5">
              <h3 class="mb-2 flex items-center gap-2 text-sm font-semibold text-gray-900 dark:text-white">
                <span class="flex h-6 w-6 items-center justify-center rounded-full bg-primary-100 text-xs font-bold text-primary-700 dark:bg-primary-900/40 dark:text-primary-300">1</span>
                {{ t('admin.subscriptions.guide.step1.title') }}
              </h3>
              <ol class="ml-8 list-decimal space-y-1 text-sm text-gray-600 dark:text-gray-300">
                <li>{{ t('admin.subscriptions.guide.step1.line1') }}</li>
                <li>{{ t('admin.subscriptions.guide.step1.line2') }}</li>
                <li>{{ t('admin.subscriptions.guide.step1.line3') }}</li>
              </ol>
              <div class="ml-8 mt-2">
                <router-link
                  to="/admin/groups"
                  @click="showGuideModal = false"
                  class="inline-flex items-center gap-1 text-sm font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
                >
                  {{ t('admin.subscriptions.guide.step1.link') }}
                  <Icon name="arrowRight" size="xs" />
                </router-link>
              </div>
            </div>

            <!-- Step 2 -->
            <div class="mb-5">
              <h3 class="mb-2 flex items-center gap-2 text-sm font-semibold text-gray-900 dark:text-white">
                <span class="flex h-6 w-6 items-center justify-center rounded-full bg-primary-100 text-xs font-bold text-primary-700 dark:bg-primary-900/40 dark:text-primary-300">2</span>
                {{ t('admin.subscriptions.guide.step2.title') }}
              </h3>
              <ol class="ml-8 list-decimal space-y-1 text-sm text-gray-600 dark:text-gray-300">
                <li>{{ t('admin.subscriptions.guide.step2.line1') }}</li>
                <li>{{ t('admin.subscriptions.guide.step2.line2') }}</li>
                <li>{{ t('admin.subscriptions.guide.step2.line3') }}</li>
              </ol>
            </div>

            <!-- Step 3 -->
            <div class="mb-5">
              <h3 class="mb-2 flex items-center gap-2 text-sm font-semibold text-gray-900 dark:text-white">
                <span class="flex h-6 w-6 items-center justify-center rounded-full bg-primary-100 text-xs font-bold text-primary-700 dark:bg-primary-900/40 dark:text-primary-300">3</span>
                {{ t('admin.subscriptions.guide.step3.title') }}
              </h3>
              <div class="ml-8 overflow-hidden rounded-lg border border-gray-200 dark:border-dark-600">
                <table class="w-full text-sm">
                  <tbody>
                    <tr v-for="(row, i) in guideActionRows" :key="i" class="border-b border-gray-100 dark:border-dark-700 last:border-0">
                      <td class="whitespace-nowrap bg-gray-50 px-3 py-2 font-medium text-gray-700 dark:bg-dark-700 dark:text-gray-300">{{ row.action }}</td>
                      <td class="px-3 py-2 text-gray-600 dark:text-gray-400">{{ row.desc }}</td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </div>

            <!-- Tip -->
            <div class="rounded-lg bg-blue-50 p-3 text-xs text-blue-700 dark:bg-blue-900/20 dark:text-blue-300">
              {{ t('admin.subscriptions.guide.tip') }}
            </div>

            <div class="mt-4 text-right">
              <button type="button" class="btn btn-primary btn-sm" @click="showGuideModal = false">{{ t('common.close') }}</button>
            </div>
          </div>
        </div>
      </transition>
    </teleport>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import type {
  SubscriptionOrderUsageItem,
  SubscriptionOrderUsageResponse,
  WeekendSkipPreview
} from '@/api/admin/subscriptions'
import type {
  UserSubscription,
  Group,
  GroupPlatform,
  SubscriptionType,
  ExtendSubscriptionRequest
} from '@/types'
import type { SimpleUser } from '@/api/admin/usage'
import type { Column } from '@/components/common/types'
import { formatDateOnly, formatDateTime, formatRemainingDuration } from '@/utils/format'
import { getPersistedPageSize } from '@/composables/usePersistedPageSize'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Select from '@/components/common/Select.vue'
import GroupBadge from '@/components/common/GroupBadge.vue'
import GroupOptionItem from '@/components/common/GroupOptionItem.vue'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const appStore = useAppStore()

interface GroupOption {
  value: number
  label: string
  description: string | null
  platform: GroupPlatform
  subscriptionType: SubscriptionType
  rate: number
}

// Guide modal state
const showGuideModal = ref(false)

const guideActionRows = computed(() => [
  { action: t('admin.subscriptions.guide.actions.adjust'), desc: t('admin.subscriptions.guide.actions.adjustDesc') },
  { action: t('admin.subscriptions.guide.actions.resetQuota'), desc: t('admin.subscriptions.guide.actions.resetQuotaDesc') },
  { action: t('admin.subscriptions.guide.actions.revoke'), desc: t('admin.subscriptions.guide.actions.revokeDesc') }
])

// User column display mode: 'email' or 'username'
const userColumnMode = ref<'email' | 'username'>('email')
const USER_COLUMN_MODE_KEY = 'subscription-user-column-mode'

const loadUserColumnMode = () => {
  try {
    const saved = localStorage.getItem(USER_COLUMN_MODE_KEY)
    if (saved === 'email' || saved === 'username') {
      userColumnMode.value = saved
    }
  } catch (e) {
    console.error('Failed to load user column mode:', e)
  }
}

const saveUserColumnMode = () => {
  try {
    localStorage.setItem(USER_COLUMN_MODE_KEY, userColumnMode.value)
  } catch (e) {
    console.error('Failed to save user column mode:', e)
  }
}

const setUserColumnMode = (mode: 'email' | 'username') => {
  userColumnMode.value = mode
  saveUserColumnMode()
}

// All available columns
const allColumns = computed<Column[]>(() => [
  {
    key: 'user',
    label: userColumnMode.value === 'email'
      ? t('admin.subscriptions.columns.user')
      : t('admin.users.columns.username'),
    sortable: false
  },
  { key: 'group', label: t('admin.subscriptions.columns.group'), sortable: false },
  { key: 'starts_at', label: t('admin.subscriptions.columns.startsAt'), sortable: false },
  { key: 'usage', label: t('admin.subscriptions.columns.usage'), sortable: false },
  { key: 'last_used_at', label: t('admin.subscriptions.columns.lastUsed'), sortable: true },
  { key: 'expires_at', label: t('admin.subscriptions.columns.expires'), sortable: true },
  { key: 'status', label: t('admin.subscriptions.columns.status'), sortable: true },
  { key: 'actions', label: t('admin.subscriptions.columns.actions'), sortable: false }
])

// Columns that can be toggled (exclude user and actions which are always visible)
const toggleableColumns = computed(() =>
  allColumns.value.filter(col => col.key !== 'user' && col.key !== 'actions')
)

// Hidden columns set
const hiddenColumns = reactive<Set<string>>(new Set())

// Default hidden columns
const DEFAULT_HIDDEN_COLUMNS: string[] = []

// localStorage key
const HIDDEN_COLUMNS_KEY = 'subscription-hidden-columns'

// Load saved column settings
const loadSavedColumns = () => {
  try {
    const saved = localStorage.getItem(HIDDEN_COLUMNS_KEY)
    if (saved) {
      const parsed = JSON.parse(saved) as string[]
      parsed.forEach(key => hiddenColumns.add(key))
    } else {
      DEFAULT_HIDDEN_COLUMNS.forEach(key => hiddenColumns.add(key))
    }
  } catch (e) {
    console.error('Failed to load saved columns:', e)
    DEFAULT_HIDDEN_COLUMNS.forEach(key => hiddenColumns.add(key))
  }
}

// Save column settings to localStorage
const saveColumnsToStorage = () => {
  try {
    localStorage.setItem(HIDDEN_COLUMNS_KEY, JSON.stringify([...hiddenColumns]))
  } catch (e) {
    console.error('Failed to save columns:', e)
  }
}

// Toggle column visibility
const toggleColumn = (key: string) => {
  if (hiddenColumns.has(key)) {
    hiddenColumns.delete(key)
  } else {
    hiddenColumns.add(key)
  }
  saveColumnsToStorage()
}

// Check if column is visible
const isColumnVisible = (key: string) => !hiddenColumns.has(key)

// Filtered columns for display
const columns = computed<Column[]>(() =>
  allColumns.value.filter(col =>
    col.key === 'user' || col.key === 'actions' || !hiddenColumns.has(col.key)
  )
)

// Column dropdown state
const showColumnDropdown = ref(false)
const columnDropdownRef = ref<HTMLElement | null>(null)

// Filter options
const statusOptions = computed(() => [
  { value: '', label: t('admin.subscriptions.allStatus') },
  { value: 'active', label: t('admin.subscriptions.status.active') },
  { value: 'expired', label: t('admin.subscriptions.status.expired') },
  { value: 'quota_exhausted', label: t('admin.subscriptions.status.quota_exhausted') },
  { value: 'revoked', label: t('admin.subscriptions.status.revoked') }
])

const subscriptions = ref<UserSubscription[]>([])
const groups = ref<Group[]>([])
const loading = ref(false)
let abortController: AbortController | null = null

// Toolbar user filter (fuzzy search -> select user_id)
const filterUserKeyword = ref('')
const filterUserResults = ref<SimpleUser[]>([])
const filterUserLoading = ref(false)
const showFilterUserDropdown = ref(false)
const selectedFilterUser = ref<SimpleUser | null>(null)
let filterUserSearchTimeout: ReturnType<typeof setTimeout> | null = null

// User search state
const userSearchKeyword = ref('')
const userSearchResults = ref<SimpleUser[]>([])
const userSearchLoading = ref(false)
const showUserDropdown = ref(false)
const selectedUser = ref<SimpleUser | null>(null)
let userSearchTimeout: ReturnType<typeof setTimeout> | null = null

const filters = reactive({
  status: 'active',
  group_id: '',
  platform: '',
  user_id: null as number | null
})

// Sorting state
const sortState = reactive({
  sort_by: 'last_used_at',
  sort_order: 'desc' as 'asc' | 'desc'
})

const pagination = reactive({
  page: 1,
  page_size: getPersistedPageSize(),
  total: 0,
  pages: 0
})

const showAssignModal = ref(false)
const showExtendModal = ref(false)
const showRevokeDialog = ref(false)
const showResetQuotaConfirm = ref(false)
const submitting = ref(false)
const resettingSubscription = ref<UserSubscription | null>(null)
const resettingQuota = ref(false)
const extendingSubscription = ref<UserSubscription | null>(null)
const revokingSubscription = ref<UserSubscription | null>(null)
const showOrderUsageModal = ref(false)
const orderUsageLoading = ref(false)
const orderUsageSubscription = ref<UserSubscription | null>(null)
const orderUsageData = ref<SubscriptionOrderUsageResponse | null>(null)
const showWeekendSkipConfirm = ref(false)
const weekendSkipSubmitting = ref(false)
const weekendSkipTarget = ref<{
  subscription: UserSubscription
  enabled: boolean
  preview: WeekendSkipPreview
} | null>(null)

const assignForm = reactive({
  user_id: null as number | null,
  group_id: null as number | null,
  validity_days: 30,
  mode: 'days' as 'days' | 'range',
  starts_at_local: '',
  expires_at_local: ''
})

const assignModeOptions = computed(() => [
  { value: 'days', label: t('admin.subscriptions.assignModeDays') },
  { value: 'range', label: t('admin.subscriptions.assignModeRange') }
])
const extendModeLabel = computed(() =>
  extendForm.mode === 'range'
    ? t('admin.subscriptions.assignModeRange')
    : t('admin.subscriptions.assignModeDays')
)
// 时间段模式下天数字段是「结束时间偏移量」，与 days 模式语义不同，需要更明确的文案
const adjustDaysLabel = computed(() =>
  extendForm.mode === 'range'
    ? t('admin.subscriptions.form.adjustDaysRange')
    : t('admin.subscriptions.form.adjustDays')
)
const adjustDaysHint = computed(() =>
  extendForm.mode === 'range'
    ? t('admin.subscriptions.adjustRangeDaysHint')
    : t('admin.subscriptions.adjustHint')
)

const extendForm = reactive({
  days: 30,
  mode: 'days' as 'days' | 'range',
  range_starts_at: '',
  range_expires_at: '',
  range_starts_at_local: ''
})

// Group options for filter (all groups)
const groupOptions = computed(() => [
  { value: '', label: t('admin.subscriptions.allGroups') },
  ...groups.value.map((g) => ({ value: g.id.toString(), label: g.name }))
])

const platformFilterOptions = computed(() => [
  { value: '', label: t('admin.subscriptions.allPlatforms') },
  { value: 'anthropic', label: 'Anthropic' },
  { value: 'openai', label: 'OpenAI' },
  { value: 'gemini', label: 'Gemini' },
  { value: 'antigravity', label: 'Antigravity' },
  { value: 'grok', label: 'Grok' }
])

// Group options for assign (only subscription type groups)
const subscriptionGroupOptions = computed(() =>
  groups.value
    .filter((g) => g.subscription_type === 'subscription' && g.status === 'active')
    .map((g) => ({
      value: g.id,
      label: g.name,
      description: g.description,
      platform: g.platform,
      subscriptionType: g.subscription_type,
      rate: g.rate_multiplier
    }))
)

const applyFilters = () => {
  pagination.page = 1
  loadSubscriptions()
}

const loadSubscriptions = async () => {
  if (abortController) {
    abortController.abort()
  }
  const requestController = new AbortController()
  abortController = requestController
  const { signal } = requestController

  loading.value = true
  try {
    const response = await adminAPI.subscriptions.list(
      pagination.page,
      pagination.page_size,
      {
        status: (filters.status as any) || undefined,
        group_id: filters.group_id ? parseInt(filters.group_id) : undefined,
        platform: filters.platform || undefined,
        user_id: filters.user_id || undefined,
        sort_by: sortState.sort_by,
        sort_order: sortState.sort_order
      },
      {
        signal
      }
    )
    if (signal.aborted || abortController !== requestController) return
    subscriptions.value = response.items
    pagination.total = response.total
    pagination.pages = response.pages
  } catch (error: any) {
    if (signal.aborted || error?.name === 'AbortError' || error?.code === 'ERR_CANCELED') {
      return
    }
    appStore.showError(t('admin.subscriptions.failedToLoad'))
    console.error('Error loading subscriptions:', error)
  } finally {
    if (abortController === requestController) {
      loading.value = false
      abortController = null
    }
  }
}

const loadGroups = async () => {
  try {
    groups.value = await adminAPI.groups.getAll()
  } catch (error) {
    console.error('Error loading groups:', error)
  }
}

// Toolbar user filter search with debounce
const debounceSearchFilterUsers = () => {
  if (filterUserSearchTimeout) {
    clearTimeout(filterUserSearchTimeout)
  }
  filterUserSearchTimeout = setTimeout(searchFilterUsers, 300)
}

const searchFilterUsers = async () => {
  const keyword = filterUserKeyword.value.trim()

  // Clear active user filter if user modified the search keyword
  if (selectedFilterUser.value && keyword !== selectedFilterUser.value.email) {
    selectedFilterUser.value = null
    filters.user_id = null
    applyFilters()
  }

  if (!keyword) {
    filterUserResults.value = []
    return
  }

  filterUserLoading.value = true
  try {
    filterUserResults.value = await adminAPI.usage.searchUsers(keyword)
  } catch (error) {
    console.error('Failed to search users:', error)
    filterUserResults.value = []
  } finally {
    filterUserLoading.value = false
  }
}

const selectFilterUser = (user: SimpleUser) => {
  selectedFilterUser.value = user
  filterUserKeyword.value = user.email
  showFilterUserDropdown.value = false
  filters.user_id = user.id
  applyFilters()
}

const clearFilterUser = () => {
  selectedFilterUser.value = null
  filterUserKeyword.value = ''
  filterUserResults.value = []
  showFilterUserDropdown.value = false
  filters.user_id = null
  applyFilters()
}

// User search with debounce
const debounceSearchUsers = () => {
  if (userSearchTimeout) {
    clearTimeout(userSearchTimeout)
  }
  userSearchTimeout = setTimeout(searchUsers, 300)
}

const searchUsers = async () => {
  const keyword = userSearchKeyword.value.trim()

  // Clear selection if user modified the search keyword
  if (selectedUser.value && keyword !== selectedUser.value.email) {
    selectedUser.value = null
    assignForm.user_id = null
  }

  if (!keyword) {
    userSearchResults.value = []
    return
  }

  userSearchLoading.value = true
  try {
    userSearchResults.value = await adminAPI.usage.searchUsers(keyword)
  } catch (error) {
    console.error('Failed to search users:', error)
    userSearchResults.value = []
  } finally {
    userSearchLoading.value = false
  }
}

const selectUser = (user: SimpleUser) => {
  selectedUser.value = user
  userSearchKeyword.value = user.email
  showUserDropdown.value = false
  assignForm.user_id = user.id
}

const clearUserSelection = () => {
  selectedUser.value = null
  userSearchKeyword.value = ''
  userSearchResults.value = []
  assignForm.user_id = null
}

const handlePageChange = (page: number) => {
  pagination.page = page
  loadSubscriptions()
}

const handlePageSizeChange = (pageSize: number) => {
  pagination.page_size = pageSize
  pagination.page = 1
  loadSubscriptions()
}

const handleSort = (key: string, order: 'asc' | 'desc') => {
  sortState.sort_by = key
  sortState.sort_order = order
  pagination.page = 1
  loadSubscriptions()
}

const localDateTimeToRFC3339 = (value: string): string | null => {
  if (!value) return null
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return null
  return d.toISOString()
}

const rfc3339ToLocalDateTime = (value?: string | null): string => {
  if (!value) return ''
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return ''
  const year = d.getFullYear()
  const month = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  const hours = String(d.getHours()).padStart(2, '0')
  const minutes = String(d.getMinutes()).padStart(2, '0')
  return `${year}-${month}-${day}T${hours}:${minutes}`
}

const closeAssignModal = () => {
  showAssignModal.value = false
  assignForm.user_id = null
  assignForm.group_id = null
  assignForm.validity_days = 30
  assignForm.mode = 'days'
  assignForm.starts_at_local = ''
  assignForm.expires_at_local = ''
  // Clear user search state
  selectedUser.value = null
  userSearchKeyword.value = ''
  userSearchResults.value = []
  showUserDropdown.value = false
}

const handleAssignSubscription = async () => {
  if (!assignForm.user_id) {
    appStore.showError(t('admin.subscriptions.pleaseSelectUser'))
    return
  }
  if (!assignForm.group_id) {
    appStore.showError(t('admin.subscriptions.pleaseSelectGroup'))
    return
  }

  const payload: {
    user_id: number
    group_id: number
    validity_days?: number
    starts_at?: string
    expires_at?: string
  } = {
    user_id: assignForm.user_id,
    group_id: assignForm.group_id
  }

  if (assignForm.mode === 'days') {
    if (!assignForm.validity_days || assignForm.validity_days < 1) {
      appStore.showError(t('admin.subscriptions.validityDaysRequired'))
      return
    }
    payload.validity_days = assignForm.validity_days
  } else {
    const startsAt = localDateTimeToRFC3339(assignForm.starts_at_local)
    const expiresAt = localDateTimeToRFC3339(assignForm.expires_at_local)
    if (!startsAt || !expiresAt) {
      appStore.showError(t('admin.subscriptions.timeRangeRequired'))
      return
    }
    if (new Date(expiresAt).getTime() <= new Date(startsAt).getTime()) {
      appStore.showError(t('admin.subscriptions.timeRangeInvalid'))
      return
    }
    if (new Date(expiresAt).getTime() <= Date.now()) {
      appStore.showError(t('admin.subscriptions.timeRangeMustBeFuture'))
      return
    }
    payload.starts_at = startsAt
    payload.expires_at = expiresAt
  }

  submitting.value = true
  try {
    await adminAPI.subscriptions.assign(payload)
    appStore.showSuccess(t('admin.subscriptions.subscriptionAssigned'))
    closeAssignModal()
    loadSubscriptions()
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.subscriptions.failedToAssign'))
    console.error('Error assigning subscription:', error)
  } finally {
    submitting.value = false
  }
}

const handleExtend = (subscription: UserSubscription) => {
  extendingSubscription.value = subscription
  extendForm.days = 30
  const useRangeMode = shouldAdjustAsRange(subscription)
  extendForm.mode = useRangeMode ? 'range' : 'days'
  if (useRangeMode) {
    extendForm.range_starts_at = subscription.starts_at || ''
    extendForm.range_expires_at = subscription.expires_at || ''
    extendForm.range_starts_at_local = rfc3339ToLocalDateTime(subscription.starts_at)
  } else {
    extendForm.range_starts_at = ''
    extendForm.range_expires_at = ''
    extendForm.range_starts_at_local = ''
  }
  showExtendModal.value = true
}

const closeExtendModal = () => {
  showExtendModal.value = false
  extendingSubscription.value = null
  extendForm.days = 30
  extendForm.mode = 'days'
  extendForm.range_starts_at = ''
  extendForm.range_expires_at = ''
  extendForm.range_starts_at_local = ''
}

const shouldAdjustAsRange = (subscription: UserSubscription): boolean => {
  if (!subscription.starts_at || !subscription.expires_at) return false
  const startsAtMs = new Date(subscription.starts_at).getTime()
  const createdAtMs = new Date(subscription.created_at).getTime()
  if (Number.isNaN(startsAtMs) || Number.isNaN(createdAtMs)) return false
  if (startsAtMs > Date.now()) return true
  return Math.abs(startsAtMs - createdAtMs) > 60 * 1000
}

const rangeAdjustedExpiresAtRFC3339 = computed(() => {
  if (extendForm.mode !== 'range') return null
  const baseExpiresAt = new Date(extendForm.range_expires_at)
  if (Number.isNaN(baseExpiresAt.getTime())) return null
  const adjusted = new Date(
    baseExpiresAt.getTime() + extendForm.days * 24 * 60 * 60 * 1000
  )
  if (Number.isNaN(adjusted.getTime())) return null
  return adjusted.toISOString()
})

const rangeAdjustedExpiresAtLocal = computed(() =>
  rfc3339ToLocalDateTime(rangeAdjustedExpiresAtRFC3339.value)
)

const handleExtendSubscription = async () => {
  if (!extendingSubscription.value) return

  const payload: ExtendSubscriptionRequest = {}
  if (extendForm.mode === 'range') {
    const startsAt = new Date(extendForm.range_starts_at)
    const expiresAt = new Date(rangeAdjustedExpiresAtRFC3339.value || '')
    if (Number.isNaN(startsAt.getTime()) || Number.isNaN(expiresAt.getTime())) {
      appStore.showError(t('admin.subscriptions.timeRangeRequired'))
      return
    }
    if (expiresAt.getTime() <= startsAt.getTime()) {
      appStore.showError(t('admin.subscriptions.timeRangeInvalid'))
      return
    }
    if (expiresAt.getTime() <= Date.now()) {
      appStore.showError(t('admin.subscriptions.timeRangeMustBeFuture'))
      return
    }
    payload.starts_at = startsAt.toISOString()
    payload.expires_at = expiresAt.toISOString()
  } else {
    // 前端验证：调整后的过期时间必须在未来
    if (extendingSubscription.value.expires_at) {
      const expiresAt = new Date(extendingSubscription.value.expires_at)
      const newExpiresAt = new Date(
        expiresAt.getTime() + extendForm.days * 24 * 60 * 60 * 1000
      )
      if (newExpiresAt <= new Date()) {
        appStore.showError(t('admin.subscriptions.adjustWouldExpire'))
        return
      }
    }
    payload.days = extendForm.days
  }

  if (payload.days === undefined && (!payload.starts_at || !payload.expires_at)) {
    appStore.showError(t('admin.subscriptions.failedToAdjust'))
    return
  }

  submitting.value = true
  try {
    await adminAPI.subscriptions.extend(extendingSubscription.value.id, payload)
    appStore.showSuccess(t('admin.subscriptions.subscriptionAdjusted'))
    closeExtendModal()
    loadSubscriptions()
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.subscriptions.failedToAdjust'))
    console.error('Error adjusting subscription:', error)
  } finally {
    submitting.value = false
  }
}

const handleRevoke = (subscription: UserSubscription) => {
  revokingSubscription.value = subscription
  showRevokeDialog.value = true
}

const handleOrderUsage = async (subscription: UserSubscription) => {
  orderUsageSubscription.value = subscription
  orderUsageData.value = null
  showOrderUsageModal.value = true
  orderUsageLoading.value = true
  try {
    orderUsageData.value = await adminAPI.subscriptions.getOrderUsage(subscription.id)
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || '加载订单用量失败')
    console.error('Error loading subscription order usage:', error)
  } finally {
    orderUsageLoading.value = false
  }
}

const closeOrderUsageModal = () => {
  showOrderUsageModal.value = false
  orderUsageSubscription.value = null
  orderUsageData.value = null
  orderUsageLoading.value = false
}

const confirmRevoke = async () => {
  if (!revokingSubscription.value) return

  try {
    await adminAPI.subscriptions.revoke(revokingSubscription.value.id)
    appStore.showSuccess(t('admin.subscriptions.subscriptionRevoked'))
    showRevokeDialog.value = false
    revokingSubscription.value = null
    loadSubscriptions()
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.subscriptions.failedToRevoke'))
    console.error('Error revoking subscription:', error)
  }
}

const handleResetQuota = (subscription: UserSubscription) => {
  resettingSubscription.value = subscription
  // 默认勾选所有「合法可重置」的档（与 backend validateResetTargets 规则对齐）
  resetSelection.daily = isResetWindowEnabled(subscription, 'daily')
  resetSelection.weekly = isResetWindowEnabled(subscription, 'weekly')
  resetSelection.monthly = isResetWindowEnabled(subscription, 'monthly')
  showResetQuotaConfirm.value = true
}

const formatDeltaSeconds = (seconds: number): string => {
  const sign = seconds > 0 ? '+' : seconds < 0 ? '-' : ''
  const abs = Math.abs(seconds)
  const days = Math.floor(abs / 86400)
  const hours = Math.floor((abs % 86400) / 3600)
  const minutes = Math.floor((abs % 3600) / 60)
  const parts: string[] = []
  if (days) parts.push(`${days} 天`)
  if (hours) parts.push(`${hours} 小时`)
  if (minutes || parts.length === 0) parts.push(`${minutes} 分钟`)
  return `${sign}${parts.join(' ')}`
}

const toggleWeekendSkip = async (subscription: UserSubscription) => {
  try {
    const enabled = !subscription.skip_weekends
    const preview = await adminAPI.subscriptions.previewWeekendSkip(subscription.id, enabled)
    weekendSkipTarget.value = { subscription, enabled, preview }
    showWeekendSkipConfirm.value = true
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || '获取跳过非工作日预览失败')
  }
}

const closeWeekendSkipConfirm = () => {
  showWeekendSkipConfirm.value = false
  weekendSkipTarget.value = null
  weekendSkipSubmitting.value = false
}

const confirmWeekendSkipToggle = async () => {
  if (!weekendSkipTarget.value || weekendSkipSubmitting.value) return
  weekendSkipSubmitting.value = true
  try {
    await adminAPI.subscriptions.setWeekendSkip(
      weekendSkipTarget.value.subscription.id,
      weekendSkipTarget.value.enabled
    )
    appStore.showSuccess(weekendSkipTarget.value.enabled ? '已开启跳过非工作日，到期时间已调整' : '已关闭跳过非工作日，到期时间已回算')
    closeWeekendSkipConfirm()
    await loadSubscriptions()
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || '跳过非工作日设置更新失败')
  } finally {
    weekendSkipSubmitting.value = false
  }
}

const resetWeekendSkipChange = async (subscription: UserSubscription) => {
  try {
    await adminAPI.subscriptions.resetWeekendSkipUserChange(subscription.id)
    appStore.showSuccess('已重置用户修改机会')
    await loadSubscriptions()
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || '重置用户修改机会失败')
  }
}

const closeResetQuotaDialog = () => {
  showResetQuotaConfirm.value = false
  resettingSubscription.value = null
  resetSelection.daily = false
  resetSelection.weekly = false
  resetSelection.monthly = false
}

const confirmResetQuota = async () => {
  if (!resettingSubscription.value) return
  if (resettingQuota.value) return
  if (!canSubmitResetQuota.value) return
  resettingQuota.value = true
  try {
    await adminAPI.subscriptions.resetQuota(resettingSubscription.value.id, {
      daily: resetSelection.daily,
      weekly: resetSelection.weekly,
      monthly: resetSelection.monthly
    })
    appStore.showSuccess(t('admin.subscriptions.quotaResetSuccess'))
    closeResetQuotaDialog()
    await loadSubscriptions()
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.subscriptions.failedToResetQuota'))
    console.error('Error resetting quota:', error)
  } finally {
    resettingQuota.value = false
  }
}

// ===== Reset quota dialog helpers =====

type ResetWindowKey = 'daily' | 'weekly' | 'monthly'

const resetSelection = reactive({ daily: false, weekly: false, monthly: false })

const isPaidSubscription = (sub: UserSubscription): boolean => sub.source === 'payment'

const hasConfiguredLimit = (sub: UserSubscription, key: ResetWindowKey): boolean => {
  if (sub.allow_daily_overdraft) return key === 'daily'
  const limit = sub.group?.[`${key}_limit_usd` as 'daily_limit_usd' | 'weekly_limit_usd' | 'monthly_limit_usd']
  return typeof limit === 'number' && limit > 0
}

const hasAnyConfiguredWindow = (sub: UserSubscription): boolean =>
  hasConfiguredLimit(sub, 'daily') ||
  hasConfiguredLimit(sub, 'weekly') ||
  hasConfiguredLimit(sub, 'monthly')

// 上限档 = 透支模式下只允许 daily；普通模式下按已配置的最长窗口（monthly > weekly > daily）
const upperBoundWindow = (sub: UserSubscription): ResetWindowKey | null => {
  if (sub.allow_daily_overdraft) return 'daily'
  if (hasConfiguredLimit(sub, 'monthly')) return 'monthly'
  if (hasConfiguredLimit(sub, 'weekly')) return 'weekly'
  if (hasConfiguredLimit(sub, 'daily')) return 'daily'
  return null
}

const isResetWindowEnabled = (sub: UserSubscription, key: ResetWindowKey): boolean => {
  if (isPaidSubscription(sub)) return false
  if (!hasConfiguredLimit(sub, key)) return false
  return upperBoundWindow(sub) !== key
}

const formatUsageText = (sub: UserSubscription, key: ResetWindowKey): string => {
  if (sub.allow_daily_overdraft) {
    const limit = sub.overdraft_limit_usd
    const used = getOverdraftDisplayUsed(sub) ?? 0
    return typeof limit === 'number' ? `$${used.toFixed(2)} / $${limit.toFixed(2)}` : ''
  }
  const limit = sub.group?.[`${key}_limit_usd` as 'daily_limit_usd' | 'weekly_limit_usd' | 'monthly_limit_usd']
  if (typeof limit !== 'number') return ''
  const used = sub[`${key}_usage_usd` as 'daily_usage_usd' | 'weekly_usage_usd' | 'monthly_usage_usd'] ?? 0
  return `$${used.toFixed(2)} / $${limit.toFixed(2)}`
}

const resetWindowOptions = computed(() => {
  const sub = resettingSubscription.value
  if (!sub) return []
  const upper = upperBoundWindow(sub)
  const labels: Record<ResetWindowKey, string> = {
    daily: t('admin.subscriptions.daily'),
    weekly: t('admin.subscriptions.weekly'),
    monthly: t('admin.subscriptions.monthly')
  }
  const keys: ResetWindowKey[] = ['daily', 'weekly', 'monthly']
  return keys
    .filter((key) => hasConfiguredLimit(sub, key))
    .map((key) => {
      const isUpper = upper === key
      return {
        key,
        label: labels[key],
        usageText: formatUsageText(sub, key),
        disabled: isUpper,
        disabledReason: isUpper ? t('admin.subscriptions.resetQuotaUpperBoundDisabled') : ''
      }
    })
})

const canSubmitResetQuota = computed(() => {
  const sub = resettingSubscription.value
  if (!sub) return false
  if (isPaidSubscription(sub)) return false
  if (!hasAnyConfiguredWindow(sub)) return false
  return resetSelection.daily || resetSelection.weekly || resetSelection.monthly
})

const toggleResetSelection = (key: ResetWindowKey) => {
  resetSelection[key] = !resetSelection[key]
}

// Helper functions
const getDaysRemaining = (expiresAt: string): number | null => {
  const now = new Date()
  const expires = new Date(expiresAt)
  const diff = expires.getTime() - now.getTime()
  if (diff < 0) return null
  return Math.ceil(diff / (1000 * 60 * 60 * 24))
}

const getRemainingText = (expiresAt: string): string | null => formatRemainingDuration(expiresAt)

const formatOptionalMoney = (value?: number | null): string => {
  if (typeof value !== 'number' || Number.isNaN(value)) return '-'
  return `$${value.toFixed(2)}`
}

const formatCompactNumber = (value: number): string => {
  if (!Number.isFinite(value)) return '0'
  return new Intl.NumberFormat(undefined, { notation: 'compact', maximumFractionDigits: 1 }).format(value)
}

const formatOrderWindowKind = (order: SubscriptionOrderUsageItem): string => {
  if (order.renewal_mode === 'restart' || order.window_kind === 'restart') return '重开'
  if (order.window_kind === 'renewal') return '续费'
  return '购买'
}

const isOrderOverQuota = (order: SubscriptionOrderUsageItem): boolean => {
  return typeof order.quota_usd === 'number' && order.used_actual_cost_usd > order.quota_usd + 0.000001
}

const isExpiringSoon = (expiresAt: string): boolean => {
  const days = getDaysRemaining(expiresAt)
  return days !== null && days <= 7
}

const isSubscriptionNotStarted = (sub: UserSubscription): boolean => {
  if (sub.status !== 'active' || !sub.starts_at) return false
  const startsAtMs = new Date(sub.starts_at).getTime()
  if (Number.isNaN(startsAtMs)) return false
  return startsAtMs > Date.now()
}

const hasTotalQuota = (sub: UserSubscription): boolean => {
  return sub.quota_limit_usd != null && sub.quota_limit_usd > 0
}

const getTotalQuotaLimit = (sub: UserSubscription): number => {
  return sub.quota_limit_usd && sub.quota_limit_usd > 0 ? sub.quota_limit_usd : 0
}

const getTotalQuotaUsed = (sub: UserSubscription): number => {
  return Math.max(sub.quota_used_usd || 0, 0)
}

const getTotalQuotaRemaining = (sub: UserSubscription): number => {
  if (typeof sub.quota_remaining_usd === 'number') {
    return Math.max(sub.quota_remaining_usd, 0)
  }
  return Math.max(getTotalQuotaLimit(sub) - getTotalQuotaUsed(sub), 0)
}

const getProgressWidth = (used: number | null | undefined, limit: number | null): string => {
  if (!limit || limit === 0) return '0%'
  const usedValue = used ?? 0
  const percentage = Math.min((usedValue / limit) * 100, 100)
  return `${percentage}%`
}

const getProgressClass = (used: number | null | undefined, limit: number | null): string => {
  if (!limit || limit === 0) return 'bg-gray-400'
  const usedValue = used ?? 0
  const percentage = (usedValue / limit) * 100
  if (percentage >= 90) return 'bg-red-500'
  if (percentage >= 70) return 'bg-orange-500'
  return 'bg-green-500'
}

const getOverdraftLimit = (sub: UserSubscription): number | null => {
  return typeof sub.overdraft_limit_usd === 'number'
    && sub.overdraft_limit_usd > 0
    ? sub.overdraft_limit_usd
    : null
}

const getOverdraftDisplayUsed = (sub: UserSubscription): number | null => {
  const limit = getOverdraftLimit(sub)
  if (limit === null) return null
  if (typeof sub.overdraft_used_usd === 'number') {
    return Math.max(sub.overdraft_used_usd, 0)
  }
  return isDayValidityUnit(sub.validity_unit)
    ? getDayValidityOverdraftUsed(sub)
    : (sub.overdraft_used_usd ?? sub.weekly_usage_usd ?? 0)
}

const isDayValidityUnit = (unit?: string | null): boolean => {
  const normalized = (unit || 'day').trim().toLowerCase()
  return normalized === '' || normalized === 'day' || normalized === 'days'
}

const getTodayOverdraftAmount = (sub: UserSubscription): number => {
  const dailyLimit = sub.group?.daily_limit_usd
  if (!dailyLimit || dailyLimit <= 0 || getOverdraftLimit(sub) === null) return 0
  return Math.max((sub.daily_usage_usd || 0) - dailyLimit, 0)
}

const getOverdraftRemaining = (sub: UserSubscription): number => {
  const limit = getOverdraftLimit(sub)
  if (limit === null) return 0
  return Math.max(limit - (getOverdraftDisplayUsed(sub) ?? 0), 0)
}

const getElapsedOverdraftQuota = (sub: UserSubscription): number => {
  const dailyLimit = sub.group?.daily_limit_usd
  const overdraftLimit = getOverdraftLimit(sub)
  if (!dailyLimit || dailyLimit <= 0 || overdraftLimit === null) return 0

  return Math.min(dailyLimit * getElapsedFullOverdraftDays(sub), overdraftLimit)
}

const getDayValidityOverdraftUsed = (sub: UserSubscription): number => {
  const dailyLimit = sub.group?.daily_limit_usd
  const overdraftLimit = getOverdraftLimit(sub)
  if (!dailyLimit || dailyLimit <= 0 || overdraftLimit === null) return 0

  const expiredQuota = getElapsedOverdraftQuota(sub)
  const currentDailyUsage = getCurrentDailyWindowUsage(sub)
  const effectiveUsed = expiredQuota + currentDailyUsage
  const actualUsed = sub.weekly_usage_usd ?? sub.overdraft_used_usd ?? 0
  return Math.min(Math.max(actualUsed, effectiveUsed), overdraftLimit)
}

const getElapsedFullOverdraftDays = (sub: UserSubscription): number => {
  const dailyLimit = sub.group?.daily_limit_usd
  const overdraftLimit = getOverdraftLimit(sub)
  if (!dailyLimit || dailyLimit <= 0 || overdraftLimit === null) return 0

  const startsAt = sub.starts_at ? new Date(sub.starts_at).getTime() : NaN
  if (!Number.isFinite(startsAt)) return 0

  const dayMs = 24 * 60 * 60 * 1000
  const elapsedDays = Math.max(0, Math.floor((Date.now() - startsAt) / dayMs))
  const validityDays = Math.max(1, Math.ceil(overdraftLimit / dailyLimit))
  return Math.min(elapsedDays, validityDays)
}

const getCurrentDailyWindowUsage = (sub: UserSubscription): number => {
  if (!sub.daily_window_start || !sub.starts_at) return 0

  const startsAt = new Date(sub.starts_at).getTime()
  const dailyWindowStart = new Date(sub.daily_window_start).getTime()
  if (!Number.isFinite(startsAt) || !Number.isFinite(dailyWindowStart)) return 0

  const dayMs = 24 * 60 * 60 * 1000
  const elapsedDays = Math.max(0, Math.floor((Date.now() - startsAt) / dayMs))
  const currentWindowStart = startsAt + elapsedDays * dayMs
  if (dailyWindowStart !== currentWindowStart) return 0

  return Math.max(sub.daily_usage_usd || 0, 0)
}

// Format reset time based on window start and period type
const formatResetTime = (windowStart: string, period: 'daily' | 'weekly' | 'monthly'): string => {
  if (!windowStart) return t('admin.subscriptions.windowNotActive')

  const start = new Date(windowStart)
  const now = new Date()

  // Calculate reset time based on period
  let resetTime: Date
  switch (period) {
    case 'daily':
      resetTime = new Date(start.getTime() + 24 * 60 * 60 * 1000)
      break
    case 'weekly':
      resetTime = new Date(start.getTime() + 7 * 24 * 60 * 60 * 1000)
      break
    case 'monthly':
      resetTime = new Date(start)
      resetTime.setMonth(resetTime.getMonth() + 1)
      break
  }

  if (resetTime <= now) return t('admin.subscriptions.windowNotActive')

  const diff = resetTime.getTime() - now.getTime()
  const days = Math.floor(diff / (24 * 60 * 60 * 1000))
  const hours = Math.floor((diff % (24 * 60 * 60 * 1000)) / (60 * 60 * 1000))
  const minutes = Math.floor((diff % (60 * 60 * 1000)) / (60 * 1000))

  if (days > 0) return t('admin.subscriptions.resetInDaysHours', { days, hours })
  if (hours > 0) return t('admin.subscriptions.resetInHoursMinutes', { hours, minutes })
  return t('admin.subscriptions.resetInMinutes', { minutes })
}

const handleClickOutside = (event: MouseEvent) => {
  const target = event.target as HTMLElement
  if (!target.closest('[data-assign-user-search]')) showUserDropdown.value = false
  if (!target.closest('[data-filter-user-search]')) showFilterUserDropdown.value = false
  if (columnDropdownRef.value && !columnDropdownRef.value.contains(target)) {
    showColumnDropdown.value = false
  }
}

onMounted(() => {
  loadUserColumnMode()
  loadSavedColumns()
  loadGroups()
  loadSubscriptions()
  document.addEventListener('click', handleClickOutside)
})

onUnmounted(() => {
  if (abortController) {
    abortController.abort()
    abortController = null
  }
  document.removeEventListener('click', handleClickOutside)
  if (filterUserSearchTimeout) {
    clearTimeout(filterUserSearchTimeout)
  }
  if (userSearchTimeout) {
    clearTimeout(userSearchTimeout)
  }
})
</script>

<style scoped>
.usage-row {
  @apply space-y-1;
}

.usage-label {
  @apply w-10 flex-shrink-0 text-xs font-medium text-gray-500 dark:text-gray-400;
}

.usage-amount {
  @apply whitespace-nowrap text-xs tabular-nums text-gray-600 dark:text-gray-300;
}

.reset-info {
  @apply flex items-center gap-1 pl-12 text-[10px] text-blue-600 dark:text-blue-400;
}
</style>
