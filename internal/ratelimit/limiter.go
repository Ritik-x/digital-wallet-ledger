package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type Result struct {
	Allowed    bool
	Limit      int
	Remaining  int
	RetryAfter time.Duration
}

type Limiter struct {
	client *redis.Client
}

func NewLimiter(client *redis.Client) *Limiter {
	return &Limiter{
		client: client,
	}
}

var fixedWindowScript = redis.NewScript(`
local current = redis.call("INCR", KEYS[1])

if current == 1 then
    redis.call("PEXPIRE", KEYS[1], ARGV[1])
end

local ttl = redis.call("PTTL", KEYS[1])

if current > tonumber(ARGV[2]) then
    return {0, 0, ttl}
end

return {1, tonumber(ARGV[2]) - current, ttl}
`)

func (l *Limiter) Allow(
	ctx context.Context,
	key string,
	limit int,
	window time.Duration,
) (Result, error) {
	if key == "" {
		return Result{}, fmt.Errorf("rate limit key cannot be empty")
	}

	if limit <= 0 {
		return Result{}, fmt.Errorf("rate limit must be positive")
	}

	if window <= 0 {
		return Result{}, fmt.Errorf("rate limit window must be positive")
	}

	windowMillis := window.Milliseconds()
	if windowMillis < 1 {
		return Result{}, fmt.Errorf("rate limit window must be at least 1 millisecond")
	}

	values, err := fixedWindowScript.Run(
		ctx,
		l.client,
		[]string{key},
		windowMillis,
		limit,
	).Slice()
	if err != nil {
		return Result{}, fmt.Errorf("execute rate limit script: %w", err)
	}

	if len(values) != 3 {
		return Result{}, fmt.Errorf("unexpected rate limit response")
	}

	allowed, err := redisInt(values[0])
	if err != nil {
		return Result{}, err
	}

	remaining, err := redisInt(values[1])
	if err != nil {
		return Result{}, err
	}

	ttlMillis, err := redisInt64(values[2])
	if err != nil {
		return Result{}, err
	}

	result := Result{
		Allowed:   allowed == 1,
		Limit:     limit,
		Remaining: remaining,
	}

	if !result.Allowed && ttlMillis > 0 {
		result.RetryAfter = time.Duration(ttlMillis) * time.Millisecond
	}

	return result, nil
}

func redisInt(value interface{}) (int, error) {
	number, err := redisInt64(value)
	if err != nil {
		return 0, err
	}
	return int(number), nil
}

func redisInt64(value interface{}) (int64, error) {
	switch number := value.(type) {
	case int64:
		return number, nil
	case int:
		return int64(number), nil
	case string:
		var parsed int64
		_, err := fmt.Sscan(number, &parsed)
		if err != nil {
			return 0, fmt.Errorf("parse Redis integer: %w", err)
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("unexpected Redis value type %T", value)
	}
}
