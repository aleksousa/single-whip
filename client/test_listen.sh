#!/bin/bash

ROOM_ID=${1:-"test-room"}
CLIENT_URL=${2:-"http://localhost:8081"}

echo "Testing WebRTC Listen Client"
echo "Room ID: $ROOM_ID"
echo "Client URL: $CLIENT_URL"
echo ""

echo "=== Starting Listen Mode ==="
curl -X POST $CLIENT_URL/listen \
  -H "Content-Type: application/json" \
  -d "{
    \"room_id\": \"$ROOM_ID\"
  }" | jq .

echo ""
echo "Listener connected to room: $ROOM_ID"
echo "The client will now:"
echo "  1. Connect to the room via WebRTC"
echo "  2. Listen for incoming audio"
echo "  3. Convert audio to text using OpenAI Whisper"
echo "  4. Print the transcribed text"
echo ""
echo "Check the client logs for transcribed text output"
