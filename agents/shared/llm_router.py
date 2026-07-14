import os
from enum import Enum

from langchain_ollama import ChatOllama
from langchain_google_genai import ChatGoogleGenerativeAI


class LLMProvider(str, Enum):
    OLLAMA = "ollama"
    GEMINI = "gemini"


class LLMRouter:
    def __init__(self):
        self.ollama_base_url = os.getenv("OLLAMA_BASE_URL", "http://localhost:11434")
        self.ollama_model = os.getenv("OLLAMA_MODEL", "llama3.2")
        self.gemini_api_key = os.getenv("GEMINI_API_KEY", "")
        self.gemini_model = os.getenv("GEMINI_MODEL", "gemini-2.0-flash")

    def route(self, sensitivity: str) -> object:
        if sensitivity == "high":
            return self._ollama()
        return self._gemini()

    def _ollama(self) -> ChatOllama:
        return ChatOllama(
            model=self.ollama_model,
            base_url=self.ollama_base_url,
            temperature=0.1,
        )

    def _gemini(self) -> ChatGoogleGenerativeAI:
        return ChatGoogleGenerativeAI(
            model=self.gemini_model,
            google_api_key=self.gemini_api_key,
            temperature=0.2,
        )


router = LLMRouter()
