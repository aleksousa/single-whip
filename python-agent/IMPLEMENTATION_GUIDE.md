# Guia de Implementação - Agente de Voz Completo

Este documento explica como completar a implementação do agente de voz com OpenAI ou proxy compatível.

> 📘 **Usando Proxy OpenAI?** Veja [PROXY_SETUP.md](PROXY_SETUP.md) para configuração detalhada de proxies.

## Status Atual

A implementação atual fornece:
- ✅ Estrutura Flask funcionando
- ✅ Conexão WebRTC via WHIP
- ✅ Sistema de rooms
- ✅ Placeholder para áudio (silêncio)
- ⚠️ Pipeline de voz simplificado (não processa áudio real)

## O Que Falta Implementar

### 1. Pipeline Completo de Áudio

#### Fluxo Desejado:
```
Áudio Entrada (WebRTC)
    ↓
Whisper STT (áudio → texto)
    ↓
GPT-4 Chat (texto → resposta)
    ↓
OpenAI TTS (resposta → áudio)
    ↓
Áudio Saída (WebRTC)
```

### 2. Implementação com OpenAI SDK

Aqui está um exemplo de como implementar cada parte:

#### A. Speech-to-Text (Whisper)

```python
import openai
from openai import OpenAI

# Cliente com suporte a proxy
client = OpenAI(
    api_key=settings.openai_api_key,
    base_url=settings.openai_base_url  # Suporta proxy
)

async def transcribe_audio(audio_data: bytes) -> str:
    """
    Transcreve áudio usando Whisper

    Args:
        audio_data: Bytes de áudio (formato: WAV, MP3, etc)

    Returns:
        Texto transcrito
    """
    # Salvar áudio temporariamente
    with open("temp_audio.wav", "wb") as f:
        f.write(audio_data)

    # Transcrever
    with open("temp_audio.wav", "rb") as audio_file:
        transcript = client.audio.transcriptions.create(
            model="whisper-1",
            file=audio_file,
            language="pt"  # ou "en" para inglês
        )

    return transcript.text
```

#### B. Chat Completion (GPT-4)

```python
async def get_chat_response(user_message: str, conversation_history: list) -> str:
    """
    Obtém resposta do GPT-4

    Args:
        user_message: Mensagem do usuário
        conversation_history: Histórico da conversa

    Returns:
        Resposta do assistente
    """
    # Adicionar mensagem do usuário
    conversation_history.append({
        "role": "user",
        "content": user_message
    })

    # Obter resposta
    response = client.chat.completions.create(
        model="gpt-4o-mini",
        messages=conversation_history,
        temperature=0.7,
        max_tokens=150
    )

    assistant_message = response.choices[0].message.content

    # Adicionar ao histórico
    conversation_history.append({
        "role": "assistant",
        "content": assistant_message
    })

    return assistant_message
```

#### C. Text-to-Speech

```python
async def text_to_speech(text: str) -> bytes:
    """
    Converte texto em áudio usando OpenAI TTS

    Args:
        text: Texto para converter

    Returns:
        Bytes de áudio
    """
    response = client.audio.speech.create(
        model="tts-1",
        voice="alloy",  # opções: alloy, echo, fable, onyx, nova, shimmer
        input=text,
        response_format="opus"  # Opus é ideal para WebRTC
    )

    return response.content
```

### 3. Integrar com WebRTC

#### Modificar `voice_agent.py`:

