import logging
from langchain_core.messages import HumanMessage

from agents.shared.llm_router import router
from agents.shared.models import DunningStrategy

logger = logging.getLogger(__name__)


def generate_strategy(attempt_number: int, customer_history: dict) -> DunningStrategy:
    llm = router.route("high")

    prompt = (
        f"You are a payment recovery specialist. A customer's payment has failed.\n\n"
        f"Attempt number: {attempt_number}\n"
        f"Customer history: {customer_history}\n\n"
        f"Based on this information, recommend a dunning strategy. Respond in EXACTLY this JSON format:\n"
        f'{{\n'
        f'  "retry_delay_hours": <integer>,\n'
        f'  "email_tone": "<gentle|urgent|final_notice>",\n'
        f'  "offer_discount": <true|false>,\n'
        f'  "suspend_access": <true|false>,\n'
        f'  "reasoning": "<brief explanation>"\n'
        f'}}\n\n'
        f"Rules:\n"
        f"- Attempt 1: gentle tone, 24-48h delay, no discount, no suspension\n"
        f"- Attempt 2: urgent tone, 12-24h delay, consider small discount\n"
        f"- Attempt 3+: final_notice tone, 4-8h delay, offer discount, suspend access\n"
        f"Respond with ONLY the JSON object, no extra text."
    )

    response = llm.invoke([HumanMessage(content=prompt)])
    content = response.content.strip()

    if content.startswith("```"):
        content = content.split("\n", 1)[1].rsplit("```", 1)[0].strip()

    import json
    try:
        data = json.loads(content)
        return DunningStrategy(**data)
    except (json.JSONDecodeError, ValueError):
        logger.warning("llm returned unparseable strategy, using fallback for attempt %d", attempt_number)
        return _fallback_strategy(attempt_number)


def _fallback_strategy(attempt_number: int) -> DunningStrategy:
    if attempt_number <= 1:
        return DunningStrategy(
            retry_delay_hours=24,
            email_tone="gentle",
            offer_discount=False,
            suspend_access=False,
            reasoning="first attempt: standard gentle reminder",
        )
    if attempt_number == 2:
        return DunningStrategy(
            retry_delay_hours=12,
            email_tone="urgent",
            offer_discount=False,
            suspend_access=False,
            reasoning="second attempt: increased urgency",
        )
    return DunningStrategy(
        retry_delay_hours=6,
        email_tone="final_notice",
        offer_discount=True,
        suspend_access=True,
        reasoning="third+ attempt: final notice with discount offer and access suspension",
    )
