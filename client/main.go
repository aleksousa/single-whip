package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/pion/webrtc/v4"
)

var (
	serverAddr = "127.0.0.1:8080"
	openAIKey  = ""
	webrtcAPI  *webrtc.API
)

func init() {
	mediaEngine := &webrtc.MediaEngine{}
	if err := mediaEngine.RegisterDefaultCodecs(); err != nil {
		panic(err)
	}

	settingEngine := webrtc.SettingEngine{}
	settingEngine.SetReceiveMTU(8192)
	settingEngine.SetSRTPReplayProtectionWindow(1024)

	webrtcAPI = webrtc.NewAPI(
		webrtc.WithMediaEngine(mediaEngine),
		webrtc.WithSettingEngine(settingEngine),
	)
}

type TTSRequest struct {
	RoomID string `json:"room_id"`
	Text   string `json:"text"`
}

func main() {
	fmt.Printf("Test Client API starting on :8082\n")

	if openAIKey == "" {
		fmt.Println("Warning: OPENAI_API_KEY not set. TTS/STT will not work.")
	}

	http.HandleFunc("/tts", handleTTS)

	if err := http.ListenAndServe(":8082", nil); err != nil {
		panic(err)
	}
}

func handleTTS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req TTSRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	if req.RoomID == "" || req.Text == "" {
		http.Error(w, "room_id and text are required", http.StatusBadRequest)
		return
	}

	fmt.Printf("Received TTS request for room: %s, text: %s\n", req.RoomID, req.Text)

	go processTTSRequest(req)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"room_id": req.RoomID,
		"message": "Converting text to speech and sending",
	})
}

func processTTSRequest(req TTSRequest) {
	fmt.Printf("Starting TTS conversion for: %s\n", req.Text)

	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}

	peerConnection, err := webrtcAPI.NewPeerConnection(config)
	if err != nil {
		fmt.Printf("Error creating peer connection: %v\n", err)
		return
	}
	defer peerConnection.Close()

	iceConnectedCtx, iceConnectedCtxCancel := context.WithCancel(context.Background())

	peerConnection.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		fmt.Printf("Peer Connection State: %s\n", state.String())
	})

	audioTrack, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{
			MimeType: webrtc.MimeTypeOpus,
		}, "audio", "pion",
	)
	if err != nil {
		fmt.Printf("Error creating audio track: %v\n", err)
		return
	}

	rtpSender, audioTrackErr := peerConnection.AddTrack(audioTrack)
	if audioTrackErr != nil {
		fmt.Printf("Error adding track: %v\n", audioTrackErr)
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

	go func() {
		audioStream, err := callOpenAITTS(req.Text)
		if err != nil {
			fmt.Printf("Error callOpenAITTS: %v\n", err)
			return
		}
		defer audioStream.Close()

		audio, err := io.ReadAll(audioStream)
		if err != nil {
			fmt.Printf("Error reading audio: %v\n", err)
			return
		}

		<-iceConnectedCtx.Done()

		err = sendPCMToWebRTCTrack(audioTrack, audio)
		if err != nil {
			fmt.Printf("Error sending PCM to WebRTC track: %v\n", err)
		}
	}()

	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		fmt.Printf("Connection State has changed %s \n", connectionState.String())
		if connectionState == webrtc.ICEConnectionStateConnected {
			iceConnectedCtxCancel()
		}
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
	select {}
}

func callOpenAITTS(text string) (io.ReadCloser, error) {
	if openAIKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY not set")
	}

	reqBody := map[string]interface{}{
		"model":           "tts-1",
		"input":           text,
		"voice":           "alloy",
		"response_format": "pcm",
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("error marshaling request: %w", err)
	}

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/audio/speech", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+openAIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error calling OpenAI: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("OpenAI API error: %s - %s", resp.Status, string(body))
	}

	return resp.Body, nil
}
