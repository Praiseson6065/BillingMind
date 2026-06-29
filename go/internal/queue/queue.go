package queue

import (
	"Billingmind/config"

	"github.com/redis/go-redis/v9"
)

const (
	StreamName    = "billingmind:tasks"
	ConsumerGroup = "orchestrator"
)

func NewRedisClient(cfg config.RedisConfig) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr: cfg.Addr,
	})
}
