package main

import (
	"fmt"
	"time"

	"gobot.io/x/gobot"
	"gobot.io/x/gobot/platforms/dji/tello"
)

type Tello struct {
	*tello.Driver
	frames chan []byte
}

func NewTello() Tello {
	drone := tello.NewDriver("8890")

	t := Tello{
		Driver: drone,
		frames: make(chan []byte),
	}

	robot := gobot.NewRobot("tello",
		[]gobot.Connection{},
		[]gobot.Device{drone},
		t.startVideo,
	)

	robot.Start(false)
	return t
}

func (t Tello) Frames() <-chan []byte {
	return t.frames
}

func (t Tello) startVideo() {
	drone := t.Driver

	drone.On(tello.ConnectedEvent, func(data interface{}) {
		fmt.Println("Connected")
		drone.StartVideo()
		drone.SetVideoEncoderRate(tello.VideoBitRateAuto)
		gobot.Every(100*time.Millisecond, func() {
			drone.StartVideo()
		})
	})

	var buf []byte

	drone.On(tello.VideoFrameEvent, func(data interface{}) {
		b := data.([]byte)
		if len(buf) > 0 && b[0] == 0 && b[1] == 0 && b[2] == 0 && b[3] == 1 {
			t.frames <- buf
			buf = b
		} else {
			buf = append(buf, b...)
		}
	})

	drone.On(tello.FlightDataEvent, func(data interface{}) {
		flightData := data.(*tello.FlightData)
		fmt.Println("Height:", flightData.Height)
		fmt.Println("BatteryPercentage:", flightData.BatteryPercentage)
		fmt.Println("BatteryPercentage:", flightData.DroneBatteryLeft)
		fmt.Println("AirSpeed:", flightData.AirSpeed())
		fmt.Println("GroundSpeed:", flightData.GroundSpeed())
	})
}
