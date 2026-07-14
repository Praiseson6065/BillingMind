    import logging
from typing import TypedDict, Annotated

from langgraph.graph import StateGraph, START, END
from langchain_core.messages import BaseMessage

from agents.shared.models import AgentTask, A2AMessage, TaskResult, TaskStatus, AuditReport
from .analyzer import analyze_invoice_patterns

logger = logging.getLogger(__name__)


class AuditState(TypedDict):
    task: AgentTask | None
    a2a_message: A2AMessage | None
    messages: Annotated[list[BaseMessage], lambda a, b: a + b]
    invoices: list[dict]
    report: AuditReport | None
    error: str | None


def collect_data(state: AuditState) -> dict:
    task = state.get("task")
    a2a = state.get("a2a_message")

    invoices = []
    if task and task.payload:
        invoices = task.payload.get("invoices", [])
        if not invoices and task.payload.get("id"):
            invoices = [task.payload]

    if a2a and a2a.payload:
        flagged = a2a.payload
        invoices = [flagged] if not isinstance(flagged, list) else flagged

    return {"invoices": invoices}


def run_analysis(state: AuditState) -> dict:
    invoices = state.get("invoices", [])
    if not invoices:
        return {"error": "no invoice data to analyze"}

    report = analyze_invoice_patterns(invoices)
    logger.info(
        "audit complete: risk_score=%.2f findings=%d actions=%d",
        report.risk_score,
        len(report.findings),
        len(report.recommended_actions),
    )
    return {"report": report}


def build_graph() -> StateGraph:
    graph = StateGraph(AuditState)
    graph.add_node("collect_data", collect_data)
    graph.add_node("run_analysis", run_analysis)

    graph.add_edge(START, "collect_data")
    graph.add_edge("collect_data", "run_analysis")
    graph.add_edge("run_analysis", END)

    return graph.compile()


audit_graph = build_graph()


async def handle_task(task: AgentTask) -> TaskResult:
    logger.info("processing audit task %s: %s", task.task_id, task.task_type)

    initial_state = {
        "task": task,
        "a2a_message": None,
        "messages": [],
        "invoices": [],
        "report": None,
        "error": None,
    }

    final_state = audit_graph.invoke(initial_state)

    if final_state.get("error"):
        return TaskResult(task_id=task.task_id, status=TaskStatus.FAILED, error=final_state["error"])

    report = final_state.get("report")
    result = report.model_dump(mode="json") if report else {}
    return TaskResult(task_id=task.task_id, status=TaskStatus.COMPLETED, result=result)


async def handle_a2a(message: A2AMessage) -> dict:
    logger.info("processing audit.flag from %s", message.from_agent)

    initial_state = {
        "task": None,
        "a2a_message": message,
        "messages": [],
        "invoices": [],
        "report": None,
        "error": None,
    }

    final_state = audit_graph.invoke(initial_state)
    report = final_state.get("report")
    return report.model_dump(mode="json") if report else {"error": final_state.get("error")}
