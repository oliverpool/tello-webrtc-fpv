package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"

	"github.com/pion/webrtc/v3"
	"gobot.io/x/gobot/platforms/dji/tello"
)

type Drone interface {
	VideoTrack() webrtc.TrackLocal
	FlightData() <-chan FlightData
	Forward(int) error
	Clockwise(int) error
	Right(int) error
	Up(int) error
	Flip(tello.FlipType) error
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
		log.Fatal("error", err)
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
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = "localhost:3000"
	}
	fmt.Println("serving on http://" + addr)

	return http.ListenAndServe(addr, nil)
}

func startSession(drone Drone) http.HandlerFunc {
	flightData := &broadcast{}
	go flightData.ForwardFlightData(drone.FlightData())

	return func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("\n[User-Agent]", r.Header.Get("User-Agent"))
		param := r.FormValue("offer")
		offer := webrtc.SessionDescription{}

		err := json.Unmarshal([]byte(param), &offer)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		answer, err := startStreaming(offer, drone.VideoTrack(), flightData, drone)
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

type trackDebugger struct {
	webrtc.TrackLocal
}

func (td trackDebugger) Bind(ctx webrtc.TrackLocalContext) (webrtc.RTPCodecParameters, error) {
	for _, p := range ctx.CodecParameters() {
		fmt.Println(p.PayloadType, p)
	}
	params, err := td.TrackLocal.Bind(ctx)
	fmt.Println("===>", params.PayloadType)
	return params, err
}

func startStreaming(offer webrtc.SessionDescription, videoTrack webrtc.TrackLocal, flightData *broadcast, drone Drone) (*webrtc.SessionDescription, error) {
	peerConnection, err := webrtc.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	})
	if err != nil {
		return nil, err
	}

	rtpSender, err := peerConnection.AddTrack(trackDebugger{videoTrack})
	if err != nil {
		return nil, err
	}

	// Read incoming RTCP packets
	// Before these packets are returned they are processed by interceptors. For things
	// like NACK this needs to be called.
	go func() {
		rtcpBuf := make([]byte, 1500)
		for {
			if _, _, rtcpErr := rtpSender.Read(rtcpBuf); rtcpErr != nil {
				return
			}
		}
	}()

	peerConnection.OnDataChannel(func(d *webrtc.DataChannel) {
		fmt.Printf("New DataChannel %s %d\n", d.Label(), d.ID())

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
			case "flip":
				ft := tello.FlipType(b[0] - '0')
				err := drone.Flip(ft)
				fmt.Println("ft", ft, err)
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
