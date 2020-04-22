package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"os/signal"

	log "github.com/mgutz/logxi/v1"
	"github.com/pion/webrtc/v2"
	"github.com/pion/webrtc/v2/pkg/media"
)

type Drone interface {
	Frames() <-chan []byte
	FlightData() <-chan FlightData
	Forward(int) error
	Clockwise(int) error
	Right(int) error
	Up(int) error
	Hover()
	TakeOff() error
	Land() error
}

type FlightData struct {
	Height            int
	BatteryPercentage int
}

func main() {
	go panicOnInterrupt()
	if err := run(); err != nil {
		log.Fatal("errror", err)
	}
}

func panicOnInterrupt() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		<-c
		panic("interrupt")
	}()
}

func run() error {
	var drone Drone
	var err error

	if os.Getenv("MOCK") != "" {
		drone, err = NewMock("recorded.h264")
	} else {
		drone, err = NewTello()
	}

	if err != nil {
		return err
	}

	http.Handle("/session", startSession(drone))
	http.Handle("/", http.FileServer(http.Dir(".")))
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	fmt.Println("serving on http://localhost:" + port)

	return http.ListenAndServe(":"+port, nil)
}

func startSession(drone Drone) http.HandlerFunc {
	frames := &broadcast{}
	go frames.Forward(drone.Frames())

	flightData := &broadcast{}
	go flightData.ForwardFlightData(drone.FlightData())

	return func(w http.ResponseWriter, r *http.Request) {
		fmt.Println(r.Header.Get("User-Agent"))
		param := r.FormValue("offer")
		offer := webrtc.SessionDescription{}

		err := json.Unmarshal([]byte(param), &offer)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		answer, err := startStreaming(offer, frames, flightData, drone)
		if err != nil {
			fmt.Println("error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		err = json.NewEncoder(w).Encode(answer)
		if err != nil {
			fmt.Println(err)
		}
	}
}

func startStreaming(offer webrtc.SessionDescription, frames *broadcast, flightData *broadcast, drone Drone) (*webrtc.SessionDescription, error) {
	// We make our own mediaEngine so we can place the sender's codecs in it.  This because we must use the
	// dynamic media type from the sender in our answer. This is not required if we are the offerer
	mediaEngine := webrtc.MediaEngine{}
	err := mediaEngine.PopulateFromSDP(offer)
	if err != nil {
		return nil, err
	}

	for _, videoCodec := range mediaEngine.GetCodecsByKind(webrtc.RTPCodecTypeVideo) {
		fmt.Println(videoCodec)
	}

	// Search for H264 Payload type. If the offer doesn't support H264 exit since
	// since they won't be able to decode anything we send them
	var payloadType uint8
	for _, videoCodec := range mediaEngine.GetCodecsByKind(webrtc.RTPCodecTypeVideo) {
		if videoCodec.Name == "H264" {
			payloadType = videoCodec.PayloadType
			break
		}
	}
	if payloadType == 0 {
		return nil, fmt.Errorf("Remote peer does not support H264")
	}
	if payloadType != 126 {
		fmt.Println("Video might not work with codec", payloadType)
	}

	// Create a new RTCPeerConnection
	api := webrtc.NewAPI(webrtc.WithMediaEngine(mediaEngine))
	peerConnection, err := api.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	})
	if err != nil {
		return nil, err
	}

	// Create a video track
	videoTrack, err := peerConnection.NewTrack(payloadType, rand.Uint32(), "video", "pion")
	if err != nil {
		return nil, err
	}
	if _, err = peerConnection.AddTrack(videoTrack); err != nil {
		return nil, err
	}

	removeListener := func() {}
	// Set the handler for ICE connection state
	// This will notify you when the peer has connected/disconnected
	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		if connectionState == webrtc.ICEConnectionStateConnected {
			fmt.Println("Peer connected")
			ch := make(chan []byte)
			removeListener = frames.Listen(ch)
			for frame := range ch {
				if err = videoTrack.WriteSample(media.Sample{Data: frame, Samples: 1}); err != nil {
					fmt.Println(err)
				}
			}
		} else {
			removeListener()
		}
	})

	peerConnection.OnDataChannel(func(d *webrtc.DataChannel) {
		fmt.Printf("New DataChannel %s %d\n", d.Label(), d.ID())

		d.OnClose(func() {
			removeListener()
		})
		d.OnMessage(func(msg webrtc.DataChannelMessage) {
			b := msg.Data
			factor := 2
			if b[0] == '-' {
				factor = -2
			}

			switch string(b[1:]) {
			case "forwa":
				drone.Forward(20 * factor)
			case "clock":
				drone.Clockwise(25 * factor)
			case "right":
				drone.Right(20 * factor)
			case "up":
				drone.Up(20 * factor)
			case "hover":
				drone.Hover()
			case "takeoff":
				drone.TakeOff()
			case "land":
				drone.Land()
			default:
				fmt.Println("unknown command", string(b), factor)
			}
		})

		d.OnOpen(func() {
			ch := make(chan []byte, 1)
			_ = flightData.Listen(ch)
			for fd := range ch {
				d.Send(fd)
			}
		})
	})

	// Set the remote SessionDescription
	if err = peerConnection.SetRemoteDescription(offer); err != nil {
		return nil, err
	}

	// Create answer
	answer, err := peerConnection.CreateAnswer(nil)
	if err != nil {
		return nil, err
	}

	// Sets the LocalDescription, and starts our UDP listeners
	if err = peerConnection.SetLocalDescription(answer); err != nil {
		return nil, err
	}

	return &answer, nil
}
