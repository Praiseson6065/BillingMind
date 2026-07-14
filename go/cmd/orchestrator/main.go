package main

import (
	"Billingmind/config"
	"Billingmind/internal/db"
	"Billingmind/internal/ontology"
	"Billingmind/internal/queue"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type Orchestrator struct {
	queries     *db.Queries
	resolver    *ontology.Resolver
	ont         *ontology.Ontology
	agentURLs   map[string]string
	redisClient *queue.TaskConsumer
	httpClient  *http.Client
}

func main() {
	_ = godotenv.Load()

	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatal("unable to load config: ", err)
	}

	pool, err := db.NewPool(context.Background(), cfg.Postgres)
	if err != nil {
		log.Fatal("unable to create db pool: ", err)
	}
	defer pool.Close()

	queries := db.NewQueries(pool)

	ont, err := ontology.LoadOntology(cfg.OntologyPath)
	if err != nil {
		log.Fatal("unable to load ontology: ", err)
	}
	resolver := ontology.NewResolver(ont)

	redisClient := queue.NewRedisClient(cfg.Redis)
	defer redisClient.Close()

	consumer := queue.NewTaskConsumer(redisClient, "orchestrator-main")

	orch := &Orchestrator{
		queries:   queries,
		resolver:  resolver,
		ont:       ont,
		redisClient: consumer,
		agentURLs: map[string]string{
			"billing":  cfg.Agents.BillingURL,
			"recovery": cfg.Agents.RecoveryURL,
			"support":  cfg.Agents.SupportURL,
			"audit":    cfg.Agents.AuditURL,
		},
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}

	mcpServer := server.NewMCPServer(
		"BillingMind Orchestrator",
		"1.0.0",
	)

	mcpServer.AddTool(
		mcp.NewTool("route_task",
			mcp.WithDescription("Routes a pending task to the correct agent based on ontology resolution"),
			mcp.WithString("task_id", mcp.Required(), mcp.Description("UUID of the task to route")),
		),
		orch.routeTaskHandler,
	)

	mcpServer.AddTool(
		mcp.NewTool("get_agent_status",
			mcp.WithDescription("Checks the health status of a specific agent"),
			mcp.WithString("agent_name", mcp.Required(), mcp.Description("Name of the agent: billing, recovery, support, audit")),
		),
		orch.getAgentStatusHandler,
	)

	mcpServer.AddTool(
		mcp.NewTool("list_pending_tasks",
			mcp.WithDescription("Lists all tasks in pending status from the database"),
		),
		orch.listPendingTasksHandler,
	)

	mcpServer.AddTool(
		mcp.NewTool("get_ontology_entity",
			mcp.WithDescription("Returns the full entity definition from the loaded ontology"),
			mcp.WithString("entity_type", mcp.Required(), mcp.Description("Entity ID, e.g. billingmind:DunningCycle")),
		),
		orch.getOntologyEntityHandler,
	)

	mcpServer.AddTool(
		mcp.NewTool("create_task",
			mcp.WithDescription("Creates a new agent task in DB and routes it immediately"),
			mcp.WithString("task_type", mcp.Required(), mcp.Description("Task type from ontology, e.g. subscription.create")),
			mcp.WithString("payload", mcp.Required(), mcp.Description("JSON payload for the task")),
			mcp.WithNumber("priority", mcp.Description("Task priority (default 1)")),
		),
		orch.createTaskHandler,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go orch.consumeLoop(ctx)

	httpServer := server.NewStreamableHTTPServer(mcpServer)
	addr := fmt.Sprintf(":%d", 9090)
	log.Printf("orchestrator MCP server starting on %s", addr)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		log.Println("shutting down orchestrator")
		cancel()
	}()

	if err := httpServer.Start(addr); err != nil {
		log.Fatal("orchestrator server error: ", err)
	}
}

func (o *Orchestrator) consumeLoop(ctx context.Context) {
	err := o.redisClient.Consume(ctx, func(ctx context.Context, task db.AgentTask) error {
		return o.dispatchTask(ctx, task)
	})
	if err != nil {
		log.Printf("consumer loop exited: %v", err)
	}
}

