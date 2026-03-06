// 修罗斗场 Demo 主应用
const APP = {
  accountId: 0,
  username: '',
  character: null,
  equipments: [],
  skills: [],
  selectedClass: 'warrior',
};

// 页面切换
function showPage(id) {
  document.querySelectorAll('.page').forEach(p => p.classList.remove('active'));
  document.getElementById(id).classList.add('active');
}

// 显示消息
function showMsg(elemId, text, isError = true) {
  const el = document.getElementById(elemId);
  el.textContent = text;
  el.style.color = isError ? '#e94560' : '#4caf50';
}

// 初始化
async function init() {
  try {
    bindEvents();
    console.log('事件绑定完成');
  } catch (e) {
    console.error('绑定事件失败:', e);
  }
  try {
    bindWSHandlers();
    console.log('WS处理器绑定完成');
  } catch (e) {
    console.error('绑定WS处理器失败:', e);
  }
  await connectWS();
}

async function connectWS() {
  const host = location.host || 'localhost:9100';
  const proto = location.protocol === 'https:' ? 'wss' : 'ws';
  const btn = document.getElementById('btn-reconnect');
  if (btn) { btn.disabled = true; btn.textContent = '连接中...'; }
  try {
    await ws.connect(`${proto}://${host}/ws`);
    showMsg('auth-msg', '已连接服务器', false);
  } catch (e) {
    showMsg('auth-msg', '无法连接服务器，请点击"连接服务器"重试');
  } finally {
    if (btn) { btn.disabled = false; btn.textContent = '🔌 连接服务器'; }
  }
}

function bindEvents() {
  // 连接服务器
  document.getElementById('btn-reconnect').addEventListener('click', () => {
    connectWS();
  });

  // 登录/注册
  document.getElementById('btn-login').addEventListener('click', () => {
    console.log('登录按钮被点击');
    const u = document.getElementById('auth-user').value.trim();
    const p = document.getElementById('auth-pass').value;
    if (!ws.connected) { showMsg('auth-msg', '未连接服务器，请点击"连接服务器"按钮'); return; }
    if (!u || !p) { showMsg('auth-msg', '请输入用户名和密码'); return; }
    console.log('发送登录请求:', u);
    ws.send('login', { username: u, password: p });
  });
  document.getElementById('btn-register').addEventListener('click', () => {
    console.log('注册按钮被点击');
    const u = document.getElementById('auth-user').value.trim();
    const p = document.getElementById('auth-pass').value;
    if (!ws.connected) { showMsg('auth-msg', '未连接服务器，请点击"连接服务器"按钮'); return; }
    if (!u || !p) { showMsg('auth-msg', '请输入用户名和密码'); return; }
    console.log('发送注册请求:', u);
    ws.send('register', { username: u, password: p });
  });
  // 回车登录
  document.getElementById('auth-pass').addEventListener('keydown', (e) => {
    if (e.key === 'Enter') document.getElementById('btn-login').click();
  });

  // 职业选择
  document.querySelectorAll('.class-card').forEach(card => {
    card.addEventListener('click', () => {
      document.querySelectorAll('.class-card').forEach(c => c.classList.remove('selected'));
      card.classList.add('selected');
      APP.selectedClass = card.dataset.class;
    });
  });
  document.getElementById('btn-create').addEventListener('click', () => {
    const name = document.getElementById('char-name').value.trim();
    if (!name) { showMsg('create-msg', '请输入角色名称'); return; }
    ws.send('select_class', { account_id: APP.accountId, name, class: APP.selectedClass });
  });

  // 进入竞技场
  document.getElementById('btn-enter-arena').addEventListener('click', () => {
    ws.send('enter_arena');
  });
  document.getElementById('btn-leave-arena').addEventListener('click', () => {
    ws.send('leave_arena');
    arena.stop();
    showPage('page-lobby');
    ws.send('get_char_info');
  });

  // 聊天
  document.getElementById('btn-chat-send').addEventListener('click', sendChat);
  document.getElementById('chat-input').addEventListener('keydown', (e) => {
    if (e.key === 'Enter') sendChat();
  });
}

