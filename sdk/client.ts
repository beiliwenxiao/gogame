/**
 * MMRPG Game Engine — JavaScript/TypeScript Client SDK
 *
 * Provides WebSocket connection management, heartbeat, reconnection,
 * message encoding/decoding, and high-level game API.
 *
 * Protocol version: 1
 */

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

export const PROTOCOL_VERSION = 1;

/** Message IDs matching the server codec. */
export const MsgID = {
  LoginRequest:     0x0001,
  MoveRequest:      0x0002,
  SkillCastRequest: 0x0003,
  ChatMessage:      0x0004,
} as const;

/** Chat channel constants. */
export const ChatChannel = {
  World:   0,
  Zone:    1,
  Party:   2,
  Private: 3,
} as const;

/** Error codes returned by the server. */
export const ErrorCode = {
  INVALID_TOKEN:     1001,
  VERSION_MISMATCH:  1002,
  ROOM_FULL:         1003,
  ADMISSION_DENIED:  1004,
  SKILL_ON_COOLDOWN: 1005,
  EQUIPMENT_LOCKED:  1006,
  MALFORMED_MESSAGE: 1007,
} as const;

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface LoginRequest {
  token: string;
  protocolVersion: number;
}

export interface MoveRequest {
  x: number;
  y: number;
  z: number;
}

export interface SkillCastRequest {
  skillId: number;
  targetId: bigint;
  targetX: number;
  targetY: number;
  targetZ: number;
}

export interface ChatMessagePayload {
  senderId: bigint;
  channel: number;
  content: string;
}

export interface ServerMessage {
  msgId: number;
  body: unknown;
}

export type EventCallback<T = unknown> = (data: T) => void;

export interface SDKOptions {
  /** WebSocket URL, e.g. "ws://localhost:8080/ws" */
  url: string;
  /** Heartbeat interval in ms (default: 30000) */
  heartbeatInterval?: number;
  /** Heartbeat timeout in ms (default: 10000) */
  heartbeatTimeout?: number;
  /** Max reconnect attempts (default: 5, 0 = unlimited) */
  maxReconnectAttempts?: number;
  /** Base reconnect delay in ms (default: 1000) */
  reconnectDelay?: number;
}

// ---------------------------------------------------------------------------
// Binary codec helpers
// ---------------------------------------------------------------------------

const encoder = new TextEncoder();
const decoder = new TextDecoder();

/**
 * Encodes a LoginRequest into a binary frame.
 * Frame: [4B length][2B msgId][4B tokenLen][tokenBytes][4B version]
 */
function encodeLoginRequest(req: LoginRequest): ArrayBuffer {
  const tokenBytes = encoder.encode(req.token);
  const bodyLen = 4 + tokenBytes.byteLength + 4;
  const buf = new ArrayBuffer(4 + 2 + bodyLen);
  const view = new DataView(buf);
  let offset = 0;
  view.setUint32(offset, 2 + bodyLen, false); offset += 4; // length = msgId + body
  view.setUint16(offset, MsgID.LoginRequest, false); offset += 2;
  view.setUint32(offset, tokenBytes.byteLength, false); offset += 4;
  new Uint8Array(buf, offset, tokenBytes.byteLength).set(tokenBytes); offset += tokenBytes.byteLength;
  view.setUint32(offset, req.protocolVersion, false);
  return buf;
}

/**
 * Encodes a MoveRequest into a binary frame.
 * Body: [4B x][4B y][4B z]
 */
function encodeMoveRequest(req: MoveRequest): ArrayBuffer {
  const buf = new ArrayBuffer(4 + 2 + 12);
  const view = new DataView(buf);
  view.setUint32(0, 14, false); // length = 2 + 12
  view.setUint16(4, MsgID.MoveRequest, false);
  view.setFloat32(6, req.x, false);
  view.setFloat32(10, req.y, false);
  view.setFloat32(14, req.z, false);
  return buf;
}

/**
 * Encodes a SkillCastRequest into a binary frame.
 * Body: [4B skillId][8B targetId][4B tx][4B ty][4B tz]
 */
