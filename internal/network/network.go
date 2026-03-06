// Package network 为 MMRPG 游戏引擎提供统一的网络层，
// 通过公共 Session 接口支持 TCP 和 WebSocket 两种传输协议。
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

	"gogame/internal/engine"
)

// ---------- Session 接口及实现 ----------

// Session 是统一的会话抽象，屏蔽了 TCP/WebSocket 的差异。
type Session interface {
	ID() string
	Send(msg []byte) error
	Close() error
	RemoteAddr() string
	Protocol() engine.TransportProtocol
}

// ---------- TCP 会话 ----------

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
		return nil // 已关闭
	}
	return s.conn.Close()
}

// ---------- WebSocket 会话 ----------

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

// ---------- NetworkLayer 接口 ----------

// NetworkLayer 是顶层网络抽象。
type NetworkLayer interface {
	Start() error
	Stop() error
	OnConnect(handler func(session Session))
	OnDisconnect(handler func(session Session))
	OnMessage(handler func(session Session, data []byte))
	ConnectionCount() int
}

// ---------- NetworkConfig ----------

// NetworkConfig 保存所有网络相关配置。
type NetworkConfig struct {
	TCPAddr           string
	WSAddr            string
	WSSEnabled        bool
	TLSCertFile       string
	TLSKeyFile        string
	HeartbeatInterval time.Duration
	HeartbeatTimeout  time.Duration
	MaxConnections    int // 默认 5000
}

// DefaultNetworkConfig 返回带有合理默认值的 NetworkConfig。
func DefaultNetworkConfig() NetworkConfig {
	return NetworkConfig{
		TCPAddr:           ":9000",
		WSAddr:            ":9001",
		HeartbeatInterval: 15 * time.Second,
		HeartbeatTimeout:  45 * time.Second,
		MaxConnections:    5000,
	}
}

// ---------- networkLayer 实现 ----------

type networkLayer struct {
	config NetworkConfig

	// 回调函数
	onConnect    func(Session)
	onDisconnect func(Session)
	onMessage    func(Session, []byte)

	// 会话跟踪
	sessions sync.Map // map[string]Session
	connCount atomic.Int64

	// TCP
	tcpListener net.Listener

	// WebSocket
	wsHTTPServer *http.Server
	wsUpgrader   websocket.Upgrader

	// 生命周期
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// New 使用给定配置创建一个新的 NetworkLayer。
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

// Start 开始在 TCP 和 WebSocket 端点上监听。
func (nl *networkLayer) Start() error {
	// 启动 TCP 监听
	if nl.config.TCPAddr != "" {
		ln, err := net.Listen("tcp", nl.config.TCPAddr)
		if err != nil {
			return fmt.Errorf("tcp listen: %w", err)
		}
		nl.tcpListener = ln
		nl.wg.Add(1)
		go nl.acceptTCP()
	}

	// 启动 WebSocket 监听
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

// Stop 优雅地关闭所有监听器并关闭所有会话。
func (nl *networkLayer) Stop() error {
	close(nl.stopCh)

	if nl.tcpListener != nil {
		nl.tcpListener.Close()
	}
	if nl.wsHTTPServer != nil {
		nl.wsHTTPServer.Close()
	}

	// 关闭所有活跃会话
	nl.sessions.Range(func(key, value any) bool {
		if sess, ok := value.(Session); ok {
			sess.Close()
		}
		return true
	})

	nl.wg.Wait()
	return nil
}

// ---------- TCP 处理 ----------

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

		// 检查最大连接数
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
		// 设置读取超时用于心跳/断线检测
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
			// 检测 TCP 心跳（单字节 0x00）
			if n == 1 && buf[0] == 0x00 {
				// 心跳 ping — 回复 pong
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

// ---------- WebSocket 处理 ----------

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
	// 检查最大连接数
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

	// 配置 WebSocket 心跳（ping/pong）
	conn.SetReadDeadline(time.Now().Add(nl.config.HeartbeatTimeout))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(nl.config.HeartbeatTimeout))
		return nil
	})

	if nl.onConnect != nil {
		nl.onConnect(sess)
	}

	// 启动 ping 定时器协程
	nl.wg.Add(1)
	go nl.wsPingLoop(sess)

	// 读取循环
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

// ---------- 会话管理 ----------

func (nl *networkLayer) addSession(sess Session) {
	nl.sessions.Store(sess.ID(), sess)
	nl.connCount.Add(1)
}

func (nl *networkLayer) removeSession(sess Session) {
	if _, loaded := nl.sessions.LoadAndDelete(sess.ID()); !loaded {
		return // 已移除
	}
	nl.connCount.Add(-1)
	sess.Close()

	if nl.onDisconnect != nil {
		nl.onDisconnect(sess)
	}
}
