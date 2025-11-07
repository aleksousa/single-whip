# Exemplos de Uso

## Cenários de Teste

### Cenário 1: Agente atende em uma sala de suporte

```bash
# 1. Iniciar servidor Go
cd ../server
go run main.go

# 2. Iniciar agente Python (em outro terminal)
cd ../python-agent
python main.py

# 3. Conectar agente na sala "suporte-123"
curl -X POST "http://localhost:8000/join_room" \
  -H "Content-Type: application/json" \
  -d '{
    "room_id": "suporte-123",
    "system_prompt": "Você é um assistente de suporte técnico amigável e prestativo. Ajude os usuários com seus problemas de forma clara e profissional."
  }'

# 4. Cliente conecta na mesma sala (em outro terminal)
cd ../client
go run main.go suporte-123

# Agora o cliente e o agente estão conectados!
```

### Cenário 2: Agente como professor de idiomas

```bash
# Conectar agente como professor de inglês
curl -X POST "http://localhost:8000/join_room" \
  -H "Content-Type: application/json" \
  -d '{
    "room_id": "english-101",
    "system_prompt": "You are an English teacher. Help students practice their English conversation skills. Speak clearly, correct mistakes gently, and encourage them to speak more."
  }'

# Cliente conecta para praticar
cd ../client
go run main.go english-101
```

### Cenário 3: Agente como recepcionista virtual

```bash
# Conectar agente como recepcionista
curl -X POST "http://localhost:8000/join_room" \
  -H "Content-Type: application/json" \
  -d '{
    "room_id": "reception-main",
    "system_prompt": "Você é a recepcionista virtual da empresa TechCorp. Seja cordial, cumprimente os visitantes e pergunte como pode ajudar. Se necessário, direcione para os departamentos apropriados."
  }'
```

## Comandos Úteis da API

### Verificar status do servidor

```bash
curl http://localhost:8000/
```

Resposta:
```json
{
  "service": "Voice Agent API",
  "status": "running",
  "active_sessions": 1
}
```

### Listar salas ativas

```bash
curl http://localhost:8000/rooms
```

Resposta:
```json
{
  "active_rooms": ["suporte-123", "english-101"],
  "count": 2
}
```

### Desconectar agente de uma sala

```bash
curl -X POST "http://localhost:8000/leave_room/suporte-123"
```

Resposta:
```json
{
  "success": true,
  "message": "Left room suporte-123"
}
```

## Testes com Python

### Teste básico usando requests

```python
import requests

# Conectar agente
response = requests.post(
    "http://localhost:8000/join_room",
    json={
        "room_id": "test-room",
        "system_prompt": "You are a friendly test bot."
    }
)

print(response.json())

# Listar rooms
rooms = requests.get("http://localhost:8000/rooms")
print(rooms.json())

# Desconectar
leave = requests.post("http://localhost:8000/leave_room/test-room")
print(leave.json())
```

### Usando o script de teste incluído

```bash
python test_api.py
```

## Fluxo Completo de Teste

### Terminal 1: Servidor Go
```bash
cd server
go run main.go
```

### Terminal 2: Agente Python
```bash
cd python-agent
python main.py
```

### Terminal 3: Conectar agente via API
```bash
curl -X POST "http://localhost:8000/join_room" \
  -H "Content-Type: application/json" \
  -d '{"room_id": "123", "system_prompt": "You are a helpful assistant."}'
```

### Terminal 4: Cliente Go
```bash
cd client
go run main.go 123
```

### Terminal 5: Outro Cliente Go (opcional)
```bash
cd client
# Usar porta diferente editando o código ou rodando em outro processo
go run main.go 123
```

## Verificar Logs

O agente Python usa `loguru` para logs estruturados. Você verá:

```
INFO: Starting voice agent...
INFO: Joining room: 123
INFO: Successfully connected to room: 123
INFO: Receiving audio track from peer
```

O servidor Go também mostra logs:

```
Client connecting to room: 123
Created new room: 123
Peer waiting in room 123
Client connecting to room: 123
Pairing peers in room 123
```

## Troubleshooting por Cenário

### Problema: Agente não conecta

**Sintoma:**
```json
{
  "detail": "Failed to connect to room"
}
```

**Solução:**
1. Verificar se servidor Go está rodando
2. Verificar firewall/portas
3. Ver logs do servidor Go

### Problema: Room já existe

**Sintoma:**
```json
{
  "detail": "Agent already active in room 123"
}
```

**Solução:**
```bash
# Desconectar primeiro
curl -X POST "http://localhost:8000/leave_room/123"

# Depois reconectar
curl -X POST "http://localhost:8000/join_room" \
  -H "Content-Type: application/json" \
  -d '{"room_id": "123"}'
```

## Monitoramento

### Verificar salas ativas a cada 5 segundos

Bash:
```bash
watch -n 5 'curl -s http://localhost:8000/rooms | jq'
```

PowerShell:
```powershell
while ($true) {
    curl http://localhost:8000/rooms | ConvertFrom-Json | ConvertTo-Json
    Start-Sleep -Seconds 5
}
```

## Próximos Passos

Depois de testar a conexão básica:

1. Implementar o pipeline de áudio real (ver IMPLEMENTATION_GUIDE.md)
2. Adicionar captura de áudio do microfone no client Go
3. Testar conversação real por voz
4. Ajustar prompts do sistema para diferentes casos de uso
5. Implementar métricas e monitoramento
