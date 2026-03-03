// Package main 提供修罗斗场多人对战 Demo 服务器。
// 使用 SQLite 存储（可替换为 MySQL 等），基于 gfgame 引擎模块。
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"gfgame/cmd/demo/store"

	"github.com/gorilla/websocket"
)

// ---------- 配置 ----------

const (
	WSAddr       = ":9100"
	DBPath       = "demo.db"
	ArenaWidth   = 960.0
	ArenaHeight  = 960.0
	CampfireX    = 0.0
	CampfireY    = 464.0
	TickRate     = 20 // 每秒 tick 数
	AOIRadius    = 500.0
)

// ---------- 消息类型 ----------

const (
	MsgRegister       = "register"
	MsgLogin          = "login"
	MsgSelectClass    = "select_class"
	MsgEquip          = "equip"
	MsgUnequip        = "unequip"
	MsgEnterArena     = "enter_arena"
	MsgLeaveArena     = "leave_arena"
	MsgMove           = "move"
	MsgAttack         = "attack"
	MsgCastSkill      = "cast_skill"
	MsgChat           = "chat"
	MsgGetCharInfo    = "get_char_info"
	MsgGetEquipList   = "get_equip_list"

	// 服务端推送
	MsgLoginOK        = "login_ok"
	MsgRegisterOK     = "register_ok"
	MsgClassSelected  = "class_selected"
	MsgEquipOK        = "equip_ok"
	MsgUnequipOK      = "unequip_ok"
	MsgArenaState     = "arena_state"
	MsgPlayerJoined   = "player_joined"
	MsgPlayerLeft     = "player_left"
	MsgPlayerMoved    = "player_moved"
	MsgPlayerAttacked = "player_attacked"
	MsgSkillCasted    = "skill_casted"
	MsgDamageDealt    = "damage_dealt"
	MsgPlayerDied     = "player_died"
	MsgPlayerRespawn  = "player_respawn"
	MsgChatMsg        = "chat_msg"
	MsgCharInfo       = "char_info"
	MsgEquipList      = "equip_list"
	MsgError          = "error"
)

// ---------- 消息结构 ----------

// ClientMessage 客户端发送的消息
type ClientMessage struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// ServerMessage 服务端发送的消息
type ServerMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// ---------- 玩家会话 ----------

type PlayerSession struct {
	conn      *websocket.Conn
	mu        sync.Mutex
	charID    int64
	charName  string
	charClass string
	inArena   bool

	// 竞技场内状态
	x, y      float64
	hp, maxHP float64
	mp, maxMP float64
	attack    float64
	defense   float64
	speed     float64
	level     int
	dead      bool
	direction string // "up","down","left","right"
}

func (ps *PlayerSession) Send(msg ServerMessage) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("序列化消息失败: %v", err)
		return
	}
	log.Printf("发送消息: type=%s, len=%d", msg.Type, len(data))
	if err := ps.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		log.Printf("发送消息失败: %v", err)
	}
}

// ---------- 竞技场 ----------

type Arena struct {
	mu      sync.RWMutex
	players map[int64]*PlayerSession // charID -> session
}

func NewArena() *Arena {
	return &Arena{
		players: make(map[int64]*PlayerSession),
	}
}

func (a *Arena) Broadcast(msg ServerMessage, exclude int64) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	for id, ps := range a.players {
		if id != exclude {
			ps.Send(msg)
		}
	}
}

func (a *Arena) BroadcastAll(msg ServerMessage) {
	a.Broadcast(msg, -1)
}

// ---------- Demo 服务器 ----------

type DemoServer struct {
	db       *store.Store
	arena    *Arena
	sessions sync.Map // conn -> *PlayerSession
	upgrader websocket.Upgrader
}

func NewDemoServer(db *store.Store) *DemoServer {
	return &DemoServer{
		db:    db,
		arena: NewArena(),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

func (s *DemoServer) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.handleWS)

	// 静态文件服务
	mux.Handle("/", http.FileServer(http.Dir("static")))

	log.Printf("Demo 服务器启动在 %s", WSAddr)
	return http.ListenAndServe(WSAddr, mux)
}

func (s *DemoServer) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket 升级失败: %v", err)
		return
	}
	defer conn.Close()

	session := &PlayerSession{conn: conn, direction: "down"}
	s.sessions.Store(conn, session)
	defer s.sessions.Delete(conn)
	defer s.handleDisconnect(session)

	log.Printf("新连接: %s", conn.RemoteAddr())

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			log.Printf("读取消息失败: %v", err)
			return
		}

		var msg ClientMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			session.Send(ServerMessage{Type: MsgError, Data: "消息格式错误"})
			continue
		}

		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("处理消息 panic: %v", r)
					session.Send(ServerMessage{Type: MsgError, Data: "服务器内部错误"})
				}
			}()
			s.handleMessage(session, msg)
		}()
	}
}

func (s *DemoServer) handleDisconnect(session *PlayerSession) {
	if session.inArena && session.charID > 0 {
		s.arena.mu.Lock()
		delete(s.arena.players, session.charID)
		s.arena.mu.Unlock()

		s.arena.BroadcastAll(ServerMessage{
			Type: MsgPlayerLeft,
			Data: map[string]interface{}{
				"char_id": session.charID,
				"name":    session.charName,
			},
		})
		log.Printf("玩家 %s 离开竞技场", session.charName)
	}
}

func main() {
	// 初始化数据库
	db, err := store.NewStore("sqlite", DBPath)
	if err != nil {
		log.Fatalf("初始化数据库失败: %v", err)
	}
	defer db.Close()

	if err := db.AutoMigrate(); err != nil {
		log.Fatalf("数据库迁移失败: %v", err)
	}

	// 初始化默认装备数据
	if err := db.SeedDefaultEquipment(); err != nil {
		log.Printf("初始化默认装备数据: %v", err)
	}

	server := NewDemoServer(db)

	// 优雅关闭
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		fmt.Println("\n正在关闭服务器...")
		os.Exit(0)
	}()

	if err := server.Start(); err != nil {
		log.Fatalf("服务器启动失败: %v", err)
	}
}
