# Voice Agent - Python Client

Cliente Python com Flask e Pipecat que funciona como um agente de voz conversacional usando OpenAI.

## Características

- **Flask**: API REST para controlar o agente
- **WebRTC**: Conexão com servidor WHIP para comunicação de áudio em tempo real
- **OpenAI Integration**: STT (Speech-to-Text), TTS (Text-to-Speech) e Chat
- **Pipecat**: Framework para voice agents
- **Sistema de Rooms**: Conecta-se a salas específicas via room ID

## Instalação

### 1. Criar ambiente virtual

```bash
cd python-agent
python -m venv venv
```

### 2. Ativar ambiente virtual

Windows:
```bash
venv\Scripts\activate
```

Linux/Mac:
```bash
source venv/bin/activate
```

### 3. Instalar dependências

```bash
pip install -r requirements.txt
```

### 4. Configurar variáveis de ambiente

Copie o arquivo de exemplo:
```bash
cp .env.example .env
```

Edite `.env` e configure suas credenciais:

**Usando OpenAI diretamente:**
```bash
OPENAI_API_KEY=sk-...
OPENAI_BASE_URL=https://api.openai.com/v1
```

**Usando um proxy da OpenAI:**
```bash
OPENAI_API_KEY=seu_token_do_proxy
OPENAI_BASE_URL=http://localhost:3000/v1
```

> 📘 **Nota**: Este projeto suporta proxies compatíveis com a API da OpenAI. Para mais detalhes sobre configuração de proxy, veja [PROXY_SETUP.md](PROXY_SETUP.md)

## Uso

### 1. Iniciar o servidor

Certifique-se de que o servidor Go está rodando:
```bash
cd ../server
go run main.go
```

### 2. Iniciar o agente Python

```bash
python main.py
```

O servidor Flask será iniciado em `http://localhost:8000`

### 3. Conectar o agente a uma sala

Use a API para conectar o agente a uma sala:

```bash
curl -X POST "http://localhost:8000/join_room" \
  -H "Content-Type: application/json" \
  -d '{
    "room_id": "123",
    "system_prompt": "You are a friendly assistant who loves to help people."
  }'
```

### 4. Conectar um cliente na mesma sala

```bash
cd ../client
go run main.go 123
```

Agora o cliente Go e o agente Python estarão na mesma sala e podem trocar áudio!

## Endpoints da API

### `GET /`
Health check

**Resposta:**
```json
{
  "service": "Voice Agent API",
  "status": "running",
  "active_sessions": 0
}
```

### `POST /join_room`
Conecta o agente a uma sala

**Request:**
```json
{
  "room_id": "123",
  "system_prompt": "You are a helpful assistant." // opcional
}
```

**Resposta:**
```json
{
  "success": true,
  "room_id": "123",
  "message": "Agent successfully joined room 123 and is ready to chat"
}
```

### `POST /leave_room/{room_id}`
Desconecta o agente da sala

**Resposta:**
```json
{
  "success": true,
  "message": "Left room 123"
}
```

### `GET /rooms`
Lista todas as salas ativas

**Resposta:**
```json
{
  "active_rooms": ["123", "456"],
  "count": 2
}
```

## Arquitetura

```
┌─────────────────┐
│   Flask App     │
│   (main.py)     │
└────────┬────────┘
         │
         ├─────────────┐
         │             │
    ┌────▼──────┐  ┌──▼─────────────┐
    │  WebRTC   │  │  Voice Agent   │
    │  Client   │  │  (Pipecat +    │
    │           │  │   OpenAI)      │
    └────┬──────┘  └───────┬────────┘
         │                 │
         │        ┌────────▼────────┐
         │        │  OpenAI APIs    │
         │        │  - Whisper STT  │
         │        │  - GPT-4 Chat   │
         │        │  - TTS          │
         │        └─────────────────┘
         │
    ┌────▼──────────────┐
    │  WHIP Server (Go) │
    │  Room: 123        │
    └────┬──────────────┘
         │
    ┌────▼──────────────┐
    │  Other Clients    │
    │  (Go client, etc) │
    └───────────────────┘
```

## Desenvolvimento

### Estrutura de arquivos

```
python-agent/
├── main.py              # Flask application
├── config.py            # Configuration and settings
├── webrtc_client.py     # WHIP WebRTC client
├── voice_agent.py       # Voice agent with Pipecat
├── requirements.txt     # Python dependencies
├── .env.example         # Environment variables template
└── README.md           # This file
```

## Próximos Passos

A implementação atual é uma base funcional. Para ter um agente de voz completo, você precisa:

1. **Implementar Audio Pipeline Completo**:
   - Capturar áudio do WebRTC
   - Processar com OpenAI Whisper (STT)
   - Enviar transcrição para GPT-4
   - Gerar resposta com OpenAI TTS
   - Enviar áudio de volta via WebRTC

2. **Integração Pipecat Completa**:
   - Configurar transporte WebRTC no Pipecat
   - Integrar serviços OpenAI (Whisper, GPT, TTS)
   - Criar pipeline completo de processamento

3. **Melhorias**:
   - Tratamento de erros robusto
   - Logs estruturados
   - Métricas e monitoramento
   - Testes automatizados

## Troubleshooting

### Erro: "Failed to connect to room"
- Verifique se o servidor Go está rodando
- Verifique as configurações de host/port no .env

### Erro: "OpenAI API key not found"
- Certifique-se de ter configurado OPENAI_API_KEY no arquivo .env

### Áudio não está sendo transmitido
- A implementação atual usa um placeholder de áudio (silêncio)
- Você precisa implementar a captura/geração de áudio real

## Licença

MIT
