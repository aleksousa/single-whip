package main

import (
	"bytes"
	"encoding/binary"
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
		fmt.Printf("[%d/%d] Sending text: %s...\n", i+1, len(req.Phrases), preview)

		audioData, err := textToSpeech(phrase)
		if err != nil {
			fmt.Printf("Error converting text to speech: %v\n", err)
			continue
		}

		if err := sendAudioToTrack(audioTrack, audioData); err != nil {
			fmt.Printf("Error sending audio: %v\n", err)
			continue
		}

		fmt.Printf("[%d/%d] Audio sent successfully\n", i+1, len(req.Phrases))

		if i < len(req.Phrases)-1 {
			fmt.Println("Waiting 15 seconds...")
			time.Sleep(15 * time.Second)
		}
	}

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

	time.Sleep(3 * time.Second)

	fmt.Println("Closing connection...")
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

				// Store each Opus packet separately
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

					// Create OGG Opus file
					oggData := createOggOpusFile(packetsCopy)
					fmt.Printf("Created OGG file with %d bytes from %d packets\n", len(oggData), len(packetsCopy))

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

// createOggOpusFile creates an OGG container with Opus packets
func createOggOpusFile(opusPackets [][]byte) []byte {
	var buf bytes.Buffer

	const sampleRate = 48000
	const channels = 2
	serialNumber := uint32(time.Now().Unix())
	var granulePosition uint64 = 0
	pageSequence := uint32(0)

	// Helper function to write OGG page
	writeOggPage := func(data []byte, headerType byte, granule uint64, isFirst bool) {
		// OGG page header
		buf.WriteString("OggS")                               // Capture pattern
		buf.WriteByte(0)                                      // Version
		buf.WriteByte(headerType)                             // Header type
		binary.Write(&buf, binary.LittleEndian, granule)      // Granule position
		binary.Write(&buf, binary.LittleEndian, serialNumber) // Serial number
		binary.Write(&buf, binary.LittleEndian, pageSequence) // Page sequence number
		pageSequence++
		checksumPos := buf.Len()
		binary.Write(&buf, binary.LittleEndian, uint32(0)) // Checksum (placeholder)
		buf.WriteByte(1)                                   // Number of segments
		buf.WriteByte(byte(len(data)))                     // Segment table
		buf.Write(data)                                    // Page data

		// Calculate CRC32 checksum
		pageData := buf.Bytes()[checksumPos-22:]
		checksum := crc32Ogg(pageData)
		binary.LittleEndian.PutUint32(buf.Bytes()[checksumPos:], checksum)
	}

	// OpusHead header
	opusHead := make([]byte, 19)
	copy(opusHead[0:8], "OpusHead")
	opusHead[8] = 1                                                  // Version
	opusHead[9] = byte(channels)                                     // Channel count
	binary.LittleEndian.PutUint16(opusHead[10:], 0)                  // Pre-skip
	binary.LittleEndian.PutUint32(opusHead[12:], uint32(sampleRate)) // Sample rate
	binary.LittleEndian.PutUint16(opusHead[16:], 0)                  // Output gain
	opusHead[18] = 0                                                 // Channel mapping family

	writeOggPage(opusHead, 0x02, 0, true) // BOS (Beginning of Stream)

	// OpusTags header
	opusTags := bytes.NewBuffer(nil)
	opusTags.WriteString("OpusTags")
	vendorString := "Go WebRTC Client"
	binary.Write(opusTags, binary.LittleEndian, uint32(len(vendorString)))
	opusTags.WriteString(vendorString)
	binary.Write(opusTags, binary.LittleEndian, uint32(0)) // User comment list length

	writeOggPage(opusTags.Bytes(), 0x00, 0, false)

	// Write Opus data packets
	samplesPerPacket := uint64(960) // 20ms at 48kHz
	for i, packet := range opusPackets {
		if len(packet) == 0 || len(packet) > 255 {
			continue
		}

		granulePosition += samplesPerPacket
		headerType := byte(0x00)
		if i == len(opusPackets)-1 {
			headerType = 0x04 // EOS (End of Stream)
		}

		writeOggPage(packet, headerType, granulePosition, false)
	}

	return buf.Bytes()
}

