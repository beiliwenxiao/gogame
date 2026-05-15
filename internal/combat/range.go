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

// ---------- 通用区域判定 ----------

// ZoneConfig 安全区/兴趣区域配置
type ZoneConfig struct {
	CenterX, CenterY float64
	Radius           float64 // 椭圆水平半径（ellipse/circle 用）
	Shape            string  // "ellipse", "circle", "rect"
	// rect 专用
	Width, Height float64
}

// IsInZone 通用区域判定。
// 支持三种形状：
//   - "ellipse": 2.5D 椭圆，rx=Radius, ry=Radius/2
//   - "circle":  标准圆形，半径=Radius
//   - "rect":    矩形，宽=Width, 高=Height
//
// 未知形状返回 false。
func IsInZone(zone *ZoneConfig, x, y float64) bool {
	dx := x - zone.CenterX
	dy := y - zone.CenterY
	switch zone.Shape {
	case "ellipse":
		rx := zone.Radius
		ry := zone.Radius / 2
		if rx == 0 || ry == 0 {
			return false
		}
		return (dx*dx)/(rx*rx)+(dy*dy)/(ry*ry) <= 1
	case "circle":
		return dx*dx+dy*dy <= zone.Radius*zone.Radius
	case "rect":
		return math.Abs(dx) <= zone.Width/2 && math.Abs(dy) <= zone.Height/2
	default:
		return false
	}
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
