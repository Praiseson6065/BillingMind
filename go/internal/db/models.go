package db

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Customer struct {
	ID               uuid.UUID `json:"id" db:"id"`
	StripeCustomerID string    `json:"stripe_customer_id" db:"stripe_customer_id"`
	Email            string    `json:"email" db:"email"`
	CreatedAt        time.Time `json:"created_at" db:"created_at"`
}

type Subscription struct {
	ID               uuid.UUID  `json:"id" db:"id"`
	CustomerID       uuid.UUID  `json:"customer_id" db:"customer_id"`
	StripeSubID      string     `json:"stripe_sub_id" db:"stripe_sub_id"`
	Status           string     `json:"status" db:"status"`
	PlanID           string     `json:"plan_id" db:"plan_id"`
	CurrentPeriodEnd *time.Time `json:"current_period_end" db:"current_period_end"`
	CreatedAt        time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at" db:"updated_at"`
}

type Invoice struct {
	ID              uuid.UUID  `json:"id" db:"id"`
	StripeInvoiceID string     `json:"stripe_invoice_id" db:"stripe_invoice_id"`
	SubscriptionID  uuid.UUID  `json:"subscription_id" db:"subscription_id"`
	Amount          int64      `json:"amount" db:"amount"`
	Status          string     `json:"status" db:"status"`
	DueDate         *time.Time `json:"due_date" db:"due_date"`
	CreatedAt       *time.Time `json:"created_at" db:"created_at"`
}

type AgentTask struct {
	ID          uuid.UUID       `json:"id"`
	TaskType    string          `json:"task_type"`
	TargetAgent string          `json:"target_agent"`
	Priority    int             `json:"priority"`
	Payload     json.RawMessage `json:"payload"`
	Status      string          `json:"status"`
	CreatedAt   *time.Time      `json:"created_at"`
	CompletedAt *time.Time      `json:"completed_at"`
}

type DunningCycle struct {
	ID             uuid.UUID  `json:"id"`
	SubscriptionID uuid.UUID  `json:"subscription_id"`
	InvoiceID      uuid.UUID  `json:"invoice_id"`
	AttemptNumber  int        `json:"attempt_number"`
	NextRetryAt    *time.Time `json:"next_retry_at"`
	Strategy       string     `json:"strategy"`
	CreatedAt      time.Time  `json:"created_at"`
}
