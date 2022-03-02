package tcp_ping

import (
	"fmt"
	"github.com/sergey-kurenkov/my_ping/internal/helper"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type myPingTCPServer struct {
	allBPS       string
	testDuration string
	verbose      bool
	port         string
	noDelay      bool

	conn *net.TCPConn

	mt                 sync.Mutex
	singleTestDuration time.Duration
	testBPSs           []float64
	maxTestBPS         float64
	roundtrips         chan time.Duration
	signalingCh        chan struct{}
	waitForTest        sync.WaitGroup
}

func NewMyPingTCPServer(bps string, testDuration string, isVerbose bool, port string, noDelay bool) *myPingTCPServer {
	server := myPingTCPServer{
		allBPS:       bps,
		testDuration: testDuration,
		verbose:      isVerbose,
		port:         port,
		noDelay:      noDelay,
	}

	return &server
}

func (s *myPingTCPServer) Run() {
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

	socket := fmt.Sprintf(":%s", s.port)
	fmt.Printf("start server, listening on: %s\n", socket)

	addAddr, err := net.ResolveTCPAddr("tcp", socket)
	if err != nil {
		fmt.Printf("myPingTCPServer.run: ResolveTCPAddr: %v", err)
		os.Exit(1)
	}

	l, err := net.ListenTCP("tcp", addAddr)
	if err != nil {
		fmt.Printf("myPingTCPServer.run: ListenTCP: %v", err)
		os.Exit(1)
	}

	// Close the listener when the application closes.
	defer l.Close()

	conn, err := l.AcceptTCP()
	if err != nil {
		fmt.Printf("myPingTCPServer.run: AcceptTCP:%v", err)
		os.Exit(1)
	}

	conn.SetNoDelay(s.noDelay)
	s.conn = conn

	s.waitForTest.Add(1)

	// Handle connections in a new goroutine.
	go s.sendToServerConnection()
	go s.readFromServerConnection()

	s.waitForTest.Wait()
}

func (s *myPingTCPServer) readFromServerConnection() {
	readBuf := make([]byte, helper.DefaultMessageSize)
	receivedBuf := make([]byte, 0, 2*helper.DefaultMessageSize)
	buf := make([]byte, helper.DefaultMessageSize)

	for {
		// Read the incoming connection into the buffer.
		reqLen, err := s.conn.Read(readBuf)
		readTime := time.Now()
		if err != nil {
			fmt.Printf("readToServerConnection: %v", err)
			os.Exit(1)
		}

		receivedBuf = append(receivedBuf, readBuf[:reqLen]...)
		if len(receivedBuf) < helper.DefaultMessageSize {
			continue
		}

		copy(buf, receivedBuf[:helper.DefaultMessageSize])
		receivedBuf = receivedBuf[helper.DefaultMessageSize:]

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
func (s *myPingTCPServer) sendToServerConnection() {
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
			s.conn.Write(buf)

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

		helper.PrintStat(bps, s.roundtrips, s.singleTestDuration)
	}
}

type myPingTCPClient struct {
	connect string
	port    string
	noDelay bool
}

func NewMyPingTCPClient(connect string, port string, noDelay bool) *myPingTCPClient {
	c := myPingTCPClient{
		connect: connect,
		port:    port,
		noDelay: noDelay,
	}

	return &c
}

func (c *myPingTCPClient) Run() {
	var conn *net.TCPConn
	var err error

	socket := fmt.Sprintf("%s:%s", c.connect, c.port)
	fmt.Printf("start client, connect to: %s, noDelay: %v\n", socket, c.noDelay)

	addAddr, err := net.ResolveTCPAddr("tcp", socket)
	if err != nil {
		fmt.Printf("myPingTCPClient.run: ResolveTCPAddr: %v", err)
		os.Exit(1)
	}

	for {
		conn, err = net.DialTCP("tcp", nil, addAddr)
		if err != nil {
			fmt.Printf("myPingTCPClient.run: DialTCP:%v\n", err)
			time.Sleep(time.Second)
			continue
		}

		break
	}

	conn.SetNoDelay(c.noDelay)

	readBuf := make([]byte, helper.DefaultMessageSize)

	for {
		// Read the incoming connection into the buffer.
		reqLen, err := conn.Read(readBuf)
		if err != nil {
			fmt.Printf("myPingTCPClient.run: Read:%v\n", err)
			os.Exit(1)
		}

		start := 0
		for start < reqLen {
			l, err := conn.Write(readBuf[start:reqLen])
			if err != nil {
				fmt.Printf("myPingTCPClient.run: Write:%v\n", err)
				os.Exit(1)
			}
			start += l
		}
	}
}
