package main

import (
	"encoding/json"
	"fmt"
	"sync"
)

// Slow readers will be ignored
type broadcast struct {
	lock      sync.Mutex
	listeners []chan<- []byte
}

func (b *broadcast) Listen(ch chan<- []byte) func() {
	b.lock.Lock()
	defer b.lock.Unlock()
	b.listeners = append(b.listeners, ch)
	return func() {
		b.lock.Lock()
		defer b.lock.Unlock()
		old := b.listeners
		b.listeners = make([]chan<- []byte, len(b.listeners)-1)
		for _, l := range old {
			if l == ch {
				continue
			}
			b.listeners = append(b.listeners, l)
		}
	}
}

func (b *broadcast) Forward(ch <-chan []byte) {
	for buf := range ch {
		b.forward(buf)
	}
}

func (b *broadcast) forward(buf []byte) {
	b.lock.Lock()
	defer b.lock.Unlock()
	for _, l := range b.listeners {
		select {
		case l <- buf: // slow readers are skipped
		default:
		}
	}
}

func (b *broadcast) ForwardFlightData(ch <-chan FlightData) {
	for fd := range ch {
		buf, err := json.Marshal(fd)
		if err != nil {
			fmt.Println(err)
			continue
		}
		b.forward(buf)
	}
}
