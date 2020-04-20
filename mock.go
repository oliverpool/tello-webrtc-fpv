package main

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"time"

	"gopkg.in/metakeule/loop.v4"
)

type DroneMock struct {
	video  io.Reader
	sleep  time.Duration
	frames chan []byte
}

func NewMock(filepath string) (*DroneMock, error) {
	buf, err := ioutil.ReadFile(filepath)
	if err != nil {
		return nil, err
	}
	d := &DroneMock{
		video: loop.New(buf),
		sleep: 40 * time.Millisecond,
		// sleep:  30 * time.Millisecond,
		frames: make(chan []byte, 1),
	}
	go d.startVideo2()
	return d, nil
}

func (d DroneMock) Forward(v int) error {
	fmt.Println("Forward", v)
	return nil
}
func (d DroneMock) Clockwise(v int) error {
	fmt.Println("Clockwise", v)
	return nil
}
func (d DroneMock) Right(v int) error {
	fmt.Println("Right", v)
	return nil
}
func (d DroneMock) Up(v int) error {
	fmt.Println("Up", v)
	return nil
}
func (d DroneMock) Hover() {
	fmt.Println("Hover")
}
func (d DroneMock) TakeOff() error {
	fmt.Println("TakeOff")
	return nil
}
func (d DroneMock) Land() error {
	fmt.Println("Land")
	return nil
}

func (d DroneMock) FlightData() <-chan FlightData {
	ch := make(chan FlightData)

	go func() {
		for range time.NewTicker(time.Second).C {
			ch <- FlightData{
				BatteryPercentage: rand.Intn(100),
				Height:            rand.Intn(10),
			}
		}
	}()
	return ch
}

func (d DroneMock) Frames() <-chan []byte {
	return d.frames
}
func (d DroneMock) startVideo() error {
	scanner := bufio.NewScanner(d.video)
	scanner.Split(ScanFrames)

	for scanner.Scan() {
		select {
		case d.frames <- scanner.Bytes():
		case <-time.After(d.sleep):
		}
		time.Sleep(d.sleep)
	}
	return scanner.Err()
}
func (d DroneMock) startVideo2() error {
	scanner := bufio.NewScanner(d.video)
	scanner.Split(ScanFrames)

	var buf []byte
	var units int
	for scanner.Scan() {
		b := scanner.Bytes()
		if len(b) < 5 {
			buf = append(buf, b...)
			continue
		}
		nal_unit_type := b[4] & 0b11111
		if nal_unit_type <= 5 {
			units++
		}

		// 5: best for safari ios
		if nal_unit_type == 5 && len(buf) > 0 {
			d.frames <- buf
			time.Sleep(time.Duration(units) * d.sleep)
			units = 0
			buf = append(buf[:0], b...)
		} else {
			buf = append(buf, b...)
		}
	}
	return scanner.Err()
}

// ScanFrames splits the buffer into h264 frames
// Which start with 0x00 00 01 according to https://dsp.stackexchange.com/a/27297
func ScanFrames(data []byte, atEOF bool) (advance int, token []byte, err error) {
	zeros := 0
	for i := 3; i < len(data); i++ {
		if data[i] == 0x00 {
			zeros++
		} else if data[i] == 0x01 && zeros >= 3 {
			//end of frame
			return i - 3, data[:i-3], nil
		} else {
			zeros = 0
		}
	}
	if atEOF && len(data) > 0 {
		// need more data
		return len(data), data, nil
	}
	return 0, nil, nil
}

// To do some tests with a file encoded as to split the frames coming from the drone
// Those frames are not h264 aligned.
// func (d DroneMock) startVideo() error {
// 	var l uint64
// 	var frame []byte
// 	for {
// 		binary.Read(d.video, binary.LittleEndian, &l)
// 		buf := make([]byte, l)
// 		n, _ := d.video.Read(buf)

// 		if len(frame) > 0 && len(buf) > 3 && buf[0] == 0 && buf[1] == 0 && buf[2] == 0 && buf[3] == 1 {
// 			select {
// 			case d.frames <- frame:
// 			case <-time.After(d.sleep):
// 			}
// 			time.Sleep(d.sleep)
// 			frame = buf[:n]
// 		} else {
// 			frame = append(frame, buf[:n]...)
// 		}

// 	}
// }
