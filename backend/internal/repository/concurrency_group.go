// Adapted from DeanZFC/sub2api-custom 915cc0ed (LGPL-3.0).
package repository

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

const userGroupSlotKeyPrefix = "concurrency:user_group:"
const liveUserGroupSlotKeyPrefix = "concurrency:live:user_group:"

func userGroupSlotKey(userID, groupID int64) string {
	return fmt.Sprintf("%s%d:%d", userGroupSlotKeyPrefix, userID, groupID)
}
func liveUserGroupSlotKey(userID, groupID int64) string {
	return fmt.Sprintf("%s%d:%d", liveUserGroupSlotKeyPrefix, userID, groupID)
}

var acquireUserGroupScript = redis.NewScript(`
		redis.replicate_commands()
		local userKey = KEYS[1]
		local userLiveKey = KEYS[2]
		local groupKey = KEYS[3]
		local groupLiveKey = KEYS[4]
		local userMax = tonumber(ARGV[1])
		local groupMax = tonumber(ARGV[2])
		local ttl = tonumber(ARGV[3])
		local requestID = ARGV[4]
		local now = tonumber(redis.call('TIME')[1])
		local expireBefore = now - ttl
		redis.call('ZREMRANGEBYSCORE', userKey, '-inf', expireBefore)
		redis.call('ZREMRANGEBYSCORE', userLiveKey, '-inf', now - 60)
		redis.call('ZREMRANGEBYSCORE', groupKey, '-inf', expireBefore)
		redis.call('ZREMRANGEBYSCORE', groupLiveKey, '-inf', now - 60)
		-- Startup cleanup owns the global set. Drop matching group orphans too.
		redis.call('ZINTERSTORE', groupKey, 2, groupKey, userKey, 'AGGREGATE', 'MIN')
		if redis.call('ZCARD', groupKey) > 0 then redis.call('EXPIRE', groupKey, ttl) end
		if redis.call('ZSCORE', userKey, requestID) ~= false then
			redis.call('ZADD', userKey, now, requestID)
			redis.call('ZADD', groupKey, now, requestID)
			redis.call('EXPIRE', userKey, ttl)
			redis.call('EXPIRE', groupKey, ttl)
			return {1, now}
		end
		if userMax > 0 and (redis.call('ZCARD', userKey) + redis.call('ZCARD', userLiveKey)) >= userMax then
			return {0, now}
		end
		if groupMax > 0 and (redis.call('ZCARD', groupKey) + redis.call('ZCARD', groupLiveKey)) >= groupMax then
			return {0, now}
		end
		redis.call('ZADD', userKey, now, requestID)
		redis.call('ZADD', groupKey, now, requestID)
		redis.call('EXPIRE', userKey, ttl)
		redis.call('EXPIRE', groupKey, ttl)
		return {1, now}
	`)

var acquireLiveLeaseForGroupScript = redis.NewScript(`
		redis.replicate_commands()
		local accountRegular = KEYS[1]
		local accountLive = KEYS[2]
		local userRegular = KEYS[3]
		local userLive = KEYS[4]
		local groupRegular = KEYS[5]
		local groupLive = KEYS[6]
		local apiLive = KEYS[7]
		local accountMax = tonumber(ARGV[1])
		local userMax = tonumber(ARGV[2])
		local groupMax = tonumber(ARGV[3])
		local ttl = tonumber(ARGV[4])
		local leaseID = ARGV[5]
		local replacing = tonumber(ARGV[6])
		local now = tonumber(redis.call('TIME')[1])
		local liveExpireBefore = now - ttl
		redis.call('ZREMRANGEBYSCORE', accountLive, '-inf', liveExpireBefore)
		redis.call('ZREMRANGEBYSCORE', userLive, '-inf', liveExpireBefore)
		redis.call('ZREMRANGEBYSCORE', groupLive, '-inf', liveExpireBefore)
		redis.call('ZREMRANGEBYSCORE', apiLive, '-inf', liveExpireBefore)
		if redis.call('ZSCORE', accountLive, leaseID) ~= false then
			return 1
		end
		local accountCount = redis.call('ZCARD', accountRegular) + redis.call('ZCARD', accountLive)
		local userCount = redis.call('ZCARD', userRegular) + redis.call('ZCARD', userLive)
		local groupCount = redis.call('ZCARD', groupRegular) + redis.call('ZCARD', groupLive)
		local allowance = 0
		if replacing == 1 then allowance = 1 end
		if accountMax > 0 and accountCount >= accountMax + allowance then return 0 end
		if userMax > 0 and userCount >= userMax + allowance then return 0 end
		if groupMax > 0 and groupCount >= groupMax + allowance then return 0 end
		redis.call('ZADD', accountLive, now, leaseID)
		redis.call('ZADD', userLive, now, leaseID)
		redis.call('ZADD', groupLive, now, leaseID)
		redis.call('ZADD', apiLive, now, leaseID)
		redis.call('EXPIRE', accountLive, ttl)
		redis.call('EXPIRE', userLive, ttl)
		redis.call('EXPIRE', groupLive, ttl)
		redis.call('EXPIRE', apiLive, ttl)
		return 1
	`)

