package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media/oggreader"
	"github.com/pion/webrtc/v4/pkg/media/oggwriter"
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

type SpeakRequest struct {
	RoomID string `json:"room_id"`
}

type TTSRequest struct {
	RoomID string `json:"room_id"`
	Text   string `json:"text"`
}

type ListenRequest struct {
	RoomID string `json:"room_id"`
}

type TranscriptionResponse struct {
	RoomID string `json:"room_id"`
	Text   string `json:"text"`
}

func main() {
	fmt.Printf("Test Client API starting on :8081\n")

	if openAIKey == "" {
		fmt.Println("Warning: OPENAI_API_KEY not set. TTS/STT will not work.")
	}

	http.HandleFunc("/speak", handleSpeak)
	http.HandleFunc("/tts", handleTTS)
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

	fmt.Printf("Received speak request for room: %s\n", req.RoomID)

	go processSpeakRequest(req)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"room_id": req.RoomID,
		"message": "Sending synthetic audio",
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

func processSpeakRequest(req SpeakRequest) {
	fmt.Printf("Starting WebRTC connection to room: %s\n", req.RoomID)

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

	_, err = peerConnection.AddTrack(audioTrack)
	if err != nil {
		fmt.Printf("Error adding track: %v\n", err)
		return
	}

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

	time.Sleep(1 * time.Second)

	fmt.Println("Generating synthetic audio...")
	syntheticAudio := generateSyntheticAudio(3 * time.Second)
	fmt.Printf("Generated OGG file: %d bytes\n", len(syntheticAudio))

	if err := os.WriteFile("synthetic_original.ogg", syntheticAudio, 0644); err != nil {
		fmt.Printf("Warning: Could not save original: %v\n", err)
	} else {
		fmt.Println("Saved synthetic_original.ogg for comparison")
	}

	fmt.Println("Sending synthetic audio...")
	if err := sendAudioToTrack(audioTrack, syntheticAudio); err != nil {
		fmt.Printf("Error sending audio: %v\n", err)
		return
	}

	fmt.Println("Audio sent successfully")
	time.Sleep(2 * time.Second)
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

	peerConnection, err := webrtcAPI.NewPeerConnection(config)
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

	_, err = peerConnection.AddTrack(audioTrack)
	if err != nil {
		fmt.Printf("Error adding track: %v\n", err)
		return
	}

	peerConnection.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		fmt.Printf("Receiving audio track: %s\n", track.ID())

		go func() {
			const packetsPerBatch = 150
			var opusPackets [][]byte
			batchCount := 0
			packetCount := 0

			processBatch := func() {
				if len(opusPackets) == 0 {
					return
				}

				batchCount++
				fmt.Printf("Processing batch %d with %d packets (total received: %d)\n",
					batchCount, len(opusPackets), packetCount)

				oggData := createOggOpusFile(opusPackets)
				fmt.Printf("Created OGG: %d bytes from %d packets\n", len(oggData), len(opusPackets))

				if len(oggData) > 0 {
					filename := fmt.Sprintf("received_audio_%d.ogg", batchCount)
					if err := os.WriteFile(filename, oggData, 0644); err != nil {
						fmt.Printf("Error saving file: %v\n", err)
					} else {
						fmt.Printf("Saved: %s (%d bytes)\n", filename, len(oggData))

						fmt.Println("Sending audio to OpenAI STT...")
						transcription, err := callOpenAISTT(oggData)
						if err != nil {
							fmt.Printf("Error calling STT: %v\n", err)
						} else {
							fmt.Printf("Transcription: %s\n", transcription)
						}
					}
				}

				opusPackets = nil
			}

			for {
				pkt, _, err := track.ReadRTP()
				if err != nil {
					fmt.Printf("Track ended after %d packets\n", packetCount)
					processBatch()
					return
				}

				packetCount++
				if packetCount == 1 {
					fmt.Printf("First packet received: %d bytes payload, PT=%d, TS=%d, Marker=%v\n",
						len(pkt.Payload), pkt.Header.PayloadType, pkt.Header.Timestamp, pkt.Header.Marker)
				}

				packetCopy := make([]byte, len(pkt.Payload))
				copy(packetCopy, pkt.Payload)
				opusPackets = append(opusPackets, packetCopy)

				if len(opusPackets) >= packetsPerBatch || pkt.Header.Marker {
					if pkt.Header.Marker {
						fmt.Printf("End of stream detected (marker bit), processing %d packets\n", len(opusPackets))
					}

					batchCount++
					totalSize := 0
					for _, p := range opusPackets {
						totalSize += len(p)
					}

					fmt.Printf("\n=== SERVER RELAY VALIDATED ===\n")
					fmt.Printf("Batch %d: Received %d packets (%d bytes total)\n", batchCount, len(opusPackets), totalSize)
					fmt.Printf("Server successfully relayed audio stream!\n")
					fmt.Printf("==============================\n\n")

					filename := fmt.Sprintf("received_batch_%d.bin", batchCount)
					var allData []byte
					for _, p := range opusPackets {
						allData = append(allData, p...)
					}
					os.WriteFile(filename, allData, 0644)
					fmt.Printf("Saved raw data to: %s\n\n", filename)

					opusPackets = nil
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

func processTTSRequest(req TTSRequest) {
	fmt.Printf("Starting TTS conversion for: %s\n", req.Text)

	audioData, err := callOpenAITTS(req.Text)
	if err != nil {
		fmt.Printf("Error calling OpenAI TTS: %v\n", err)
		return
	}

	fmt.Printf("TTS audio received: %d bytes\n", len(audioData))

	fmt.Printf("Starting WebRTC connection to room: %s\n", req.RoomID)

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

	_, err = peerConnection.AddTrack(audioTrack)
	if err != nil {
		fmt.Printf("Error adding track: %v\n", err)
		return
	}

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

	time.Sleep(1 * time.Second)

	fmt.Println("Sending TTS audio...")
	if err := sendAudioToTrack(audioTrack, audioData); err != nil {
		fmt.Printf("Error sending audio: %v\n", err)
		return
	}

	fmt.Println("Audio sent successfully")
	time.Sleep(2 * time.Second)
}

func callOpenAITTS(text string) ([]byte, error) {
	if openAIKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY not set")
	}

	reqBody := map[string]interface{}{
		"model":           "tts-1",
		"input":           text,
		"voice":           "alloy",
		"response_format": "opus",
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
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OpenAI API error: %s - %s", resp.Status, string(body))
	}

	audioData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %w", err)
	}

	return audioData, nil
}

func callOpenAISTT(audioData []byte) (string, error) {
	if openAIKey == "" {
		return "", fmt.Errorf("OPENAI_API_KEY not set")
	}

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile("file", "audio.ogg")
	if err != nil {
		return "", fmt.Errorf("error creating form file: %w", err)
	}

	if _, err := part.Write(audioData); err != nil {
		return "", fmt.Errorf("error writing audio data: %w", err)
	}

	if err := writer.WriteField("model", "whisper-1"); err != nil {
		return "", fmt.Errorf("error writing model field: %w", err)
	}

	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("error closing writer: %w", err)
	}

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/audio/transcriptions", &buf)
	if err != nil {
		return "", fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+openAIKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error calling OpenAI: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("OpenAI API error: %s - %s", resp.Status, string(body))
	}

	var result struct {
		Text string `json:"text"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("error decoding response: %w", err)
	}

	return result.Text, nil
}

func generateSyntheticAudio(duration time.Duration) []byte {
	var buf bytes.Buffer
	writer, _ := oggwriter.NewWith(&buf, 48000, 2)

	numPackets := int(duration.Milliseconds() / 20)
	timestamp := uint32(0)

	for i := 0; i < numPackets; i++ {
		silenceFrame := []byte{
			0xFC,
		}

		for j := 0; j < 10; j++ {
			silenceFrame = append(silenceFrame, 0x00)
		}

		writer.WriteRTP(&rtp.Packet{
			Header: rtp.Header{
				Timestamp: timestamp,
			},
			Payload: silenceFrame,
		})

		timestamp += 960
	}

	writer.Close()
	return buf.Bytes()
}

func createOggOpusFile(opusPackets [][]byte) []byte {
	var buf bytes.Buffer

	writer, err := oggwriter.NewWith(&buf, 48000, 2)
	if err != nil {
		fmt.Printf("Error creating OGG writer: %v\n", err)
		return nil
	}

	const samplesPerPacket = 960
	timestamp := uint32(0)
	written := 0

	for i, packet := range opusPackets {
		if len(packet) == 0 {
			fmt.Printf("Warning: Empty packet at index %d\n", i)
			continue
		}

		if err := writer.WriteRTP(&rtp.Packet{
			Header: rtp.Header{
				Timestamp: timestamp,
			},
			Payload: packet,
		}); err != nil {
			fmt.Printf("Error writing packet %d to OGG: %v\n", i, err)
			continue
		}

		written++
		timestamp += samplesPerPacket
	}

	if err := writer.Close(); err != nil {
		fmt.Printf("Error closing OGG writer: %v\n", err)
	}

	fmt.Printf("OGG writer: wrote %d/%d packets successfully\n", written, len(opusPackets))
	return buf.Bytes()
}

func extractOpusPacketsFromOgg(oggData []byte) ([][]byte, error) {
	reader, _, err := oggreader.NewWith(bytes.NewReader(oggData))
	if err != nil {
		return nil, fmt.Errorf("failed to create OGG reader: %w", err)
	}

	var packets [][]byte
	pageCount := 0
	for {
		packet, _, err := reader.ParseNextPage()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("error reading OGG page: %w", err)
		}

		pageCount++
		if len(packet) > 0 {
			if len(packet) >= 8 {
				header := string(packet[:8])
				if header == "OpusHead" || header == "OpusTags" {
					continue
				}
			}

			packetCopy := make([]byte, len(packet))
			copy(packetCopy, packet)
			packets = append(packets, packetCopy)
		}
	}

	fmt.Printf("OGG reader: extracted %d packets from %d pages (%d bytes total)\n",
		len(packets), pageCount, len(oggData))
	return packets, nil
}

func sendAudioToTrack(track *webrtc.TrackLocalStaticRTP, audioData []byte) error {
	const frameDuration = 20 * time.Millisecond
	const samplesPerFrame = 960

	opusPackets, err := extractOpusPacketsFromOgg(audioData)
	if err != nil {
		return fmt.Errorf("error extracting Opus packets: %w", err)
	}

	fmt.Printf("Extracted %d Opus packets from OGG\n", len(opusPackets))
	if len(opusPackets) > 0 {
		fmt.Printf("First packet size: %d bytes, Last packet size: %d bytes\n",
			len(opusPackets[0]), len(opusPackets[len(opusPackets)-1]))
	}

	sequenceNumber := uint16(0)
	timestamp := uint32(0)

	for i, opusPacket := range opusPackets {
		if len(opusPacket) == 0 {
			fmt.Printf("Warning: Empty packet at index %d\n", i)
			continue
		}

		packet := &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				Padding:        false,
				Extension:      false,
				Marker:         i == len(opusPackets)-1,
				PayloadType:    96,
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

	fmt.Printf("Sent %d RTP packets successfully\n", len(opusPackets))
	return nil
}
