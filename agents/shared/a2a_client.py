import httpx
import logging
from .models import A2AMessage

logger = logging.getLogger(__name__)


class A2AClient:
    def __init__(self, agent_endpoints: dict[str, str]):
        self.endpoints = agent_endpoints
        self.http = httpx.AsyncClient(timeout=30.0)

    async def send(self, message: A2AMessage) -> dict:
        url = self.endpoints.get(message.to_agent)
        if not url:
            raise ValueError(f"no endpoint registered for agent: {message.to_agent}")

        target = f"{url}/a2a"
        logger.info(
            "sending a2a message: %s -> %s [%s]",
            message.from_agent,
            message.to_agent,
            message.message_type,
        )

        response = await self.http.post(target, json=message.model_dump(mode="json"))
        response.raise_for_status()
        return response.json()

    async def close(self):
        await self.http.aclose()