function encodeSkillCastRequest(req: SkillCastRequest): ArrayBuffer {
  const buf = new ArrayBuffer(4 + 2 + 24);
  const view = new DataView(buf);
  view.setUint32(0, 26, false); // length = 2 + 24
  view.setUint16(4, MsgID.SkillCastRequest, false);
  view.setUint32(6, req.skillId, false);
  view.setBigUint64(10, req.targetId, false);
  view.setFloat32(18, req.targetX, false);
  view.setFloat32(22, req.targetY, false);
  view.setFloat32(26, req.targetZ, false);
  return buf;
}

/**
 * Encodes a ChatMessage into a binary frame.
 * Body: [8B senderId][1B channel][4B contentLen][contentBytes]
 */
function encodeChatMessage(msg: ChatMessagePayload): ArrayBuffer {
  const contentBytes = encoder.encode(msg.content);
  const bodyLen = 8 + 1 + 4 + contentBytes.byteLength;
  const buf = new ArrayBuffer(4 + 2 + bodyLen);
  const view = new DataView(buf);
  let offset = 0;
  view.setUint32(offset, 2 + bodyLen, false); offset += 4;
  view.setUint16(offset, MsgID.ChatMessage, false); offset += 2;
  view.setBigUint64(offset, msg.senderId, false); offset += 8;
  view.setUint8(offset, msg.channel); offset += 1;
  view.setUint32(offset, contentBytes.byteLength, false); offset += 4;
  new Uint8Array(buf, offset, contentBytes.byteLength).set(contentBytes);
  return buf;
}

/**
 * Decodes a raw binary frame from the server.
 * Returns { msgId, body: ArrayBuffer } or null if malformed.
 */
function decodeFrame(data: ArrayBuffer): { msgId: number; body: ArrayBuffer } | null {
  if (data.byteLength < 6) return null;
  const view = new DataView(data);
  const length = view.getUint32(0, false);
  if (data.byteLength < 4 + length || length < 2) return null;
  const msgId = view.getUint16(4, false);
  const body = data.slice(6, 4 + length);
  return { msgId, body };
}

// ---------------------------------------------------------------------------
// GameClient
// ---------------------------------------------------------------------------

/**
 * GameClient manages the WebSocket connection to the MMRPG game server.
 *
 * Usage:
 * ```ts
 * const client = new GameClient({ url: 'ws://localhost:8080/ws' });
 * client.on('open', () => client.login('my-token'));
 * client.on('message', (msg) => console.log(msg));
 * client.connect();
 * ```
 */
export class GameClient {
  private ws: WebSocket | null = null;
  private opts: Required<SDKOptions>;
  private reconnectAttempts = 0;
  private heartbeatTimer: ReturnType<typeof setInterval> | null = null;
  private heartbeatTimeoutTimer: ReturnType<typeof setTimeout> | null = null;
  private stopped = false;

  private listeners: Map<string, Set<EventCallback>> = new Map();

  constructor(opts: SDKOptions) {
    this.opts = {
      url: opts.url,
      heartbeatInterval: opts.heartbeatInterval ?? 30_000,
      heartbeatTimeout: opts.heartbeatTimeout ?? 10_000,
      maxReconnectAttempts: opts.maxReconnectAttempts ?? 5,
      reconnectDelay: opts.reconnectDelay ?? 1_000,
    };
  }

  // ---- Event emitter ----

  /** Register an event callback. */
  on<T = unknown>(event: string, cb: EventCallback<T>): this {
    if (!this.listeners.has(event)) this.listeners.set(event, new Set());
    this.listeners.get(event)!.add(cb as EventCallback);
    return this;
  }

  /** Remove an event callback. */
  off<T = unknown>(event: string, cb: EventCallback<T>): this {
    this.listeners.get(event)?.delete(cb as EventCallback);
    return this;
  }

  private emit(event: string, data?: unknown): void {
    this.listeners.get(event)?.forEach(cb => cb(data));
  }

  // ---- Connection ----

  /** Open the WebSocket connection. */
  connect(): void {
    this.stopped = false;
    this._connect();
  }

