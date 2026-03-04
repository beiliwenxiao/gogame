// 修罗斗场 - 引擎模式竞技场渲染器
// 使用 html5-mmrpg-game 引擎的 SceneManager + ArenaScene

class ArenaRenderer {
    constructor() {
        this.canvas = null;
        this.ctx = null;
        this.selfId = 0;
        this.running = false;
        this.skills = [];
        this.skillCooldowns = {};
        this.selectedTarget = null;

        // 引擎模块（动态加载）
        this.sceneManager = null;
        this.assetManager = null;
        this.arenaScene = null;
        this.engineReady = false;

        // 兼容旧接口的 players Map
        this.players = new Map();
    }

    init(canvasId) {
        this.canvas = document.getElementById('engineCanvas') || document.getElementById(canvasId);
        if (this.canvas) {
            this.ctx = this.canvas.getContext('2d');
            this.resize();
            window.addEventListener('resize', () => this.resize());
        }
    }

    resize() {
        if (!this.canvas) return;
        // canvas 直接使用容器实际尺寸，不做 CSS 缩放
        const container = this.canvas.parentElement;
        const cw = (container && container.clientWidth > 0) ? container.clientWidth : window.innerWidth;
        const ch = (container && container.clientHeight > 0) ? container.clientHeight : (window.innerHeight - 120);
        this.canvas.width = cw;
        this.canvas.height = ch;
        this.canvas.style.width = cw + 'px';
        this.canvas.style.height = ch + 'px';
    }

    async start(state) {
        this.selfId = state.self_id;
        this.skills = state.skills || [];
        this.running = true;

        // 构建兼容的 players Map（用于 HUD 更新）
        this.players.clear();
        for (const p of state.players) {
            this.players.set(p.char_id, { ...p, targetX: p.x, targetY: p.y });
        }

        // 不再渲染 HTML 技能栏，改用引擎 BottomControlBar

        // 动态加载引擎模块
        try {
            await this.initEngine(state);
        } catch (e) {
            console.error('引擎初始化失败，回退到基础渲染:', e);
            this.engineReady = false;
            this.fallbackLoop();
            return;
        }

        // 启动引擎渲染循环
        this.engineLoop();
    }

    async initEngine(state) {
        const { SceneManager } = await import('../src/core/SceneManager.js');
        const { AssetManager } = await import('../src/core/AssetManager.js');
        const { ArenaScene } = await import('../src/demo/scenes/ArenaScene.js');

        this.assetManager = new AssetManager();
        this.assetManager.loadPlaceholderAssets();

        this.sceneManager = new SceneManager();
        this.sceneManager.setRenderSize(this.canvas.width, this.canvas.height);

        this.arenaScene = new ArenaScene();
        this.arenaScene.assetManager = this.assetManager;
        this.arenaScene.setSceneManager(this.sceneManager);
        this.arenaScene.setWebSocket(ws); // ws 是全局的 WebSocket 实例

        this.sceneManager.registerScene('ArenaScene', this.arenaScene);
        this.sceneManager.switchTo('ArenaScene', state);

        this.engineReady = true;
        console.log('引擎竞技场场景初始化完成');
    }

    stop() {
        this.running = false;
        this.players.clear();
        if (this.arenaScene) {
            this.arenaScene.exit();
            this.arenaScene = null;
        }
        this.sceneManager = null;
        this.engineReady = false;
    }

    // ===== 引擎渲染循环 =====

    engineLoop() {
        if (!this.running) return;

        const now = performance.now();
        if (!this._lastTime) this._lastTime = now;
        const dt = (now - this._lastTime) / 1000;
        this._lastTime = now;

        if (this.ctx && this.sceneManager) {
            this.ctx.clearRect(0, 0, this.canvas.width, this.canvas.height);
            this.sceneManager.update(dt);
            this.sceneManager.render(this.ctx);
        }

        requestAnimationFrame(() => this.engineLoop());
    }

    // ===== 基础回退渲染（引擎加载失败时） =====

