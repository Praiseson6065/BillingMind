import os
import logging
from contextlib import asynccontextmanager

from dotenv import load_dotenv
from fastapi import FastAPI
from fastapi.responses import JSONResponse

from agents.shared.models import AgentTask, A2AMessage
from .agent import handle_task, handle_a2a

load_dotenv()
logging.basicConfig(level=logging.INFO, format="%(asctime)s %(name)s %(levelname)s %(message)s")
logger = logging.getLogger(__name__)

AGENT_NAME = "audit"


@asynccontextmanager
async def lifespan(app: FastAPI):
    logger.info("audit agent started")
    yield
    logger.info("audit agent stopped")


app = FastAPI(title="AuditAgent", lifespan=lifespan)


@app.post("/task")
async def receive_task(task: AgentTask):
    result = await handle_task(task)
    return JSONResponse(
        status_code=200,
        content={
            "status": "accepted",
            "task_id": str(task.task_id),
            "result_status": result.status,
            "result": result.result,
        },
    )


@app.post("/a2a")
async def receive_a2a(message: A2AMessage):
    logger.info("received a2a from %s: %s", message.from_agent, message.message_type)

    if message.message_type == "audit.flag":
        report = await handle_a2a(message)
        return {"status": "accepted", "correlation_id": message.correlation_id, "report": report}

    return {"status": "ignored", "correlation_id": message.correlation_id}


@app.get("/health")
async def health():
    return {"agent": AGENT_NAME, "status": "healthy"}


if __name__ == "__main__":
    import uvicorn
    port = int(os.getenv("AUDIT_AGENT_PORT", "8004"))
    uvicorn.run(app, host="0.0.0.0", port=port)
