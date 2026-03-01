// Package network provides the unified network layer for the MMRPG game engine,
// supporting both TCP and WebSocket transports behind a common Session interface.
package network

import (
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"gfgame/internal/engine"
)

// ---------- Session interface & implementations ----------

// Session is the unified session abstraction that hides TCP/WebSocket differences.
type Session interface {
	ID() string
	Send(msg []byte) error
	Close() error
	RemoteAddr() string
	Protocol() engine.TransportProtocol
}

// ---------- TCP Session ----------

type tcpSession struct {
	id       string
	conn     net.Conn
	protocol engine.TransportProtocol
	closed   atomic.Bool
	mu       sync.Mutex
}

func newTCPSession(conn net.Conn) *tcpSession {
	return &tcpSession{
		id:       uuid.New().String(),
		conn:     conn,
		protocol: engine.ProtocolTCP,
	}
}

func (s *tcpSession) ID() string                          { return s.id }
func (s *tcpSession) Protocol() engine.TransportProtocol  { return s.protocol }
func (s *tcpSession) RemoteAddr() string                  { return s.conn.RemoteAddr().String() }

func (s *tcpSession) Send(msg []byte) error {
	if s.closed.Load() {
		return errors.New("session closed")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.conn.Write(msg)
	return err
}

func (s *tcpSession) Close() error {
	if s.closed.Swap(true) {
		return nil // already closed
	}
	return s.conn.Close()
}

// ---------- WebSocket Session ----------

type wsSession struct {
	id       string
	conn     *websocket.Conn
	protocol engine.TransportProtocol
	closed   atomic.Bool
	mu       sync.Mutex
}

func newWSSession(conn *websocket.Conn) *wsSession {
	return &wsSession{
		id:       uuid.New().String(),
		conn:     conn,
		protocol: engine.ProtocolWebSocket,
	}
}

func (s *wsSession) ID() string                          { return s.id }
func (s *wsSession) Protocol() engine.TransportProtocol  { return s.protocol }
func (s *wsSession) RemoteAddr() string                  { return s.conn.RemoteAddr().String() }

func (s *wsSession) Send(msg []byte) error {
	if s.closed.Load() {
		return errors.New("session closed")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.conn.WriteMessage(websocket.BinaryMessage, msg)
}

func (s *wsSession) Close() error {
	if s.closed.Swap(true) {
		return nil
	}
	return s.conn.Close()
}

// ---------- NetworkLayer interface ----------

// NetworkLayer is the top-level network abstraction.
type NetworkLayer interface {
	Start() error
	Stop() error
	OnConnect(handler func(session Session))
	OnDisconnect(handler func(session Session))
	OnMessage(handler func(session Session, data []byte))
	ConnectionCount() int
}

// ---------- NetworkConfig ----------

// NetworkConfig holds all network-related configuration.
type NetworkConfig struct {
	TCPAddr           string
	WSAddr            string
	WSSEnabled        bool
	TLSCertFile       string
	TLSKeyFile        string
	HeartbeatInterval time.Duration
	HeartbeatTimeout  time.Duration
	MaxConnections    int // default 5000
}

// DefaultNetworkConfig returns a NetworkConfig with sensible defaults.
func DefaultNetworkConfig() NetworkConfig {
	return NetworkConfig{
		TCPAddr:           ":9000",
		WSAddr:            ":9001",
		HeartbeatInterval: 15 * time.Second,
		HeartbeatTimeout:  45 * time.Second,
		MaxConnections:    5000,
	}
}

// ---------- networkLayer implementation ----------

type networkLayer struct {
	config NetworkConfig

	// callbacks
	onConnect    func(Session)
	onDisconnect func(Session)
	onMessage    func(Session, []byte)

	// session tracking
	sessions sync.Map // map[string]Session
	connCount atomic.Int64

	// TCP
	tcpListener net.Listener

	// WebSocket
	wsHTTPServer *http.Server
	wsUpgrader   websocket.Upgrader

	// lifecycle
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// New creates a new NetworkLayer with the given config.
func New(config NetworkConfig) NetworkLayer {
	if config.MaxConnections <= 0 {
		config.MaxConnections = 5000
	}
	if config.HeartbeatInterval <= 0 {
		config.HeartbeatInterval = 15 * time.Second
	}
	if config.HeartbeatTimeout <= 0 {
		config.HeartbeatTimeout = 45 * time.Second
	}

	nl := &networkLayer{
		config: config,
		stopCh: make(chan struct{}),
		wsUpgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
	return nl
}

func (nl *networkLayer) OnConnect(handler func(Session))              { nl.onConnect = handler }
func (nl *networkLayer) OnDisconnect(handler func(Session))           { nl.onDisconnect = handler }
func (nl *networkLayer) OnMessage(handler func(Session, []byte))      { nl.onMessage = handler }
func (nl *networkLayer) ConnectionCount() int                         { return int(nl.connCount.Load()) }

// Start begins listening on both TCP and WebSocket endpoints.
func (nl *networkLayer) Start() error {
	// Start TCP listener
	if nl.config.TCPAddr != "" {
		ln, err := net.Listen("tcp", nl.config.TCPAddr)
		if err != nil {
			return fmt.Errorf("tcp listen: %w", err)
		}
		nl.tcpListener = ln
		nl.wg.Add(1)
		go nl.acceptTCP()
	}

	// Start WebSocket listener
	if nl.config.WSAddr != "" {
		mux := http.NewServeMux()
		mux.HandleFunc("/ws", nl.handleWS)

		nl.wsHTTPServer = &http.Server{
			Addr:    nl.config.WSAddr,
			Handler: mux,
		}

		nl.wg.Add(1)
		go nl.serveWS()
	}

	return nil
}

// Stop gracefully shuts down all listeners and closes all sessions.
func (nl *networkLayer) Stop() error {
	close(nl.stopCh)

	if nl.tcpListener != nil {
		nl.tcpListener.Close()
	}
	if nl.wsHTTPServer != nil {
		nl.wsHTTPServer.Close()
	}

	// Close all active sessions
	nl.sessions.Range(func(key, value any) bool {
		if sess, ok := value.(Session); ok {
			sess.Close()
		}
		return true
	})

	nl.wg.Wait()
	return nil
}

// ---------- TCP handling ----------

func (nl *networkLayer) acceptTCP() {
	defer nl.wg.Done()
	for {
		conn, err := nl.tcpListener.Accept()
		if err != nil {
			select {
			case <-nl.stopCh:
				return
			default:
				log.Printf("[network] tcp accept error: %v", err)
				continue
			}
		}

		// Check max connections
		if int(nl.connCount.Load()) >= nl.config.MaxConnections {
			log.Printf("[network] max connections reached (%d), rejecting TCP connection from %s",
				nl.config.MaxConnections, conn.RemoteAddr())
			conn.Close()
			continue
		}

		sess := newTCPSession(conn)
		nl.addSession(sess)

		nl.wg.Add(1)
		go nl.handleTCPSession(sess)
	}
}

func (nl *networkLayer) handleTCPSession(sess *tcpSession) {
	defer nl.wg.Done()
	defer nl.removeSession(sess)

	if nl.onConnect != nil {
		nl.onConnect(sess)
	}

	buf := make([]byte, 4096)
	for {
		// Set read deadline for heartbeat/disconnect detection
		deadline := nl.config.HeartbeatTimeout
		if deadline <= 0 {
			deadline = 45 * time.Second
		}
		sess.conn.SetReadDeadline(time.Now().Add(deadline))

		n, err := sess.conn.Read(buf)
		if err != nil {
			if !sess.closed.Load() {
				select {
				case <-nl.stopCh:
				default:
					log.Printf("[network] tcp session %s read error: %v", sess.id, err)
				}
			}
			return
		}

		if n > 0 {
			// Check for TCP heartbeat (single byte 0x00)
			if n == 1 && buf[0] == 0x00 {
				// Heartbeat ping — respond with pong
				sess.Send([]byte{0x00})
				continue
			}

			data := make([]byte, n)
			copy(data, buf[:n])
			if nl.onMessage != nil {
				nl.onMessage(sess, data)
			}
		}
	}
}

// ---------- WebSocket handling ----------

func (nl *networkLayer) serveWS() {
	defer nl.wg.Done()

	var err error
	if nl.config.WSSEnabled && nl.config.TLSCertFile != "" && nl.config.TLSKeyFile != "" {
		tlsCert, loadErr := tls.LoadX509KeyPair(nl.config.TLSCertFile, nl.config.TLSKeyFile)
		if loadErr != nil {
			log.Printf("[network] failed to load TLS cert: %v", loadErr)
			return
		}
		nl.wsHTTPServer.TLSConfig = &tls.Config{
			Certificates: []tls.Certificate{tlsCert},
		}
		err = nl.wsHTTPServer.ListenAndServeTLS("", "")
	} else {
		err = nl.wsHTTPServer.ListenAndServe()
	}

	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Printf("[network] ws server error: %v", err)
	}
}

func (nl *networkLayer) handleWS(w http.ResponseWriter, r *http.Request) {
	// Check max connections
	if int(nl.connCount.Load()) >= nl.config.MaxConnections {
		log.Printf("[network] max connections reached (%d), rejecting WS connection from %s",
			nl.config.MaxConnections, r.RemoteAddr)
		http.Error(w, "max connections reached", http.StatusServiceUnavailable)
		return
	}

	conn, err := nl.wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[network] ws upgrade error: %v", err)
		return
	}

	sess := newWSSession(conn)
	nl.addSession(sess)

	// Configure WebSocket heartbeat (ping/pong)
	conn.SetReadDeadline(time.Now().Add(nl.config.HeartbeatTimeout))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(nl.config.HeartbeatTimeout))
		return nil
	})

	if nl.onConnect != nil {
		nl.onConnect(sess)
	}

	// Start ping ticker goroutine
	nl.wg.Add(1)
	go nl.wsPingLoop(sess)

	// Read loop
	nl.wsReadLoop(sess)
}

