# Single WHIP - WebRTC Room System with TTS

Sistema de comunicação em tempo real usando WebRTC com servidor WHIP e cliente Text-to-Speech.

## Visão Geral

Este projeto implementa:
- **Server (Go)**: Servidor WHIP que gerencia rooms e faz relay de áudio entre peers
- **Client (Go)**: Cliente HTTP que converte texto em fala e envia via WebRTC

## Arquitetura

```
┌─────────────────┐
│  Client 1 (Go)  │
│  TTS Client     │
│  :8081          │
└────────┬────────┘
         │ HTTP POST /speak
         │ (text phrases)
         │
         ▼
    ┌────────────┐     WebRTC Audio
    │   Client   │◄───────────────────┐
    │  (WebRTC)  │                    │
    └──────┬─────┘                    │
           │                          │
           │ WHIP Protocol            │
           │ (Room: 123)              │
           │                          │
           ▼                          │
    ┌─────────────┐                   │
    │   Server    │                   │
    │    :8080    │                   │
    │             │                   │
    │  Room: 123  │                   │
    │  ┌────────┐ │                   │
    │  │ Peer A │─┼───────────────────┘
    │  │ Peer B │─┼────────────────────►
    │  └────────┘ │                    │
    └─────────────┘                    │
                                       │
                                       ▼
                              ┌─────────────────┐
                              │  Client 2       │
                              │  (Outro peer)   │
                              │  Ouve o áudio   │
                              └─────────────────┘
```

## Componentes

### Server (Go)

Servidor WHIP que:
- Gerencia múltiplas rooms simultaneamente
- Pareia clientes na mesma room (máximo 2 por room)
- Faz relay de áudio entre peers pareados
- Suporta protocolo WHIP (WebRTC-HTTP Ingestion Protocol)

**Porta**: 8080

### Client (Go)

Cliente HTTP com TTS que:
- Recebe requisições HTTP com texto
- Converte texto em fala via OpenAI TTS
- Conecta via WebRTC ao servidor
- Envia áudio para a room
- Suporta múltiplas frases com intervalo configurável
- Encerra automaticamente após completar

**Porta**: 8081

## Quick Start

### 1. Configurar Variáveis de Ambiente

```bash
# Obrigatório para o client
export OPENAI_API_KEY=sk-...

# Opcional: usar proxy da OpenAI
export OPENAI_BASE_URL=http://localhost:3000/v1
```

### 2. Iniciar o Servidor

```bash
cd server
go run main.go
```

Output:
```
Server started on :8080
```

### 3. Iniciar o Cliente

```bash
cd client
go run main.go
```

Output:
```
Client API starting on :8081
OpenAI Base URL: https://api.openai.com/v1
```

### 4. Enviar Frases para Falar

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

### 5. (Opcional) Conectar Outro Peer

Para ouvir o áudio, você pode conectar outro cliente na mesma room ou usar qualquer aplicação WebRTC compatível.

## Fluxo de Funcionamento

1. **Client recebe requisição HTTP** com room_id e frases
2. **Client conecta via WebRTC** ao servidor na room especificada
3. **Server pareia** o client com outro peer (se disponível)
4. **Para cada frase**:
   - Client converte texto → áudio (OpenAI TTS)
   - Client envia áudio via WebRTC
   - Server faz relay para o outro peer
   - Aguarda 15 segundos
5. **Client envia mensagem final**: "Isso é tudo pessoal"
6. **Client encerra** conexão WebRTC

## Exemplo de Uso

### Cenário 1: Anúncios em Sala Virtual

```bash
# Terminal 1: Servidor
cd server && go run main.go

# Terminal 2: Cliente TTS
cd client && go run main.go

# Terminal 3: Enviar anúncio
curl -X POST http://localhost:8081/speak \
  -H "Content-Type: application/json" \
  -d '{
    "room_id": "sala-reuniao",
    "phrases": [
      "Atenção, a reunião começará em 5 minutos",
      "Por favor, verifiquem seus microfones e câmeras"
    ]
  }'
```

