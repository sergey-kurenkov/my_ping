package main

import (
	"flag"
	"github.com/sergey-kurenkov/my_ping/internal/udp_ping"
)

// ./udp_ping -testDuration=10s -connect=localhost -isServer=false -rps=1,10
// ./udp_ping -isServer=true
//

func main() {
	var isVerbose bool
	var isServer bool
	var rps string
	var connect string
	var port string
	var testDuration string
	var noDelay bool

	flag.StringVar(&rps, "rps", "1,10,100,1000", "requests per second")
	flag.StringVar(&testDuration, "testDuration", "60s", "test duration")
	flag.BoolVar(&isServer, "isServer", true, "server")
	flag.StringVar(&connect, "connect", "localhost", "connect to server")
	flag.StringVar(&port, "port", "8000", "port")
	flag.BoolVar(&isVerbose, "isVerbose", false, "usage")
	flag.BoolVar(&noDelay, "noDelay", true, "usage")
	flag.Parse()

	if isServer {
		c := udp_ping.NewPingServer(port, noDelay)
		c.Run()
	} else {
		server := udp_ping.NewUDPClient(rps, testDuration, isVerbose, connect, port)
		server.Run()
	}
}
