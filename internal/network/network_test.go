package network

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"gfgame/internal/engine"
)

// ---------- helpers ----------

// freePort returns a TCP address with a random free port.
func freePort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	return addr
}

// waitFor polls until cond returns true or timeout expires.
func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("waitFor timed out")
}

// ---------- TCP tests ----------

func TestTCPConnectAndDisconnect(t *testing.T) {
	tcpAddr := freePort(t)
	nl := New(NetworkConfig{
		TCPAddr:           tcpAddr,
		HeartbeatInterval: 5 * time.Second,
		HeartbeatTimeout:  10 * time.Second,
		MaxConnections:    100,
	})

	var connected, disconnected atomic.Int32
	nl.OnConnect(func(s Session) {
		connected.Add(1)
		if s.Protocol() != engine.ProtocolTCP {
			t.Errorf("expected ProtocolTCP, got %d", s.Protocol())
		}
		if s.ID() == "" {
			t.Error("session ID should not be empty")
		}
	})
	nl.OnDisconnect(func(s Session) {
		disconnected.Add(1)
	})

	if err := nl.Start(); err != nil {
		t.Fatal(err)
	}
	defer nl.Stop()

	// Connect
	conn, err := net.Dial("tcp", tcpAddr)
	if err != nil {
		t.Fatal(err)
	}

	waitFor(t, 2*time.Second, func() bool { return connected.Load() == 1 })

	if nl.ConnectionCount() != 1 {
		t.Errorf("expected 1 connection, got %d", nl.ConnectionCount())
	}

	// Disconnect
	conn.Close()
	waitFor(t, 2*time.Second, func() bool { return disconnected.Load() == 1 })

	waitFor(t, 2*time.Second, func() bool { return nl.ConnectionCount() == 0 })
}

