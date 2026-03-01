// 修罗斗场 Canvas 渲染
class ArenaRenderer {
  constructor() {
    this.canvas = null;
    this.ctx = null;
    this.players = new Map();
    this.selfId = 0;
    this.campfire = { x: 400, y: 300 };
    this.arenaSize = { width: 800, height: 600 };
    this.camera = { x: 0, y: 0 };
    this.floatingTexts = [];
    this.animFrame = 0;
    this.keys = {};
    this.lastMoveTime = 0;
    this.skills = [];
    this.skillCooldowns = {};
    this.selectedTarget = null;
    this.running = false;
  }

  init(canvasId) {
    this.canvas = document.getElementById(canvasId);
    this.ctx = this.canvas.getContext('2d');
    this.resize();
    window.addEventListener('resize', () => this.resize());
    window.addEventListener('keydown', (e) => { this.keys[e.key.toLowerCase()] = true; });
    window.addEventListener('keyup', (e) => { this.keys[e.key.toLowerCase()] = false; });
    this.canvas.addEventListener('click', (e) => this.handleClick(e));
  }

  resize() {
    this.canvas.width = this.canvas.parentElement.clientWidth;
    this.canvas.height = this.canvas.parentElement.clientHeight - 80;
  }

  start(state) {
    this.selfId = state.self_id;
    this.campfire = state.campfire;
    this.arenaSize = { width: state.arena.width, height: state.arena.height };
    this.skills = state.skills || [];
    this.players.clear();
    for (const p of state.players) {
      this.players.set(p.char_id, { ...p, targetX: p.x, targetY: p.y });
    }
    this.running = true;
    this.renderSkillBar();
    this.loop();
  }

  stop() {
    this.running = false;
    this.players.clear();
  }

  loop() {
    if (!this.running) return;
    this.animFrame++;
    this.update();
    this.render();
    requestAnimationFrame(() => this.loop());
  }

  update() {
    const self = this.players.get(this.selfId);
    if (!self || self.dead) return;
    const now = Date.now();
    if (now - this.lastMoveTime < 50) return;
    let dx = 0, dy = 0;
    const spd = (self.speed || 150) / 20;
    if (this.keys['w'] || this.keys['arrowup']) { dy = -spd; self.direction = 'up'; }
    if (this.keys['s'] || this.keys['arrowdown']) { dy = spd; self.direction = 'down'; }
    if (this.keys['a'] || this.keys['arrowleft']) { dx = -spd; self.direction = 'left'; }
    if (this.keys['d'] || this.keys['arrowright']) { dx = spd; self.direction = 'right'; }
    if (dx !== 0 || dy !== 0) {
      self.x = Math.max(10, Math.min(this.arenaSize.width - 10, self.x + dx));
      self.y = Math.max(10, Math.min(this.arenaSize.height - 10, self.y + dy));
      self.targetX = self.x;
      self.targetY = self.y;
      ws.send('move', { x: self.x, y: self.y, direction: self.direction });
      this.lastMoveTime = now;
    }
    // 平滑插值其他玩家
    for (const [id, p] of this.players) {
      if (id === this.selfId) continue;
      p.x += (p.targetX - p.x) * 0.3;
      p.y += (p.targetY - p.y) * 0.3;
    }
    // 更新相机
    this.camera.x = self.x - this.canvas.width / 2;
    this.camera.y = self.y - this.canvas.height / 2;
    // 更新浮动文字
    this.floatingTexts = this.floatingTexts.filter(t => {
      t.y -= 1; t.life--;
      return t.life > 0;
    });
  }