// crc32Ogg calculates CRC32 for OGG pages
func crc32Ogg(data []byte) uint32 {
	var crc uint32 = 0
	for _, b := range data {
		crc = (crc << 8) ^ crc32Table[byte(crc>>24)^b]
	}
	return crc
}

var crc32Table = [256]uint32{
	0x00000000, 0x04c11db7, 0x09823b6e, 0x0d4326d9,
	0x130476dc, 0x17c56b6b, 0x1a864db2, 0x1e475005,
	0x2608edb8, 0x22c9f00f, 0x2f8ad6d6, 0x2b4bcb61,
	0x350c9b64, 0x31cd86d3, 0x3c8ea00a, 0x384fbdbd,
	0x4c11db70, 0x48d0c6c7, 0x4593e01e, 0x4152fda9,
	0x5f15adac, 0x5bd4b01b, 0x569796c2, 0x52568b75,
	0x6a1936c8, 0x6ed82b7f, 0x639b0da6, 0x675a1011,
	0x791d4014, 0x7ddc5da3, 0x709f7b7a, 0x745e66cd,
	0x9823b6e0, 0x9ce2ab57, 0x91a18d8e, 0x95609039,
	0x8b27c03c, 0x8fe6dd8b, 0x82a5fb52, 0x8664e6e5,
	0xbe2b5b58, 0xbaea46ef, 0xb7a96036, 0xb3687d81,
	0xad2f2d84, 0xa9ee3033, 0xa4ad16ea, 0xa06c0b5d,
	0xd4326d90, 0xd0f37027, 0xddb056fe, 0xd9714b49,
	0xc7361b4c, 0xc3f706fb, 0xceb42022, 0xca753d95,
	0xf23a8028, 0xf6fb9d9f, 0xfbb8bb46, 0xff79a6f1,
	0xe13ef6f4, 0xe5ffeb43, 0xe8bccd9a, 0xec7dd02d,
	0x34867077, 0x30476dc0, 0x3d044b19, 0x39c556ae,
	0x278206ab, 0x23431b1c, 0x2e003dc5, 0x2ac12072,
	0x128e9dcf, 0x164f8078, 0x1b0ca6a1, 0x1fcdbb16,
	0x018aeb13, 0x054bf6a4, 0x0808d07d, 0x0cc9cdca,
	0x7897ab07, 0x7c56b6b0, 0x71159069, 0x75d48dde,
	0x6b93dddb, 0x6f52c06c, 0x6211e6b5, 0x66d0fb02,
	0x5e9f46bf, 0x5a5e5b08, 0x571d7dd1, 0x53dc6066,
	0x4d9b3063, 0x495a2dd4, 0x44190b0d, 0x40d816ba,
	0xaca5c697, 0xa864db20, 0xa527fdf9, 0xa1e6e04e,
	0xbfa1b04b, 0xbb60adfc, 0xb6238b25, 0xb2e29692,
	0x8aad2b2f, 0x8e6c3698, 0x832f1041, 0x87ee0df6,
	0x99a95df3, 0x9d684044, 0x902b669d, 0x94ea7b2a,
	0xe0b41de7, 0xe4750050, 0xe9362689, 0xedf73b3e,
	0xf3b06b3b, 0xf771768c, 0xfa325055, 0xfef34de2,
	0xc6bcf05f, 0xc27dede8, 0xcf3ecb31, 0xcbffd686,
	0xd5b88683, 0xd1799b34, 0xdc3abded, 0xd8fba05a,
	0x690ce0ee, 0x6dcdfd59, 0x608edb80, 0x644fc637,
	0x7a089632, 0x7ec98b85, 0x738aad5c, 0x774bb0eb,
	0x4f040d56, 0x4bc510e1, 0x46863638, 0x42472b8f,
	0x5c007b8a, 0x58c1663d, 0x558240e4, 0x51435d53,
	0x251d3b9e, 0x21dc2629, 0x2c9f00f0, 0x285e1d47,
	0x36194d42, 0x32d850f5, 0x3f9b762c, 0x3b5a6b9b,
	0x0315d626, 0x07d4cb91, 0x0a97ed48, 0x0e56f0ff,
	0x1011a0fa, 0x14d0bd4d, 0x19939b94, 0x1d528623,
	0xf12f560e, 0xf5ee4bb9, 0xf8ad6d60, 0xfc6c70d7,
	0xe22b20d2, 0xe6ea3d65, 0xeba91bbc, 0xef68060b,
	0xd727bbb6, 0xd3e6a601, 0xdea580d8, 0xda649d6f,
	0xc423cd6a, 0xc0e2d0dd, 0xcda1f604, 0xc960ebb3,
	0xbd3e8d7e, 0xb9ff90c9, 0xb4bcb610, 0xb07daba7,
	0xae3afba2, 0xaafbe615, 0xa7b8c0cc, 0xa379dd7b,
	0x9b3660c6, 0x9ff77d71, 0x92b45ba8, 0x9675461f,
	0x8832161a, 0x8cf30bad, 0x81b02d74, 0x857130c3,
	0x5d8a9099, 0x594b8d2e, 0x5408abf7, 0x50c9b640,
	0x4e8ee645, 0x4a4ffbf2, 0x470cdd2b, 0x43cdc09c,
	0x7b827d21, 0x7f436096, 0x7200464f, 0x76c15bf8,
	0x68860bfd, 0x6c47164a, 0x61043093, 0x65c52d24,
	0x119b4be9, 0x155a565e, 0x18197087, 0x1cd86d30,
	0x029f3d35, 0x065e2082, 0x0b1d065b, 0x0fdc1bec,
	0x3793a651, 0x3352bbe6, 0x3e119d3f, 0x3ad08088,
	0x2497d08d, 0x2056cd3a, 0x2d15ebe3, 0x29d4f654,
	0xc5a92679, 0xc1683bce, 0xcc2b1d17, 0xc8ea00a0,
	0xd6ad50a5, 0xd26c4d12, 0xdf2f6bcb, 0xdbee767c,
	0xe3a1cbc1, 0xe760d676, 0xea23f0af, 0xeee2ed18,
	0xf0a5bd1d, 0xf464a0aa, 0xf9278673, 0xfde69bc4,
	0x89b8fd09, 0x8d79e0be, 0x803ac667, 0x84fbdbd0,
	0x9abc8bd5, 0x9e7d9662, 0x933eb0bb, 0x97ffad0c,
	0xafb010b1, 0xab710d06, 0xa6322bdf, 0xa2f33668,
	0xbcb4666d, 0xb8757bda, 0xb5365d03, 0xb1f740b4,
}

