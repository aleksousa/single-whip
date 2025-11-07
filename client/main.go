package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
)

var (
	serverAddr    = "127.0.0.1:8080" // WHIP server address
	openaiAPIKey  = os.Getenv("OPENAI_API_KEY")
	openaiBaseURL = getEnvOrDefault("OPENAI_BASE_URL", "https://api.openai.com/v1")
)

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

type SpeakRequest struct {
	RoomID  string   `json:"room_id"`
	Phrases []string `json:"phrases"`
}

type OpenAITTSRequest struct {
	Model          string  `json:"model"`
	Input          string  `json:"input"`
	Voice          string  `json:"voice"`
	ResponseFormat string  `json:"response_format,omitempty"`
	Speed          float64 `json:"speed,omitempty"`
}

func main() {
	if openaiAPIKey == "" {
		fmt.Println("Error: OPENAI_API_KEY environment variable not set")
		os.Exit(1)
	}

	fmt.Printf("Client API starting on :8081\n")
	fmt.Printf("OpenAI Base URL: %s\n", openaiBaseURL)

	http.HandleFunc("/speak", handleSpeak)
	http.HandleFunc("/", handleRoot)

	if err := http.ListenAndServe(":8081", nil); err != nil {
		panic(err)
	}
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"service": "WebRTC TTS Client",
		"status":  "running",
	})
}

func handleSpeak(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req SpeakRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	if req.RoomID == "" {
		http.Error(w, "room_id is required", http.StatusBadRequest)
		return
	}

	if len(req.Phrases) == 0 {
		http.Error(w, "phrases array cannot be empty", http.StatusBadRequest)
		return
	}

	fmt.Printf("Received speak request for room: %s with %d phrases\n", req.RoomID, len(req.Phrases))

	// Process asynchronously
	go processSpeakRequest(req)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"room_id": req.RoomID,
		"message": fmt.Sprintf("Processing %d phrases", len(req.Phrases)),
	})
}

func processSpeakRequest(req SpeakRequest) {
	fmt.Printf("Starting WebRTC connection to room: %s\n", req.RoomID)

	// Create peer connection
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}

	peerConnection, err := webrtc.NewPeerConnection(config)
	if err != nil {
		fmt.Printf("Error creating peer connection: %v\n", err)
		return
	}
	defer peerConnection.Close()

	peerConnection.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		fmt.Printf("Peer Connection State: %s\n", state.String())
	})

	// Create audio track
	audioTrack, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus},
		"audio",
		"tts-client",
	)
	if err != nil {
		fmt.Printf("Error creating audio track: %v\n", err)
		return
	}

	rtpSender, err := peerConnection.AddTrack(audioTrack)
	if err != nil {
		fmt.Printf("Error adding track: %v\n", err)
		return
	}

	// Read RTCP packets
	go func() {
		rtcpBuf := make([]byte, 1500)
		for {
			if _, _, rtcpErr := rtpSender.Read(rtcpBuf); rtcpErr != nil {
				return
			}
		}
	}()

	// Create and send offer
	gatherComplete := webrtc.GatheringCompletePromise(peerConnection)
	offer, err := peerConnection.CreateOffer(nil)
	if err != nil {
		fmt.Printf("Error creating offer: %v\n", err)
		return
	}

	if err = peerConnection.SetLocalDescription(offer); err != nil {
		fmt.Printf("Error setting local description: %v\n", err)
		return
	}

	<-gatherComplete

	// Send WHIP request
	whipURL := fmt.Sprintf("http://%s/whip?room=%s", serverAddr, req.RoomID)
	httpReq, err := http.NewRequest("POST", whipURL, bytes.NewBuffer([]byte(offer.SDP)))
	if err != nil {
		fmt.Printf("Error creating WHIP request: %v\n", err)
		return
	}
	httpReq.Header.Set("Content-Type", "application/sdp")

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		fmt.Printf("Error sending WHIP request: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error reading WHIP response: %v\n", err)
		return
	}

	if err = peerConnection.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  string(body),
	}); err != nil {
		fmt.Printf("Error setting remote description: %v\n", err)
		return
	}

	fmt.Println("WebRTC connection established")

	// Wait for connection to be fully established
	time.Sleep(2 * time.Second)

	// Process each phrase
	for i, phrase := range req.Phrases {
		fmt.Printf("[%d/%d] Converting to speech: %s\n", i+1, len(req.Phrases), phrase)

		audioData, err := textToSpeech(phrase)
		if err != nil {
			fmt.Printf("Error converting text to speech: %v\n", err)
			continue
		}

		fmt.Printf("[%d/%d] Sending audio...\n", i+1, len(req.Phrases))
		if err := sendAudioToTrack(audioTrack, audioData); err != nil {
			fmt.Printf("Error sending audio: %v\n", err)
			continue
		}

		fmt.Printf("[%d/%d] Audio sent successfully\n", i+1, len(req.Phrases))

		// Wait 15 seconds before next phrase (except for the last one)
		if i < len(req.Phrases)-1 {
			fmt.Println("Waiting 15 seconds...")
			time.Sleep(15 * time.Second)
		}
	}

	// Send final message
	fmt.Println("Sending final message...")
	finalAudio, err := textToSpeech("Isso é tudo pessoal")
	if err != nil {
		fmt.Printf("Error creating final message: %v\n", err)
	} else {
		time.Sleep(2 * time.Second)
		if err := sendAudioToTrack(audioTrack, finalAudio); err != nil {
			fmt.Printf("Error sending final audio: %v\n", err)
		}
	}

	// Wait for final audio to finish playing
	time.Sleep(3 * time.Second)

	fmt.Println("Closing connection...")
}

func textToSpeech(text string) ([]byte, error) {
	ttsURL := fmt.Sprintf("%s/audio/speech", openaiBaseURL)

	reqBody := OpenAITTSRequest{
		Model:          "tts-1",
		Input:          text,
		Voice:          "alloy",
		ResponseFormat: "opus",
		Speed:          1.0,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("error marshaling request: %w", err)
	}

	req, err := http.NewRequest("POST", ttsURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+openaiAPIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("TTS API error (status %d): %s", resp.StatusCode, string(body))
	}

	audioData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %w", err)
	}

	return audioData, nil
}

func sendAudioToTrack(track *webrtc.TrackLocalStaticSample, audioData []byte) error {
	// For Opus format, we need to send the raw audio in chunks
	// Opus frames are typically 20ms at 48kHz
	const frameDuration = 20 * time.Millisecond

	// Send audio data as a sample
	sample := media.Sample{
		Data:     audioData,
		Duration: time.Duration(len(audioData)/960) * frameDuration, // Approximate duration
	}

	if err := track.WriteSample(sample); err != nil {
		return fmt.Errorf("error writing sample: %w", err)
	}

	// Give time for the audio to be transmitted
	time.Sleep(sample.Duration + time.Second)

	return nil
}
