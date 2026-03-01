# MMRPG Game Engine — Protocol Documentation

**Protocol Version:** 1  
**Encoding:** Binary (default) or JSON (debug)  
**Transport:** TCP long-connection or WebSocket (ws/wss)

---

## 1. Frame Format

All messages use the following wire frame:

```
+------------------+------------------+---------------------------+
| Length (4 bytes) | MsgID (2 bytes)  | Body (Length-2 bytes)     |
+------------------+------------------+---------------------------+
```

| Field  | Type     | Description                              |
|--------|----------|------------------------------------------|
| Length | uint32 BE | Total byte count of MsgID + Body         |
| MsgID  | uint16 BE | Message type identifier (see table below)|
| Body   | bytes    | Message-specific payload                 |

Minimum valid frame size: **6 bytes** (4 + 2 + 0-byte body).  
Frames with `Length < 2` are discarded and the connection is logged as malformed.

---

## 2. Protocol Version

The client sends `ProtocolVersion` in `LoginRequest` (MsgID `0x0001`).  
The server rejects connections with an incompatible version and closes the connection.

Current supported version: **1**

---

## 3. Message ID Table

| MsgID  | Name              | Direction       | Description                    |
|--------|-------------------|-----------------|--------------------------------|
| 0x0001 | LoginRequest      | Client → Server | Authentication and version check |
| 0x0002 | MoveRequest       | Client → Server | Movement command               |
| 0x0003 | SkillCastRequest  | Client → Server | Skill cast command             |
| 0x0004 | ChatMessage       | Client → Server | Chat message                   |

---

## 4. Message Definitions

### 4.1 LoginRequest (0x0001)

**Direction:** Client → Server

**Binary layout:**

```
[4 bytes: token length (uint32 BE)]
[N bytes: token (UTF-8 string)]
[4 bytes: protocol_version (uint32 BE)]
```

**JSON example:**
```json
{
  "msg_type": "LoginRequest",
  "data": {
    "token": "eyJhbGciOiJIUzI1NiJ9...",
    "protocol_version": 1
  }
}
```

| Field            | Type   | Description                              |
|------------------|--------|------------------------------------------|
| token            | string | Authentication token (JWT or session key)|
| protocol_version | uint32 | Client protocol version                  |

**Success response:** Server begins sending state sync messages.  
**Failure:** Server closes connection with error log.

---

### 4.2 MoveRequest (0x0002)

**Direction:** Client → Server

**Binary layout:**

```
[4 bytes: X (float32 BE)]
[4 bytes: Y (float32 BE)]
[4 bytes: Z (float32 BE)]
```

**JSON example:**
```json
{
  "msg_type": "MoveRequest",
  "data": {
    "x": 128.5,
    "y": 0.0,
    "z": 64.25
  }
}
```

| Field | Type    | Description              |
|-------|---------|--------------------------|
| x     | float32 | Target X world coordinate|
| y     | float32 | Target Y world coordinate|
| z     | float32 | Target Z world coordinate|

---

### 4.3 SkillCastRequest (0x0003)

**Direction:** Client → Server

**Binary layout:**

```
[4 bytes: skill_id (uint32 BE)]
[8 bytes: target_id (uint64 BE)]
[4 bytes: target_x (float32 BE)]
[4 bytes: target_y (float32 BE)]
[4 bytes: target_z (float32 BE)]
```

**JSON example:**
```json
{
  "msg_type": "SkillCastRequest",
  "data": {
    "skill_id": 1001,
    "target_id": 88001234,
    "target_x": 130.0,
    "target_y": 0.0,
    "target_z": 65.0
  }
}
```

| Field     | Type    | Description                              |
|-----------|---------|------------------------------------------|
| skill_id  | uint32  | Skill definition ID                      |
| target_id | uint64  | Target entity ID (0 = ground/position)   |
| target_x  | float32 | Target X coordinate                      |
| target_y  | float32 | Target Y coordinate                      |
| target_z  | float32 | Target Z coordinate                      |

---

### 4.4 ChatMessage (0x0004)

**Direction:** Client → Server

**Binary layout:**

```
[8 bytes: sender_id (uint64 BE)]
[1 byte:  channel (uint8)]
[4 bytes: content length (uint32 BE)]
[N bytes: content (UTF-8 string)]
```

**JSON example:**
```json
{
  "msg_type": "ChatMessage",
  "data": {
    "sender_id": 88001234,
    "channel": 1,
    "content": "Hello world!"
  }
}
```

| Field     | Type   | Description                                      |
|-----------|--------|--------------------------------------------------|
| sender_id | uint64 | Entity ID of the sender                          |
| channel   | uint8  | Chat channel: 0=World, 1=Zone, 2=Party, 3=Private|
| content   | string | Message text (UTF-8, max 512 bytes)              |

---

## 5. Compact Codec (Lockstep Mode)

In lockstep sync mode, high-frequency operations are encoded as compact strings to minimise bandwidth.

**Format:** `[ShortID][OpCode][...][ShortID][OpCode]...`

**Operation codes:**

| Code | Operation  | Description          |
|------|------------|----------------------|
| `u`  | MoveUp     | Move entity upward   |
| `d`  | MoveDown   | Move entity downward |
| `l`  | MoveLeft   | Move entity left     |
| `r`  | MoveRight  | Move entity right    |
| `a`  | Attack     | Basic attack         |
| `s`  | Skill      | Skill cast           |
| `i`  | Interact   | Interaction          |
| `c`  | Chat       | Chat message         |

**Example:** `"Au10aB"` — entity A moves up 10 units, entity B attacks.

Batch encoding merges multiple operations for the same entity in one tick.

---

## 6. Client Connection Flow

```
Client                          Server
  |                               |
  |--- TCP/WebSocket connect ----->|
  |                               | (assign session ID, start heartbeat)
  |--- LoginRequest (0x0001) ----->|
  |                               | (validate token + protocol version)
  |<-- state sync begins ---------|
  |                               |
  |--- MoveRequest (0x0002) ------>|
  |--- SkillCastRequest (0x0003) ->|
  |--- ChatMessage (0x0004) ------>|
  |                               |
  |   [heartbeat ping/pong]        |
  |                               |
  |--- disconnect / timeout ------>|
  |                               | (session cleanup, entity removal)
```

---

## 7. Heartbeat

- Server sends a heartbeat ping every **30 seconds** (configurable).
- Client must respond within **10 seconds** (configurable).
- No response → server closes the connection and fires a disconnect event.

---

## 8. Reconnection

1. Client reconnects with the same `token`.
2. Server matches the token to the existing session.
3. If the session is still alive (within grace period), state is restored.
4. If the session has expired, the client must re-enter the scene.

---

## 9. Error Codes

| Code | Name                  | Description                              |
|------|-----------------------|------------------------------------------|
| 1001 | ERR_INVALID_TOKEN     | Authentication token invalid or expired  |
| 1002 | ERR_VERSION_MISMATCH  | Protocol version not supported           |
| 1003 | ERR_ROOM_FULL         | Target room has reached capacity         |
| 1004 | ERR_ADMISSION_DENIED  | Admission check failed                   |
| 1005 | ERR_SKILL_ON_COOLDOWN | Skill is not ready                       |
| 1006 | ERR_EQUIPMENT_LOCKED  | Equipment change blocked during combat   |
| 1007 | ERR_MALFORMED_MESSAGE | Message frame could not be parsed        |

---

## 10. Versioning

- The protocol version is a monotonically increasing integer.
- Breaking changes increment the version.
- Clients with an unsupported version receive `ERR_VERSION_MISMATCH` and are disconnected.
- The server may support a range of versions for backward compatibility.
