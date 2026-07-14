import os
import stripe
from langchain_core.tools import tool

stripe.api_key = os.getenv("STRIPE_SECRET_KEY", "")


@tool
def get_customer(customer_id: str) -> dict:
    """Retrieve a Stripe customer by ID."""
    customer = stripe.Customer.retrieve(customer_id)
    return {
        "id": customer.id,
        "email": customer.email,
        "name": customer.name,
        "created": customer.created,
    }


@tool
def create_subscription(customer_id: str, price_id: str) -> dict:
    """Create a new subscription for a customer with the given price."""
    sub = stripe.Subscription.create(
        customer=customer_id,
        items=[{"price": price_id}],
    )
    return {
        "id": sub.id,
        "status": sub.status,
        "customer": sub.customer,
    }


@tool
def cancel_subscription(subscription_id: str) -> dict:
    """Cancel a subscription immediately."""
    sub = stripe.Subscription.cancel(subscription_id)
    return {
        "id": sub.id,
        "status": sub.status,
    }


@tool
def upgrade_subscription(subscription_id: str, new_price_id: str) -> dict:
    """Upgrade a subscription to a new price by replacing the first item."""
    sub = stripe.Subscription.retrieve(subscription_id)
    if not sub.get("items") or not sub["items"]["data"]:
        return {"error": "no items found on subscription"}

    item_id = sub["items"]["data"][0]["id"]
    updated = stripe.Subscription.modify(
        subscription_id,
        items=[{"id": item_id, "price": new_price_id}],
        proration_behavior="create_prorations",
    )
    return {
        "id": updated.id,
        "status": updated.status,
    }


BILLING_TOOLS = [get_customer, create_subscription, cancel_subscription, upgrade_subscription]
