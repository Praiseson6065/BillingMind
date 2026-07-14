import os
import logging
import asyncio
from contextlib import asynccontextmanager

from dotenv import load_dotenv
load_dotenv()

from fastapi import FastAPI
from fastapi.responses import JSONResponse

from agents.shared.models import AgentTask, A2AMessage, TaskStatus
from agents.shared.redis_consumer import RedisTaskConsumer
from .agent import handle_task

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(name)s %(levelname)s %(message)s")
logger = logging.getLogger(__name__)

REDIS_URL = os.getenv("REDIS_URL", "redis://localhost:6379")
AGENT_NAME = "billing"


consumer = RedisTaskConsumer(redis_url=REDIS_URL, agent_name=AGENT_NAME)


async def task_consumer_loop():
    async def on_task(task: AgentTask):
        await handle_task(task)

    await consumer.consume(on_task)


@asynccontextmanager
async def lifespan(app: FastAPI):
    consumer_task = asyncio.create_task(task_consumer_loop())
    logger.info("billing agent started")
    yield
    consumer_task.cancel()
    await consumer.close()
    logger.info("billing agent stopped")


app = FastAPI(title="BillingAgent", lifespan=lifespan)


@app.post("/task")
async def receive_task(task: AgentTask):
    result = await handle_task(task)
    return JSONResponse(
        status_code=200,
        content={"status": "accepted", "task_id": str(task.task_id), "result_status": result.status},
    )


@app.post("/a2a")
async def receive_a2a(message: A2AMessage):
    logger.info("received a2a from %s: %s", message.from_agent, message.message_type)

    if message.message_type == "escalate.refund":
        task = AgentTask(
            task_type="subscription.cancel",
            target_agent=AGENT_NAME,
            payload=message.payload,
        )
        result = await handle_task(task)
        return {"status": "accepted", "correlation_id": message.correlation_id, "result_status": result.status}

    return {"status": "ignored", "correlation_id": message.correlation_id}


@app.get("/health")
async def health():
    return {"agent": AGENT_NAME, "status": "healthy"}


if __name__ == "__main__":
    import uvicorn
    port = int(os.getenv("BILLING_AGENT_PORT", "8001"))
    uvicorn.run(app, host="0.0.0.0", port=port)
