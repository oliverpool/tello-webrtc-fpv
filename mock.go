package main

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
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
		video:  loop.New(buf),
		sleep:  30 * time.Millisecond,
		frames: make(chan []byte, 1),
	}
	go d.startVideo()
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

// ScanFrames splits the buffer into h264 frames
// Which start with 0x00 00 01 according to https://dsp.stackexchange.com/a/27297
func ScanFrames(data []byte, atEOF bool) (advance int, token []byte, err error) {
	zeros := 0
	for i := 3; i < len(data); i++ {
		if data[i] == 0x00 {
			zeros++
		} else if data[i] == 0x01 && zeros >= 2 {
			//end of frame
			return i - 2, data[:i-2], nil
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