function sendChat() {
  const input = document.getElementById('chat-input');
  const content = input.value.trim();
  if (!content) return;
  ws.send('chat', { content });
  input.value = '';
}

function bindWSHandlers() {
  ws.on('error', (data) => {
    showMsg('auth-msg', data);
    showMsg('create-msg', data);
  });

  ws.on('register_ok', (data) => {
    showMsg('auth-msg', '注册成功，请登录', false);
  });

  ws.on('login_ok', (data) => {
    APP.accountId = data.account_id;
    APP.username = data.username;
    if (data.character) {
      APP.character = data.character;
      APP.skills = data.skills || [];
      APP.equipments = data.equipments || [];
      enterLobby();
    } else {
      showPage('page-create');
    }
  });

  ws.on('class_selected', (data) => {
    APP.character = data.character;
    APP.skills = data.skills || [];
    APP.equipments = data.equipments || [];
    enterLobby();
  });

  ws.on('char_info', (data) => {
    APP.character = data.character;
    APP.equipments = data.equipments || [];
    APP.skills = data.skills || [];
    renderLobby();
    // 如果在竞技场内，同步更新 ArenaScene 中自己的属性
    if (arena.engineReady && arena.arenaScene && arena.arenaScene.playerEntity) {
      const ch = data.character;
      const stats = arena.arenaScene.playerEntity.getComponent('stats');
      if (stats && ch) {
        stats.attack = ch.attack;
        stats.defense = ch.defense;
        stats.speed = ch.speed;
        if (ch.crit_rate !== undefined) stats.critRate = ch.crit_rate;
        if (ch.max_hp) {
          const hpRatio = stats.maxHp > 0 ? stats.hp / stats.maxHp : 1;
          stats.maxHp = ch.max_hp;
          stats.hp = Math.floor(stats.maxHp * hpRatio);
        }
        if (ch.max_mp) {
          const mpRatio = stats.maxMp > 0 ? stats.mp / stats.maxMp : 1;
          stats.maxMp = ch.max_mp;
          stats.mp = Math.floor(stats.maxMp * mpRatio);
        }
      }
      // 同步装备到 ArenaScene 的 EquipmentComponent
      if (data.equipments && arena.arenaScene.loadBackendEquipments) {
        arena.arenaScene.loadBackendEquipments(data.equipments);
      }
    }
  });

  ws.on('equip_list', (data) => {
    renderEquipList(data);
  });

  // 竞技场事件
  ws.on('arena_state', async (data) => {
    try {
      showPage('page-arena');
      arena.init('engineCanvas');
      await arena.start(data);
      const self = arena.players.get(arena.selfId);
      if (self) {
        document.getElementById('hud-name').textContent = self.name;
        arena.updateHUD(self);
      }
    } catch (e) {
      console.error('进入竞技场失败:', e);
    }
  });

  ws.on('player_joined', (data) => arena.onPlayerJoined(data));
  ws.on('player_left', (data) => arena.onPlayerLeft(data));
  ws.on('player_moved', (data) => arena.onPlayerMoved(data));
  ws.on('damage_dealt', (data) => arena.onDamage(data));
  ws.on('player_died', (data) => arena.onPlayerDied(data));
  ws.on('player_respawn', (data) => arena.onPlayerRespawn(data));
  ws.on('skill_casted', (data) => arena.onSkillCasted(data));
  ws.on('state_sync', (data) => arena.onStateSync(data));
  ws.on('npc_spawn', (data) => arena.onNPCSpawn(data));
  ws.on('npc_died', (data) => arena.onNPCDied(data));
  ws.on('npc_update', (data) => arena.onNPCUpdate(data));
  ws.on('npc_drop', (data) => arena.onNPCDrop(data));
  ws.on('campfire_tick', (data) => arena.onCampfireTick(data));

  ws.on('chat_msg', (data) => {
    const box = document.getElementById('chat-messages');
    const line = document.createElement('div');
    line.className = 'chat-line';
    line.innerHTML = `<span class="chat-name">${data.name}:</span> ${data.content}`;
    box.appendChild(line);
    box.scrollTop = box.scrollHeight;
  });

  ws.on('_disconnected', () => {
    showPage('page-auth');
    showMsg('auth-msg', '连接已断开，请重新登录');
  });
}

