from pydantic import BaseModel, Field
from typing import Optional
from uuid import UUID, uuid4
from datetime import datetime
from enum import Enum


class TaskStatus(str, Enum):
    PENDING = "pending"
    IN_PROGRESS = "in_progress"
    COMPLETED = "completed"
    FAILED = "failed"


class Sensitivity(str, Enum):
    HIGH = "high"
    LOW = "low"


class AgentTask(BaseModel):
    task_id: UUID = Field(default_factory=uuid4)
    task_type: str
    priority: int = 1
    payload: dict
    target_agent: str
    status: TaskStatus = TaskStatus.PENDING
    created_at: Optional[datetime] = None
    completed_at: Optional[datetime] = None


class A2AMessage(BaseModel):
    from_agent: str
    to_agent: str
    message_type: str
    payload: dict
    correlation_id: str = Field(default_factory=lambda: str(uuid4()))


class TaskResult(BaseModel):
    task_id: UUID
    status: TaskStatus
    result: Optional[dict] = None
    error: Optional[str] = None


class DunningStrategy(BaseModel):
    retry_delay_hours: int
    email_tone: str
    offer_discount: bool
    suspend_access: bool
    reasoning: str


class AuditReport(BaseModel):
    findings: list[dict]
    risk_score: float = Field(ge=0.0, le=1.0)
    recommended_actions: list[str]
    analyzed_at: datetime = Field(default_factory=datetime.utcnow)
