# Test script for WebRTC TTS Client (PowerShell)

param(
    [string]$RoomId = "test-room",
    [string]$ClientUrl = "http://localhost:8081"
)

Write-Host "Testing WebRTC TTS Client" -ForegroundColor Green
Write-Host "Room ID: $RoomId"
Write-Host "Client URL: $ClientUrl"
Write-Host ""

# Test 1: Health check
Write-Host "=== Test 1: Health Check ===" -ForegroundColor Yellow
try {
    $response = Invoke-RestMethod -Uri "$ClientUrl/" -Method Get
    $response | ConvertTo-Json
} catch {
    Write-Host "Error: $_" -ForegroundColor Red
}
Write-Host ""

# Test 2: Speak request
Write-Host "=== Test 2: Speak Request ===" -ForegroundColor Yellow

$body = @{
    room_id = $RoomId
    phrases = @(
        "Olá, este é um teste de áudio",
        "Esta é a segunda frase do teste",
        "E esta é a última frase antes do encerramento"
    )
} | ConvertTo-Json

try {
    $response = Invoke-RestMethod -Uri "$ClientUrl/speak" -Method Post -Body $body -ContentType "application/json"
    $response | ConvertTo-Json
} catch {
    Write-Host "Error: $_" -ForegroundColor Red
}

Write-Host ""
Write-Host "Request sent! Check the client logs for progress." -ForegroundColor Green
Write-Host "The client will:"
Write-Host "  1. Connect to room: $RoomId"
Write-Host "  2. Speak 3 phrases with 15 second intervals"
Write-Host "  3. Say 'Isso é tudo pessoal'"
Write-Host "  4. Close connection"
