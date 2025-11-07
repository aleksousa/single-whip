package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/pion/webrtc/v4"
)

func main() {
	// Get room ID from command line argument
	roomID := "123" // Default room ID
	if len(os.Args) > 1 {
		roomID = os.Args[1]
	}

	answerAddr := "127.0.0.1:8080"
	fmt.Printf("Connecting to room: %s\n", roomID)

	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}

	peerConnection, err := webrtc.NewPeerConnection(config)
	if err != nil {
		fmt.Printf("Error on NewPeerConnection: %s\n", err.Error())
		return
	}

	peerConnection.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		fmt.Println("Peer Connection has changed to " + state.String())
	})

	audioTrack, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus}, "audio", "pion")
	if err != nil {
		panic(err)
	}

	rtpSender, err := peerConnection.AddTrack(audioTrack)
	if err != nil {
		panic(err)
	}

	// Read incoming RTCP packets
	go func() {
		rtcpBuf := make([]byte, 1500)
		for {
			if _, _, rtcpErr := rtpSender.Read(rtcpBuf); rtcpErr != nil {
				return
			}
		}
	}()

	// Handle incoming audio from the other peer
	peerConnection.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		fmt.Printf("Receiving audio track from peer: %s\n", track.ID())

		// Read RTP packets from remote peer
		go func() {
			for {
				_, _, err := track.ReadRTP()
				if err != nil {
					fmt.Printf("Error reading RTP: %s\n", err.Error())
					return
				}
				// Audio is being received - you could play it, save it, etc.
			}
		}()
	})

	gatherComplete := webrtc.GatheringCompletePromise(peerConnection)
	offer, err := peerConnection.CreateOffer(nil)
	if err != nil {
		fmt.Printf("Error on CreateOffer: %s\n", err.Error())
	}

	if err = peerConnection.SetLocalDescription(offer); err != nil {
		fmt.Printf("Error on SetLocalDescription: %s\n", err.Error())
	}
	<-gatherComplete

	req, err := http.NewRequest(
		"POST",
		fmt.Sprintf("http://%s/whip?room=%s", answerAddr, roomID),
		bytes.NewBuffer([]byte(offer.SDP)))
	if err != nil {
		panic(err)
	}
	req.Header.Set("Content-Type", "application/sdp")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}

	if err = peerConnection.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer, SDP: string(body),
	}); err != nil {
		panic(err)
	}

	fmt.Println("Client is running and connected to the room. Press Ctrl+C to exit.")
	panic(http.ListenAndServe(":8081", nil))
}
