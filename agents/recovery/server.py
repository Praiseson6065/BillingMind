import os
import logging
import asyncio
from contextlib import asynccontextmanager

from dotenv import load_dotenv
from fastapi import FastAPI
from fastapi.responses import JSONResponse

from agents.shared.models import AgentTask, A2AMessage
from agents.shared.a2a_client import A2AClient
from agents.shared.redis_consumer import RedisTaskConsumer
from .agent import handle_task

load_dotenv()
logging.basicConfig(level=logging.INFO, format="%(asctime)s %(name)s %(levelname)s %(message)s")
logger = logging.getLogger(__name__)

REDIS_URL = os.getenv("REDIS_URL", "redis://localhost:6379")
AGENT_NAME = "recovery"

AGENT_ENDPOINTS = {
    "billing": os.getenv("BILLING_AGENT_URL", "http://localhost:8001"),
    "support": os.getenv("SUPPORT_AGENT_URL", "http://localhost:8003"),
    "audit": os.getenv("AUDIT_AGENT_URL", "http://localhost:8004"),
}

consumer = RedisTaskConsumer(redis_url=REDIS_URL, agent_name=AGENT_NAME)
a2a_client = A2AClient(agent_endpoints=AGENT_ENDPOINTS)


async def task_consumer_loop():
    async def on_task(task: AgentTask):
        await handle_task(task, a2a_client=a2a_client)

    await consumer.consume(on_task)


@asynccontextmanager
async def lifespan(app: FastAPI):
    consumer_task = asyncio.create_task(task_consumer_loop())
    logger.info("recovery agent started")
    yield
    consumer_task.cancel()
    await consumer.close()
    await a2a_client.close()
    logger.info("recovery agent stopped")


app = FastAPI(title="RecoveryAgent", lifespan=lifespan)


@app.post("/task")
async def receive_task(task: AgentTask):
    result = await handle_task(task, a2a_client=a2a_client)
    return JSONResponse(
        status_code=200,
        content={"status": "accepted", "task_id": str(task.task_id), "result_status": result.status},
    )


@app.post("/a2a")
async def receive_a2a(message: A2AMessage):
    logger.info("received a2a from %s: %s", message.from_agent, message.message_type)
    return {"status": "ignored", "correlation_id": message.correlation_id}


@app.get("/health")
async def health():
    return {"agent": AGENT_NAME, "status": "healthy"}


if __name__ == "__main__":
    import uvicorn
    port = int(os.getenv("RECOVERY_AGENT_PORT", "8002"))
    uvicorn.run(app, host="0.0.0.0", port=port)
