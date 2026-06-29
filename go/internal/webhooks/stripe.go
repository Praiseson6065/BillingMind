package webhooks

import (
	"Billingmind/internal/db"
	"Billingmind/internal/ontology"
	"Billingmind/internal/queue"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/stripe/stripe-go/v86"
	"github.com/stripe/stripe-go/v86/webhook"
)

type StripeWebhookHandler struct {
	client        *stripe.Client
	webhookSecret string
	queries       *db.Queries
	resolver      *ontology.Resolver
	producer      *queue.TaskProducer
}

func NewStripeWebhookHandler(client *stripe.Client, webhookSecret string, queries *db.Queries, resolver *ontology.Resolver, producer *queue.TaskProducer) *StripeWebhookHandler {
	return &StripeWebhookHandler{
		client:        client,
		webhookSecret: webhookSecret,
		queries:       queries,
		resolver:      resolver,
		producer:      producer,
	}
}

func (s *StripeWebhookHandler) HandleWebhook() gin.HandlerFunc {
	return func(c *gin.Context) {
		const maxBodyBytes = int64(65536)
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBodyBytes)

		payload, err := io.ReadAll(c.Request.Body)
		if err != nil {
			log.Printf("error reading webhook body: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "unable to read request body"})
			return
		}

		sigHeader := c.GetHeader("Stripe-Signature")
		event, err := webhook.ConstructEventWithOptions(payload, sigHeader, s.webhookSecret, webhook.ConstructEventOptions{
			IgnoreAPIVersionMismatch: true,
		})
		if err != nil {
			log.Printf("webhook signature verification failed: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid signature"})
			return
		}

		if err := s.processEvent(c.Request.Context(), event); err != nil {
			log.Printf("error processing event %s (%s): %v", event.ID, event.Type, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "processing failed"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"received": true})
	}
}

func (s *StripeWebhookHandler) processEvent(ctx context.Context, event stripe.Event) error {
	switch event.Type {
	case "checkout.session.completed":
		return s.handleCheckoutSessionCompleted(ctx, event)
	case "invoice.paid":
		return s.handleInvoicePaid(ctx, event)
	case "invoice.payment_failed":
		return s.handleInvoicePaymentFailed(ctx, event)
	case "customer.subscription.created":
		return s.handleSubscriptionCreated(ctx, event)
	case "customer.subscription.updated":
		return s.handleSubscriptionUpdated(ctx, event)
	case "customer.subscription.deleted":
		return s.handleSubscriptionDeleted(ctx, event)
	default:
		log.Printf("unhandled event type: %s", event.Type)
		return nil
	}
}

func (s *StripeWebhookHandler) handleCheckoutSessionCompleted(ctx context.Context, event stripe.Event) error {
	var session stripe.CheckoutSession
	if err := json.Unmarshal(event.Data.Raw, &session); err != nil {
		return err
	}

	if session.Customer != nil {
		_, err := s.queries.UpsertCustomer(ctx, db.Customer{
			StripeCustomerID: session.Customer.ID,
			Email:            session.CustomerEmail,
		})
		if err != nil {
			return err
		}
	}

	return s.createTaskFromEvent(ctx, string(event.Type), event.Data.Raw)
}

func (s *StripeWebhookHandler) handleInvoicePaid(ctx context.Context, event stripe.Event) error {
	var invoice stripe.Invoice
	if err := json.Unmarshal(event.Data.Raw, &invoice); err != nil {
		return err
	}

	subID := s.extractSubscriptionID(invoice)
	if subID != "" {
		sub, err := s.queries.GetSubscriptionByStripeID(ctx, subID)
		if err == nil {
			_, err = s.queries.UpsertInvoice(ctx, db.Invoice{
				StripeInvoiceID: invoice.ID,
				SubscriptionID:  sub.ID,
				Amount:          invoice.AmountPaid,
				Status:          string(invoice.Status),
			})
			if err != nil {
				return err
			}
		}
	}

	return s.createTaskFromEvent(ctx, string(event.Type), event.Data.Raw)
}