func (o *Orchestrator) dispatchTask(ctx context.Context, task db.AgentTask) error {
	agentURL, ok := o.agentURLs[task.TargetAgent]
	if !ok {
		log.Printf("no URL configured for agent: %s", task.TargetAgent)
		return fmt.Errorf("unknown agent: %s", task.TargetAgent)
	}

	body, err := json.Marshal(map[string]interface{}{
		"task_id":      task.ID.String(),
		"task_type":    task.TaskType,
		"priority":     task.Priority,
		"payload":      task.Payload,
		"target_agent": task.TargetAgent,
		"status":       task.Status,
	})
	if err != nil {
		return fmt.Errorf("marshal task: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, agentURL+"/task", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.httpClient.Do(req)
	if err != nil {
		log.Printf("failed to dispatch task %s to %s: %v", task.ID, task.TargetAgent, err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("agent %s returned status %d", task.TargetAgent, resp.StatusCode)
	}

	_ = o.queries.UpdateTaskStatus(ctx, task.ID, "in_progress")
	log.Printf("dispatched task %s to %s", task.ID, task.TargetAgent)
	return nil
}

func (o *Orchestrator) routeTaskHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	taskID, _ := args["task_id"].(string)
	if taskID == "" {
		return mcp.NewToolResultError("task_id is required"), nil
	}

	tasks, err := o.queries.ListPendingTasks(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to list tasks: %v", err)), nil
	}

	for _, task := range tasks {
		if task.ID.String() == taskID {
			if err := o.dispatchTask(ctx, task); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("dispatch failed: %v", err)), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("task %s routed to %s", taskID, task.TargetAgent)), nil
		}
	}

	return mcp.NewToolResultError(fmt.Sprintf("task %s not found in pending tasks", taskID)), nil
}

func (o *Orchestrator) getAgentStatusHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	agentName, _ := args["agent_name"].(string)
	if agentName == "" {
		return mcp.NewToolResultError("agent_name is required"), nil
	}

	agentURL, ok := o.agentURLs[agentName]
	if !ok {
		return mcp.NewToolResultError(fmt.Sprintf("unknown agent: %s", agentName)), nil
	}

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, agentURL+"/health", nil)
	resp, err := o.httpClient.Do(req)
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf(`{"agent": "%s", "status": "unreachable", "error": "%v"}`, agentName, err)), nil
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	resultJSON, _ := json.Marshal(result)
	return mcp.NewToolResultText(string(resultJSON)), nil
}

func (o *Orchestrator) listPendingTasksHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	tasks, err := o.queries.ListPendingTasks(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to list tasks: %v", err)), nil
	}

	type taskSummary struct {
		ID          string `json:"id"`
		TaskType    string `json:"task_type"`
		TargetAgent string `json:"target_agent"`
		Priority    int    `json:"priority"`
		Status      string `json:"status"`
	}

	summaries := make([]taskSummary, len(tasks))
	for i, t := range tasks {
		summaries[i] = taskSummary{
			ID:          t.ID.String(),
			TaskType:    t.TaskType,
			TargetAgent: t.TargetAgent,
			Priority:    t.Priority,
			Status:      t.Status,
		}
	}

	data, _ := json.Marshal(summaries)
	return mcp.NewToolResultText(string(data)), nil
}

func (o *Orchestrator) getOntologyEntityHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	entityType, _ := args["entity_type"].(string)
	if entityType == "" {
		return mcp.NewToolResultError("entity_type is required"), nil
	}

	entity := o.ont.GetEntity(entityType)
	if entity == nil {
		return mcp.NewToolResultError(fmt.Sprintf("entity %s not found in ontology", entityType)), nil
	}

	data, _ := json.Marshal(entity)
	return mcp.NewToolResultText(string(data)), nil
}

func (o *Orchestrator) createTaskHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	taskType, _ := args["task_type"].(string)
	payloadStr, _ := args["payload"].(string)
	priority := 1
	if p, ok := args["priority"].(float64); ok {
		priority = int(p)
	}

	if taskType == "" || payloadStr == "" {
		return mcp.NewToolResultError("task_type and payload are required"), nil
	}

	resolution, err := o.resolver.ResolveWebhook(taskType)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("cannot resolve task type: %v", err)), nil
	}

	task, err := o.queries.CreateAgentTask(ctx, taskType, string(resolution.TargetAgent), priority, json.RawMessage(payloadStr))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to create task: %v", err)), nil
	}

	if dispatchErr := o.dispatchTask(ctx, task); dispatchErr != nil {
		return mcp.NewToolResultText(fmt.Sprintf("task %s created but dispatch failed: %v", task.ID, dispatchErr)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("task %s created and routed to %s", task.ID, task.TargetAgent)), nil
}
