package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"Billingmind/internal/db"
)

type TaskHandler func(ctx context.Context, task db.AgentTask) error

type TaskConsumer struct {
	client       *redis.Client
	group        string
	consumerName string
}

func NewTaskConsumer(client *redis.Client, consumerName string) *TaskConsumer {
	return &TaskConsumer{
		client:       client,
		group:        ConsumerGroup,
		consumerName: consumerName,
	}
}

func (c *TaskConsumer) EnsureGroup(ctx context.Context) error {
	err := c.client.XGroupCreateMkStream(ctx, StreamName, c.group, "0").Err()
	if err != nil && !isGroupExistsError(err) {
		return fmt.Errorf("create consumer group: %w", err)
	}
	return nil
}

func (c *TaskConsumer) Consume(ctx context.Context, handler TaskHandler) error {
	if err := c.EnsureGroup(ctx); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		streams, err := c.client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    c.group,
			Consumer: c.consumerName,
			Streams:  []string{StreamName, ">"},
			Count:    10,
			Block:    5 * time.Second,
		}).Result()

		if err != nil {
			if errors.Is(err, redis.Nil) {
				continue
			}
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil
			}
			log.Printf("xreadgroup error: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}

		for _, stream := range streams {
			for _, msg := range stream.Messages {
				task, parseErr := parseStreamMessage(msg)
				if parseErr != nil {
					log.Printf("failed to parse stream message %s: %v", msg.ID, parseErr)
					c.client.XAck(ctx, StreamName, c.group, msg.ID)
					continue
				}

				if handleErr := handler(ctx, task); handleErr != nil {
					log.Printf("failed to handle task %s: %v", task.ID, handleErr)
					continue
				}

				c.client.XAck(ctx, StreamName, c.group, msg.ID)
			}
		}
	}
}

func parseStreamMessage(msg redis.XMessage) (db.AgentTask, error) {
	var task db.AgentTask

	taskIDStr, ok := msg.Values["task_id"].(string)
	if !ok {
		return task, fmt.Errorf("missing task_id")
	}
	id, err := uuid.Parse(taskIDStr)
	if err != nil {
		return task, fmt.Errorf("invalid task_id: %w", err)
	}
	task.ID = id

	task.TaskType, _ = msg.Values["task_type"].(string)
	task.TargetAgent, _ = msg.Values["target_agent"].(string)

	if priorityStr, ok := msg.Values["priority"].(string); ok {
		task.Priority, _ = strconv.Atoi(priorityStr)
	}

	if payloadStr, ok := msg.Values["payload"].(string); ok {
		task.Payload = json.RawMessage(payloadStr)
	}

	task.Status = "pending"
	return task, nil
}

func isGroupExistsError(err error) bool {
	return err != nil && err.Error() == "BUSYGROUP Consumer Group name already exists"
}