  private _connect(): void {
    if (this.ws) {
      this.ws.onopen = null;
      this.ws.onclose = null;
      this.ws.onerror = null;
      this.ws.onmessage = null;
      this.ws.close();
    }

    this.ws = new WebSocket(this.opts.url);
    this.ws.binaryType = 'arraybuffer';

    this.ws.onopen = () => {
      this.reconnectAttempts = 0;
      this._startHeartbeat();
      this.emit('open');
    };

    this.ws.onclose = (ev) => {
      this._stopHeartbeat();
      this.emit('close', ev);
      if (!this.stopped) this._scheduleReconnect();
    };

    this.ws.onerror = (ev) => {
      this.emit('error', ev);
    };

    this.ws.onmessage = (ev: MessageEvent<ArrayBuffer>) => {
      this._resetHeartbeatTimeout();
      const frame = decodeFrame(ev.data);
      if (!frame) {
        this.emit('error', new Error('malformed frame'));
        return;
      }
      this.emit('message', frame);
    };
  }

  /** Gracefully close the connection without reconnecting. */
  disconnect(): void {
    this.stopped = true;
    this._stopHeartbeat();
    this.ws?.close();
    this.ws = null;
  }

  private _scheduleReconnect(): void {
    const max = this.opts.maxReconnectAttempts;
    if (max > 0 && this.reconnectAttempts >= max) {
      this.emit('reconnect_failed');
      return;
    }
    this.reconnectAttempts++;
    const delay = this.opts.reconnectDelay * this.reconnectAttempts;
    this.emit('reconnecting', { attempt: this.reconnectAttempts, delay });
    setTimeout(() => {
      if (!this.stopped) this._connect();
    }, delay);
  }

  // ---- Heartbeat ----

  private _startHeartbeat(): void {
    this._stopHeartbeat();
    this.heartbeatTimer = setInterval(() => {
      if (this.ws?.readyState === WebSocket.OPEN) {
        // Send a zero-length ping frame (msgId 0x0000, no body).
        const ping = new ArrayBuffer(6);
        new DataView(ping).setUint32(0, 2, false); // length = 2 (just msgId)
        this.ws.send(ping);
        this._startHeartbeatTimeout();
      }
    }, this.opts.heartbeatInterval);
  }

  private _startHeartbeatTimeout(): void {
    this._clearHeartbeatTimeout();
    this.heartbeatTimeoutTimer = setTimeout(() => {
      this.emit('error', new Error('heartbeat timeout'));
      this.ws?.close();
    }, this.opts.heartbeatTimeout);
  }

  private _resetHeartbeatTimeout(): void {
    this._clearHeartbeatTimeout();
  }

  private _clearHeartbeatTimeout(): void {
    if (this.heartbeatTimeoutTimer !== null) {
      clearTimeout(this.heartbeatTimeoutTimer);
      this.heartbeatTimeoutTimer = null;
    }
  }

  private _stopHeartbeat(): void {
    if (this.heartbeatTimer !== null) {
      clearInterval(this.heartbeatTimer);
      this.heartbeatTimer = null;
    }
    this._clearHeartbeatTimeout();
  }

  // ---- High-level API ----

  /**
   * Send a LoginRequest. Call this after the 'open' event fires.
   * @param token - Authentication token
   */
  login(token: string): void {
    this._send(encodeLoginRequest({ token, protocolVersion: PROTOCOL_VERSION }));
  }

  /**
   * Send a movement command.
   */
  move(x: number, y: number, z: number): void {
    this._send(encodeMoveRequest({ x, y, z }));
  }

  /**
   * Cast a skill at a target entity or position.
   */
  castSkill(skillId: number, targetId: bigint, tx: number, ty: number, tz: number): void {
    this._send(encodeSkillCastRequest({ skillId, targetId, targetX: tx, targetY: ty, targetZ: tz }));
  }

  /**
   * Send a chat message.
   */
  chat(senderId: bigint, channel: number, content: string): void {
    this._send(encodeChatMessage({ senderId, channel, content }));
  }

  private _send(data: ArrayBuffer): void {
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.ws.send(data);
    } else {
      this.emit('error', new Error('send called while not connected'));
    }
  }

  /** Returns true if the WebSocket is currently open. */
  get isConnected(): boolean {
    return this.ws?.readyState === WebSocket.OPEN;
  }
}
