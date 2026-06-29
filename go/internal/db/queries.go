package db

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Queries struct {
	pool *pgxpool.Pool
}

func NewQueries(pool *pgxpool.Pool) *Queries {
	return &Queries{pool: pool}
}

func (q *Queries) UpsertCustomer(ctx context.Context, customer Customer) (Customer, error) {
	var c Customer
	err := q.pool.QueryRow(ctx, `
		INSERT INTO customers (stripe_customer_id, email)
		VALUES ($1, $2)
		ON CONFLICT (stripe_customer_id) DO UPDATE
		SET email = EXCLUDED.email
		RETURNING id, stripe_customer_id, email, created_at
	`, customer.StripeCustomerID, customer.Email).Scan(
		&c.ID, &c.StripeCustomerID, &c.Email, &c.CreatedAt,
	)
	return c, err
}

func (q *Queries) GetCustomerByStripeID(ctx context.Context, stripeID string) (Customer, error) {
	var c Customer
	err := q.pool.QueryRow(ctx, `
		SELECT id, stripe_customer_id, email, created_at
		FROM customers WHERE stripe_customer_id = $1
	`, stripeID).Scan(
		&c.ID, &c.StripeCustomerID, &c.Email, &c.CreatedAt,
	)
	return c, err
}

func (q *Queries) UpsertSubscription(ctx context.Context, sub Subscription) (Subscription, error) {
	var s Subscription
	err := q.pool.QueryRow(ctx, `
		INSERT INTO subscriptions (stripe_sub_id, customer_id, status, plan_id, current_period_end)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (stripe_sub_id) DO UPDATE
		SET status = EXCLUDED.status, plan_id = EXCLUDED.plan_id,
		    current_period_end = EXCLUDED.current_period_end, updated_at = now()
		RETURNING id, stripe_sub_id, customer_id, status, plan_id, current_period_end, created_at, updated_at
	`, sub.StripeSubID, sub.CustomerID, sub.Status, sub.PlanID, sub.CurrentPeriodEnd).Scan(
		&s.ID, &s.StripeSubID, &s.CustomerID, &s.Status, &s.PlanID,
		&s.CurrentPeriodEnd, &s.CreatedAt, &s.UpdatedAt,
	)
	return s, err
}

func (q *Queries) GetSubscriptionByStripeID(ctx context.Context, stripeSubID string) (Subscription, error) {
	var s Subscription
	err := q.pool.QueryRow(ctx, `
		SELECT id, stripe_sub_id, customer_id, status, plan_id, current_period_end, created_at, updated_at
		FROM subscriptions WHERE stripe_sub_id = $1
	`, stripeSubID).Scan(
		&s.ID, &s.StripeSubID, &s.CustomerID, &s.Status, &s.PlanID,
		&s.CurrentPeriodEnd, &s.CreatedAt, &s.UpdatedAt,
	)
	return s, err
}

func (q *Queries) UpsertInvoice(ctx context.Context, inv Invoice) (Invoice, error) {
	var i Invoice
	err := q.pool.QueryRow(ctx, `
		INSERT INTO invoices (stripe_invoice_id, subscription_id, amount, status, due_date)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (stripe_invoice_id) DO UPDATE
		SET status = EXCLUDED.status, amount = EXCLUDED.amount, due_date = EXCLUDED.due_date
		RETURNING id, stripe_invoice_id, subscription_id, amount, status, due_date, created_at
	`, inv.StripeInvoiceID, inv.SubscriptionID, inv.Amount, inv.Status, inv.DueDate).Scan(
		&i.ID, &i.StripeInvoiceID, &i.SubscriptionID, &i.Amount, &i.Status, &i.DueDate, &i.CreatedAt,
	)
	return i, err
}

func (q *Queries) GetInvoiceByStripeID(ctx context.Context, stripeInvoiceID string) (Invoice, error) {
	var i Invoice
	err := q.pool.QueryRow(ctx, `
		SELECT id, stripe_invoice_id, subscription_id, amount, status, due_date, created_at
		FROM invoices WHERE stripe_invoice_id = $1
	`, stripeInvoiceID).Scan(
		&i.ID, &i.StripeInvoiceID, &i.SubscriptionID, &i.Amount, &i.Status, &i.DueDate, &i.CreatedAt,
	)
	return i, err
}

func (q *Queries) CreateAgentTask(ctx context.Context, taskType, targetAgent string, priority int, payload json.RawMessage) (AgentTask, error) {
	var t AgentTask
	err := q.pool.QueryRow(ctx, `
		INSERT INTO agent_tasks (task_type, target_agent, priority, payload)
		VALUES ($1, $2, $3, $4)
		RETURNING id, task_type, target_agent, priority, payload, status, created_at, completed_at
	`, taskType, targetAgent, priority, payload).Scan(
		&t.ID, &t.TaskType, &t.TargetAgent, &t.Priority, &t.Payload, &t.Status, &t.CreatedAt, &t.CompletedAt,
	)
	return t, err
}

func (q *Queries) UpdateTaskStatus(ctx context.Context, taskID uuid.UUID, status string) error {
	_, err := q.pool.Exec(ctx, `
		UPDATE agent_tasks SET status = $2, completed_at = CASE WHEN $2 = 'completed' THEN now() ELSE completed_at END
		WHERE id = $1
	`, taskID, status)
	return err
}

func (q *Queries) ListPendingTasks(ctx context.Context) ([]AgentTask, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT id, task_type, target_agent, priority, payload, status, created_at, completed_at
		FROM agent_tasks WHERE status = 'pending'
		ORDER BY priority DESC, created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (AgentTask, error) {
		var t AgentTask
		err := row.Scan(&t.ID, &t.TaskType, &t.TargetAgent, &t.Priority, &t.Payload, &t.Status, &t.CreatedAt, &t.CompletedAt)
		return t, err
	})
}

func (q *Queries) CreateDunningCycle(ctx context.Context, dc DunningCycle) (DunningCycle, error) {
	var d DunningCycle
	err := q.pool.QueryRow(ctx, `
		INSERT INTO dunning_cycles (subscription_id, invoice_id, attempt_number, next_retry_at, strategy)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, subscription_id, invoice_id, attempt_number, next_retry_at, strategy, created_at
	`, dc.SubscriptionID, dc.InvoiceID, dc.AttemptNumber, dc.NextRetryAt, dc.Strategy).Scan(
		&d.ID, &d.SubscriptionID, &d.InvoiceID, &d.AttemptNumber, &d.NextRetryAt, &d.Strategy, &d.CreatedAt,
	)
	return d, err
}