func (c *concurrencyCache) AcquireUserGroupSlot(ctx context.Context, userID, groupID int64, userMax, groupMax int, requestID string) (bool, error) {
	if c == nil || c.rdb == nil || userID <= 0 || groupID <= 0 || requestID == "" || groupMax <= 0 {
		return false, nil
	}
	result, now, err := runScriptInt64Pair(ctx, c.rdb, acquireUserGroupScript, []string{
		userSlotKey(userID),
		liveUserSlotKey(userID),
		userGroupSlotKey(userID, groupID),
		liveUserGroupSlotKey(userID, groupID),
	}, userMax, groupMax, c.slotTTLSeconds, requestID)
	if err != nil {
		return false, err
	}
	if result == 1 {
		c.touchActiveIndexAt(ctx, userActiveIndexKey, userID, now+int64(c.slotTTLSeconds))
	}
	return result == 1, nil
}

func (c *concurrencyCache) ReleaseUserGroupSlot(ctx context.Context, userID, groupID int64, requestID string) error {
	if c == nil || c.rdb == nil || userID <= 0 || groupID <= 0 || requestID == "" {
		return nil
	}
	pipe := c.rdb.TxPipeline()
	pipe.ZRem(ctx, userSlotKey(userID), requestID)
	pipe.ZRem(ctx, userGroupSlotKey(userID, groupID), requestID)
	if _, err := pipe.Exec(ctx); err != nil {
		return err
	}
	c.refreshUserActiveIndex(ctx, userID)
	return nil
}

func (c *concurrencyCache) AcquireLiveLeaseForGroup(
	ctx context.Context,
	accountID int64,
	accountMax int,
	userID int64,
	userMax int,
	groupID int64,
	groupMax int,
	apiKeyID int64,
	leaseID string,
	replacingRegularSlots bool,
) (bool, error) {
	if c == nil || c.rdb == nil || accountID <= 0 || userID <= 0 || groupID <= 0 || apiKeyID <= 0 || leaseID == "" || groupMax <= 0 {
		return false, nil
	}
	replacing := 0
	if replacingRegularSlots {
		replacing = 1
	}
	result, err := acquireLiveLeaseForGroupScript.Run(ctx, c.rdb, []string{
		accountSlotKey(accountID),
		liveAccountSlotKey(accountID),
		userSlotKey(userID),
		liveUserSlotKey(userID),
		userGroupSlotKey(userID, groupID),
		liveUserGroupSlotKey(userID, groupID),
		liveAPIKeySlotKey(apiKeyID),
	}, accountMax, userMax, groupMax, liveLeaseTTLSeconds, leaseID, replacing).Int()
	return result == 1, err
}

func (c *concurrencyCache) RefreshLiveLeaseForGroup(ctx context.Context, accountID, userID, groupID, apiKeyID int64, leaseID string) (bool, error) {
	if c == nil || c.rdb == nil || groupID <= 0 || leaseID == "" {
		return false, nil
	}
	result, err := refreshLiveLeaseScript.Run(ctx, c.rdb, []string{
		liveAccountSlotKey(accountID),
		liveUserSlotKey(userID),
		liveUserGroupSlotKey(userID, groupID),
		liveAPIKeySlotKey(apiKeyID),
	}, liveLeaseTTLSeconds, leaseID).Int()
	return result == 1, err
}

func (c *concurrencyCache) ReleaseLiveLeaseForGroup(ctx context.Context, accountID, userID, groupID, apiKeyID int64, leaseID string) error {
	if c == nil || c.rdb == nil || groupID <= 0 || leaseID == "" {
		return nil
	}
	pipe := c.rdb.TxPipeline()
	pipe.ZRem(ctx, liveAccountSlotKey(accountID), leaseID)
	pipe.ZRem(ctx, liveUserSlotKey(userID), leaseID)
	pipe.ZRem(ctx, liveUserGroupSlotKey(userID, groupID), leaseID)
	pipe.ZRem(ctx, liveAPIKeySlotKey(apiKeyID), leaseID)
	_, err := pipe.Exec(ctx)
	return err
}
