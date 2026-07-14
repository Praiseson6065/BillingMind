import logging
from typing import TypedDict, Annotated

from langgraph.graph import StateGraph, START, END
from langchain_core.messages import HumanMessage, BaseMessage

from agents.shared.llm_router import router
from agents.shared.models import AgentTask, A2AMessage, TaskResult, TaskStatus

logger = logging.getLogger(__name__)


class SupportState(TypedDict):
    task: AgentTask | None
    a2a_message: A2AMessage | None
    messages: Annotated[list[BaseMessage], lambda a, b: a + b]
    email_draft: str | None
    error: str | None


def parse_input(state: SupportState) -> dict:
    a2a = state.get("a2a_message")
    if a2a and a2a.message_type == "notify.customer":
        strategy = a2a.payload.get("strategy", {})
        customer_id = a2a.payload.get("customer_id", "unknown")
        invoice_id = a2a.payload.get("invoice_id", "unknown")

        prompt = (
            f"You are a customer support specialist for a SaaS billing platform.\n\n"
            f"A payment has failed for customer {customer_id} on invoice {invoice_id}.\n"
            f"The recovery team has decided on this strategy:\n"
            f"- Tone: {strategy.get('email_tone', 'gentle')}\n"
            f"- Retry delay: {strategy.get('retry_delay_hours', 24)} hours\n"
            f"- Offer discount: {strategy.get('offer_discount', False)}\n"
            f"- Suspend access: {strategy.get('suspend_access', False)}\n\n"
            f"Draft a short, professional email to the customer about the failed payment. "
            f"Match the tone specified. If offering a discount, mention a 10% discount on their next invoice. "
            f"If suspending access, warn them politely.\n\n"
            f"Return ONLY the email body text, no subject line."
        )
        return {"messages": [HumanMessage(content=prompt)]}

    return {"error": "unsupported input type"}


def draft_email(state: SupportState) -> dict:
    if state.get("error"):
        return {"email_draft": None}

    llm = router.route("high")
    response = llm.invoke(state["messages"])
    return {"email_draft": response.content}


def finalize(state: SupportState) -> dict:
    if state.get("email_draft"):
        logger.info("email draft generated (%d chars)", len(state["email_draft"]))
    return {}


def build_graph() -> StateGraph:
    graph = StateGraph(SupportState)
    graph.add_node("parse_input", parse_input)
    graph.add_node("draft_email", draft_email)
    graph.add_node("finalize", finalize)

    graph.add_edge(START, "parse_input")
    graph.add_edge("parse_input", "draft_email")
    graph.add_edge("draft_email", "finalize")
    graph.add_edge("finalize", END)

    return graph.compile()


support_graph = build_graph()


async def handle_a2a(message: A2AMessage) -> dict:
    logger.info("processing a2a message: %s from %s", message.message_type, message.from_agent)

    initial_state = {
        "task": None,
        "a2a_message": message,
        "messages": [],
        "email_draft": None,
        "error": None,
    }

    final_state = support_graph.invoke(initial_state)

    return {
        "email_draft": final_state.get("email_draft"),
        "error": final_state.get("error"),
    }


async def handle_task(task: AgentTask) -> TaskResult:
    logger.info("processing support task %s: %s", task.task_id, task.task_type)
    return TaskResult(
        task_id=task.task_id,
        status=TaskStatus.COMPLETED,
        result={"message": "support task acknowledged"},
    )
