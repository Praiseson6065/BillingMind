package main

import (
	"Billingmind/config"
	"Billingmind/internal/db"
	"Billingmind/internal/ontology"
	stripeClient "Billingmind/internal/stripe"
	"Billingmind/internal/webhooks"
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func main() {
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

	sc := stripeClient.NewStripeClient(cfg.Stripe.SecretKey)
	stripeWebhook := webhooks.NewStripeWebhookHandler(sc, cfg.Stripe.WebhookSecret, queries, resolver)

	app := gin.New()
	app.Use(gin.Logger())
	app.Use(gin.Recovery())

	Router(app, stripeWebhook)

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      app,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	log.Println("gateway running on port", cfg.Server.Port)

	if err := server.ListenAndServe(); err != nil {
		log.Fatal("server error: ", err)
	}
}
