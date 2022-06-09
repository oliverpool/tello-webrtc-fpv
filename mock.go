package main

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"time"

	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media"
	"gobot.io/x/gobot/platforms/dji/tello"
	"gopkg.in/metakeule/loop.v4"
)

type DroneMock struct {
	video      io.Reader
	sleep      time.Duration
	videoTrack *webrtc.TrackLocalStaticSample
}

func NewMock(filepath string) (*DroneMock, error) {
	buf, err := ioutil.ReadFile(filepath)
	if err != nil {
		return nil, err
	}
	videoTrack, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264},
		"video",
		"tello",
	)
	if err != nil {
		return nil, err
	}
	d := &DroneMock{
		video:      loop.New(buf),
		sleep:      40 * time.Millisecond,
		videoTrack: videoTrack,
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

func (d DroneMock) Flip(v tello.FlipType) error {
	fmt.Println("Flip", v)
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

func (d DroneMock) VideoTrack() webrtc.TrackLocal {
	return d.videoTrack
}

// func (d DroneMock) startVideo() error {
// 	scanner := bufio.NewScanner(d.video)
// 	scanner.Split(ScanFrames)

// 	for scanner.Scan() {
// 		select {
// 		case d.frames <- scanner.Bytes():
// 		case <-time.After(d.sleep):
// 		}
// 		time.Sleep(d.sleep)
// 	}
// 	return scanner.Err()
// }

func (d DroneMock) startVideo2() error {
	scanner := bufio.NewScanner(d.video)
	scanner.Split(ScanFrames)

	var buf []byte
	ticker := time.NewTicker(d.sleep)
	defer ticker.Stop()
	for scanner.Scan() {
		b := scanner.Bytes()
		if len(b) < 5 {
			buf = append(buf, b...)
			continue
		}

		// We buffer the bytes, until it looks like a good h264 frame
		//
		// Thanks to https://yumichan.net/video-processing/video-compression/introduction-to-h264-nal-unit/
		nal_unit_type := b[4] & 0b11111
		if (nal_unit_type == 7 || nal_unit_type == 1) && len(buf) > 0 {
			ts := <-ticker.C
			d.videoTrack.WriteSample(media.Sample{
				Data:      buf,
				Duration:  d.sleep,
				Timestamp: ts,
			})

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
