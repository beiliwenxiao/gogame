// Package room — broadcast.go 定义通用广播接口。
//
// Broadcaster 是房间级别的消息广播抽象，供不同游戏/场景实现各自的广播逻辑。
// 接口使用 []byte（已序列化的消息）和 string（sessionID）作为参数，
// 保持与引擎层 Session 接口的一致性。
//
// 注意：demo 的 Arena 已有 Broadcast/BroadcastAll 方法（使用 ServerMessage + charID），
// 为避免 Go 方法签名冲突，接口方法命名为 BroadcastBytes/BroadcastAllBytes。
// 其他游戏实现此接口时可自由选择内部广播策略。
package room

// Broadcaster 通用广播接口。
// 实现方负责将 msg 发送给房间内的所有（或部分）客户端。
type Broadcaster interface {
	// BroadcastBytes 向房间内所有客户端发送已序列化的消息，
	// 排除指定 excludeSessionID 的客户端。
	// excludeSessionID 为空字符串时等同于 BroadcastAllBytes。
	BroadcastBytes(msg []byte, excludeSessionID string)

	// BroadcastAllBytes 向房间内所有客户端发送已序列化的消息。
	BroadcastAllBytes(msg []byte)
}
