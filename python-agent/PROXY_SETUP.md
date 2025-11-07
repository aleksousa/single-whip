# Configuração de Proxy OpenAI

Este guia explica como configurar o agente para usar um proxy da OpenAI ao invés da API oficial.

## Por que usar um Proxy?

Algumas razões para usar um proxy da OpenAI:

- **Controle de custos**: Monitorar e limitar gastos
- **Caching**: Reduzir chamadas repetidas
- **Segurança**: Gerenciar chaves de API centralmente
- **Custom models**: Usar modelos proprietários ou fine-tuned
- **Logging**: Auditar todas as chamadas à API
- **Rate limiting**: Controlar uso por usuário/aplicação

## Configuração

### 1. Configurar o arquivo .env

Edite o arquivo `.env` e configure:

```bash
# Sua chave de API ou token do proxy
OPENAI_API_KEY=seu_token_aqui

# URL do seu proxy (importante terminar com /v1)
OPENAI_BASE_URL=http://localhost:3000/v1
```

### Exemplos de Configuração

#### Proxy Local
```bash
OPENAI_API_KEY=your_proxy_token
OPENAI_BASE_URL=http://localhost:3000/v1
```

#### Proxy na Nuvem
```bash
OPENAI_API_KEY=your_proxy_token
OPENAI_BASE_URL=https://seu-proxy.com/v1
```

#### OpenAI Oficial (sem proxy)
```bash
OPENAI_API_KEY=sk-...
OPENAI_BASE_URL=https://api.openai.com/v1
```

## Como Funciona

O Pipecat usa o SDK oficial da OpenAI por baixo, que suporta o parâmetro `base_url`. Quando você configura `OPENAI_BASE_URL`, todas as chamadas são redirecionadas para seu proxy.

### Fluxo:

```
Voice Agent (Pipecat)
    ↓
OpenAI SDK (com base_url customizada)
    ↓
Seu Proxy (http://localhost:3000/v1)
    ↓
OpenAI API (ou seu backend customizado)
```

## Endpoints que o Proxy precisa suportar

Para o agente funcionar completamente, seu proxy deve implementar:

### 1. Chat Completions (LLM)
```
POST /v1/chat/completions
```

Usado pelo Pipecat para gerar respostas do agente.

Exemplo de request:
```json
{
  "model": "gpt-4o-mini",
  "messages": [
    {"role": "system", "content": "You are a helpful assistant."},
    {"role": "user", "content": "Hello!"}
  ]
}
```

### 2. Speech-to-Text (Whisper)
```
POST /v1/audio/transcriptions
```

Usado para transcrever áudio do usuário.

Exemplo de request (multipart/form-data):
```
file: (binary audio data)
model: whisper-1
language: pt
```

### 3. Text-to-Speech
```
POST /v1/audio/speech
```

Usado para gerar fala do agente.

Exemplo de request:
```json
{
  "model": "tts-1",
  "voice": "alloy",
  "input": "Hello, how can I help you?",
  "response_format": "opus"
}
```

## Proxies Compatíveis

### LiteLLM Proxy

[LiteLLM](https://github.com/BerriAI/litellm) é um proxy popular que suporta múltiplos providers:

```bash
# Instalar
pip install litellm[proxy]

# Executar
litellm --model gpt-4o-mini --port 3000
```

Configuração:
```bash
OPENAI_BASE_URL=http://localhost:3000/v1
```

### OpenAI-Compatible Proxies

Qualquer proxy que implemente a API compatível com OpenAI pode ser usado:
- [Portkey](https://portkey.ai/)
- [Helicone](https://www.helicone.ai/)
- [OpenRouter](https://openrouter.ai/)
- Custom proxy

## Exemplo: Proxy Simples em Python

Se quiser criar seu próprio proxy para testes:

```python
from flask import Flask, request, jsonify
import requests

app = Flask(__name__)

OPENAI_API_KEY = "sk-..."
OPENAI_BASE = "https://api.openai.com/v1"

@app.route("/v1/<path:path>", methods=["POST"])
def proxy(path):
    # Log da chamada
    print(f"Request to: /v1/{path}")

    # Forward para OpenAI
    headers = {
        "Authorization": f"Bearer {OPENAI_API_KEY}",
        "Content-Type": request.headers.get("Content-Type", "application/json")
    }

    body = request.get_data()

    response = requests.post(
        f"{OPENAI_BASE}/{path}",
        headers=headers,
        data=body
    )

    return response.json(), response.status_code

# Executar: python proxy.py
if __name__ == "__main__":
    app.run(port=3000)
```

## Testando a Configuração

### 1. Verificar se o proxy está acessível

```bash
curl http://localhost:3000/v1/models
```

### 2. Testar com Python

```python
from openai import OpenAI

client = OpenAI(
    api_key="your_proxy_token",
    base_url="http://localhost:3000/v1"
)

# Testar chat
response = client.chat.completions.create(
    model="gpt-4o-mini",
    messages=[
        {"role": "user", "content": "Hello!"}
    ]
)

print(response.choices[0].message.content)
```

### 3. Executar o Voice Agent

```bash
python main.py
```

Verifique os logs para confirmar que está usando o proxy:
```
INFO: Starting voice agent...
INFO: Using OpenAI base URL: http://localhost:3000/v1
```

## Troubleshooting

### Erro: "Connection refused"

**Problema**: Proxy não está rodando ou URL incorreta

**Solução**:
1. Verifique se o proxy está executando
2. Confirme a URL e porta no .env
3. Teste com curl: `curl http://localhost:3000/v1/models`

### Erro: "Unauthorized" ou "Invalid API Key"

**Problema**: Token/chave incorreta

**Solução**:
1. Verifique OPENAI_API_KEY no .env
2. Confirme que o proxy aceita esse token
3. Teste autenticação: `curl -H "Authorization: Bearer seu_token" http://localhost:3000/v1/models`

### Erro: "Model not found"

**Problema**: Proxy não suporta o modelo especificado

**Solução**:
1. Verifique modelos disponíveis no proxy
2. Configure o modelo no código (voice_agent.py)
3. Use modelo compatível com seu proxy

### Requisições indo para OpenAI direto

**Problema**: OPENAI_BASE_URL não está sendo usado

**Solução**:
1. Certifique-se que o .env está no diretório correto
2. Reinicie a aplicação Python
3. Verifique logs de startup

## Segurança

### Boas Práticas:

1. **Não commitar .env**: Já está no .gitignore
2. **HTTPS em produção**: Use SSL/TLS para proxy em produção
3. **Autenticação**: Implemente auth no proxy
4. **Rate limiting**: Limite requisições por usuário
5. **Logging**: Mantenha logs de auditoria
6. **Secrets**: Use gerenciador de secrets (AWS Secrets, Vault, etc)

## Próximos Passos

Depois de configurar o proxy:

1. Teste cada endpoint individualmente (chat, whisper, tts)
2. Implemente monitoramento e métricas
3. Configure alertas para erros
4. Otimize caching para reduzir custos
5. Adicione fallback para OpenAI em caso de falha do proxy

## Recursos

- [OpenAI API Reference](https://platform.openai.com/docs/api-reference)
- [LiteLLM Proxy](https://docs.litellm.ai/docs/proxy/quick_start)
- [Pipecat Documentation](https://docs.pipecat.ai/)
