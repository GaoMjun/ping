package ping

import (
	"testing"
	"time"
)

func TestPing(t *testing.T) {
	var (
		// err error

		logChan = make(chan string)
	)

	go func() {
		for s := range logChan {
			print(s)
		}
	}()

	p := New("", "8.8.8.8", logChan)

	p.Run(time.Second * 10)
}
