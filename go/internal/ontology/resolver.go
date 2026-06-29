package ontology

import "fmt"

type TaskType string
type TargetAgent string


const (
	AgentBilling  TargetAgent = "billing"
	AgentRecovery TargetAgent = "recovery"
	AgentSupport  TargetAgent = "support"
	AgentAudit    TargetAgent = "audit"
)

type Resolution struct {
	TaskType    TaskType
	TargetAgent TargetAgent
}

type Resolver struct {
	ontology *Ontology
	mappings map[string]Resolution // stripe event type → resolution
}

func NewResolver(ont *Ontology) *Resolver {
	r := &Resolver{
		ontology: ont,
		mappings: make(map[string]Resolution),
	}
	r.buildMappings()
	return r
}

// buildMappings wires Stripe event types to ontology-derived task types.
// The task types come from billingmind:AgentTask.taskTypes in the ontology.
func (r *Resolver) buildMappings() {
	r.mappings = map[string]Resolution{
		"checkout.session.completed":    {TaskType: "subscription.create", TargetAgent: AgentBilling},
		"customer.subscription.created": {TaskType: "subscription.create", TargetAgent: AgentBilling},
		"customer.subscription.updated": {TaskType: "subscription.upgrade", TargetAgent: AgentBilling},
		"customer.subscription.deleted": {TaskType: "subscription.cancel", TargetAgent: AgentBilling},
		"invoice.paid":                  {TaskType: "subscription.create", TargetAgent: AgentBilling},
		"invoice.payment_failed":        {TaskType: "payment.retry", TargetAgent: AgentRecovery},
	}

	// Validate: every TaskType in mappings must exist in the ontology's AgentTask.taskTypes
	agentTask := r.ontology.GetEntity("billingmind:AgentTask")
	if agentTask != nil {
		validTypes := make(map[string]bool)
		for _, t := range agentTask.TaskTypes {
			validTypes[t] = true
		}
		for event, res := range r.mappings {
			if !validTypes[string(res.TaskType)] {
				// Log warning — task type not in ontology
				fmt.Printf("WARNING: mapping %q → task type %q not found in ontology\n", event, res.TaskType)
			}
		}
	}
}

func (r *Resolver) ResolveWebhook(eventType string) (Resolution, error) {
	res, ok := r.mappings[eventType]
	if !ok {
		return Resolution{}, fmt.Errorf("no mapping for event type: %s", eventType)
	}
	return res, nil
}