func (s *StripeWebhookHandler) handleInvoicePaymentFailed(ctx context.Context, event stripe.Event) error {
	var invoice stripe.Invoice
	if err := json.Unmarshal(event.Data.Raw, &invoice); err != nil {
		return err
	}

	subID := s.extractSubscriptionID(invoice)
	if subID != "" {
		sub, err := s.queries.GetSubscriptionByStripeID(ctx, subID)
		if err == nil {
			_, err = s.queries.UpsertInvoice(ctx, db.Invoice{
				StripeInvoiceID: invoice.ID,
				SubscriptionID:  sub.ID,
				Amount:          invoice.AmountDue,
				Status:          string(invoice.Status),
			})
			if err != nil {
				return err
			}
		}
	}

	return s.createTaskFromEvent(ctx, string(event.Type), event.Data.Raw)
}

func (s *StripeWebhookHandler) handleSubscriptionCreated(ctx context.Context, event stripe.Event) error {
	return s.upsertSubscriptionFromEvent(ctx, event)
}

func (s *StripeWebhookHandler) handleSubscriptionUpdated(ctx context.Context, event stripe.Event) error {
	return s.upsertSubscriptionFromEvent(ctx, event)
}

func (s *StripeWebhookHandler) handleSubscriptionDeleted(ctx context.Context, event stripe.Event) error {
	return s.upsertSubscriptionFromEvent(ctx, event)
}

func (s *StripeWebhookHandler) upsertSubscriptionFromEvent(ctx context.Context, event stripe.Event) error {
	var sub stripe.Subscription
	if err := json.Unmarshal(event.Data.Raw, &sub); err != nil {
		return err
	}

	if sub.Customer == nil {
		log.Printf("subscription event %s has no customer, skipping DB write", event.ID)
		return s.createTaskFromEvent(ctx, string(event.Type), event.Data.Raw)
	}

	customer, err := s.queries.UpsertCustomer(ctx, db.Customer{
		StripeCustomerID: sub.Customer.ID,
		Email:            sub.Customer.Email,
	})
	if err != nil {
		return err
	}

	planID := ""
	if sub.Items != nil && len(sub.Items.Data) > 0 && sub.Items.Data[0].Price != nil {
		planID = sub.Items.Data[0].Price.ID
	}

	_, err = s.queries.UpsertSubscription(ctx, db.Subscription{
		StripeSubID: sub.ID,
		CustomerID:  customer.ID,
		Status:      string(sub.Status),
		PlanID:      planID,
	})
	if err != nil {
		return err
	}

	return s.createTaskFromEvent(ctx, string(event.Type), event.Data.Raw)
}

func (s *StripeWebhookHandler) createTaskFromEvent(ctx context.Context, eventType string, rawPayload json.RawMessage) error {
	resolution, err := s.resolver.ResolveWebhook(eventType)
	if err != nil {
		log.Printf("no task mapping for event type %s, skipping task creation", eventType)
		return nil
	}

	task, err := s.queries.CreateAgentTask(
		ctx,
		string(resolution.TaskType),
		string(resolution.TargetAgent),
		1,
		rawPayload,
	)
	if err != nil {
		return err
	}

	if s.producer != nil {
		if pubErr := s.producer.Publish(ctx, task); pubErr != nil {
			log.Printf("failed to publish task %s to redis stream: %v", task.ID, pubErr)
		}
	}

	log.Printf("created task %s: type=%s agent=%s", task.ID, task.TaskType, task.TargetAgent)
	return nil
}

func (s *StripeWebhookHandler) extractSubscriptionID(invoice stripe.Invoice) string {
	if invoice.Parent == nil {
		return ""
	}
	if invoice.Parent.SubscriptionDetails == nil {
		return ""
	}
	if invoice.Parent.SubscriptionDetails.Subscription == nil {
		return ""
	}
	return invoice.Parent.SubscriptionDetails.Subscription.ID
}