### Cenário 2: Testes Automatizados

```bash
# PowerShell
.\client\test_speak.ps1 -RoomId "test" -ClientUrl "http://localhost:8081"

# Bash
./client/test_speak.sh test http://localhost:8081
```

## Configurações

### Server

Editar `server/main.go`:
- Porta: linha 48 (padrão: 8080)

### Client

Editar `client/main.go`:
- Porta do client: linha 54 (padrão: 8081)
- Endereço do server: linha 17 (padrão: 127.0.0.1:8080)
- Voz TTS: linha 252 (opções: alloy, echo, fable, onyx, nova, shimmer)
- Intervalo entre frases: linha 224 (padrão: 15 segundos)

## Variáveis de Ambiente

### Client

| Variável | Obrigatório | Padrão | Descrição |
|----------|-------------|--------|-----------|
| `OPENAI_API_KEY` | Sim | - | Chave da API OpenAI ou token do proxy |
| `OPENAI_BASE_URL` | Não | `https://api.openai.com/v1` | URL base da API (use para proxy) |

## API Reference

### Server: `POST /whip?room={room_id}`

WHIP endpoint para estabelecer conexão WebRTC

**Headers**:
- `Content-Type: application/sdp`

**Body**: SDP Offer

**Response**: SDP Answer (201 Created)

### Client: `GET /`

Health check

**Response**:
```json
{
  "service": "WebRTC TTS Client",
  "status": "running"
}
```

### Client: `POST /speak`

Enviar frases para serem faladas

**Request**:
```json
{
  "room_id": "string",
  "phrases": ["string", "string", ...]
}
```

**Response** (imediata):
```json
{
  "success": true,
  "room_id": "string",
  "message": "Processing N phrases"
}
```

## Estrutura do Projeto

```
single-whip/
├── server/
│   ├── main.go          # Servidor WHIP
│   └── README.md
├── client/
│   ├── main.go          # Cliente TTS
│   ├── README.md
│   ├── test_speak.sh    # Script de teste (Bash)
│   └── test_speak.ps1   # Script de teste (PowerShell)
├── go.mod
├── go.sum
├── .gitignore
└── README.md            # Este arquivo
```

## Dependências

- Go 1.21+
- [Pion WebRTC](https://github.com/pion/webrtc) - WebRTC em Go
- OpenAI API (ou proxy compatível)

## Troubleshooting

### Client não conecta ao servidor

**Sintomas**: Erro "Error sending WHIP request"

**Soluções**:
1. Verifique se o servidor está rodando
2. Verifique firewall/portas
3. Confirme endereço do servidor no código

### Erro de autenticação OpenAI

**Sintomas**: "TTS API error (status 401)"

**Soluções**:
1. Verifique `OPENAI_API_KEY`
2. Se usando proxy, verifique `OPENAI_BASE_URL`
3. Teste a chave manualmente: `curl -H "Authorization: Bearer $OPENAI_API_KEY" https://api.openai.com/v1/models`

### Áudio não chega ao outro peer

**Sintomas**: Um peer não ouve o outro

**Soluções**:
1. Verifique se ambos os peers estão na mesma room
2. Verifique logs do servidor para confirmar pareamento
3. Aguarde alguns segundos após conexão

## Limitações Conhecidas

- Máximo de 2 peers por room
- Client processa uma requisição por vez (assíncrono)
- Formato de áudio fixo: Opus 48kHz
- Sem suporte a renegociação WebRTC

## Próximos Passos

Melhorias possíveis:
- [ ] Suporte a mais de 2 peers por room
- [ ] Broadcasting para múltiplos peers
- [ ] Configuração dinâmica de voz/velocidade TTS
- [ ] WebSocket para notificações em tempo real
- [ ] Interface web para controle
- [ ] Gravação de áudio
- [ ] Transcrição de áudio recebido

## Licença

MIT

## Contribuindo

Pull requests são bem-vindos! Para mudanças maiores, por favor abra uma issue primeiro.