```python
import asyncio
from queue import Queue
import io
from pydub import AudioSegment

class RealVoiceAgent:
    def __init__(self, system_prompt: str):
        self.system_prompt = system_prompt
        self.conversation_history = [
            {"role": "system", "content": system_prompt}
        ]
        self.audio_queue = asyncio.Queue()
        self.output_queue = asyncio.Queue()
        self.is_running = False
        self.client = OpenAI(
            api_key=settings.openai_api_key,
            base_url=settings.openai_base_url  # Suporta proxy
        )

    async def start(self):
        """Inicia o agente e o processamento de áudio"""
        self.is_running = True
        asyncio.create_task(self._process_audio_loop())

    async def _process_audio_loop(self):
        """Loop principal de processamento de áudio"""
        buffer = []
        silence_threshold = 10  # frames de silêncio antes de processar
        silence_count = 0

        while self.is_running:
            try:
                # Receber frame de áudio
                audio_frame = await asyncio.wait_for(
                    self.audio_queue.get(),
                    timeout=0.1
                )

                # Detectar se é silêncio (implementação simplificada)
                is_silence = self._is_silence(audio_frame)

                if is_silence:
                    silence_count += 1
                else:
                    silence_count = 0
                    buffer.append(audio_frame)

                # Se detectou pausa (silêncio após fala)
                if silence_count >= silence_threshold and len(buffer) > 0:
                    # Processar o buffer acumulado
                    await self._process_speech(buffer)
                    buffer = []
                    silence_count = 0

            except asyncio.TimeoutError:
                continue
            except Exception as e:
                logger.error(f"Erro no loop de áudio: {e}")

    def _is_silence(self, audio_frame) -> bool:
        """Detecta se um frame de áudio é silêncio"""
        # Implementação simplificada - você pode usar bibliotecas
        # como webrtcvad para detecção de voz mais robusta
        audio_data = audio_frame.to_ndarray()
        rms = np.sqrt(np.mean(audio_data**2))
        return rms < 500  # threshold ajustável

    async def _process_speech(self, audio_frames):
        """Processa uma sequência de frames de áudio"""
        try:
            # 1. Converter frames para formato WAV
            audio_bytes = self._frames_to_wav(audio_frames)

            # 2. Transcrever com Whisper
            user_text = await self._transcribe(audio_bytes)
            logger.info(f"Usuário disse: {user_text}")

            # 3. Obter resposta do GPT
            response_text = await self._get_response(user_text)
            logger.info(f"Assistente responde: {response_text}")

            # 4. Converter resposta para áudio
            response_audio = await self._synthesize_speech(response_text)

            # 5. Enviar áudio de resposta
            await self.output_queue.put(response_audio)

        except Exception as e:
            logger.error(f"Erro processando fala: {e}")

    def _frames_to_wav(self, frames) -> bytes:
        """Converte frames de áudio para WAV"""
        # Combinar todos os frames
        samples = []
        for frame in frames:
            samples.extend(frame.to_ndarray().flatten())

        # Converter para WAV usando pydub
        audio = AudioSegment(
            data=np.array(samples, dtype=np.int16).tobytes(),
            sample_width=2,  # 16-bit
            frame_rate=48000,
            channels=2
        )

        # Exportar para bytes
        buffer = io.BytesIO()
        audio.export(buffer, format="wav")
        return buffer.getvalue()

    async def _transcribe(self, audio_bytes: bytes) -> str:
        """Transcreve áudio"""
        # Salvar temporariamente
        with open("temp.wav", "wb") as f:
            f.write(audio_bytes)

        with open("temp.wav", "rb") as f:
            transcript = self.client.audio.transcriptions.create(
                model="whisper-1",
                file=f,
                language="pt"
            )
        return transcript.text

    async def _get_response(self, user_message: str) -> str:
        """Obtém resposta do GPT"""
        self.conversation_history.append({
            "role": "user",
            "content": user_message
        })

        response = self.client.chat.completions.create(
            model="gpt-4o-mini",
            messages=self.conversation_history,
            temperature=0.7,
            max_tokens=150
        )

        assistant_message = response.choices[0].message.content

        self.conversation_history.append({
            "role": "assistant",
            "content": assistant_message
        })

        return assistant_message

    async def _synthesize_speech(self, text: str) -> bytes:
        """Sintetiza fala"""
        response = self.client.audio.speech.create(
            model="tts-1",
            voice="alloy",
            input=text,
            response_format="opus"
        )
        return response.content
```

### 4. Modificar `main.py` para usar RealVoiceAgent

```python
# Substituir a classe de audio track
class AgentAudioTrack(MediaStreamTrack):
    """Audio track que envia áudio do agente"""
    kind = "audio"

    def __init__(self, agent):
        super().__init__()
        self.agent = agent
        self.sample_rate = 48000

    async def recv(self):
        """Envia frames de áudio do agente"""
        # Receber áudio processado do agente
        audio_bytes = await self.agent.output_queue.get()

        # Converter para AudioFrame
        # ... implementação de conversão

        return audio_frame

# No endpoint join_room
async def on_remote_track(track):
    """Processa áudio recebido"""
    while True:
        try:
            frame = await track.recv()
            # Enviar para o agente processar
            await agent.audio_queue.put(frame)
        except Exception as e:
            logger.error(f"Erro recebendo áudio: {e}")
            break
```

### 5. Dependências Adicionais

Adicione ao `requirements.txt`:
```
pydub==0.25.1
webrtcvad==2.0.10
```

## Testes

### 1. Testar Whisper isoladamente:

```python
from openai import OpenAI
from config import settings

client = OpenAI(
    api_key=settings.openai_api_key,
    base_url=settings.openai_base_url  # Funciona com proxy
)

with open("audio.mp3", "rb") as f:
    transcript = client.audio.transcriptions.create(
        model="whisper-1",
        file=f
    )
print(transcript.text)
```

### 2. Testar TTS:

```python
response = client.audio.speech.create(
    model="tts-1",
    voice="alloy",
    input="Olá! Como posso ajudar você hoje?"
)

with open("output.mp3", "wb") as f:
    f.write(response.content)
```

## Próximos Passos

1. Implementar `RealVoiceAgent` com pipeline completo
2. Testar cada componente isoladamente (STT, LLM, TTS)
3. Integrar com WebRTC
4. Adicionar detecção de voz (VAD - Voice Activity Detection)
5. Otimizar latência
6. Adicionar tratamento de interrupções
7. Implementar cancelamento de resposta quando usuário fala

## Recursos

- [OpenAI Audio API](https://platform.openai.com/docs/guides/speech-to-text)
- [OpenAI TTS](https://platform.openai.com/docs/guides/text-to-speech)
- [Pipecat Docs](https://docs.pipecat.ai/)
- [aiortc](https://aiortc.readthedocs.io/)
