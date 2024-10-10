package rate

import (
	"context"
	"fmt"
	"time"

	"github.com/valkey-io/valkey-go"
	"go.uber.org/zap"
)

// Limiter manages rate limiting across multiple providers, regions, and models using Redis.
type Limiter struct {
	valkeyClient valkey.Client
	logger       *zap.SugaredLogger
}

// NewLimiter creates a new MultiRateLimiter with the given Redis client and interval.
func NewLimiter(valkeyClient valkey.Client, logger *zap.SugaredLogger) *Limiter {
	return &Limiter{
		valkeyClient: valkeyClient,
		logger:       logger,
	}
}

func (l *Limiter) CanProceed(ctx context.Context, provider string, region string, model string, interval time.Duration) (bool, time.Duration, error) {
	key := fmt.Sprintf("ogem:disabled:%s:%s:%s", provider, region, model)

	// Lua script to atomically check and update the disabled status using microseconds
	script := `
local redis_time = redis.call('TIME')
local current_time_micro = tonumber(redis_time[1]) * 1000000 + tonumber(redis_time[2])
local disabled_until_micro = redis.call('GET', KEYS[1])

-- If no disabled_until exists, or it has passed, set a new disabled_until and proceed
if not disabled_until_micro or tonumber(disabled_until_micro) <= current_time_micro then
	local new_disabled_until_micro = current_time_micro + tonumber(ARGV[1]) * 1000
	redis.call('SET', KEYS[1], new_disabled_until_micro)
	redis.call('PEXPIRE', KEYS[1], ARGV[1])  -- Set expiration time in milliseconds
	return {1}
else
	-- Otherwise, return the remaining disabled time
	return {0, tonumber(disabled_until_micro) - current_time_micro}
end
`

	resp := l.valkeyClient.Do(ctx, l.valkeyClient.B().Eval().Script(script).Numkeys(1).Key(key).Arg(
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

func (l *Limiter) DisableEndpointTemporarily(ctx context.Context, provider, region, model string, duration time.Duration) error {
	key := fmt.Sprintf("ogem:disabled:%s:%s:%s", provider, region, model)

	// Lua script to atomically set the disabled time using Redis server's time in microseconds, and set expiration time
	script := `
local redis_time = redis.call('TIME')
local current_time_micro = tonumber(redis_time[1]) * 1000000 + tonumber(redis_time[2])
local new_disabled_until_micro = current_time_micro + tonumber(ARGV[1]) * 1000
redis.call('SET', KEYS[1], new_disabled_until_micro)
redis.call('PEXPIRE', KEYS[1], ARGV[1])
return new_disabled_until_micro
`

	resp := l.valkeyClient.Do(ctx, l.valkeyClient.B().Eval().Script(script).Numkeys(1).Key(key).Arg(
		fmt.Sprintf("%d", duration.Milliseconds()),
	).Build())

	return resp.Error()
}
