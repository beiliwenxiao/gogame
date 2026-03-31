package combat

import (
	"math"
	"math/rand"
)

// ---------- 2.5D 范围判定工具函数 ----------
// 用于多人游戏场景中的技能/攻击范围判定，支持等距视角（2.5D）。
// 前后端必须保持一致：Y 轴压缩比均为 0.5（ry = rx / 2）。

// Distance 计算两点之间的欧氏距离。
func Distance(x1, y1, x2, y2 float64) float64 {
	dx := x1 - x2
	dy := y1 - y2
	return math.Sqrt(dx*dx + dy*dy)
}

// IsInEllipseRange 2.5D 椭圆范围判定。
// 等距视角下 Y 轴被压缩为 0.5，水平半径 rx=radius，垂直半径 ry=radius/2。
func IsInEllipseRange(cx, cy, tx, ty, radius float64) bool {
	dx := tx - cx
	dy := ty - cy
	rx := radius
	ry := radius / 2
	return (dx*dx)/(rx*rx)+(dy*dy)/(ry*ry) <= 1
}

// IsInFanRange 2.5D 扇形范围判定。
// dir 为攻击方向角（弧度），halfAngle 为半角（弧度）。
// Y 轴乘以 2 还原等距压缩后再计算角度。
func IsInFanRange(cx, cy, tx, ty, radius, dir, halfAngle float64) bool {
	dx := tx - cx
	dy := (ty - cy) * 2 // 还原 2.5D Y 轴压缩
	dist := math.Sqrt(dx*dx + dy*dy)
	if dist > radius {
		return false
	}
	angle := math.Atan2(dy, dx)
	diff := angle - dir
	for diff > math.Pi {
		diff -= math.Pi * 2
	}
	for diff < -math.Pi {
		diff += math.Pi * 2
	}
	return math.Abs(diff) <= halfAngle
}

// CalcDamage 基础伤害计算公式：atk - def*0.5，带随机浮动 ±15%。
// 最低伤害为 1。
func CalcDamage(atk, def float64) float64 {
	base := atk - def*0.5
	if base < 1 {
		base = 1
	}
	return base * (0.85 + rand.Float64()*0.3)
}
