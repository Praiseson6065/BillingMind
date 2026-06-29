package queue

import (
	"Billingmind/internal/db"
	"context"
	"encoding/json"
	"fmt"

	"github.com/redis/go-redis/v9"
)

type TaskProducer struct {
	client *redis.Client
}

func NewTaskProducer(client *redis.Client) *TaskProducer {
	return &TaskProducer{client: client}
}

func (p *TaskProducer) Publish(ctx context.Context, task db.AgentTask) error {
	payload, err := json.Marshal(task.Payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	return p.client.XAdd(ctx, &redis.XAddArgs{
		Stream: StreamName,
		Values: map[string]interface{}{
			"task_id":      task.ID.String(),
			"task_type":    task.TaskType,
			"target_agent": task.TargetAgent,
			"priority":     task.Priority,
			"payload":      string(payload),
		},
	}).Err()
}
