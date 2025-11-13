package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
	"github.com/pion/webrtc/v4/pkg/media/oggreader"
)

var (
	serverAddr = "127.0.0.1:8080"
	roomID     = "room123"
)

func main() {
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

	audioTrack, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{
			MimeType: webrtc.MimeTypeOpus,
		},
		"audio",
		"pion",
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

	peerConnection.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		fmt.Printf("Peer Connection State: %s\n", state.String())
		if state == webrtc.PeerConnectionStateConnected {
			go func() {
				file, oggErr := os.Open("debug_audio.ogg")
				if oggErr != nil {
					panic(oggErr)
				}

				ogg, _, err := oggreader.NewWith(file)
				if err != nil {
					fmt.Printf("Error NewWith: %v\n", err)
					return
				}

				var lastGranule uint64
				var oggPageDuration = time.Millisecond * 20

				ticker := time.NewTicker(oggPageDuration)
				for ; true; <-ticker.C {
					pageData, pageHeader, oggErr := ogg.ParseNextPage()
					if errors.Is(oggErr, io.EOF) {
						fmt.Printf("All audio pages parsed and sent")
						break
					}
					if oggErr != nil {
						fmt.Printf("Error ParseNextPage: %v\n", oggErr)
						break
					}
					sampleCount := float64(pageHeader.GranulePosition - lastGranule)
					lastGranule = pageHeader.GranulePosition
					sampleDuration := time.Duration((sampleCount/48000)*1000) * time.Millisecond

					if err = audioTrack.WriteSample(media.Sample{Data: pageData, Duration: sampleDuration}); err != nil {
						fmt.Printf("Error WriteSample: %v\n", err)
						break
					}
				}
			}()
		}
	})

	offer, err := peerConnection.CreateOffer(nil)
	if err != nil {
		fmt.Printf("Error creating offer: %v\n", err)
		return
	}

	gatherComplete := webrtc.GatheringCompletePromise(peerConnection)
	if err = peerConnection.SetLocalDescription(offer); err != nil {
		fmt.Printf("Error setting local description: %v\n", err)
		return
	}
	<-gatherComplete

	whipURL := fmt.Sprintf("http://%s/whip?room=%s", serverAddr, roomID)
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
	gatherComplete = webrtc.GatheringCompletePromise(peerConnection)
	if err = peerConnection.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  string(body),
	}); err != nil {
		<-gatherComplete
		fmt.Printf("Error setting remote description: %v\n", err)
		return
	}
	<-gatherComplete

	select {}
}
