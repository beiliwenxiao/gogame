package network

import (
	"encoding/json"
	"testing"
)

func TestMessageRouter_RegisterAndDispatch(t *testing.T) {
	router := NewMessageRouter()

	var calledType string
	var calledSession any
	var calledData json.RawMessage

	router.Register("move", func(session any, data json.RawMessage) {
		calledSession = session
		calledType = "move"
		calledData = data
	})

	rawMsg := []byte(`{"type":"move","data":{"x":10,"y":20}}`)
	sess := "test-session"

	err := router.Dispatch(sess, rawMsg)
	if err != nil {
		t.Fatalf("Dispatch 不应返回错误: %v", err)
	}

	if calledType != "move" {
		t.Errorf("期望 handler 类型 'move'，实际 %q", calledType)
	}
	if calledSession != sess {
		t.Errorf("期望 session 为 %q，实际 %v", sess, calledSession)
	}

	// 验证 data 内容
	var pos struct {
		X int `json:"x"`
		Y int `json:"y"`
	}
	if err := json.Unmarshal(calledData, &pos); err != nil {
		t.Fatalf("解析 data 失败: %v", err)
	}
	if pos.X != 10 || pos.Y != 20 {
		t.Errorf("期望 data {x:10, y:20}，实际 {x:%d, y:%d}", pos.X, pos.Y)
	}
}

func TestMessageRouter_UnknownType(t *testing.T) {
	router := NewMessageRouter()

	router.Register("move", func(session any, data json.RawMessage) {
		t.Fatal("不应调用 move handler")
	})

	rawMsg := []byte(`{"type":"unknown_action","data":{}}`)
	err := router.Dispatch("sess", rawMsg)
	if err == nil {
		t.Fatal("未知消息类型应返回错误")
	}

	expected := "未知消息类型: unknown_action"
	if err.Error() != expected {
		t.Errorf("期望错误 %q，实际 %q", expected, err.Error())
	}
}

func TestMessageRouter_InvalidJSON(t *testing.T) {
	router := NewMessageRouter()

	err := router.Dispatch("sess", []byte(`not json`))
	if err == nil {
		t.Fatal("无效 JSON 应返回错误")
	}
}

func TestMessageRouter_MultipleHandlers(t *testing.T) {
	router := NewMessageRouter()

	calls := make(map[string]int)

	router.Register("attack", func(session any, data json.RawMessage) {
		calls["attack"]++
	})
	router.Register("chat", func(session any, data json.RawMessage) {
		calls["chat"]++
	})

	// 分发 attack
	if err := router.Dispatch("s", []byte(`{"type":"attack","data":{}}`)); err != nil {
		t.Fatalf("Dispatch attack 失败: %v", err)
	}
	// 分发 chat
	if err := router.Dispatch("s", []byte(`{"type":"chat","data":{}}`)); err != nil {
		t.Fatalf("Dispatch chat 失败: %v", err)
	}
	// 再次分发 attack
	if err := router.Dispatch("s", []byte(`{"type":"attack","data":{}}`)); err != nil {
		t.Fatalf("Dispatch attack 失败: %v", err)
	}

	if calls["attack"] != 2 {
		t.Errorf("期望 attack 调用 2 次，实际 %d", calls["attack"])
	}
	if calls["chat"] != 1 {
		t.Errorf("期望 chat 调用 1 次，实际 %d", calls["chat"])
	}
}

func TestMessageRouter_OverwriteHandler(t *testing.T) {
	router := NewMessageRouter()

	var version int

	router.Register("move", func(session any, data json.RawMessage) {
		version = 1
	})
	router.Register("move", func(session any, data json.RawMessage) {
		version = 2
	})

	if err := router.Dispatch("s", []byte(`{"type":"move","data":{}}`)); err != nil {
		t.Fatalf("Dispatch 失败: %v", err)
	}

	if version != 2 {
		t.Errorf("期望覆盖后 handler 版本为 2，实际 %d", version)
	}
}

func TestMessageRouter_NilData(t *testing.T) {
	router := NewMessageRouter()

	var calledData json.RawMessage

	router.Register("ping", func(session any, data json.RawMessage) {
		calledData = data
	})

	// data 字段为 null
	if err := router.Dispatch("s", []byte(`{"type":"ping","data":null}`)); err != nil {
		t.Fatalf("Dispatch 失败: %v", err)
	}

	if string(calledData) != "null" {
		t.Errorf("期望 data 为 'null'，实际 %q", string(calledData))
	}
}

func TestMessageRouter_NoDataField(t *testing.T) {
	router := NewMessageRouter()

	var called bool

	router.Register("ping", func(session any, data json.RawMessage) {
		called = true
	})

	// 没有 data 字段
	if err := router.Dispatch("s", []byte(`{"type":"ping"}`)); err != nil {
		t.Fatalf("Dispatch 失败: %v", err)
	}

	if !called {
		t.Error("handler 应被调用")
	}
}

func TestMessageRouter_AnySessionType(t *testing.T) {
	router := NewMessageRouter()

	// 模拟 demo 的 PlayerSession 场景：传入自定义结构体指针
	type FakeSession struct {
		Name string
	}

	var receivedName string

	router.Register("greet", func(session any, data json.RawMessage) {
		fs, ok := session.(*FakeSession)
		if !ok {
			t.Fatal("session 类型断言失败")
		}
		receivedName = fs.Name
	})

	sess := &FakeSession{Name: "player1"}
	if err := router.Dispatch(sess, []byte(`{"type":"greet","data":{}}`)); err != nil {
		t.Fatalf("Dispatch 失败: %v", err)
	}

	if receivedName != "player1" {
		t.Errorf("期望 name 'player1'，实际 %q", receivedName)
	}
}
