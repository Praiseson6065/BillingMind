import json
import logging
import asyncio
from typing import Callable, Awaitable

import redis.asyncio as redis
from .models import AgentTask

logger = logging.getLogger(__name__)

STREAM_NAME = "billingmind:tasks"
CONSUMER_GROUP = "orchestrator"


class RedisTaskConsumer:
    def __init__(self, redis_url: str, agent_name: str):
        self.client = redis.from_url(redis_url)
        self.agent_name = agent_name
        self.consumer_name = f"{agent_name}-consumer"

    async def ensure_group(self):
        try:
            await self.client.xgroup_create(
                STREAM_NAME, CONSUMER_GROUP, id="0", mkstream=True
            )
        except redis.ResponseError as e:
            if "BUSYGROUP" not in str(e):
                raise

    async def consume(
        self, handler: Callable[[AgentTask], Awaitable[None]], poll_interval_ms: int = 5000
    ):
        await self.ensure_group()
        logger.info("consumer started: agent=%s", self.agent_name)

        while True:
            try:
                results = await self.client.xreadgroup(
                    groupname=CONSUMER_GROUP,
                    consumername=self.consumer_name,
                    streams={STREAM_NAME: ">"},
                    count=10,
                    block=poll_interval_ms,
                )

                if not results:
                    continue

                for _, messages in results:
                    for msg_id, data in messages:
                        target = data.get(b"target_agent", b"").decode()
                        if target != self.agent_name:
                            await self.client.xack(STREAM_NAME, CONSUMER_GROUP, msg_id)
                            continue

                        task = self._parse_message(data)
                        if task is None:
                            logger.warning("failed to parse message %s", msg_id)
                            await self.client.xack(STREAM_NAME, CONSUMER_GROUP, msg_id)
                            continue

                        try:
                            await handler(task)
                        except Exception:
                            logger.exception("handler failed for task %s", task.task_id)

                        await self.client.xack(STREAM_NAME, CONSUMER_GROUP, msg_id)

            except asyncio.CancelledError:
                break
            except Exception:
                logger.exception("consumer loop error")
                await asyncio.sleep(1)

    def _parse_message(self, data: dict) -> AgentTask | None:
        try:
            return AgentTask(
                task_id=data[b"task_id"].decode(),
                task_type=data[b"task_type"].decode(),
                target_agent=data[b"target_agent"].decode(),
                priority=int(data.get(b"priority", b"1").decode()),
                payload=json.loads(data[b"payload"].decode()),
            )
        except (KeyError, json.JSONDecodeError, ValueError):
            return None

    async def close(self):
        await self.client.aclose()
