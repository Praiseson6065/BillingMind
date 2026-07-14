import logging
from typing import TypedDict, Annotated

from langgraph.graph import StateGraph, START, END
from langchain_core.messages import BaseMessage

from agents.shared.models import AgentTask, A2AMessage, TaskResult, TaskStatus, DunningStrategy
from agents.shared.a2a_client import A2AClient
from .strategy import generate_strategy

logger = logging.getLogger(__name__)

ESCALATION_THRESHOLD = 3


class RecoveryState(TypedDict):
    task: AgentTask
    messages: Annotated[list[BaseMessage], lambda a, b: a + b]
    attempt_number: int
    strategy: DunningStrategy | None
    a2a_client: A2AClient | None
    result: dict | None
    error: str | None


def check_dunning_history(state: RecoveryState) -> dict:
    payload = state["task"].payload
    attempt = payload.get("attempt_count", 1)
    return {"attempt_number": attempt}


def compute_strategy(state: RecoveryState) -> dict:
    payload = state["task"].payload
    customer_history = {
        "customer_id": payload.get("customer"),
        "invoice_id": payload.get("id"),
        "amount_due": payload.get("amount_due"),
        "currency": payload.get("currency"),
        "attempt_count": state["attempt_number"],
    }

    strategy = generate_strategy(state["attempt_number"], customer_history)
    logger.info(
        "strategy for attempt %d: tone=%s delay=%dh discount=%s suspend=%s",
        state["attempt_number"],
        strategy.email_tone,
        strategy.retry_delay_hours,
        strategy.offer_discount,
        strategy.suspend_access,
    )
    return {"strategy": strategy}


def should_escalate(state: RecoveryState) -> str:
    if state["attempt_number"] >= ESCALATION_THRESHOLD:
        return "escalate"
    return "notify_support"


async def notify_support(state: RecoveryState) -> dict:
    client = state.get("a2a_client")
    if not client:
        logger.warning("no a2a client available, skipping support notification")
        return {"result": {"action": "notify_skipped", "strategy": state["strategy"].model_dump()}}

    message = A2AMessage(
        from_agent="recovery",
        to_agent="support",
        message_type="notify.customer",
        payload={
            "customer_id": state["task"].payload.get("customer"),
            "invoice_id": state["task"].payload.get("id"),
            "strategy": state["strategy"].model_dump(),
        },
    )

    try:
        await client.send(message)
        logger.info("sent notify.customer to support agent")
    except Exception as e:
        logger.error("failed to send a2a to support: %s", e)

    return {"result": {"action": "notified_support", "strategy": state["strategy"].model_dump()}}


async def escalate_to_billing(state: RecoveryState) -> dict:
    client = state.get("a2a_client")
    if not client:
        logger.warning("no a2a client available, skipping escalation")
        return {"result": {"action": "escalation_skipped", "strategy": state["strategy"].model_dump()}}

    message = A2AMessage(
        from_agent="recovery",
        to_agent="billing",
        message_type="escalate.refund",
        payload={
            "customer_id": state["task"].payload.get("customer"),
            "subscription_id": state["task"].payload.get("subscription"),
            "invoice_id": state["task"].payload.get("id"),
            "reason": "payment_failed_max_attempts",
        },
    )

    try:
        await client.send(message)
        logger.info("escalated refund to billing agent")
    except Exception as e:
        logger.error("failed to send escalation to billing: %s", e)

    return {"result": {"action": "escalated_to_billing", "strategy": state["strategy"].model_dump()}}


def build_graph() -> StateGraph:
    graph = StateGraph(RecoveryState)
    graph.add_node("check_dunning_history", check_dunning_history)
    graph.add_node("compute_strategy", compute_strategy)
    graph.add_node("notify_support", notify_support)
    graph.add_node("escalate", escalate_to_billing)

    graph.add_edge(START, "check_dunning_history")
    graph.add_edge("check_dunning_history", "compute_strategy")
    graph.add_conditional_edges("compute_strategy", should_escalate)
    graph.add_edge("notify_support", END)
    graph.add_edge("escalate", END)

    return graph.compile()


recovery_graph = build_graph()


async def handle_task(task: AgentTask, a2a_client: A2AClient | None = None) -> TaskResult:
    logger.info("processing recovery task %s", task.task_id)

    initial_state = {
        "task": task,
        "messages": [],
        "attempt_number": 1,
        "strategy": None,
        "a2a_client": a2a_client,
        "result": None,
        "error": None,
    }

    final_state = await recovery_graph.ainvoke(initial_state)

    if final_state.get("error"):
        return TaskResult(task_id=task.task_id, status=TaskStatus.FAILED, error=final_state["error"])

    return TaskResult(task_id=task.task_id, status=TaskStatus.COMPLETED, result=final_state.get("result"))
