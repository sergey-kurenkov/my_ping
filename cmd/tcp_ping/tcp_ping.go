package main

import (
	"flag"
	"github.com/sergey-kurenkov/my_ping/internal/tcp_ping"
)

// ./tcp_ping -connect=localhost -isServer=false
// ./tcp_ping -testDuration=10s -rps=1,10
//
// ./tcp_ping -connect=research-06.mo.test.env -isServer=false
// ./tcp_ping -testDuration=10s -rps=1,10

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
		server := tcp_ping.NewMyPingTCPServer(rps, testDuration, isVerbose, port, noDelay)
		server.Run()
	} else {
		c := tcp_ping.NewMyPingTCPClient(connect, port, noDelay)
		c.Run()
	}
}
