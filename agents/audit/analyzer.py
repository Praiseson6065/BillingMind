import json
import logging
from datetime import datetime

from langchain_core.messages import HumanMessage

from agents.shared.llm_router import router
from agents.shared.models import AuditReport

logger = logging.getLogger(__name__)


def analyze_invoice_patterns(invoices: list[dict]) -> AuditReport:
    llm = router.route("low")

    invoice_summary = json.dumps(invoices[:50], indent=2, default=str)

    prompt = (
        f"You are a financial audit analyst for a SaaS billing platform.\n\n"
        f"Analyze the following invoice records for anomalies and patterns:\n\n"
        f"{invoice_summary}\n\n"
        f"Look for:\n"
        f"1. Sudden churn spikes (many cancellations in a short period)\n"
        f"2. Recurring payment failure patterns (same customers failing repeatedly)\n"
        f"3. Customers with multiple failed payment methods\n"
        f"4. Revenue at risk calculation\n"
        f"5. Unusual amounts or frequency changes\n\n"
        f"Respond in EXACTLY this JSON format:\n"
        f'{{\n'
        f'  "findings": [{{"type": "...", "description": "...", "severity": "low|medium|high"}}],\n'
        f'  "risk_score": 0.0 to 1.0,\n'
        f'  "recommended_actions": ["action1", "action2"]\n'
        f'}}\n\n'
        f"Respond with ONLY the JSON object."
    )

    response = llm.invoke([HumanMessage(content=prompt)])
    content = response.content.strip()

    if content.startswith("```"):
        content = content.split("\n", 1)[1].rsplit("```", 1)[0].strip()

    try:
        data = json.loads(content)
        return AuditReport(
            findings=data.get("findings", []),
            risk_score=float(data.get("risk_score", 0.5)),
            recommended_actions=data.get("recommended_actions", []),
            analyzed_at=datetime.utcnow(),
        )
    except (json.JSONDecodeError, ValueError, KeyError):
        logger.warning("gemini returned unparseable audit report, using fallback")
        return AuditReport(
            findings=[{"type": "parse_error", "description": "LLM output could not be parsed", "severity": "low"}],
            risk_score=0.5,
            recommended_actions=["manual review recommended"],
            analyzed_at=datetime.utcnow(),
        )