// fragmentOpusAudio divides Opus audio data into RTP-sized chunks
func fragmentOpusAudio(audioData []byte) [][]byte {
	const maxPayloadSize = 960 // Typical Opus frame size for 20ms at 48kHz

	var fragments [][]byte
	for i := 0; i < len(audioData); i += maxPayloadSize {
		end := i + maxPayloadSize
		if end > len(audioData) {
			end = len(audioData)
		}
		fragment := make([]byte, end-i)
		copy(fragment, audioData[i:end])
		fragments = append(fragments, fragment)
	}

	return fragments
}

func sendAudioToTrack(track *webrtc.TrackLocalStaticRTP, audioData []byte) error {
	const frameDuration = 20 * time.Millisecond
	const sampleRate = 48000    // Opus sample rate
	const samplesPerFrame = 960 // 20ms at 48kHz

	// Fragment the audio data
	fragments := fragmentOpusAudio(audioData)

	fmt.Printf("Sending %d RTP packets\n", len(fragments))

	// Send each fragment as an RTP packet
	sequenceNumber := uint16(0)
	timestamp := uint32(0)

	for i, fragment := range fragments {
		packet := &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				Padding:        false,
				Extension:      false,
				Marker:         i == len(fragments)-1, // Mark last packet
				PayloadType:    97,                    // Opus payload type
				SequenceNumber: sequenceNumber,
				Timestamp:      timestamp,
				SSRC:           0, // Will be set by track
			},
			Payload: fragment,
		}

		if err := track.WriteRTP(packet); err != nil {
			return fmt.Errorf("error writing RTP packet %d: %w", i, err)
		}

		sequenceNumber++
		timestamp += samplesPerFrame

		// Small delay between packets to simulate real-time streaming
		time.Sleep(frameDuration)
	}

	fmt.Printf("Sent all %d packets successfully\n", len(fragments))

	return nil
}
