#!/bin/bash

# Test script for WebRTC TTS Client

ROOM_ID=${1:-"test-room"}
CLIENT_URL=${2:-"http://localhost:8081"}

echo "Testing WebRTC TTS Client"
echo "Room ID: $ROOM_ID"
echo "Client URL: $CLIENT_URL"
echo ""

# Test 1: Health check
echo "=== Test 1: Health Check ==="
curl -s $CLIENT_URL/ | jq .
echo ""

# Test 2: Simple speak request
echo "=== Test 2: Speak Request ==="
curl -X POST $CLIENT_URL/speak \
  -H "Content-Type: application/json" \
  -d "{
    \"room_id\": \"$ROOM_ID\",
    \"phrases\": [
      \"Olá, este é um teste de áudio\",
      \"Esta é a segunda frase do teste\",
      \"E esta é a última frase antes do encerramento\"
    ]
  }" | jq .

echo ""
echo "Request sent! Check the client logs for progress."
echo "The client will:"
echo "  1. Connect to room: $ROOM_ID"
echo "  2. Speak 3 phrases with 15 second intervals"
echo "  3. Say 'Isso é tudo pessoal'"
echo "  4. Close connection"