func (nl *networkLayer) wsPingLoop(sess *wsSession) {
	defer nl.wg.Done()
	ticker := time.NewTicker(nl.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-nl.stopCh:
			return
		case <-ticker.C:
			if sess.closed.Load() {
				return
			}
			sess.mu.Lock()
			err := sess.conn.WriteControl(
				websocket.PingMessage, nil,
				time.Now().Add(5*time.Second),
			)
			sess.mu.Unlock()
			if err != nil {
				if !sess.closed.Load() {
					log.Printf("[network] ws session %s ping error: %v", sess.id, err)
				}
				sess.Close()
				return
			}
		}
	}
}

func (nl *networkLayer) wsReadLoop(sess *wsSession) {
	defer nl.removeSession(sess)

	for {
		_, data, err := sess.conn.ReadMessage()
		if err != nil {
			if !sess.closed.Load() {
				select {
				case <-nl.stopCh:
				default:
					log.Printf("[network] ws session %s read error: %v", sess.id, err)
				}
			}
			return
		}

		if nl.onMessage != nil {
			nl.onMessage(sess, data)
		}
	}
}

// ---------- Session management ----------

func (nl *networkLayer) addSession(sess Session) {
	nl.sessions.Store(sess.ID(), sess)
	nl.connCount.Add(1)
}

func (nl *networkLayer) removeSession(sess Session) {
	if _, loaded := nl.sessions.LoadAndDelete(sess.ID()); !loaded {
		return // already removed
	}
	nl.connCount.Add(-1)
	sess.Close()

	if nl.onDisconnect != nil {
		nl.onDisconnect(sess)
	}
}
