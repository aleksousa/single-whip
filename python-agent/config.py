from pydantic_settings import BaseSettings
from pydantic import Field


class Settings(BaseSettings):
    """Application settings loaded from environment variables"""

    # OpenAI / Proxy Configuration
    openai_api_key: str = Field(..., description="OpenAI API key or proxy auth token")
    openai_base_url: str = Field(
        default="https://api.openai.com/v1",
        description="OpenAI API base URL or your proxy URL"
    )

    # Server
    server_host: str = Field(default="127.0.0.1", description="WHIP server host")
    server_port: int = Field(default=8080, description="WHIP server port")

    # Agent
    agent_default_prompt: str = Field(
        default="You are a friendly and helpful voice assistant. You speak clearly and concisely.",
        description="Default system prompt for the agent"
    )

    # Flask API
    api_host: str = Field(default="0.0.0.0", description="Flask API host")
    api_port: int = Field(default=8000, description="Flask API port")

    class Config:
        env_file = ".env"
        case_sensitive = False


settings = Settings()