  render() {
    const ctx = this.ctx;
    const cam = this.camera;
    ctx.clearRect(0, 0, this.canvas.width, this.canvas.height);
    // 背景
    ctx.fillStyle = '#2d5a27';
    ctx.fillRect(0, 0, this.canvas.width, this.canvas.height);
    // 草地纹理
    ctx.fillStyle = '#3a6b30';
    for (let x = 0; x < this.arenaSize.width; x += 40) {
      for (let y = 0; y < this.arenaSize.height; y += 40) {
        if ((x + y) % 80 === 0) {
          ctx.fillRect(x - cam.x, y - cam.y, 20, 20);
        }
      }
    }
    // 边界
    ctx.strokeStyle = '#555';
    ctx.lineWidth = 2;
    ctx.strokeRect(-cam.x, -cam.y, this.arenaSize.width, this.arenaSize.height);
    // 火堆
    this.renderCampfire(ctx, cam);
    // 玩家
    for (const [id, p] of this.players) {
      this.renderPlayer(ctx, cam, p, id === this.selfId);
    }
    // 浮动文字
    for (const t of this.floatingTexts) {
      ctx.font = `bold ${t.size || 14}px sans-serif`;
      ctx.fillStyle = t.color;
      ctx.globalAlpha = Math.min(1, t.life / 20);
      ctx.textAlign = 'center';
      ctx.fillText(t.text, t.x - cam.x, t.y - cam.y);
      ctx.globalAlpha = 1;
    }
  }

  renderCampfire(ctx, cam) {
    const cx = this.campfire.x - cam.x;
    const cy = this.campfire.y - cam.y;
    // 光晕
    const glow = ctx.createRadialGradient(cx, cy, 5, cx, cy, 60);
    glow.addColorStop(0, 'rgba(255,150,50,0.3)');
    glow.addColorStop(1, 'rgba(255,150,50,0)');
    ctx.fillStyle = glow;
    ctx.fillRect(cx - 60, cy - 60, 120, 120);
    // 木柴
    ctx.fillStyle = '#8B4513';
    ctx.fillRect(cx - 12, cy - 4, 24, 8);
    ctx.fillRect(cx - 4, cy - 12, 8, 24);
    // 火焰
    const flicker = Math.sin(this.animFrame * 0.15) * 3;
    ctx.fillStyle = '#ff6600';
    ctx.beginPath();
    ctx.arc(cx, cy - 8 + flicker, 8, 0, Math.PI * 2);
    ctx.fill();
    ctx.fillStyle = '#ffcc00';
    ctx.beginPath();
    ctx.arc(cx, cy - 10 + flicker, 5, 0, Math.PI * 2);
    ctx.fill();
  }

  renderPlayer(ctx, cam, p, isSelf) {
    const px = p.x - cam.x;
    const py = p.y - cam.y;
    if (px < -50 || px > this.canvas.width + 50 || py < -50 || py > this.canvas.height + 50) return;
    const isSelected = this.selectedTarget === p.char_id;
    // 阴影
    ctx.fillStyle = 'rgba(0,0,0,0.3)';
    ctx.beginPath();
    ctx.ellipse(px, py + 16, 14, 6, 0, 0, Math.PI * 2);
    ctx.fill();
    if (p.dead) {
      ctx.globalAlpha = 0.4;
    }
    // 选中指示
    if (isSelected && !isSelf) {
      ctx.strokeStyle = '#ff0';
      ctx.lineWidth = 2;
      ctx.beginPath();
      ctx.arc(px, py, 22, 0, Math.PI * 2);
      ctx.stroke();
    }
    // 身体
    const bodyColor = p.class === 'warrior' ? '#e94560' : '#4fc3f7';
    const isWalking = this.animFrame % 30 < 15;
    // 腿
    ctx.fillStyle = '#555';
    if (isWalking && !p.dead) {
      ctx.fillRect(px - 6, py + 6, 5, 10);
      ctx.fillRect(px + 1, py + 4, 5, 10);
    } else {
      ctx.fillRect(px - 5, py + 6, 4, 10);
      ctx.fillRect(px + 1, py + 6, 4, 10);
    }
    // 躯干
    ctx.fillStyle = bodyColor;
    ctx.fillRect(px - 8, py - 8, 16, 16);
    // 头
    ctx.fillStyle = '#ffd5a0';
    ctx.beginPath();
    ctx.arc(px, py - 14, 8, 0, Math.PI * 2);
    ctx.fill();
    // 武器
    if (p.class === 'warrior') {
      ctx.fillStyle = '#aaa';
      ctx.fillRect(px + 10, py - 10, 3, 18);
      ctx.fillStyle = '#ddd';
      ctx.fillRect(px + 8, py - 12, 7, 4);
    } else {
      ctx.fillStyle = '#8B4513';
      ctx.fillRect(px + 10, py - 16, 2, 22);
      ctx.strokeStyle = '#aaa';
      ctx.lineWidth = 1;
      ctx.beginPath();
      ctx.arc(px + 11, py - 5, 10, -0.8, 0.8);
      ctx.stroke();
    }
    ctx.globalAlpha = 1;
    // 名字
    ctx.font = '11px sans-serif';
    ctx.textAlign = 'center';
    ctx.fillStyle = isSelf ? '#ffeb3b' : '#fff';
    ctx.fillText(p.name, px, py - 26);
    // 血条
    if (!p.dead) {
      const bw = 30;
      const bh = 4;
      const bx = px - bw / 2;
      const by = py - 34;
      ctx.fillStyle = '#333';
      ctx.fillRect(bx, by, bw, bh);
      const ratio = Math.max(0, p.hp / p.max_hp);
      ctx.fillStyle = ratio > 0.5 ? '#4caf50' : ratio > 0.2 ? '#ff9800' : '#f44336';
      ctx.fillRect(bx, by, bw * ratio, bh);
    }
    // 死亡标记
    if (p.dead) {
      ctx.font = 'bold 16px sans-serif';
      ctx.fillStyle = '#f44336';
      ctx.fillText('💀', px, py);
    }
  }

