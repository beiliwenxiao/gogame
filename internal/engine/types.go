// Package engine 为 MMRPG 游戏引擎提供核心类型和接口定义。
package engine

// EntityID 是 ECS 世界中实体的全局唯一标识符。
type EntityID uint64

// ComponentType 标识组件的类型。
type ComponentType uint16

// Vector3 表示三维坐标或方向向量。
type Vector3 struct {
	X float32
	Y float32
	Z float32
}

// TransportProtocol 标识底层网络传输协议。
type TransportProtocol int

const (
	// ProtocolTCP 表示 TCP 长连接传输。
	ProtocolTCP TransportProtocol = iota
	// ProtocolWebSocket 表示 WebSocket 传输。
	ProtocolWebSocket
)

// SyncMode 标识房间使用的同步策略。
type SyncMode int

const (
	// SyncModeLockstep 使用帧同步（锁步）。
	SyncModeLockstep SyncMode = iota
	// SyncModeState 使用权威状态同步。
	SyncModeState
)

// TickPhase 表示单次 Tick 内四个有序阶段之一。
type TickPhase int

const (
	// PhaseInput 是输入采集阶段。
	PhaseInput TickPhase = iota
	// PhaseUpdate 是逻辑更新阶段。
	PhaseUpdate
	// PhaseSync 是状态同步阶段。
	PhaseSync
	// PhaseCleanup 是清理阶段。
	PhaseCleanup
)

// EquipmentSlotType 标识角色身上的装备槽位。
type EquipmentSlotType int

const (
	// SlotWeapon 武器槽。
	SlotWeapon EquipmentSlotType = iota
	// SlotHelmet 头盔槽。
	SlotHelmet
	// SlotArmor 护甲/胸甲槽。
	SlotArmor
	// SlotBoots 靴子槽。
	SlotBoots
	// SlotNecklace 项链槽。
	SlotNecklace
	// SlotRing 戒指槽。
	SlotRing
)

// EquipmentQuality 表示装备的品质等级。
type EquipmentQuality int

const (
	// QualityNormal 普通品质。
	QualityNormal EquipmentQuality = iota
	// QualityRare 稀有品质。
	QualityRare
	// QualityEpic 史诗品质。
	QualityEpic
	// QualityLegendary 传说品质。
	QualityLegendary
)

// TargetMode 定义技能选择目标的方式。
type TargetMode int

const (
	// TargetSingle 单体目标。
	TargetSingle TargetMode = iota
	// TargetFan 扇形/锥形范围目标。
	TargetFan
	// TargetCircle 圆形范围目标。
	TargetCircle
	// TargetRectangle 矩形范围目标。
	TargetRectangle
)

// InterruptPolicy 定义技能施法被打断时的处理方式。
type InterruptPolicy int

const (
	// InterruptCancel 取消剩余技能阶段。
	InterruptCancel InterruptPolicy = iota
	// InterruptContinue 继续执行剩余技能阶段。
	InterruptContinue
)

// SkillPhase 表示技能执行流水线中的一个阶段。
type SkillPhase int

const (
	// SkillPhaseWindup 前摇（施法）阶段。
	SkillPhaseWindup SkillPhase = iota
	// SkillPhaseHit 命中检测阶段。
	SkillPhaseHit
	// SkillPhaseSettle 伤害结算阶段。
	SkillPhaseSettle
	// SkillPhaseRecovery 后摇（冷却）阶段。
	SkillPhaseRecovery
)

// OperationCode 是紧凑编解码协议中表示玩家操作的单字节码。
type OperationCode byte

const (
	// OpMoveUp 实体向上移动。
	OpMoveUp OperationCode = 'u'
	// OpMoveDown 实体向下移动。
	OpMoveDown OperationCode = 'd'
	// OpMoveLeft 实体向左移动。
	OpMoveLeft OperationCode = 'l'
	// OpMoveRight 实体向右移动。
	OpMoveRight OperationCode = 'r'
	// OpAttack 触发普通攻击。
	OpAttack OperationCode = 'a'
	// OpSkill 触发技能释放。
	OpSkill OperationCode = 's'
	// OpInteract 触发交互操作。
	OpInteract OperationCode = 'i'
	// OpChat 发送聊天消息。
	OpChat OperationCode = 'c'
)

// MigrationPhase 追踪实体在房间间迁移的进度。
type MigrationPhase int

const (
	// MigrationPrepare 迁移前快照实体数据。
	MigrationPrepare MigrationPhase = iota
	// MigrationTransfer 在目标房间创建实体。
	MigrationTransfer
	// MigrationConfirm 等待目标房间确认接收。
	MigrationConfirm
	// MigrationCleanup 从源房间移除实体。
	MigrationCleanup
	// MigrationComplete 迁移成功完成。
	MigrationComplete
)
