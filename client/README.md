# WebRTC TTS/STT Client

Cliente Go com dois modos de operação:
- **Speak**: Converte texto em fala e envia via WebRTC
- **Listen**: Recebe áudio via WebRTC e converte para texto

## Funcionalidades

### Modo Speak (`/speak`)
- Recebe frases em texto via HTTP
- Converte texto para áudio (OpenAI TTS)
- Envia áudio via WebRTC
- Intervalo de 15 segundos entre frases
- Mensagem final automática

### Modo Listen (`/listen`)
- Conecta em uma room via WebRTC
- Recebe áudio de outros peers
- Converte áudio para texto (OpenAI Whisper)
- Imprime texto transcrito no console

## Configuração

### Variáveis de Ambiente

```bash
export OPENAI_API_KEY=sk-...
export OPENAI_BASE_URL=http://localhost:3000/v1
```

Se `OPENAI_BASE_URL` não for definida, usa: `https://api.openai.com/v1`

## Uso

### 1. Iniciar Servidor WHIP

```bash
cd ../server
go run main.go
```

### 2. Iniciar Cliente

```bash
cd client
go run main.go
```

O cliente inicia na porta `:8081`

### 3A. Modo Speak - Enviar Frases

```bash
curl -X POST http://localhost:8081/speak \
  -H "Content-Type: application/json" \
  -d '{
    "room_id": "123",
    "phrases": [
      "Primeira frase",
      "Segunda frase",
      "Terceira frase"
    ]
  }'
```

**Output esperado:**
```
[1/3] Sending text: Primeira frase...
[1/3] Audio sent successfully
Waiting 15 seconds...
[2/3] Sending text: Segunda frase...
```

### 3B. Modo Listen - Ouvir e Transcrever

```bash
curl -X POST http://localhost:8081/listen \
  -H "Content-Type: application/json" \
  -d '{
    "room_id": "123"
  }'
```

**Output esperado:**
```
Starting WebRTC connection to room: 123 (listen mode)
Receiving audio track: audio
Primeira frase
Segunda frase
Terceira frase
```

## Exemplo Completo: Speak + Listen

### Terminal 1: Servidor
```bash
cd server
go run main.go
```

### Terminal 2: Cliente (modo listen)
```bash
cd client
export OPENAI_API_KEY=sk-...
go run main.go
```

Em outro terminal:
```bash
curl -X POST http://localhost:8081/listen \
  -H "Content-Type: application/json" \
  -d '{"room_id": "demo"}'
```

### Terminal 3: Cliente (modo speak)
```bash
curl -X POST http://localhost:8081/speak \
  -H "Content-Type: application/json" \
  -d '{
    "room_id": "demo",
    "phrases": ["Olá, este é um teste de áudio"]
  }'
```

**Resultado**: Terminal 2 imprimirá "Olá, este é um teste de áudio"

## API

### `GET /`

Health check

**Response:**
```json
{
  "service": "WebRTC TTS Client",
  "status": "running"
}
```

### `POST /speak`

Converte texto para áudio e envia via WebRTC

**Request:**
```json
{
  "room_id": "string",
  "phrases": ["string", "string"]
}
```

**Response:**
```json
{
  "success": true,
  "room_id": "string",
  "message": "Processing N phrases"
}
```

### `POST /listen`

Conecta na room e transcreve áudio recebido

**Request:**
```json
{
  "room_id": "string"
}
```

**Response:**
```json
{
  "success": true,
  "room_id": "string",
  "message": "Listening for audio"
}
```

## Fluxos

### Fluxo Speak
1. Recebe HTTP POST com frases
2. Conecta WebRTC → room
3. Para cada frase:
   - Mostra primeiros 20 caracteres
   - Converte texto → áudio (TTS)
   - Envia via WebRTC
   - Aguarda 15 segundos
4. Envia "Isso é tudo pessoal"
5. Encerra conexão

### Fluxo Listen
1. Recebe HTTP POST com room_id
2. Conecta WebRTC → room
3. Escuta áudio continuamente
4. A cada 3 segundos:
   - Processa buffer de áudio
   - Converte áudio → texto (Whisper)
   - Imprime texto completo
5. Continua até ser interrompido

## Scripts de Teste

### Bash

```bash
./test_speak.sh room-id
./test_listen.sh room-id
```

### PowerShell

```powershell
.\test_speak.ps1 -RoomId "room-id"
.\test_listen.ps1 -RoomId "room-id"
```

## Configurações Avançadas

### Alterar Voz TTS

Edite `main.go` linha 421:
```go
Voice: "alloy",
```

Opções: `alloy`, `echo`, `fable`, `onyx`, `nova`, `shimmer`

### Alterar Intervalo Entre Frases

Edite `main.go` linha 254:
```go
time.Sleep(15 * time.Second)
```

### Alterar Frequência de Transcrição

Edite `main.go` linha 342:
```go
ticker := time.NewTicker(3 * time.Second)
```

## Troubleshooting

### Erro: "OPENAI_API_KEY environment variable not set"

```bash
export OPENAI_API_KEY=sk-...
```

### Erro: "TTS API error (status 401)"

Verifique se a chave da API está correta

### Erro: "Error sending WHIP request"

Verifique se o servidor WHIP está rodando

### Listen não recebe áudio

1. Verifique se há outro peer na mesma room
2. Verifique logs do servidor para confirmar pareamento
3. Aguarde alguns segundos após conexão

### Transcrição vazia ou incorreta

1. Verifique se há áudio sendo enviado
2. Aumente o tempo do buffer (linha 342)
3. Verifique formato do áudio (deve ser Opus)

## Notas Técnicas

- Formato áudio: Opus 48kHz
- Buffer de transcrição: 3 segundos
- Preview de texto: 20 caracteres
- Timeout API: 30 segundos
- Conexão WebRTC: mantida até fim do processo
