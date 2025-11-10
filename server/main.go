package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/pion/interceptor"
	"github.com/pion/rtp"
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
	webrtcAPI *webrtc.API
)

type Room struct {
	ID    string
	PeerA *Peer
	PeerB *Peer
	mutex sync.Mutex
}

type Peer struct {
	PeerConnection *webrtc.PeerConnection
	AudioTrack     *webrtc.TrackLocalStaticRTP
}

type RoomManager struct {
	rooms map[string]*Room
	mutex sync.RWMutex
}

var roomManager = &RoomManager{
	rooms: make(map[string]*Room),
}

func main() {
	settingEngine := webrtc.SettingEngine{}
	settingEngine.SetNetworkTypes([]webrtc.NetworkType{
		webrtc.NetworkTypeUDP4,
		webrtc.NetworkTypeUDP6,
	})

	mediaEngine := &webrtc.MediaEngine{}
	if err := mediaEngine.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:     webrtc.MimeTypeOpus,
			ClockRate:    48000,
			Channels:     2,
			SDPFmtpLine:  "minptime=10;useinbandfec=1",
			RTCPFeedback: nil,
		},
		PayloadType: 111,
	}, webrtc.RTPCodecTypeAudio); err != nil {
		panic(err)
	}

	interceptorRegistry := &interceptor.Registry{}
	if err := webrtc.RegisterDefaultInterceptors(mediaEngine, interceptorRegistry); err != nil {
		panic(err)
	}

	webrtcAPI = webrtc.NewAPI(
		webrtc.WithMediaEngine(mediaEngine),
		webrtc.WithInterceptorRegistry(interceptorRegistry),
		webrtc.WithSettingEngine(settingEngine),
	)

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

	peerConnection, err := webrtcAPI.NewPeerConnection(peerConnectionConfiguration)
	if err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}

	audioTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypeOpus,
			ClockRate: 48000,
			Channels:  2,
		},
		"audio",
		"pion",
	)
	if err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}

	rtpSender, err := peerConnection.AddTrack(audioTrack)
	if err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}

	go func() {
		rtcpBuf := make([]byte, 4096)
		for {
			if _, _, rtcpErr := rtpSender.Read(rtcpBuf); rtcpErr != nil {
				return
			}
		}
	}()

	peer := &Peer{
		PeerConnection: peerConnection,
		AudioTrack:     audioTrack,
	}

	room := roomManager.getOrCreateRoom(roomID)
	otherPeer := room.addPeer(peer)

	if otherPeer != nil {
		fmt.Printf("Pairing peers in room %s\n", roomID)
		connectPeers(peer, otherPeer)
		connectPeers(otherPeer, peer)
	} else {
		fmt.Printf("Peer waiting in room %s\n", roomID)
	}

	peerConnection.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		fmt.Printf("Connection state: %s (Room: %s)\n", state.String(), roomID)

		if state == webrtc.PeerConnectionStateFailed || state == webrtc.PeerConnectionStateClosed {
			room.removePeer(peer)
		}
	})

	writeAnswer(res, peerConnection, offer, "/whip")
}

func (rm *RoomManager) getOrCreateRoom(roomID string) *Room {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	room, exists := rm.rooms[roomID]
	if !exists {
		room = &Room{
			ID: roomID,
		}
		rm.rooms[roomID] = room
		fmt.Printf("Created room: %s\n", roomID)
	}
	return room
}

func (r *Room) addPeer(peer *Peer) *Peer {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.PeerA == nil {
		r.PeerA = peer
		return nil
	} else if r.PeerB == nil {
		r.PeerB = peer
		return r.PeerA
	}

	return nil
}

func (r *Room) removePeer(peer *Peer) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.PeerA == peer {
		r.PeerA = nil
		fmt.Printf("Peer left room %s\n", r.ID)
	} else if r.PeerB == peer {
		r.PeerB = nil
		fmt.Printf("Peer left room %s\n", r.ID)
	}
}

func connectPeers(source *Peer, destination *Peer) {
	source.PeerConnection.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		fmt.Printf("Audio relay: %s\n", track.ID())

		go func() {
			rtcpBuf := make([]byte, 4096)
			for {
				if _, _, err := receiver.Read(rtcpBuf); err != nil {
					if errors.Is(err, io.EOF) {
						return
					}
					return
				}
			}
		}()

		go func() {
			for {
				pkt, _, err := track.ReadRTP()
				if err != nil {
					if errors.Is(err, io.EOF) {
						return
					}
					fmt.Printf("RTP read error: %s\n", err.Error())
					return
				}

				if track.Kind() != webrtc.RTPCodecTypeAudio {
					continue
				}

				if len(pkt.Payload) == 0 {
					continue
				}

				newPkt := &rtp.Packet{
					Header: rtp.Header{
						Version:        2,
						Padding:        false,
						Extension:      false,
						Marker:         pkt.Header.Marker,
						PayloadType:    pkt.Header.PayloadType,
						SequenceNumber: pkt.Header.SequenceNumber,
						Timestamp:      pkt.Header.Timestamp,
						SSRC:           pkt.Header.SSRC,
					},
					Payload: pkt.Payload,
				}

				if err = destination.AudioTrack.WriteRTP(newPkt); err != nil {
					fmt.Printf("Error relaying audio: %s\n", err.Error())
					return
				}
			}
		}()
	})
}

func writeAnswer(res http.ResponseWriter, peerConnection *webrtc.PeerConnection, offer []byte, path string) {
	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		fmt.Printf("ICE state: %s\n", connectionState.String())

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
