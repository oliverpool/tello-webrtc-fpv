package main

import (
	"fmt"
	"time"

	"gobot.io/x/gobot"
	"gobot.io/x/gobot/platforms/dji/tello"
)

type Tello struct {
	*tello.Driver
	frames     chan []byte
	flightdata chan FlightData
}

func NewTello() (Tello, error) {
	drone := tello.NewDriver("8890")

	t := Tello{
		Driver:     drone,
		flightdata: make(chan FlightData),
		frames:     make(chan []byte),
	}

	robot := gobot.NewRobot("tello",
		[]gobot.Connection{},
		[]gobot.Device{drone},
		t.startVideo,
	)

	return t, robot.Start(false)
}

func (t Tello) Frames() <-chan []byte {
	return t.frames
}

func (t Tello) FlightData() <-chan FlightData {
	return t.flightdata
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

	var previousFlightData FlightData
	drone.On(tello.FlightDataEvent, func(data interface{}) {
		allData := data.(*tello.FlightData)
		flightData := FlightData{
			Height:            int(allData.Height),
			BatteryPercentage: int(allData.BatteryPercentage),
		}
		if previousFlightData != flightData {
			t.flightdata <- flightData
			previousFlightData = flightData
		}

	})
}
