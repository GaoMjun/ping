package ping

import (
	"fmt"
	"log"
	"math"
	"math/rand"
	"net"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

type Pinger struct {
	timeout, interval                           time.Duration
	logChan                                     chan string
	srcIP, host                                 string
	laddr, raddr                                *net.IPAddr
	rtts                                        []time.Duration
	send, recv                                  int
	loss                                        float64
	minRtt, maxRtt, avgRtt, stdDevRtt, stddevm2 time.Duration
	ipConn                                      *net.IPConn
	running                                     bool
}

func New(srcIP, host string, logChan chan string) (self *Pinger) {
	self = &Pinger{
		timeout:  2 * time.Second,
		interval: 1 * time.Second,
		host:     host,
		logChan:  logChan,
	}
	if srcIP == "" {
		self.srcIP = "0.0.0.0"
	} else {
		self.srcIP = srcIP
	}

	var err error

	if self.laddr, err = net.ResolveIPAddr("ip", self.srcIP); err != nil {
		panic(err)
	}

	var ips []net.IP
	if ips, err = net.LookupIP(host); err != nil {
		panic(err)
	}

	if len(ips) <= 0 {
		err = fmt.Errorf("host %s not found", host)
		panic(err)
	}

	self.raddr = &net.IPAddr{IP: ips[0]}
	return
}

func (self *Pinger) Run(d time.Duration) {
	var (
		err error
	)
	defer func() {
		if err != nil {
			log.Println(err)
			return
		}

		if self.logChan != nil {
			self.logChan <- fmt.Sprintf("--- %s ping statistics ---\n", self.host)
			self.logChan <- fmt.Sprintf("%d packets transmitted, %d packets received, %.1f%% packet loss\n", self.send, self.recv, self.loss)
			self.logChan <- fmt.Sprintf("round-trip min/avg/max/stddev = %.03f/%.03f/%.03f/%.03f ms\n",
				self.minRtt.Seconds()*1000, self.avgRtt.Seconds()*1000, self.maxRtt.Seconds()*1000, self.stdDevRtt.Seconds()*1000)
		}
	}()

	if self.ipConn, err = net.DialIP("ip4:icmp", self.laddr, self.raddr); err != nil {
		return
	}
	defer self.ipConn.Close()

	self.running = true

	var (
		buf  = gopacket.NewSerializeBuffer()
		opts = gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
		ping = &layers.ICMPv4{
			TypeCode: layers.CreateICMPv4TypeCode(layers.ICMPv4TypeEchoRequest, layers.ICMPv4CodeNet),
			Id:       uint16(rand.Uint32()),
			Seq:      0,
			BaseLayer: layers.BaseLayer{
				Payload: make([]byte, 56),
			},
		}
		start = time.Now()
	)

	if self.logChan != nil {
		self.logChan <- fmt.Sprintf("PING %s (%s): %d data bytes\n", self.host, self.raddr, len(ping.Payload))
	}

	for {
		if !self.running {
			return
		}

		if d > 0 && time.Since(start) > d {
			return
		}

		ping.Seq += 1

		gopacket.SerializeLayers(buf, opts, ping)
		wbuf := buf.Bytes()

		self.ipConn.SetWriteDeadline(time.Now().Add(self.timeout))

		now := time.Now()

		if _, err = self.ipConn.Write(wbuf); err != nil {
			err = nil
			return
		}
		self.send += 1

		startRead := time.Now()
		rbuf := make([]byte, 2048)
		n := 0

		for {
			self.ipConn.SetReadDeadline(time.Now().Add(self.timeout))

			if n, err = self.ipConn.Read(rbuf); err != nil {
				// log.Println(err)
				if self.logChan != nil {
					self.logChan <- fmt.Sprintf("Request timeout for icmp_seq %d\n", ping.Seq)
				}
				break
			}

			rtt := time.Since(now)

			packet := gopacket.NewPacket(rbuf[:n], layers.LayerTypeIPv4, gopacket.NoCopy)
			pong := packet.Layer(layers.LayerTypeICMPv4).(*layers.ICMPv4)

			if pong.TypeCode.Type() == layers.ICMPv4TypeEchoReply &&
				pong.Id == ping.Id &&
				pong.Seq == ping.Seq {

				self.recv += 1
				self.rtts = append(self.rtts, rtt)

				if self.logChan != nil {
					self.logChan <- fmt.Sprintf("%d bytes from %s: icmp_seq=%d time=%.03f ms\n", len(pong.Payload), self.raddr.String(), pong.Seq, rtt.Seconds()*1000)
				}
				self.Stats(rtt)
				break
			}

			if time.Since(startRead) > self.timeout {
				if self.logChan != nil {
					self.logChan <- fmt.Sprintf("Request timeout for icmp_seq %d\n", ping.Seq)
				}
				break
			}
		}

		time.Sleep(self.interval)
	}
}

func (self *Pinger) Stop() {
	self.running = false
}

func (self *Pinger) Stats(rtt time.Duration) {
	if self.send <= 0 {
		self.loss = 0
	} else {
		self.loss = (1 - float64(self.recv)/float64(self.send)) * 100
	}

	// log.Println(self.loss, self.recv, self.send)

	if self.recv == 1 || rtt < self.minRtt {
		self.minRtt = rtt
	}

	if rtt > self.maxRtt {
		self.maxRtt = rtt
	}

	count := time.Duration(self.recv)
	delta := rtt - self.avgRtt
	self.avgRtt += delta / count
	delta2 := rtt - self.avgRtt
	self.stddevm2 += delta * delta2

	self.stdDevRtt = time.Duration(math.Sqrt(float64(self.stddevm2 / count)))
}

func (self *Pinger) GetStats() (float64, time.Duration, time.Duration, time.Duration, time.Duration, time.Duration) {
	return self.loss, self.minRtt, self.maxRtt, self.avgRtt, self.stdDevRtt, self.stddevm2
}
