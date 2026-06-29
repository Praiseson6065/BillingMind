package stripe

import "github.com/stripe/stripe-go/v86"

func NewStripeClient(secretKey string) *stripe.Client {
	return stripe.NewClient(secretKey)
}
