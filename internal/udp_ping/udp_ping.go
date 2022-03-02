package udp_ping

import (
	"fmt"
	"github.com/montanaflynn/stats"
	"github.com/sergey-kurenkov/my_ping/internal/helper"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ./udp_ping -connect=localhost -isServer=false -rps=1,10
// ./udp_ping -isServer=true
//
// ./udp_ping -connect=research-06.mo.test.env -isServer=false -rps=1,10 -testDuration=10s
// ./udp_ping -isServer=true

type MyPingUDPclient struct {
	allBPS       string
	testDuration string
	verbose      bool
	connect      string
	port         string
	noDelay      bool

	conn *net.UDPConn

	mt                 sync.Mutex
	singleTestDuration time.Duration
	testBPSs           []float64
	maxTestBPS         float64
	roundtrips         chan time.Duration
	signalingCh        chan struct{}
	waitForTest        sync.WaitGroup
}

func NewUDPClient(bps string, testDuration string, isVerbose bool, connect, port string) *MyPingUDPclient {
	server := MyPingUDPclient{
		allBPS:       bps,
		testDuration: testDuration,
		verbose:      isVerbose,
		port:         port,
		connect:      connect,
	}

	return &server
}

func (s *MyPingUDPclient) Run() {
	notParsedSingleBPSs := strings.Split(s.allBPS, ",")
	for _, notParsedSingleBPS := range notParsedSingleBPSs {
		bps, err := strconv.ParseFloat(notParsedSingleBPS, 64)
		if err != nil {
			fmt.Println(notParsedSingleBPS, err)
			os.Exit(1)
		}

		if s.maxTestBPS < bps {
			s.maxTestBPS = bps
		}

		s.testBPSs = append(s.testBPSs, bps)
	}

	testDuration, err := time.ParseDuration(s.testDuration)
	if err != nil {
		fmt.Println(s.testDuration, err)
		os.Exit(1)
	}

	s.singleTestDuration = testDuration

	s.roundtrips = make(chan time.Duration, 2*int(s.maxTestBPS*float64(testDuration)/float64(time.Second)))

	s.signalingCh = make(chan struct{}, 1)

	socket := fmt.Sprintf("%s:%s", s.connect, s.port)
	fmt.Printf("start client, sending to: %s\n", socket)

	addAddr, err := net.ResolveUDPAddr("udp", socket)
	if err != nil {
		fmt.Printf("MyPingUDPclient.run: ResolveUDPAddr: %v", err)
		os.Exit(1)
	}

	conn, err := net.DialUDP("udp", nil, addAddr)
	if err != nil {
		fmt.Printf("MyPingUDPclient.run: ListenTCP: %v", err)
		os.Exit(1)
	}

	s.conn = conn

	s.waitForTest.Add(1)

	// Handle connections in a new goroutine.
	go s.sendToConnection()
	go s.readFromConnection()

	s.waitForTest.Wait()
}

func (s *MyPingUDPclient) printStat(currentRPS float64, sends int64) {
	numRoundtrips := len(s.roundtrips)

	roundtrips := make(stats.Float64Data, numRoundtrips)
	for i := 0; i < numRoundtrips; i++ {
		roundtrip := <-s.roundtrips
		roundtrips[i] = float64(roundtrip)
	}

	if len(roundtrips) == 0 {
		fmt.Println("no roundtrip data")
		return
	}

	realRPS := float64(len(roundtrips)) / (float64(s.singleTestDuration) / float64(time.Second))

	stat, err := helper.GetStat(roundtrips)
	if err != nil {
		fmt.Printf("MyPingUDPclient.printStat: Percentile: %v", err)
		os.Exit(1)
	}

	fmt.Printf("rps: %v, real rps: %.1f, samples: %v, "+
		"min: %v, max: %v, mean: %v, std deviation: %v, median: %v, percentile99: %v\n",
		currentRPS, realRPS, stat.Size,
		stat.Min.Microseconds(), stat.Max.Microseconds(), stat.Mean.Microseconds(),
		stat.StandardDeviation.Microseconds(), stat.Median.Microseconds(),
		stat.Percentile99.Microseconds(),
	)
}

func (s *MyPingUDPclient) readFromConnection() {
	readBuf := make([]byte, helper.DefaultMessageSize)
	buf := make([]byte, helper.DefaultMessageSize)

	for {
		// Read the incoming connection into the buffer.
		reqLen, err := s.conn.Read(readBuf)
		readTime := time.Now()
		if err != nil {
			fmt.Printf("readToServerConnection: %v", err)
			os.Exit(1)
		}

		copy(buf, readBuf[:reqLen])

		msg := string(buf)
		index := strings.Index(msg, "...")
		timeStr := msg[:index]

		sentTime, err := time.Parse(time.RFC3339Nano, timeStr)
		if err != nil {
			fmt.Printf("readToServerConnection:\nbuf:%v\ntimeStr: %s\nerr: %v\n", msg, timeStr, err)
			os.Exit(1)
		}

		roundtrip := readTime.Sub(sentTime)
		if s.verbose {
			fmt.Printf("readFromServerConnection: sentTime: %v, readTime: %v, duration: %v\n",
				sentTime.Format(time.RFC3339Nano), readTime.Format(time.RFC3339Nano), roundtrip)
		}

		select {
		case s.signalingCh <- struct{}{}:
		default:
		}

		select {
		case s.roundtrips <- roundtrip:
		default:
		}
	}
}

// Handles incoming requests.
func (s *MyPingUDPclient) sendToConnection() {
	defer s.waitForTest.Done()

	// Make a buffer to hold incoming data.
	buf := make([]byte, helper.DefaultMessageSize)

	for _, bps := range s.testBPSs {
		tickDuration, err := time.ParseDuration(fmt.Sprintf("%fs", 1/(bps)))
		if err != nil {
			fmt.Printf("sendToServerConnection: %v\n", err)
			os.Exit(1)
		}

		startTS := time.Now()
		finishTestTS := startTS.Add(s.singleTestDuration + 2*tickDuration)

		currMsg := int64(0)

		for time.Now().Before(finishTestTS) {
			ts := time.Now()
			diffTS := ts.Sub(startTS)
			numberMsgs := int64(bps * (float64(diffTS) / float64(time.Second)))
			if currMsg >= numberMsgs {
				time.Sleep(tickDuration)
				continue
			}

			msg := fmt.Sprintf("%s...", ts.Format(time.RFC3339Nano))

			copy(buf, []byte(msg))
			_, err := s.conn.Write(buf)
			if err != nil {
				fmt.Printf("sendToServerConnection: %v\n", err)
				os.Exit(1)
			}

			currMsg++
		}

		tc := time.NewTicker(time.Second)

	LOOP_SIGNALLING_CHAN:
		for {
			select {
			case <-s.signalingCh:
				tc.Reset(time.Second)
			case <-tc.C:
				break LOOP_SIGNALLING_CHAN
			}
		}

		s.printStat(bps, currMsg)
	}
}

type myPingUDPServer struct {
	port    string
	noDelay bool
}

func NewPingServer(port string, noDelay bool) *myPingUDPServer {
	c := myPingUDPServer{
		port:    port,
		noDelay: noDelay,
	}

	return &c
}

func (c *myPingUDPServer) Run() {
	var conn *net.UDPConn
	var err error

	socket := fmt.Sprintf(":%s", c.port)
	fmt.Printf("start UDP server, socket: %s, noDelay: %v\n", socket, c.noDelay)

	addAddr, err := net.ResolveUDPAddr("udp", socket)
	if err != nil {
		fmt.Printf("myPingUDPServer.run: ResolveUDPAddr: %v", err)
		os.Exit(1)
	}

	for {
		conn, err = net.ListenUDP("udp", addAddr)
		if err != nil {
			fmt.Printf("myPingUDPServer.run: DialTCP:%v\n", err)
			time.Sleep(time.Second)
			continue
		}

		break
	}

	readBuf := make([]byte, helper.DefaultMessageSize)

	for {
		// Read the incoming connection into the buffer.
		reqLen, reqAddr, err := conn.ReadFrom(readBuf)
		if err != nil {
			fmt.Printf("myPingUDPServer.run: Read:%v\n", err)
			os.Exit(1)
		}

		start := 0
		for start < reqLen {
			l, err := conn.WriteTo(readBuf[start:reqLen], reqAddr)
			if err != nil {
				fmt.Printf("myPingUDPServer.run: Write:%v\n", err)
				os.Exit(1)
			}
			start += l
		}
	}
}
