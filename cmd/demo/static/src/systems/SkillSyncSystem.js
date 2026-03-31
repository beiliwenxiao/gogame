/**
 * SkillSyncSystem - 后端技能数据同步到前端 ECS 组件
 *
 * 将后端技能列表注入到玩家实体的 CombatComponent，
 * 供 BottomControlBar 和键盘快捷键使用。
 *
 * 使用方式：
 *   SkillSyncSystem.loadFromBackend(playerEntity, backendSkills)
 */
export class SkillSyncSystem {
    // 技能名 -> 图标 emoji 映射
    static ICON_MAP = {
        '猛击':     '🪓',
        '旋风斩':   '🌀',
        '战吼':     '📯',
        '射击':     '🏹',
        '多重射击': '🎯',
        '闪电箭':   '⚡',
        '天降箭雨': '🌧️'
    };

    // 技能名 -> 图标光晕颜色映射
    static GLOW_MAP = {
        '猛击': 'rgba(180, 0, 0, 0.4)',
        '战吼': 'rgba(180, 0, 30, 0.35)'
    };

    /**
     * 将后端技能数据注入到玩家实体的 CombatComponent
     * @param {Object} playerEntity - 玩家 ECS 实体（需有 combat 组件）
     * @param {Array}  backendSkills - 后端技能数组（含 id, name, mp_cost, cooldown 等）
     */
    static loadFromBackend(playerEntity, backendSkills) {
        if (!playerEntity || !backendSkills || backendSkills.length === 0) return;

        const combat = playerEntity.getComponent('combat');
        if (!combat) return;

        // 清空现有技能
        combat.skills = [];
        combat.skillCooldowns = new Map();

        // 只取需要 MP 的技能（跳过普通攻击）
        const skills = backendSkills.filter(s => s.mp_cost > 0);

        for (const sk of skills) {
            combat.addSkill({
                id:        `backend_${sk.id}`,
                backendId: sk.id,
                name:      sk.name,
                cooldown:  sk.cooldown,
                manaCost:  sk.mp_cost,
                effectType: sk.name,
                castTime:  0,
                icon:      SkillSyncSystem.ICON_MAP[sk.name] || '⚡',
                iconGlow:  SkillSyncSystem.GLOW_MAP[sk.name] || null
            });
        }

        console.log('SkillSyncSystem: 注入技能', skills.map(s => s.name));
    }
}