  handleClick(e) {
    const rect = this.canvas.getBoundingClientRect();
    const mx = e.clientX - rect.left + this.camera.x;
    const my = e.clientY - rect.top + this.camera.y;
    let closest = null;
    let closestDist = 30;
    for (const [id, p] of this.players) {
      if (id === this.selfId || p.dead) continue;
      const d = Math.hypot(p.x - mx, p.y - my);
      if (d < closestDist) {
        closest = id;
        closestDist = d;
      }
    }
    this.selectedTarget = closest;
  }

  attackTarget() {
    if (!this.selectedTarget) return;
    const target = this.players.get(this.selectedTarget);
    if (!target || target.dead) return;
    ws.send('attack', { target_id: this.selectedTarget });
  }

  castSkill(skillId) {
    const now = Date.now();
    const cd = this.skillCooldowns[skillId];
    if (cd && now < cd) return;
    const self = this.players.get(this.selfId);
    if (!self) return;
    let targetX = self.x, targetY = self.y;
    let targetId = 0;
    if (this.selectedTarget) {
      const t = this.players.get(this.selectedTarget);
      if (t) { targetX = t.x; targetY = t.y; targetId = this.selectedTarget; }
    }
    ws.send('cast_skill', { skill_id: skillId, target_id: targetId, target_x: targetX, target_y: targetY });
    const skill = this.skills.find(s => s.id === skillId);
    if (skill) {
      this.skillCooldowns[skillId] = now + skill.cooldown * 1000;
      this.updateSkillBarCooldowns();
    }
  }

  renderSkillBar() {
    const bar = document.getElementById('skill-bar');
    bar.innerHTML = '';
    this.skills.forEach((sk, i) => {
      const btn = document.createElement('div');
      btn.className = 'skill-btn';
      btn.dataset.skillId = sk.id;
      btn.innerHTML = `<div>${sk.name}</div><div class="mp-cost">MP:${sk.mp_cost}</div><div class="cd-overlay"></div>`;
      btn.addEventListener('click', () => this.castSkill(sk.id));
      bar.appendChild(btn);
    });
    // 快捷键 1-4
    window.addEventListener('keydown', (e) => {
      const idx = parseInt(e.key) - 1;
      if (idx >= 0 && idx < this.skills.length) {
        this.castSkill(this.skills[idx].id);
      }
      if (e.key === ' ') {
        e.preventDefault();
        this.attackTarget();
      }
    });
    this.cdInterval = setInterval(() => this.updateSkillBarCooldowns(), 100);
  }

