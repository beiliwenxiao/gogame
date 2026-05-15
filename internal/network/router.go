package network

import (
	"encoding/json"
	"fmt"
)

// MessageHandler 消息处理函数签名。
// session 使用 any 类型，调用方可传入任意会话类型（如 demo 的 *PlayerSession），
// handler 内部做类型断言即可。
type MessageHandler func(session any, data json.RawMessage)

// MessageRouter JSON 消息路由器，根据消息中的 "type" 字段分发到对应的 handler。
type MessageRouter struct {
	handlers map[string]MessageHandler
}

// routerMessage 用于解析 rawMsg 中的 type 和 data 字段。
type routerMessage struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// NewMessageRouter 创建一个新的 MessageRouter。
func NewMessageRouter() *MessageRouter {
	return &MessageRouter{
		handlers: make(map[string]MessageHandler),
	}
}

// Register 注册消息类型对应的处理函数。
// 如果 msgType 已注册，新 handler 会覆盖旧的。
func (r *MessageRouter) Register(msgType string, handler MessageHandler) {
	r.handlers[msgType] = handler
}

// Dispatch 解析 rawMsg 并分发到对应的 handler。
//
// rawMsg 格式：{"type": "xxx", "data": ...}
//
// 行为：
//  1. 将 rawMsg 解析为 {"type": "xxx", "data": ...} 格式
//  2. 根据 type 查找注册的 handler
//  3. 找到则调用 handler(session, data)
//  4. 未找到则返回错误
func (r *MessageRouter) Dispatch(session any, rawMsg []byte) error {
	var msg routerMessage
	if err := json.Unmarshal(rawMsg, &msg); err != nil {
		return fmt.Errorf("消息格式错误: %w", err)
	}

	handler, ok := r.handlers[msg.Type]
	if !ok {
		return fmt.Errorf("未知消息类型: %s", msg.Type)
	}

	handler(session, msg.Data)
	return nil
}