func TestTCPSendReceive(t *testing.T) {
	tcpAddr := freePort(t)
	nl := New(NetworkConfig{
		TCPAddr:           tcpAddr,
		HeartbeatInterval: 5 * time.Second,
		HeartbeatTimeout:  10 * time.Second,
		MaxConnections:    100,
	})

	var receivedData []byte
	var receivedMu sync.Mutex
	var connectedSess Session

	nl.OnConnect(func(s Session) { connectedSess = s })
	nl.OnMessage(func(s Session, data []byte) {
		receivedMu.Lock()
		receivedData = append([]byte{}, data...)
		receivedMu.Unlock()
	})

	if err := nl.Start(); err != nil {
		t.Fatal(err)
	}
	defer nl.Stop()

	conn, err := net.Dial("tcp", tcpAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	waitFor(t, 2*time.Second, func() bool { return connectedSess != nil })

	// Client sends data to server
	msg := []byte("hello server")
	conn.Write(msg)

	waitFor(t, 2*time.Second, func() bool {
		receivedMu.Lock()
		defer receivedMu.Unlock()
		return len(receivedData) > 0
	})

	receivedMu.Lock()
	if string(receivedData) != "hello server" {
		t.Errorf("expected 'hello server', got %q", string(receivedData))
	}
	receivedMu.Unlock()

	// Server sends data to client
	if err := connectedSess.Send([]byte("hello client")); err != nil {
		t.Fatal(err)
	}

	buf := make([]byte, 1024)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if string(buf[:n]) != "hello client" {
		t.Errorf("expected 'hello client', got %q", string(buf[:n]))
	}
}

func TestTCPHeartbeat(t *testing.T) {
	tcpAddr := freePort(t)
	nl := New(NetworkConfig{
		TCPAddr:           tcpAddr,
		HeartbeatInterval: 100 * time.Millisecond,
		HeartbeatTimeout:  500 * time.Millisecond,
		MaxConnections:    100,
	})

	var connected atomic.Int32
	nl.OnConnect(func(s Session) { connected.Add(1) })

	if err := nl.Start(); err != nil {
		t.Fatal(err)
	}
	defer nl.Stop()

	conn, err := net.Dial("tcp", tcpAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	waitFor(t, 2*time.Second, func() bool { return connected.Load() == 1 })

	// Send heartbeat ping (0x00) and expect pong (0x00)
	conn.Write([]byte{0x00})
	buf := make([]byte, 1)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 || buf[0] != 0x00 {
		t.Errorf("expected heartbeat pong 0x00, got %v", buf[:n])
	}
}

func TestTCPDisconnectDetection(t *testing.T) {
	tcpAddr := freePort(t)
	nl := New(NetworkConfig{
		TCPAddr:           tcpAddr,
		HeartbeatInterval: 100 * time.Millisecond,
		HeartbeatTimeout:  300 * time.Millisecond,
		MaxConnections:    100,
	})

	var disconnected atomic.Int32
	disconnectTime := make(chan time.Time, 1)
	nl.OnConnect(func(s Session) {})
	nl.OnDisconnect(func(s Session) {
		disconnected.Add(1)
		disconnectTime <- time.Now()
	})

	if err := nl.Start(); err != nil {
		t.Fatal(err)
	}
	defer nl.Stop()

	conn, err := net.Dial("tcp", tcpAddr)
	if err != nil {
		t.Fatal(err)
	}

	waitFor(t, 2*time.Second, func() bool { return nl.ConnectionCount() == 1 })

	closeTime := time.Now()
	conn.Close()

	// Should detect disconnect within 3 seconds (requirement 1.6)
	select {
	case dt := <-disconnectTime:
		elapsed := dt.Sub(closeTime)
		if elapsed > 3*time.Second {
			t.Errorf("disconnect detection took %v, expected < 3s", elapsed)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("disconnect not detected within 4 seconds")
	}
}

// ---------- WebSocket tests ----------

func TestWSConnectAndDisconnect(t *testing.T) {
	// Use httptest to create a WS server
	nl := New(NetworkConfig{
		HeartbeatInterval: 5 * time.Second,
		HeartbeatTimeout:  10 * time.Second,
		MaxConnections:    100,
	}).(*networkLayer)

	var connected, disconnected atomic.Int32
	nl.onConnect = func(s Session) {
		connected.Add(1)
		if s.Protocol() != engine.ProtocolWebSocket {
			t.Errorf("expected ProtocolWebSocket, got %d", s.Protocol())
		}
		if s.ID() == "" {
			t.Error("session ID should not be empty")
		}
		if s.RemoteAddr() == "" {
			t.Error("remote addr should not be empty")
		}
	}
	nl.onDisconnect = func(s Session) {
		disconnected.Add(1)
	}

	server := httptest.NewServer(http.HandlerFunc(nl.handleWS))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}

	waitFor(t, 2*time.Second, func() bool { return connected.Load() == 1 })

	if nl.ConnectionCount() != 1 {
		t.Errorf("expected 1 connection, got %d", nl.ConnectionCount())
	}

	conn.Close()
	waitFor(t, 2*time.Second, func() bool { return disconnected.Load() == 1 })
}

func TestWSSendReceive(t *testing.T) {
	nl := New(NetworkConfig{
		HeartbeatInterval: 5 * time.Second,
		HeartbeatTimeout:  10 * time.Second,
		MaxConnections:    100,
	}).(*networkLayer)

	var receivedData []byte
	var receivedMu sync.Mutex
	var connectedSess Session

	nl.onConnect = func(s Session) { connectedSess = s }
	nl.onMessage = func(s Session, data []byte) {
		receivedMu.Lock()
		receivedData = append([]byte{}, data...)
		receivedMu.Unlock()
	}

	server := httptest.NewServer(http.HandlerFunc(nl.handleWS))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	waitFor(t, 2*time.Second, func() bool { return connectedSess != nil })

	// Client sends to server
	if err := conn.WriteMessage(websocket.BinaryMessage, []byte("ws hello")); err != nil {
		t.Fatal(err)
	}

	waitFor(t, 2*time.Second, func() bool {
		receivedMu.Lock()
		defer receivedMu.Unlock()
		return len(receivedData) > 0
	})

	receivedMu.Lock()
	if string(receivedData) != "ws hello" {
		t.Errorf("expected 'ws hello', got %q", string(receivedData))
	}
	receivedMu.Unlock()

	// Server sends to client
	if err := connectedSess.Send([]byte("ws reply")); err != nil {
		t.Fatal(err)
	}

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "ws reply" {
		t.Errorf("expected 'ws reply', got %q", string(data))
	}
}

func TestWSHeartbeatPingPong(t *testing.T) {
	nl := New(NetworkConfig{
		HeartbeatInterval: 100 * time.Millisecond,
		HeartbeatTimeout:  2 * time.Second,
		MaxConnections:    100,
	}).(*networkLayer)

	var connected atomic.Int32
	nl.onConnect = func(s Session) { connected.Add(1) }

	server := httptest.NewServer(http.HandlerFunc(nl.handleWS))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	dialer := websocket.Dialer{}
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	waitFor(t, 2*time.Second, func() bool { return connected.Load() == 1 })

	// Set up pong handler to verify we receive pings
	pongReceived := make(chan struct{}, 1)
	conn.SetPingHandler(func(appData string) error {
		select {
		case pongReceived <- struct{}{}:
		default:
		}
		// Send pong back
		return conn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(time.Second))
	})

	// Need a read loop to process control frames
	go func() {
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	}()

	// Wait for at least one ping
	select {
	case <-pongReceived:
		// success
	case <-time.After(3 * time.Second):
		t.Fatal("did not receive ping within 3 seconds")
	}
}

func TestWSDisconnectDetection(t *testing.T) {
	nl := New(NetworkConfig{
		HeartbeatInterval: 100 * time.Millisecond,
		HeartbeatTimeout:  500 * time.Millisecond,
		MaxConnections:    100,
	}).(*networkLayer)

	var disconnected atomic.Int32
	disconnectTime := make(chan time.Time, 1)
	nl.onConnect = func(s Session) {}
	nl.onDisconnect = func(s Session) {
		disconnected.Add(1)
		select {
		case disconnectTime <- time.Now():
		default:
		}
	}

	server := httptest.NewServer(http.HandlerFunc(nl.handleWS))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}

	waitFor(t, 2*time.Second, func() bool { return nl.ConnectionCount() == 1 })

	closeTime := time.Now()
	conn.Close()

	select {
	case dt := <-disconnectTime:
		elapsed := dt.Sub(closeTime)
		if elapsed > 3*time.Second {
			t.Errorf("WS disconnect detection took %v, expected < 3s", elapsed)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("WS disconnect not detected within 4 seconds")
	}
}

// ---------- Max connections test ----------

func TestMaxConnections(t *testing.T) {
	tcpAddr := freePort(t)
	maxConns := 3
	nl := New(NetworkConfig{
		TCPAddr:           tcpAddr,
		HeartbeatInterval: 5 * time.Second,
		HeartbeatTimeout:  10 * time.Second,
		MaxConnections:    maxConns,
	})

	var connected atomic.Int32
	nl.OnConnect(func(s Session) { connected.Add(1) })

	if err := nl.Start(); err != nil {
		t.Fatal(err)
	}
	defer nl.Stop()

	// Fill up to max
	conns := make([]net.Conn, 0, maxConns)
	for i := 0; i < maxConns; i++ {
		c, err := net.Dial("tcp", tcpAddr)
		if err != nil {
			t.Fatal(err)
		}
		conns = append(conns, c)
	}
	defer func() {
		for _, c := range conns {
			c.Close()
		}
	}()

	waitFor(t, 2*time.Second, func() bool { return connected.Load() == int32(maxConns) })

	if nl.ConnectionCount() != maxConns {
		t.Errorf("expected %d connections, got %d", maxConns, nl.ConnectionCount())
	}

	// Next connection should be rejected (server closes it immediately)
	extra, err := net.Dial("tcp", tcpAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer extra.Close()

	// The rejected connection should get closed by the server.
	// Try to read — should get EOF or error.
	extra.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1)
	_, readErr := extra.Read(buf)
	if readErr == nil {
		t.Error("expected error reading from rejected connection")
	}

	// Connection count should still be maxConns
	if nl.ConnectionCount() != maxConns {
		t.Errorf("expected %d connections after rejection, got %d", maxConns, nl.ConnectionCount())
	}
}

func TestWSMaxConnections(t *testing.T) {
	maxConns := 2
	nl := New(NetworkConfig{
		HeartbeatInterval: 5 * time.Second,
		HeartbeatTimeout:  10 * time.Second,
		MaxConnections:    maxConns,
	}).(*networkLayer)

	var connected atomic.Int32
	nl.onConnect = func(s Session) { connected.Add(1) }

	server := httptest.NewServer(http.HandlerFunc(nl.handleWS))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	// Fill up to max
	conns := make([]*websocket.Conn, 0, maxConns)
	for i := 0; i < maxConns; i++ {
		c, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatal(err)
		}
		conns = append(conns, c)
	}
	defer func() {
		for _, c := range conns {
			c.Close()
		}
	}()

	waitFor(t, 2*time.Second, func() bool { return connected.Load() == int32(maxConns) })

	// Next WS connection should fail (HTTP 503)
	_, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Error("expected error for WS connection beyond max")
	}
	if resp != nil && resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", resp.StatusCode)
	}
}

