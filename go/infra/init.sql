-- BillingMind — Database Schema
-- Aligned with ontology/billing.jsonld

BEGIN;

-- ============================================================
-- Custom ENUM types
-- ============================================================

CREATE TYPE subscription_status AS ENUM (
    'active',
    'past_due',
    'canceled',
    'incomplete',
    'trialing'
);

CREATE TYPE invoice_status AS ENUM (
    'draft',
    'open',
    'paid',
    'void',
    'uncollectible'
);

CREATE TYPE task_status AS ENUM (
    'pending',
    'in_progress',
    'completed',
    'failed'
);

CREATE TYPE dunning_strategy AS ENUM (
    'gentle',
    'urgent',
    'final_notice'
);

-- ============================================================
-- customers
-- Ontology: billingmind:Customer
-- ============================================================

CREATE TABLE customers (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    stripe_customer_id  TEXT NOT NULL UNIQUE,
    email               TEXT NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_customers_stripe_id ON customers (stripe_customer_id);
CREATE INDEX idx_customers_email     ON customers (email);

-- ============================================================
-- subscriptions
-- Ontology: billingmind:Subscription
-- ============================================================

CREATE TABLE subscriptions (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    stripe_sub_id       TEXT NOT NULL UNIQUE,
    customer_id         UUID NOT NULL REFERENCES customers (id) ON DELETE CASCADE,
    status              subscription_status NOT NULL DEFAULT 'incomplete',
    plan_id             TEXT NOT NULL,
    current_period_end  TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_subscriptions_stripe_id   ON subscriptions (stripe_sub_id);
CREATE INDEX idx_subscriptions_customer_id ON subscriptions (customer_id);
CREATE INDEX idx_subscriptions_status      ON subscriptions (status);

-- ============================================================
-- invoices
-- Ontology: billingmind:Invoice
-- ============================================================

CREATE TABLE invoices (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    stripe_invoice_id   TEXT NOT NULL UNIQUE,
    subscription_id     UUID NOT NULL REFERENCES subscriptions (id) ON DELETE CASCADE,
    amount              BIGINT NOT NULL,          -- in smallest currency unit (cents)
    status              invoice_status NOT NULL DEFAULT 'draft',
    due_date            TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_invoices_stripe_id       ON invoices (stripe_invoice_id);
CREATE INDEX idx_invoices_subscription_id ON invoices (subscription_id);
CREATE INDEX idx_invoices_status          ON invoices (status);

-- ============================================================
-- agent_tasks
-- Ontology: billingmind:AgentTask
-- ============================================================

CREATE TABLE agent_tasks (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_type       TEXT NOT NULL,
    target_agent    TEXT NOT NULL,
    priority        INT NOT NULL DEFAULT 0,
    payload         JSONB NOT NULL DEFAULT '{}',
    status          task_status NOT NULL DEFAULT 'pending',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at    TIMESTAMPTZ
);

CREATE INDEX idx_agent_tasks_status       ON agent_tasks (status);
CREATE INDEX idx_agent_tasks_target_agent ON agent_tasks (target_agent);
CREATE INDEX idx_agent_tasks_task_type    ON agent_tasks (task_type);

-- ============================================================
-- dunning_cycles
-- Ontology: billingmind:DunningCycle
-- ============================================================

CREATE TABLE dunning_cycles (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    subscription_id UUID NOT NULL REFERENCES subscriptions (id) ON DELETE CASCADE,
    invoice_id      UUID NOT NULL REFERENCES invoices (id) ON DELETE CASCADE,
    attempt_number  INT NOT NULL DEFAULT 1,
    next_retry_at   TIMESTAMPTZ,
    strategy        dunning_strategy NOT NULL DEFAULT 'gentle',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_dunning_subscription_id ON dunning_cycles (subscription_id);
CREATE INDEX idx_dunning_invoice_id      ON dunning_cycles (invoice_id);
CREATE INDEX idx_dunning_next_retry      ON dunning_cycles (next_retry_at)
    WHERE next_retry_at IS NOT NULL;

COMMIT;