function enterLobby() {
  showPage('page-lobby');
  renderLobby();
  ws.send('get_equip_list');
}

function renderLobby() {
  const ch = APP.character;
  if (!ch) return;
  document.getElementById('lobby-title').textContent = `${ch.name} - ${ch.class === 'warrior' ? '战士' : '弓箭手'} Lv.${ch.level}`;
  // 属性
  const stats = document.getElementById('char-stats');
  stats.innerHTML = [
    ['生命', `${Math.round(ch.hp)}/${Math.round(ch.max_hp)}`],
    ['法力', `${Math.round(ch.mp)}/${Math.round(ch.max_mp)}`],
    ['攻击', Math.round(ch.attack)],
    ['防御', Math.round(ch.defense)],
    ['速度', Math.round(ch.speed)],
    ['暴击率', `${(ch.crit_rate * 100).toFixed(1)}%`],
  ].map(([k, v]) => `<div class="stat-row"><span class="stat-label">${k}</span><span class="stat-value">${v}</span></div>`).join('');

  // 装备栏
  const slots = document.getElementById('equip-slots');
  const slotNames = { weapon: '武器', helmet: '头盔', armor: '铠甲', boots: '鞋子' };
  slots.innerHTML = Object.entries(slotNames).map(([type, name]) => {
    const eq = APP.equipments.find(e => e.slot_type === type);
    if (eq) {
      return `<div class="equip-slot">
        <span class="slot-name">${name}</span>
        <span class="equip-name quality-${eq.def.quality}">${eq.def.name}</span>
        <button class="btn btn-small btn-secondary" onclick="doUnequip('${type}')">卸下</button>
      </div>`;
    }
    return `<div class="equip-slot"><span class="slot-name">${name}</span><span class="equip-name" style="color:#555">空</span></div>`;
  }).join('');

  // 技能
  const skillDiv = document.getElementById('skill-list');
  skillDiv.innerHTML = (APP.skills || []).map(sk => {
    const desc = sk.mp_cost > 0 ? `MP:${sk.mp_cost} CD:${sk.cooldown}s 范围:${sk.range}` : `CD:${sk.cooldown}s`;
    return `<div class="skill-item"><span class="sk-name">${sk.name}</span><div class="sk-desc">${desc} | ${sk.area_type}</div></div>`;
  }).join('');
}

function renderEquipList(list) {
  const div = document.getElementById('equip-list');
  div.innerHTML = (list || []).map(eq => {
    const stats = [];
    if (eq.attack > 0) stats.push(`攻+${eq.attack}`);
    if (eq.defense > 0) stats.push(`防+${eq.defense}`);
    if (eq.hp > 0) stats.push(`HP+${eq.hp}`);
    if (eq.speed > 0) stats.push(`速+${eq.speed}`);
    if (eq.crit_rate > 0) stats.push(`暴+${(eq.crit_rate * 100).toFixed(0)}%`);
    const slotNames = { weapon: '武器', helmet: '头盔', armor: '铠甲', boots: '鞋子' };
    return `<div class="equip-item" onclick="doEquip(${eq.id})">
      <div><span class="eq-name quality-${eq.quality}">${eq.name}</span> <small style="color:#666">[${slotNames[eq.slot_type] || eq.slot_type}]</small></div>
      <div class="eq-stats">${stats.join(' ')}</div>
    </div>`;
  }).join('');
}

function doEquip(defId) {
  ws.send('equip', { equip_def_id: defId });
  setTimeout(() => ws.send('get_equip_list'), 200);
}

function doUnequip(slotType) {
  ws.send('unequip', { slot_type: slotType });
  setTimeout(() => ws.send('get_equip_list'), 200);
}

// 启动
init();
