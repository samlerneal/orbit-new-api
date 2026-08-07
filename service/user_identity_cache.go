package service

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/QuantumNous/new-api/common"
)

var ErrRedisTopologyUnverified = errors.New("REDIS_TOPOLOGY_UNVERIFIED")

func verifyUserIdentityCacheTopology(ctx context.Context) error {
	if !common.RedisEnabled {
		return nil
	}
	if common.RDB == nil {
		return ErrRedisTopologyUnverified
	}
	if err := common.RDB.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("%w: Redis PING failed", ErrRedisTopologyUnverified)
	}
	readKeys := func() ([]string, error) {
		iterator := common.RDB.Scan(ctx, 0, "*", 0).Iterator()
		keys := make([]string, 0)
		for iterator.Next(ctx) {
			keys = append(keys, iterator.Val())
		}
		if err := iterator.Err(); err != nil {
			return nil, err
		}
		sort.Strings(keys)
		return keys, nil
	}
	before, err := common.RDB.DBSize(ctx).Result()
	if err != nil {
		return ErrRedisTopologyUnverified
	}
	first, err := readKeys()
	if err != nil {
		return ErrRedisTopologyUnverified
	}
	second, err := readKeys()
	if err != nil {
		return ErrRedisTopologyUnverified
	}
	after, err := common.RDB.DBSize(ctx).Result()
	if err != nil || before != after || int64(len(first)) != before || len(first) != len(second) {
		return ErrRedisTopologyUnverified
	}
	for index := range first {
		if first[index] != second[index] {
			return ErrRedisTopologyUnverified
		}
	}
	return nil
}

// InvalidateUserIdentityCaches removes only exact O-023 user namespaces.
// Shared Redis databases are rejected by the migration preflight; FLUSHDB is
// intentionally never used.
func InvalidateUserIdentityCaches(ctx context.Context, userIDs []int) error {
	if !common.RedisEnabled {
		return nil
	}
	if err := verifyUserIdentityCacheTopology(ctx); err != nil {
		return err
	}
	keys := make([]string, 0, len(userIDs)*3)
	for _, userID := range userIDs {
		keys = append(keys,
			fmt.Sprintf("user:%d", userID),
			fmt.Sprintf("auth:user:fence:%d", userID),
			fmt.Sprintf("auth:user:version:%d", userID),
		)
	}
	if len(keys) == 0 {
		return nil
	}
	return common.RDB.Del(ctx, keys...).Err()
}
