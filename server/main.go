package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/pion/webrtc/v4"
)

var (
	peerConnectionConfiguration = webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}
)

// Room represents a connection between two peers
type Room struct {
	ID    string
	PeerA *Peer
	PeerB *Peer
	mutex sync.Mutex
}

// Peer represents a single connected client
type Peer struct {
	PeerConnection *webrtc.PeerConnection
	AudioTrack     *webrtc.TrackLocalStaticRTP
}

// RoomManager manages all active rooms
type RoomManager struct {
	rooms map[string]*Room
	mutex sync.RWMutex
}

var roomManager = &RoomManager{
	rooms: make(map[string]*Room),
}

func main() {
	http.HandleFunc("/whip", whipHandler)

	fmt.Println("Server started on :8080")
	panic(http.ListenAndServe(":8080", nil))
}

func whipHandler(res http.ResponseWriter, req *http.Request) {
	res.Header().Add("Access-Control-Allow-Origin", "*")
	res.Header().Add("Access-Control-Allow-Methods", "POST")
	res.Header().Add("Access-Control-Allow-Headers", "*")
	res.Header().Add("Access-Control-Allow-Headers", "Authorization")

	if req.Method == http.MethodOptions {
		return
	}

	// Get room ID from query parameter
	roomID := req.URL.Query().Get("room")
	if roomID == "" {
		http.Error(res, "room parameter is required", http.StatusBadRequest)
		return
	}

	fmt.Printf("Client connecting to room: %s\n", roomID)

	offer, err := io.ReadAll(req.Body)
	if err != nil {
		panic(err)
	}
	// Create media engine with Opus codec
	mediaEngine := &webrtc.MediaEngine{}
	if err = mediaEngine.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2, SDPFmtpLine: "", RTCPFeedback: nil,
		},
		PayloadType: 97,
	}, webrtc.RTPCodecTypeAudio); err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}
	api := webrtc.NewAPI(webrtc.WithMediaEngine(mediaEngine))

	// Create peer connection
	peerConnection, err := api.NewPeerConnection(peerConnectionConfiguration)
	if err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}

	// Create audio track for outgoing audio
	audioTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus},
		"audio",
		"pion",
	)
	if err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}

	// Add track to peer connection
	rtpSender, err := peerConnection.AddTrack(audioTrack)
	if err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
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

	// Create peer object
	peer := &Peer{
		PeerConnection: peerConnection,
		AudioTrack:     audioTrack,
	}

	// Get or create room
	room := roomManager.getOrCreateRoom(roomID)

	// Try to add peer to room
	otherPeer := room.addPeer(peer)

	if otherPeer != nil {
		// We have two peers, connect them
		fmt.Printf("Pairing peers in room %s\n", roomID)
		connectPeers(peer, otherPeer)
		connectPeers(otherPeer, peer)
	} else {
		fmt.Printf("Peer waiting in room %s\n", roomID)
	}

	// Handle connection state changes
	peerConnection.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		fmt.Printf("Peer Connection State has changed: %s (Room: %s)\n", state.String(), roomID)

		if state == webrtc.PeerConnectionStateFailed || state == webrtc.PeerConnectionStateClosed {
			room.removePeer(peer)
		}
	})

	// Send answer back to client
	writeAnswer(res, peerConnection, offer, "/whip")
}

// getOrCreateRoom gets an existing room or creates a new one
func (rm *RoomManager) getOrCreateRoom(roomID string) *Room {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	room, exists := rm.rooms[roomID]
	if !exists {
		room = &Room{
			ID: roomID,
		}
		rm.rooms[roomID] = room
		fmt.Printf("Created new room: %s\n", roomID)
	}
	return room
}

// addPeer adds a peer to the room and returns the other peer if one is already waiting
func (r *Room) addPeer(peer *Peer) *Peer {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.PeerA == nil {
		r.PeerA = peer
		return nil // No other peer yet
	} else if r.PeerB == nil {
		r.PeerB = peer
		return r.PeerA // Return the first peer
	}

	// Room is full - this shouldn't happen with proper client logic
	return nil
}

// removePeer removes a peer from the room
func (r *Room) removePeer(peer *Peer) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.PeerA == peer {
		r.PeerA = nil
		fmt.Printf("Peer A left room %s\n", r.ID)
	} else if r.PeerB == peer {
		r.PeerB = nil
		fmt.Printf("Peer B left room %s\n", r.ID)
	}
}

// connectPeers sets up audio relay from source to destination
func connectPeers(source *Peer, destination *Peer) {
	source.PeerConnection.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		fmt.Printf("Connecting audio relay: %s -> destination\n", track.ID())

		go func() {
			for {
				_, _, err := receiver.ReadRTCP()
				if err != nil {
					if errors.Is(err, io.EOF) {
						return
					}
					fmt.Printf("RTCP read error: %s\n", err.Error())
					return
				}
			}
		}()

		go func() {
			for {
				pkt, _, err := track.ReadRTP()
				if err != nil {
					if errors.Is(err, io.EOF) {
						fmt.Println("Track ended")
						return
					}
					fmt.Printf("RTP read error: %s\n", err.Error())
					return
				}

				pkt.Header.Extensions = nil
				pkt.Header.Extension = false

				if track.Kind() == webrtc.RTPCodecTypeAudio {
					if err = destination.AudioTrack.WriteRTP(pkt); err != nil {
						fmt.Printf("Error relaying audio: %s\n", err.Error())
						return
					}
				}
			}
		}()
	})
}

func writeAnswer(res http.ResponseWriter, peerConnection *webrtc.PeerConnection, offer []byte, path string) {
	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		fmt.Printf("ICE Connection State has changed: %s\n", connectionState.String())

		if connectionState == webrtc.ICEConnectionStateFailed {
			_ = peerConnection.Close()
		}
	})

	if err := peerConnection.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer, SDP: string(offer),
	}); err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}

	gatherComplete := webrtc.GatheringCompletePromise(peerConnection)
	answer, err := peerConnection.CreateAnswer(nil)
	if err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}

	if err = peerConnection.SetLocalDescription(answer); err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}
	<-gatherComplete

	res.Header().Add("Location", path)
	res.WriteHeader(http.StatusCreated)

	_, err = fmt.Fprint(res, peerConnection.LocalDescription().SDP)
	if err != nil {
		fmt.Printf("Error writing answer: %s\n", err.Error())
	}
}
