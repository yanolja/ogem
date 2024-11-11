package state

import (
	"context"
	"fmt"
	"time"

	"github.com/valkey-io/valkey-go"
)

type ValkeyManager struct {
	client valkey.Client
}

func NewValkeyManager(client valkey.Client) *ValkeyManager {
	return &ValkeyManager{client: client}
}

func (r *ValkeyManager) Allow(ctx context.Context, provider string, region string, model string, interval time.Duration) (bool, time.Duration, error) {
	key := fmt.Sprintf("ogem:disabled:%s:%s:%s", provider, region, model)

	script := `
		local current_time_micro = redis.call('TIME')[1] * 1000000 + redis.call('TIME')[2]
		local disabled_until_micro = redis.call('GET', KEYS[1])

		if not disabled_until_micro or tonumber(disabled_until_micro) <= current_time_micro then
			local new_disabled_until_micro = current_time_micro + tonumber(ARGV[1]) * 1000
			redis.call('SET', KEYS[1], new_disabled_until_micro)
			redis.call('PEXPIRE', KEYS[1], ARGV[1])
			return {1}
		else
			return {0, tonumber(disabled_until_micro) - current_time_micro}
		end
	`

	resp := r.client.Do(ctx, r.client.B().Eval().Script(script).Numkeys(1).Key(key).Arg(
		fmt.Sprintf("%d", interval.Milliseconds()),
	).Build())

	result, err := resp.AsIntSlice()
	if err != nil {
		return false, 0, err
	}

	if result[0] == 1 {
		return true, 0, nil
	} else {
		return false, time.Duration(result[1]) * time.Microsecond, nil
	}
}

func (r *ValkeyManager) Disable(ctx context.Context, provider string, region string, model string, duration time.Duration) error {
	key := fmt.Sprintf("ogem:disabled:%s:%s:%s", provider, region, model)

	script := `
		local current_time_micro = redis.call('TIME')[1] * 1000000 + redis.call('TIME')[2]
		local new_disabled_until_micro = current_time_micro + tonumber(ARGV[1]) * 1000
		redis.call('SET', KEYS[1], new_disabled_until_micro)
		redis.call('PEXPIRE', KEYS[1], ARGV[1])
		return new_disabled_until_micro
	`

	resp := r.client.Do(ctx, r.client.B().Eval().Script(script).Numkeys(1).Key(key).Arg(
		fmt.Sprintf("%d", duration.Milliseconds()),
	).Build())

	return resp.Error()
}

func (r *ValkeyManager) SaveCache(
	ctx context.Context, key string, value []byte, duration time.Duration,
) error {
	return r.client.Do(
		ctx, r.client.B().Set().
			Key(key).
			Value(valkey.BinaryString(value)).
			Ex(duration).
			Build(),
	).Error()
}

func (r *ValkeyManager) LoadCache(ctx context.Context, key string) ([]byte, error) {
	valkeyResponse := r.client.Do(ctx, r.client.B().Get().Key(key).Build())
	if err := valkeyResponse.Error(); err != nil {
		if valkey.IsValkeyNil(err) {
			return nil, nil
		}
		return nil, err
	}
	return valkeyResponse.AsBytes()
}