// ---------- Unified session abstraction test ----------

func TestUnifiedSessionAbstraction(t *testing.T) {
	// Verify both TCP and WS sessions implement the same Session interface
	// and behave consistently.
	tcpAddr := freePort(t)
	nl := New(NetworkConfig{
		TCPAddr:           tcpAddr,
		HeartbeatInterval: 5 * time.Second,
		HeartbeatTimeout:  10 * time.Second,
		MaxConnections:    100,
	}).(*networkLayer)

	sessions := make(chan Session, 2)
	nl.onConnect = func(s Session) {
		sessions <- s
	}

	server := httptest.NewServer(http.HandlerFunc(nl.handleWS))
	defer server.Close()

	if err := nl.Start(); err != nil {
		t.Fatal(err)
	}
	defer nl.Stop()

	// TCP connection
	tcpConn, err := net.Dial("tcp", tcpAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer tcpConn.Close()

	// WS connection
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	wsConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer wsConn.Close()

	// Collect both sessions
	var tcpSess, wsSess Session
	for i := 0; i < 2; i++ {
		select {
		case s := <-sessions:
			if s.Protocol() == engine.ProtocolTCP {
				tcpSess = s
			} else {
				wsSess = s
			}
		case <-time.After(3 * time.Second):
			t.Fatal("timeout waiting for sessions")
		}
	}

	// Both should have unique IDs
	if tcpSess.ID() == wsSess.ID() {
		t.Error("TCP and WS sessions should have different IDs")
	}

	// Both should have non-empty remote addresses
	if tcpSess.RemoteAddr() == "" || wsSess.RemoteAddr() == "" {
		t.Error("remote addresses should not be empty")
	}

	// Both should support Send
	if err := tcpSess.Send([]byte("test")); err != nil {
		t.Errorf("TCP Send failed: %v", err)
	}
	if err := wsSess.Send([]byte("test")); err != nil {
		t.Errorf("WS Send failed: %v", err)
	}
}

// ---------- Session close and resource cleanup ----------

func TestSessionCloseIdempotent(t *testing.T) {
	tcpAddr := freePort(t)
	nl := New(NetworkConfig{
		TCPAddr:           tcpAddr,
		HeartbeatInterval: 5 * time.Second,
		HeartbeatTimeout:  10 * time.Second,
		MaxConnections:    100,
	})

	var sess Session
	nl.OnConnect(func(s Session) { sess = s })

	if err := nl.Start(); err != nil {
		t.Fatal(err)
	}
	defer nl.Stop()

	conn, err := net.Dial("tcp", tcpAddr)
	if err != nil {
		t.Fatal(err)
	}
	_ = conn

	waitFor(t, 2*time.Second, func() bool { return sess != nil })

	// Close multiple times should not panic
	sess.Close()
	sess.Close()

	// Send after close should return error
	if err := sess.Send([]byte("test")); err == nil {
		t.Error("expected error sending on closed session")
	}
}

func TestStartStopLifecycle(t *testing.T) {
	tcpAddr := freePort(t)
	wsAddr := freePort(t)
	nl := New(NetworkConfig{
		TCPAddr:           tcpAddr,
		WSAddr:            wsAddr,
		HeartbeatInterval: 5 * time.Second,
		HeartbeatTimeout:  10 * time.Second,
		MaxConnections:    100,
	})

	if err := nl.Start(); err != nil {
		t.Fatal(err)
	}

	// Connect a TCP client
	conn, err := net.Dial("tcp", tcpAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	waitFor(t, 2*time.Second, func() bool { return nl.ConnectionCount() == 1 })

	// Stop should close everything cleanly
	if err := nl.Stop(); err != nil {
		t.Fatal(err)
	}

	waitFor(t, 2*time.Second, func() bool { return nl.ConnectionCount() == 0 })
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultNetworkConfig()
	if cfg.MaxConnections != 5000 {
		t.Errorf("expected default MaxConnections 5000, got %d", cfg.MaxConnections)
	}
	if cfg.HeartbeatInterval != 15*time.Second {
		t.Errorf("expected default HeartbeatInterval 15s, got %v", cfg.HeartbeatInterval)
	}
	if cfg.HeartbeatTimeout != 45*time.Second {
		t.Errorf("expected default HeartbeatTimeout 45s, got %v", cfg.HeartbeatTimeout)
	}
}