  updateSkillBarCooldowns() {
    const now = Date.now();
    document.querySelectorAll('.skill-btn').forEach(btn => {
      const sid = parseInt(btn.dataset.skillId);
      const cd = this.skillCooldowns[sid];
      const overlay = btn.querySelector('.cd-overlay');
      if (cd && now < cd) {
        btn.classList.add('on-cd');
        overlay.textContent = ((cd - now) / 1000).toFixed(1) + 's';
      } else {
        btn.classList.remove('on-cd');
        overlay.textContent = '';
      }
    });
  }

  addFloatingText(x, y, text, color = '#fff', size = 14) {
    this.floatingTexts.push({ x, y: y - 20, text, color, size, life: 40 });
  }

  // 网络事件处理
  onPlayerJoined(data) {
    this.players.set(data.char_id, { ...data, targetX: data.x, targetY: data.y });
  }

  onPlayerLeft(data) {
    this.players.delete(data.char_id);
    if (this.selectedTarget === data.char_id) this.selectedTarget = null;
  }

  onPlayerMoved(data) {
    const p = this.players.get(data.char_id);
    if (p) {
      p.targetX = data.x;
      p.targetY = data.y;
      p.direction = data.direction;
    }
  }

  onDamage(data) {
    const t = this.players.get(data.target_id);
    if (t) {
      t.hp = data.target_hp;
      t.max_hp = data.target_max_hp;
      const color = data.is_crit ? '#ff0' : '#f44336';
      const size = data.is_crit ? 18 : 14;
      const txt = data.is_crit ? `暴击! ${Math.round(data.damage)}` : `${Math.round(data.damage)}`;
      this.addFloatingText(t.x, t.y, txt, color, size);
    }
    // 更新 HUD
    const self = this.players.get(this.selfId);
    if (self) this.updateHUD(self);
  }

  onPlayerDied(data) {
    const p = this.players.get(data.char_id);
    if (p) p.dead = true;
    this.addFloatingText(p ? p.x : 400, p ? p.y : 300, `${data.name} 被 ${data.killer} 击杀`, '#f44336', 16);
  }

  onPlayerRespawn(data) {
    const p = this.players.get(data.char_id);
    if (p) {
      p.dead = false;
      p.x = data.x; p.y = data.y;
      p.targetX = data.x; p.targetY = data.y;
      p.hp = data.hp; p.max_hp = data.max_hp;
      p.mp = data.mp; p.max_mp = data.max_mp;
    }
    this.addFloatingText(data.x, data.y, `${data.name} 复活了`, '#4caf50', 14);
  }

  onSkillCasted(data) {
    if (data.caster_id === this.selfId) {
      const self = this.players.get(this.selfId);
      if (self) { self.mp = data.caster_mp; self.max_mp = data.caster_max_mp; }
    }
    const caster = this.players.get(data.caster_id);
    if (caster) {
      this.addFloatingText(caster.x, caster.y - 10, data.skill_name, '#ffab40', 13);
    }
    this.updateHUD(this.players.get(this.selfId));
  }

  updateHUD(self) {
    if (!self) return;
    const hpBar = document.querySelector('#hud-hp-bar .bar-fill');
    const hpText = document.querySelector('#hud-hp-bar .bar-text');
    const mpBar = document.querySelector('#hud-mp-bar .bar-fill');
    const mpText = document.querySelector('#hud-mp-bar .bar-text');
    hpBar.style.width = `${(self.hp / self.max_hp) * 100}%`;
    hpText.textContent = `HP ${Math.round(self.hp)}/${Math.round(self.max_hp)}`;
    mpBar.style.width = `${(self.mp / self.max_mp) * 100}%`;
    mpText.textContent = `MP ${Math.round(self.mp)}/${Math.round(self.max_mp)}`;
  }
}

const arena = new ArenaRenderer();
