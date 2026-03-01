// WebSocket 通信封装
class WS {
  constructor() {
    this.conn = null;
    this.handlers = {};
    this.reconnectTimer = null;
    this.connected = false;
  }

  connect(url) {
    return new Promise((resolve, reject) => {
      this.conn = new WebSocket(url);
      this.conn.onopen = () => {
        this.connected = true;
        console.log('WebSocket 已连接');
        resolve();
      };
      this.conn.onclose = () => {
        this.connected = false;
        console.log('WebSocket 断开');
        this.emit('_disconnected');
      };
      this.conn.onerror = (e) => {
        console.error('WebSocket 错误', e);
        reject(e);
      };
      this.conn.onmessage = (e) => {
        try {
          const msg = JSON.parse(e.data);
          this.emit(msg.type, msg.data);
        } catch (err) {
          console.error('消息解析失败', err);
        }
      };
    });
  }

  send(type, data = {}) {
    if (!this.connected) return;
    this.conn.send(JSON.stringify({ type, data }));
  }

  on(type, fn) {
    if (!this.handlers[type]) this.handlers[type] = [];
    this.handlers[type].push(fn);
  }

  off(type, fn) {
    if (!this.handlers[type]) return;
    this.handlers[type] = this.handlers[type].filter(f => f !== fn);
  }

  emit(type, data) {
    const fns = this.handlers[type];
    if (fns) fns.forEach(fn => fn(data));
  }
}

const ws = new WS();