    fallbackLoop() {
        if (!this.running) return;
        if (this.ctx) {
            this.ctx.fillStyle = '#1a1a2e';
            this.ctx.fillRect(0, 0, this.canvas.width, this.canvas.height);
            this.ctx.fillStyle = '#ff6666';
            this.ctx.font = '24px Arial';
            this.ctx.textAlign = 'center';
            this.ctx.fillText('引擎加载失败，请刷新页面重试', this.canvas.width / 2, this.canvas.height / 2);
        }
        requestAnimationFrame(() => this.fallbackLoop());
    }

    // ===== 技能栏 DOM 渲染 =====

    renderSkillBar() {
        const bar = document.getElementById('skill-bar');
        if (!bar) return;
        bar.innerHTML = '';
        this.skills.forEach((sk) => {
            const btn = document.createElement('div');
            btn.className = 'skill-btn';
            btn.dataset.skillId = sk.id;
            btn.innerHTML = `<div>${sk.name}</div><div class="mp-cost">MP:${sk.mp_cost}</div><div class="cd-overlay"></div>`;
            btn.addEventListener('click', () => this.castSkill(sk.id));
            bar.appendChild(btn);
        });

        // 键盘快捷键
        this._skillKeyHandler = (e) => {
            const idx = parseInt(e.key) - 1;
            if (idx >= 0 && idx < this.skills.length) this.castSkill(this.skills[idx].id);
            if (e.key === ' ') { e.preventDefault(); this.attackTarget(); }
        };
        window.addEventListener('keydown', this._skillKeyHandler);

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
                if (overlay) overlay.textContent = ((cd - now) / 1000).toFixed(1) + 's';
            } else {
                btn.classList.remove('on-cd');
                if (overlay) overlay.textContent = '';
            }
        });
    }

    // ===== 攻击和技能 =====

    attackTarget() {
        if (this.engineReady && this.arenaScene) {
            this.arenaScene.attackTarget();
        } else if (this.selectedTarget) {
            ws.send('attack', { target_id: this.selectedTarget });
        }
    }

    castSkill(skillId) {
        const now = Date.now();
        const cd = this.skillCooldowns[skillId];
        if (cd && now < cd) return;

        if (this.engineReady && this.arenaScene) {
            this.arenaScene.castSkill(skillId);
        } else {
            // 回退模式
            ws.send('cast_skill', { skill_id: skillId, target_id: 0, target_x: 0, target_y: 0 });
        }

        const skill = this.skills.find(s => s.id === skillId);
        if (skill) {
            this.skillCooldowns[skillId] = now + skill.cooldown * 1000;
            this.updateSkillBarCooldowns();
        }
    }

    // ===== 网络事件处理（委托给引擎场景） =====

    onPlayerJoined(data) {
        this.players.set(data.char_id, { ...data, targetX: data.x, targetY: data.y });
        if (this.engineReady && this.arenaScene) {
            this.arenaScene.onPlayerJoined(data);
        }
    }

    onPlayerLeft(data) {
        this.players.delete(data.char_id);
        if (this.engineReady && this.arenaScene) {
            this.arenaScene.onPlayerLeft(data);
        }
    }

    onPlayerMoved(data) {
        const p = this.players.get(data.char_id);
        if (p) { p.targetX = data.x; p.targetY = data.y; p.direction = data.direction; }
        if (this.engineReady && this.arenaScene) {
            this.arenaScene.onPlayerMoved(data);
        }
    }

    onDamage(data) {
        // 更新兼容 Map
        const t = this.players.get(data.target_id);
        if (t) { t.hp = data.target_hp; t.max_hp = data.target_max_hp; }

        if (this.engineReady && this.arenaScene) {
            this.arenaScene.onDamage(data);
        }

        // 更新 HUD
        const self = this.players.get(this.selfId);
        if (self) this.updateHUD(self);
    }

    onPlayerDied(data) {
        const p = this.players.get(data.char_id);
        if (p) p.dead = true;
        if (this.engineReady && this.arenaScene) {
            this.arenaScene.onPlayerDied(data);
        }
    }

    onPlayerRespawn(data) {
        const p = this.players.get(data.char_id);
        if (p) {
            p.dead = false; p.x = data.x; p.y = data.y;
            p.targetX = data.x; p.targetY = data.y;
            p.hp = data.hp; p.max_hp = data.max_hp;
            p.mp = data.mp; p.max_mp = data.max_mp;
        }
        if (this.engineReady && this.arenaScene) {
            this.arenaScene.onPlayerRespawn(data);
        }
    }

    onSkillCasted(data) {
        if (data.caster_id === this.selfId) {
            const self = this.players.get(this.selfId);
            if (self) { self.mp = data.caster_mp; self.max_mp = data.caster_max_mp; }
        }
        if (this.engineReady && this.arenaScene) {
            this.arenaScene.onSkillCasted(data);
        }
        this.updateHUD(this.players.get(this.selfId));
    }

    onStateSync(data) {
        if (!data || !data.players) return;
        // 增量更新 players Map
        for (const p of data.players) {
            const existing = this.players.get(p.char_id);
            if (existing) {
                Object.assign(existing, p);
                if (p.x !== undefined) existing.targetX = p.x;
                if (p.y !== undefined) existing.targetY = p.y;
            } else if (p.name) {
                // 新玩家（有 name 字段说明是全量数据）
                this.players.set(p.char_id, { ...p, targetX: p.x, targetY: p.y });
            }
        }
        // 注意：增量同步不再移除玩家，玩家离开由 player_left 事件处理
        // 委托给引擎场景
        if (this.engineReady && this.arenaScene) {
            this.arenaScene.onStateSync(data);
        }
        // 更新 HUD
        const self = this.players.get(this.selfId);
        if (self) this.updateHUD(self);
    }

    // ===== NPC 事件处理 =====

    onNPCSpawn(data) {
        if (this.engineReady && this.arenaScene) {
            this.arenaScene.onNPCSpawn(data);
        }
    }

    onNPCDied(data) {
        if (this.engineReady && this.arenaScene) {
            this.arenaScene.onNPCDied(data);
        }
    }

    onNPCUpdate(data) {
        if (this.engineReady && this.arenaScene) {
            this.arenaScene.onNPCUpdate(data);
        }
    }

    onNPCDrop(data) {
        if (this.engineReady && this.arenaScene) {
            this.arenaScene.onNPCDrop(data);
        }
    }

    // ===== HUD 更新（复用原有 DOM） =====

    updateHUD(self) {
        if (!self) return;
        const hpRatio = Math.max(0, self.hp / self.max_hp);
        const mpRatio = Math.max(0, (self.mp || 0) / (self.max_mp || 1));

        const hpFill = document.querySelector('.hp-orb .hud-orb-fill');
        const hpText = document.querySelector('.hp-orb .hud-orb-text');
        const mpFill = document.querySelector('.mp-orb .hud-orb-fill');
        const mpText = document.querySelector('.mp-orb .hud-orb-text');
        if (hpFill) hpFill.style.height = `${hpRatio * 100}%`;
        if (hpText) hpText.textContent = `${Math.round(self.hp)}`;
        if (mpFill) mpFill.style.height = `${mpRatio * 100}%`;
        if (mpText) mpText.textContent = `${Math.round(self.mp || 0)}`;

        const hpBar = document.querySelector('#hud-hp-bar .bar-fill');
        const hpBarText = document.querySelector('#hud-hp-bar .bar-text');
        const mpBar = document.querySelector('#hud-mp-bar .bar-fill');
        const mpBarText = document.querySelector('#hud-mp-bar .bar-text');
        if (hpBar) hpBar.style.width = `${hpRatio * 100}%`;
        if (hpBarText) hpBarText.textContent = `HP ${Math.round(self.hp)}/${Math.round(self.max_hp)}`;
        if (mpBar) mpBar.style.width = `${mpRatio * 100}%`;
        if (mpBarText) mpBarText.textContent = `MP ${Math.round(self.mp || 0)}/${Math.round(self.max_mp || 0)}`;
    }
}

const arena = new ArenaRenderer();
