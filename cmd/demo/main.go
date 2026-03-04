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
	CampfireY    = 464.0 // 30x30等距地图中心: gridToScreen(14.5, 14.5) = (0, 464)
	TickRate     = 30 // 每秒 tick 数（移动同步用）
	StateSyncHz  = 5  // 状态同步频率（HP/MP/死亡等）
	AOIRadius    = 500.0
	MaxNPCCount  = 100 // NPC 上限
	NPCAITickHz  = 2   // NPC AI 更新频率（每秒）
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
	MsgStateSync      = "state_sync"
	MsgNPCSpawn       = "npc_spawn"
	MsgNPCDied        = "npc_died"
	MsgNPCUpdate      = "npc_update"
	MsgNPCDrop        = "npc_drop"
	MsgAttackNPC      = "attack_npc"
	MsgCastSkillNPC   = "cast_skill_npc"
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
	critRate  float64
	critDmg   float64
	level     int
	dead      bool
	direction string // "up","down","left","right"

	// 武器属性
	weaponAttackRange float64 // 武器攻击范围（像素）
	weaponAttackDist  float64 // 武器攻击距离（像素）

	// 增量同步：上次发送的快照（每个接收者独立）
	lastSync map[int64]*playerSnapshot
}

// playerSnapshot 记录上次同步给某个客户端的玩家状态
type playerSnapshot struct {
	x, y      float64
	hp, maxHP float64
	mp, maxMP float64
	attack    float64
	defense   float64
	critRate  float64
	critDmg   float64
	dead      bool
	direction string
}

func (ps *PlayerSession) Send(msg ServerMessage) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("序列化消息失败: %v", err)
		return
	}
	// 过滤高频 state_sync 日志，避免 I/O 阻塞
	if msg.Type != MsgStateSync && msg.Type != MsgPlayerMoved {
		log.Printf("发送消息: type=%s, len=%d", msg.Type, len(data))
	}
	if err := ps.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		log.Printf("发送消息失败: %v", err)
	}
}

// ---------- 竞技场 ----------

// ArenaNPC 竞技场NPC
type ArenaNPC struct {
	ID        int64   `json:"id"`
	Name      string  `json:"name"`
	Template  string  `json:"template"`  // 敌人模板ID（对应前端 EntityFactory）
	Level     int     `json:"level"`
	X, Y      float64 `json:"x,y"`
	HP, MaxHP float64 `json:"hp,max_hp"`
	Attack    float64 `json:"attack"`
	Defense   float64 `json:"defense"`
	Speed     float64 `json:"speed"`
	Dead      bool    `json:"dead"`

	// AI 相关
	TargetID     int64   // 当前攻击目标的 charID，0 表示无目标
	AttackRange  float64 // 攻击范围
	AttackCD     float64 // 攻击冷却（秒）
	LastAttackAt int64   // 上次攻击时间（UnixMilli）
	AggroRange   float64 // 仇恨范围（主动攻击距离）
	CritRate     float64 // 暴击率
	CritDmg      float64 // 暴击倍率
	SpawnX       float64 // 出生点 X
	SpawnY       float64 // 出生点 Y
	LeashRange   float64 // 脱战回归距离

	// 恐惧状态（战吼效果）
	FearUntil int64   // 恐惧结束时间（UnixMilli），0 表示无恐惧
	FearDirX  float64 // 恐惧逃跑方向 X
	FearDirY  float64 // 恐惧逃跑方向 Y
}

type Arena struct {
	mu      sync.RWMutex
	players map[int64]*PlayerSession // charID -> session
	npcs    map[int64]*ArenaNPC      // npcID -> NPC
	npcSeq  int64                    // NPC ID 自增序列
}

func NewArena() *Arena {
	return &Arena{
		players: make(map[int64]*PlayerSession),
		npcs:    make(map[int64]*ArenaNPC),
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

	// 启动竞技场 tick 循环（状态同步）
	go s.arenaTick()

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
