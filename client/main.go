package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media/oggreader"
	"github.com/pion/webrtc/v4/pkg/media/oggwriter"
)

var (
	serverAddr    = "127.0.0.1:8080"
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

type ListenRequest struct {
	RoomID string `json:"room_id"`
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
	http.HandleFunc("/listen", handleListen)
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

	go processSpeakRequest(req)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"room_id": req.RoomID,
		"message": fmt.Sprintf("Processing %d phrases", len(req.Phrases)),
	})
}

func handleListen(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ListenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	if req.RoomID == "" {
		http.Error(w, "room_id is required", http.StatusBadRequest)
		return
	}

	fmt.Printf("Received listen request for room: %s\n", req.RoomID)

	go processListenRequest(req)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"room_id": req.RoomID,
		"message": "Listening for audio",
	})
}

func processSpeakRequest(req SpeakRequest) {
	fmt.Printf("Starting WebRTC connection to room: %s\n", req.RoomID)

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

	audioTrack, err := webrtc.NewTrackLocalStaticRTP(
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

	go func() {
		rtcpBuf := make([]byte, 1500)
		for {
			if _, _, rtcpErr := rtpSender.Read(rtcpBuf); rtcpErr != nil {
				return
			}
		}
	}()

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

	time.Sleep(2 * time.Second)

	for i, phrase := range req.Phrases {
		preview := phrase
		if len(preview) > 20 {
			preview = preview[:20]
		}
		fmt.Printf("[%d/%d] Sending: %s...\n", i+1, len(req.Phrases), preview)

		audioData, err := textToSpeech(phrase)
		if err != nil {
			fmt.Printf("Error converting text to speech: %v\n", err)
			continue
		}

		if err := sendAudioToTrack(audioTrack, audioData); err != nil {
			fmt.Printf("Error sending audio: %v\n", err)
			continue
		}

		if i < len(req.Phrases)-1 {
			time.Sleep(15 * time.Second)
		}
	}

	finalAudio, err := textToSpeech("Isso é tudo pessoal")
	if err != nil {
		fmt.Printf("Error creating final message: %v\n", err)
	} else {
		time.Sleep(2 * time.Second)
		if err := sendAudioToTrack(audioTrack, finalAudio); err != nil {
			fmt.Printf("Error sending final audio: %v\n", err)
		}
	}

	time.Sleep(3 * time.Second)
}

func processListenRequest(req ListenRequest) {
	fmt.Printf("Starting WebRTC connection to room: %s (listen mode)\n", req.RoomID)

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
		fmt.Printf("Listen Peer Connection State: %s\n", state.String())
	})

	audioTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus},
		"audio",
		"listener-client",
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

	go func() {
		rtcpBuf := make([]byte, 1500)
		for {
			if _, _, rtcpErr := rtpSender.Read(rtcpBuf); rtcpErr != nil {
				return
			}
		}
	}()

	var opusPackets [][]byte
	var audioMutex sync.Mutex

	peerConnection.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		fmt.Printf("Receiving audio track: %s\n", track.ID())

		go func() {
			for {
				pkt, _, err := track.ReadRTP()
				if err != nil {
					fmt.Printf("Error reading RTP: %v\n", err)
					return
				}

				audioMutex.Lock()
				packetCopy := make([]byte, len(pkt.Payload))
				copy(packetCopy, pkt.Payload)
				opusPackets = append(opusPackets, packetCopy)
				audioMutex.Unlock()
			}
		}()

		go func() {
			ticker := time.NewTicker(3 * time.Second)
			defer ticker.Stop()

			for range ticker.C {
				audioMutex.Lock()
				if len(opusPackets) > 0 {
					packetsCopy := make([][]byte, len(opusPackets))
					copy(packetsCopy, opusPackets)
					opusPackets = nil
					audioMutex.Unlock()

					oggData := createOggOpusFile(packetsCopy)
					fmt.Printf("Created OGG: %d bytes from %d packets\n", len(oggData), len(packetsCopy))

					text, err := speechToText(oggData)
					if err != nil {
						fmt.Printf("Error converting speech to text: %v\n", err)
					} else {
						fmt.Println(text)
					}
				} else {
					audioMutex.Unlock()
				}
			}
		}()
	})

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

	fmt.Println("WebRTC connection established (listening)")

	select {}
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

func speechToText(audioData []byte) (string, error) {
	sttURL := fmt.Sprintf("%s/audio/transcriptions", openaiBaseURL)

	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)

	fileWriter, err := writer.CreateFormFile("file", "audio.ogg")
	if err != nil {
		return "", fmt.Errorf("error creating form file: %w", err)
	}

	if _, err := fileWriter.Write(audioData); err != nil {
		return "", fmt.Errorf("error writing audio data: %w", err)
	}

	if err := writer.WriteField("model", "whisper-1"); err != nil {
		return "", fmt.Errorf("error writing model field: %w", err)
	}

	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("error closing writer: %w", err)
	}

	req, err := http.NewRequest("POST", sttURL, &requestBody)
	if err != nil {
		return "", fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+openaiAPIKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("STT API error (status %d): %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("error decoding response: %w", err)
	}

	text, ok := result["text"].(string)
	if !ok {
		return "", fmt.Errorf("no text in response")
	}

	return text, nil
}

func createOggOpusFile(opusPackets [][]byte) []byte {
	var buf bytes.Buffer

	writer, err := oggwriter.NewWith(&buf, 48000, 2)
	if err != nil {
		fmt.Printf("Error creating OGG writer: %v\n", err)
		return nil
	}

	for _, packet := range opusPackets {
		if len(packet) == 0 {
			continue
		}

		if err := writer.WriteRTP(&rtp.Packet{
			Header:  rtp.Header{},
			Payload: packet,
		}); err != nil {
			fmt.Printf("Error writing packet to OGG: %v\n", err)
			continue
		}
	}

	if err := writer.Close(); err != nil {
		fmt.Printf("Error closing OGG writer: %v\n", err)
	}

	return buf.Bytes()
}

func extractOpusPacketsFromOgg(oggData []byte) ([][]byte, error) {
	reader, _, err := oggreader.NewWith(bytes.NewReader(oggData))
	if err != nil {
		return nil, fmt.Errorf("failed to create OGG reader: %w", err)
	}

	var packets [][]byte
	for {
		packet, _, err := reader.ParseNextPage()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("error reading OGG page: %w", err)
		}

		packetCopy := make([]byte, len(packet))
		copy(packetCopy, packet)
		packets = append(packets, packetCopy)
	}

	return packets, nil
}

func sendAudioToTrack(track *webrtc.TrackLocalStaticRTP, audioData []byte) error {
	const frameDuration = 20 * time.Millisecond
	const samplesPerFrame = 960

	opusPackets, err := extractOpusPacketsFromOgg(audioData)
	if err != nil {
		return fmt.Errorf("error extracting Opus packets: %w", err)
	}

	fmt.Printf("Extracted %d Opus packets\n", len(opusPackets))

	sequenceNumber := uint16(0)
	timestamp := uint32(0)

	for i, opusPacket := range opusPackets {
		packet := &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				Padding:        false,
				Extension:      false,
				Marker:         i == len(opusPackets)-1,
				PayloadType:    97,
				SequenceNumber: sequenceNumber,
				Timestamp:      timestamp,
				SSRC:           0,
			},
			Payload: opusPacket,
		}

		if err := track.WriteRTP(packet); err != nil {
			return fmt.Errorf("error writing RTP packet %d: %w", i, err)
		}

		sequenceNumber++
		timestamp += samplesPerFrame
		time.Sleep(frameDuration)
	}

	return nil
}
