// Package engine provides core types and interfaces for the MMRPG game engine.
package engine

// EntityID is the globally unique identifier for an entity in the ECS world.
type EntityID uint64

// ComponentType identifies the type of a Component.
type ComponentType uint16

// Vector3 represents a 3D coordinate or direction vector.
type Vector3 struct {
	X float32
	Y float32
	Z float32
}

// TransportProtocol identifies the underlying network transport.
type TransportProtocol int

const (
	// ProtocolTCP represents a TCP long-connection transport.
	ProtocolTCP TransportProtocol = iota
	// ProtocolWebSocket represents a WebSocket transport.
	ProtocolWebSocket
)

// SyncMode identifies the synchronization strategy used by a Room.
type SyncMode int

const (
	// SyncModeLockstep uses lockstep (frame) synchronization.
	SyncModeLockstep SyncMode = iota
	// SyncModeState uses authoritative state synchronization.
	SyncModeState
)

// TickPhase represents one of the four ordered phases within a single Tick.
type TickPhase int

const (
	// PhaseInput is the input-collection phase.
	PhaseInput TickPhase = iota
	// PhaseUpdate is the logic-update phase.
	PhaseUpdate
	// PhaseSync is the state-synchronization phase.
	PhaseSync
	// PhaseCleanup is the cleanup phase.
	PhaseCleanup
)

// EquipmentSlotType identifies an equipment slot on a character.
type EquipmentSlotType int

const (
	// SlotWeapon is the weapon slot.
	SlotWeapon EquipmentSlotType = iota
	// SlotHelmet is the helmet slot.
	SlotHelmet
	// SlotArmor is the armor/chest slot.
	SlotArmor
	// SlotBoots is the boots slot.
	SlotBoots
	// SlotNecklace is the necklace slot.
	SlotNecklace
	// SlotRing is the ring slot.
	SlotRing
)

// EquipmentQuality represents the quality tier of an equipment item.
type EquipmentQuality int

const (
	// QualityNormal is the base quality tier.
	QualityNormal EquipmentQuality = iota
	// QualityRare is the rare quality tier.
	QualityRare
	// QualityEpic is the epic quality tier.
	QualityEpic
	// QualityLegendary is the legendary quality tier.
	QualityLegendary
)

// TargetMode defines how a skill selects its targets.
type TargetMode int

const (
	// TargetSingle selects a single target.
	TargetSingle TargetMode = iota
	// TargetFan selects targets in a fan/cone area.
	TargetFan
	// TargetCircle selects targets in a circular area.
	TargetCircle
	// TargetRectangle selects targets in a rectangular area.
	TargetRectangle
)

// InterruptPolicy defines what happens when a skill cast is interrupted.
type InterruptPolicy int

const (
	// InterruptCancel cancels remaining skill phases.
	InterruptCancel InterruptPolicy = iota
	// InterruptContinue continues executing remaining phases.
	InterruptContinue
)

// SkillPhase represents a phase in the skill execution pipeline.
type SkillPhase int

const (
	// SkillPhaseWindup is the wind-up (cast) phase.
	SkillPhaseWindup SkillPhase = iota
	// SkillPhaseHit is the hit-detection phase.
	SkillPhaseHit
	// SkillPhaseSettle is the damage-settlement phase.
	SkillPhaseSettle
	// SkillPhaseRecovery is the recovery (cooldown) phase.
	SkillPhaseRecovery
)

// OperationCode is a single-byte code representing a player operation
// in the compact codec protocol.
type OperationCode byte

const (
	// OpMoveUp moves the entity upward.
	OpMoveUp OperationCode = 'u'
	// OpMoveDown moves the entity downward.
	OpMoveDown OperationCode = 'd'
	// OpMoveLeft moves the entity to the left.
	OpMoveLeft OperationCode = 'l'
	// OpMoveRight moves the entity to the right.
	OpMoveRight OperationCode = 'r'
	// OpAttack triggers a basic attack.
	OpAttack OperationCode = 'a'
	// OpSkill triggers a skill cast.
	OpSkill OperationCode = 's'
	// OpInteract triggers an interaction.
	OpInteract OperationCode = 'i'
	// OpChat sends a chat message.
	OpChat OperationCode = 'c'
)

// MigrationPhase tracks the progress of an entity migration between Rooms.
type MigrationPhase int

const (
	// MigrationPrepare snapshots entity data before transfer.
	MigrationPrepare MigrationPhase = iota
	// MigrationTransfer creates the entity in the target Room.
	MigrationTransfer
	// MigrationConfirm waits for the target Room to acknowledge receipt.
	MigrationConfirm
	// MigrationCleanup removes the entity from the source Room.
	MigrationCleanup
	// MigrationComplete indicates the migration finished successfully.
	MigrationComplete
)
