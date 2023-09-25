package main

import (
	"flag"
	"os"
	"os/signal"
	"time"

	"github.com/GaoMjun/ping"
)

func main() {
	srcIP := flag.String("S", "", "source ip")
	flag.Parse()

	host := flag.Arg(0)

	logChan := make(chan string)

	endChan := make(chan os.Signal, 1)

	signal.Notify(endChan, os.Interrupt)

	go func() {
		for s := range logChan {
			print(s)
		}
	}()

	ping, _ := ping.New(*srcIP, host, logChan)

	go ping.Run()

	<-endChan
	ping.Stop()
	time.Sleep(time.Millisecond * 100)
}
