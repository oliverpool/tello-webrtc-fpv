package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync/atomic"

	"github.com/pion/webrtc/v3"
	"gobot.io/x/gobot/platforms/dji/tello"
	"golang.org/x/net/websocket"
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

	http.Handle("/", http.FileServer(http.Dir(".")))
	http.Handle("/websocket", websocket.Handler(socketHandler(drone)))

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = "localhost:3000"
	}
	fmt.Println("serving on http://" + addr)

	return http.ListenAndServe(addr, nil)
}

type logger func(...interface{})

func socketHandler(drone Drone) func(*websocket.Conn) {
	flightData := &broadcast{}
	go flightData.ForwardFlightData(drone.FlightData())

	var counter int32

	return func(ws *websocket.Conn) {
		prefix := strconv.Itoa(int(atomic.AddInt32(&counter, 1)))
		fmt.Println()
		log := func(i ...interface{}) {
			fmt.Println(append([]interface{}{prefix}, i...)...)
		}
		log("[User-Agent]", ws.Request().UserAgent())

		// Create a new RTCPeerConnection
		pc, err := webrtc.NewPeerConnection(webrtc.Configuration{
			ICEServers: []webrtc.ICEServer{
				{
					URLs: []string{"stun:stun.l.google.com:19302"},
				},
			},
		})
		if err != nil {
			panic(err)
		}

		_, err = pc.AddTrack(trackDebugger{drone.VideoTrack()})
		if err != nil {
			panic(err)
		}

		pc.OnDataChannel(func(d *webrtc.DataChannel) {
			log("DataChannel", d.Label(), d.ID())

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
					log("unknown command", string(b), factor)
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

		handleICE(log, pc, ws)
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

func handleICE(log logger, pc *webrtc.PeerConnection, ws io.ReadWriter) {
	// Set the handler for ICE connection state
	// This will notify you when the peer has connected/disconnected
	pc.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		log("ICE", connectionState.String())
	})

	// When Pion gathers a new ICE Candidate send it to the client.
	encoder := json.NewEncoder(ws)
	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		if err := encoder.Encode(c.ToJSON()); err != nil {
			log("ERROR", "JSON-Encoder", err)
		}
	})

	decoder := json.NewDecoder(ws)
	for {
		var socketMsg struct {
			webrtc.ICECandidateInit
			webrtc.SessionDescription
		}

		if err := decoder.Decode(&socketMsg); err != nil {
			log("ERROR", "JSON-Decoder", err)
			return
		}

		// Attempt to unmarshal as a SessionDescription. If the SDP field is empty
		// assume it is not one.
		if socketMsg.SDP != "" {
			log("SDP")
			if err := pc.SetRemoteDescription(socketMsg.SessionDescription); err != nil {
				log("ERROR", "SDP", err)
			}

			answer, err := pc.CreateAnswer(nil)
			if err != nil {
				log("ERROR", "SDP", err)
			}

			if err := pc.SetLocalDescription(answer); err != nil {
				log("ERROR", "SDP", err)
			}

			if err := encoder.Encode(answer); err != nil {
				log("ERROR", "SDP", err)
			}
		}

		// Attempt to unmarshal as a ICECandidateInit. If the candidate field is empty
		// assume it is not one.
		if socketMsg.Candidate != "" {
			log("ICE  candidate")
			if err := pc.AddICECandidate(socketMsg.ICECandidateInit); err != nil {
				log("ERROR", "Candidate", err)
			}
		}
	}
}
