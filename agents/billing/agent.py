import logging
from typing import TypedDict, Annotated

from langgraph.graph import StateGraph, START, END
from langgraph.prebuilt import ToolNode
from langchain_core.messages import HumanMessage, BaseMessage

from agents.shared.llm_router import router
from agents.shared.models import AgentTask, TaskResult, TaskStatus
from .tools import BILLING_TOOLS

logger = logging.getLogger(__name__)

TASK_TOOL_MAP = {
    "subscription.create": "create_subscription",
    "subscription.cancel": "cancel_subscription",
    "subscription.upgrade": "upgrade_subscription",
}


class BillingState(TypedDict):
    task: AgentTask
    messages: Annotated[list[BaseMessage], lambda a, b: a + b]
    result: dict | None
    error: str | None


def parse_task(state: BillingState) -> dict:
    task = state["task"]
    payload = task.payload
    task_type = task.task_type

    tool_name = TASK_TOOL_MAP.get(task_type)
    if not tool_name:
        return {
            "error": f"unknown task type: {task_type}",
            "messages": [],
        }

    prompt = (
        f"You are a billing operations agent. Execute the following task.\n"
        f"Task type: {task_type}\n"
        f"Tool to use: {tool_name}\n"
        f"Payload data: {payload}\n\n"
        f"Extract the required parameters from the payload and call the correct tool. "
        f"For subscription.create, extract customer (Stripe customer ID) and items[0].price.id. "
        f"For subscription.cancel, extract subscription ID. "
        f"For subscription.upgrade, extract subscription ID and the new price ID."
    )
    return {"messages": [HumanMessage(content=prompt)]}


def invoke_llm(state: BillingState) -> dict:
    if state.get("error"):
        return {"messages": []}

    llm = router.route("high")
    llm_with_tools = llm.bind_tools(BILLING_TOOLS)
    response = llm_with_tools.invoke(state["messages"])
    return {"messages": [response]}


def should_use_tool(state: BillingState) -> str:
    if state.get("error"):
        return "finalize"
    last = state["messages"][-1]
    if hasattr(last, "tool_calls") and last.tool_calls:
        return "execute_tool"
    return "finalize"


def finalize(state: BillingState) -> dict:
    if state.get("error"):
        return {"result": None}

    last = state["messages"][-1]
    return {"result": {"response": last.content if hasattr(last, "content") else str(last)}}


def build_graph() -> StateGraph:
    tool_node = ToolNode(BILLING_TOOLS)

    graph = StateGraph(BillingState)
    graph.add_node("parse_task", parse_task)
    graph.add_node("invoke_llm", invoke_llm)
    graph.add_node("execute_tool", tool_node)
    graph.add_node("finalize", finalize)

    graph.add_edge(START, "parse_task")
    graph.add_edge("parse_task", "invoke_llm")
    graph.add_conditional_edges("invoke_llm", should_use_tool)
    graph.add_edge("execute_tool", "finalize")
    graph.add_edge("finalize", END)

    return graph.compile()


billing_graph = build_graph()


async def handle_task(task: AgentTask) -> TaskResult:
    logger.info("processing task %s: %s", task.task_id, task.task_type)

    initial_state = {
        "task": task,
        "messages": [],
        "result": None,
        "error": None,
    }

    final_state = billing_graph.invoke(initial_state)

    if final_state.get("error"):
        logger.error("task %s failed: %s", task.task_id, final_state["error"])
        return TaskResult(
            task_id=task.task_id,
            status=TaskStatus.FAILED,
            error=final_state["error"],
        )

    logger.info("task %s completed", task.task_id)
    return TaskResult(
        task_id=task.task_id,
        status=TaskStatus.COMPLETED,
        result=final_state.get("result"),
    )
