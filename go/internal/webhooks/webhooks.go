package webhooks

import "github.com/gin-gonic/gin"

type WebhookHandler interface {
	HandleWebhook() gin.HandlerFunc
}
