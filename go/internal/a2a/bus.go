package a2a

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/redis/go-redis/v9"
)

type Message struct {
	FromAgent     string          `json:"from_agent"`
	ToAgent       string          `json:"to_agent"`
	MessageType   string          `json:"message_type"`
	Payload       json.RawMessage `json:"payload"`
	CorrelationID string          `json:"correlation_id"`
}

type Bus struct {
	client *redis.Client
}

func NewBus(client *redis.Client) *Bus {
	return &Bus{client: client}
}

func (b *Bus) channelName(agentName string) string {
	return fmt.Sprintf("billingmind:a2a:%s", agentName)
}

func (b *Bus) Publish(ctx context.Context, msg Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal a2a message: %w", err)
	}
	return b.client.Publish(ctx, b.channelName(msg.ToAgent), data).Err()
}

func (b *Bus) Subscribe(ctx context.Context, agentName string, handler func(Message)) error {
	sub := b.client.Subscribe(ctx, b.channelName(agentName))
	defer sub.Close()

	ch := sub.Channel()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case redisMsg, ok := <-ch:
			if !ok {
				return nil
			}
			var msg Message
			if err := json.Unmarshal([]byte(redisMsg.Payload), &msg); err != nil {
				log.Printf("failed to unmarshal a2a message: %v", err)
				continue
			}
			handler(msg)
		}
	}
}
