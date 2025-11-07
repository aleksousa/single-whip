param(
    [string]$RoomId = "test-room",
    [string]$ClientUrl = "http://localhost:8081"
)

Write-Host "Testing WebRTC Listen Client" -ForegroundColor Green
Write-Host "Room ID: $RoomId"
Write-Host "Client URL: $ClientUrl"
Write-Host ""

Write-Host "=== Starting Listen Mode ===" -ForegroundColor Yellow

$body = @{
    room_id = $RoomId
} | ConvertTo-Json

try {
    $response = Invoke-RestMethod -Uri "$ClientUrl/listen" -Method Post -Body $body -ContentType "application/json"
    $response | ConvertTo-Json
} catch {
    Write-Host "Error: $_" -ForegroundColor Red
}

Write-Host ""
Write-Host "Listener connected to room: $RoomId" -ForegroundColor Green
Write-Host "The client will now:"
Write-Host "  1. Connect to the room via WebRTC"
Write-Host "  2. Listen for incoming audio"
Write-Host "  3. Convert audio to text using OpenAI Whisper"
Write-Host "  4. Print the transcribed text"
Write-Host ""
Write-Host "Check the client logs for transcribed text output"
