import asyncio
from loguru import logger
from typing import Optional
from pipecat.pipeline.pipeline import Pipeline
from pipecat.pipeline.runner import PipelineRunner
from pipecat.pipeline.task import PipelineTask
from pipecat.services.openai import OpenAILLMService
from pipecat.processors.aggregators.llm_response import (
    LLMAssistantResponseAggregator,
    LLMUserResponseAggregator,
)
from pipecat.processors.frame_processor import FrameDirection, FrameProcessor
from pipecat.frames.frames import (
    Frame,
    AudioRawFrame,
    LLMMessagesFrame,
    TextFrame,
)
from pipecat.transports.base_transport import BaseTransport
from config import settings
import numpy as np


class VoiceAgent:
    """Voice agent using Pipecat and OpenAI"""

    def __init__(self, system_prompt: Optional[str] = None):
        self.system_prompt = system_prompt or settings.agent_default_prompt
        self.pipeline: Optional[Pipeline] = None
        self.task: Optional[PipelineTask] = None
        self.runner: Optional[PipelineRunner] = None
        self.llm_service: Optional[OpenAILLMService] = None

    async def start(self, transport: BaseTransport):
        """
        Start the voice agent pipeline

        Args:
            transport: Pipecat transport (e.g., WebRTC transport)
        """
        try:
            logger.info("Starting voice agent...")

            # Create OpenAI LLM service (or proxy)
            self.llm_service = OpenAILLMService(
                api_key=settings.openai_api_key,
                base_url=settings.openai_base_url,
                model="gpt-4o-mini",
            )

            # Create message aggregators
            user_aggregator = LLMUserResponseAggregator()
            assistant_aggregator = LLMAssistantResponseAggregator()

            # Create pipeline
            self.pipeline = Pipeline(
                [
                    transport.input(),
                    user_aggregator,
                    self.llm_service,
                    assistant_aggregator,
                    transport.output(),
                ]
            )

            # Create task with initial context
            self.task = PipelineTask(
                self.pipeline,
                params={
                    "messages": [
                        {
                            "role": "system",
                            "content": self.system_prompt,
                        }
                    ]
                },
            )

            # Create and start runner
            self.runner = PipelineRunner()
            await self.runner.run(self.task)

            logger.info("Voice agent started successfully")

        except Exception as e:
            logger.error(f"Failed to start voice agent: {e}")
            raise

    async def stop(self):
        """Stop the voice agent"""
        if self.task:
            await self.task.cancel()
        logger.info("Voice agent stopped")


class SimpleVoiceAgent:
    """
    Simplified voice agent without Pipecat's full pipeline
    Uses OpenAI directly for a more straightforward implementation
    """

    def __init__(self, system_prompt: Optional[str] = None):
        self.system_prompt = system_prompt or settings.agent_default_prompt
        self.conversation_history = [
            {"role": "system", "content": self.system_prompt}
        ]
        self.is_running = False

    async def start(self):
        """Start the agent"""
        self.is_running = True
        logger.info("Simple voice agent started")

    async def process_audio(self, audio_data: bytes) -> Optional[bytes]:
        """
        Process incoming audio and generate response

        Args:
            audio_data: Raw audio bytes

        Returns:
            Response audio bytes or None
        """
        # This is a placeholder for actual audio processing
        # In a full implementation, you would:
        # 1. Use OpenAI Whisper for STT (audio -> text)
        # 2. Send text to GPT for response
        # 3. Use OpenAI TTS for text -> audio
        # 4. Return audio bytes

        logger.info("Processing audio...")
        return None

    async def stop(self):
        """Stop the agent"""
        self.is_running = False
        logger.info("Simple voice agent stopped")
