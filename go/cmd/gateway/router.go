package main

import (
	"Billingmind/internal/webhooks"
	"log"
	"net/http"
	"path/filepath"

	"github.com/gin-gonic/gin"
)

func Router(r *gin.Engine, stripeWebhook webhooks.WebhookHandler) {

	router := r.Group("/")
	{
		router.GET("/health", func(ctx *gin.Context) {
			ctx.JSON(http.StatusOK, gin.H{
				"status": "healthy",
			})
		})

		router.POST("/webhook/stripe", stripeWebhook.HandleWebhook())

		router.GET("/ontology", func(ctx *gin.Context) {
			log.Println("Ontology requested")
			ontologyPath := filepath.Join("..", "ontology", "billing.jsonld")
			ctx.Header("Content-Type", "application/ld+json")
			ctx.File(ontologyPath)
		})
	}

}
