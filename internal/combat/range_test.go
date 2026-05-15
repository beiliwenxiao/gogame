package combat

import (
	"testing"
)

// ---------- IsInZone: ellipse ----------

func TestIsInZone_Ellipse_Center(t *testing.T) {
	zone := &ZoneConfig{CenterX: 0, CenterY: 464, Radius: 200, Shape: "ellipse"}
	if !IsInZone(zone, 0, 464) {
		t.Error("中心点应在椭圆区域内")
	}
}

func TestIsInZone_Ellipse_OnHorizontalEdge(t *testing.T) {
	zone := &ZoneConfig{CenterX: 0, CenterY: 464, Radius: 200, Shape: "ellipse"}
	// 水平边界 dx=200, dy=0 → (200/200)²+(0/100)² = 1 → 刚好在边界上
	if !IsInZone(zone, 200, 464) {
		t.Error("水平边界点应在椭圆区域内（含边界）")
	}
}

func TestIsInZone_Ellipse_OnVerticalEdge(t *testing.T) {
	zone := &ZoneConfig{CenterX: 0, CenterY: 464, Radius: 200, Shape: "ellipse"}
	// 垂直边界 dx=0, dy=100 → (0/200)²+(100/100)² = 1
	if !IsInZone(zone, 0, 564) {
		t.Error("垂直边界点应在椭圆区域内（含边界）")
	}
}

func TestIsInZone_Ellipse_OutsideHorizontal(t *testing.T) {
	zone := &ZoneConfig{CenterX: 0, CenterY: 464, Radius: 200, Shape: "ellipse"}
	if IsInZone(zone, 201, 464) {
		t.Error("超出水平半径的点不应在椭圆区域内")
	}
}

func TestIsInZone_Ellipse_OutsideVertical(t *testing.T) {
	zone := &ZoneConfig{CenterX: 0, CenterY: 464, Radius: 200, Shape: "ellipse"}
	if IsInZone(zone, 0, 565) {
		t.Error("超出垂直半径的点不应在椭圆区域内")
	}
}

func TestIsInZone_Ellipse_ConsistentWithIsInEllipseRange(t *testing.T) {
	// 验证 IsInZone(ellipse) 与 IsInEllipseRange 结果一致
	zone := &ZoneConfig{CenterX: 0, CenterY: 464, Radius: 200, Shape: "ellipse"}
	testPoints := [][2]float64{
		{0, 464}, {100, 464}, {200, 464}, {201, 464},
		{0, 564}, {0, 565}, {50, 500}, {150, 500},
		{-200, 464}, {0, 364},
	}
	for _, pt := range testPoints {
		got := IsInZone(zone, pt[0], pt[1])
		want := IsInEllipseRange(0, 464, pt[0], pt[1], 200)
		if got != want {
			t.Errorf("IsInZone vs IsInEllipseRange 不一致: point=(%.0f,%.0f) zone=%v ellipse=%v",
				pt[0], pt[1], got, want)
		}
	}
}

func TestIsInZone_Ellipse_ZeroRadius(t *testing.T) {
	zone := &ZoneConfig{CenterX: 0, CenterY: 0, Radius: 0, Shape: "ellipse"}
	if IsInZone(zone, 0, 0) {
		t.Error("半径为 0 的椭圆不应包含任何点")
	}
}

// ---------- IsInZone: circle ----------

func TestIsInZone_Circle_Center(t *testing.T) {
	zone := &ZoneConfig{CenterX: 100, CenterY: 100, Radius: 50, Shape: "circle"}
	if !IsInZone(zone, 100, 100) {
		t.Error("中心点应在圆形区域内")
	}
}

func TestIsInZone_Circle_OnEdge(t *testing.T) {
	zone := &ZoneConfig{CenterX: 100, CenterY: 100, Radius: 50, Shape: "circle"}
	if !IsInZone(zone, 150, 100) {
		t.Error("边界点应在圆形区域内（含边界）")
	}
}

func TestIsInZone_Circle_Outside(t *testing.T) {
	zone := &ZoneConfig{CenterX: 100, CenterY: 100, Radius: 50, Shape: "circle"}
	if IsInZone(zone, 151, 100) {
		t.Error("超出半径的点不应在圆形区域内")
	}
}

func TestIsInZone_Circle_Diagonal(t *testing.T) {
	zone := &ZoneConfig{CenterX: 0, CenterY: 0, Radius: 100, Shape: "circle"}
	// 对角线 (71,71): 71²+71² = 10082 > 10000 → 在外面
	if IsInZone(zone, 71, 71) {
		t.Error("对角线超出半径的点不应在圆形区域内")
	}
	// (70,70): 70²+70² = 9800 < 10000 → 在里面
	if !IsInZone(zone, 70, 70) {
		t.Error("对角线内的点应在圆形区域内")
	}
}

// ---------- IsInZone: rect ----------

func TestIsInZone_Rect_Center(t *testing.T) {
	zone := &ZoneConfig{CenterX: 50, CenterY: 50, Width: 100, Height: 60, Shape: "rect"}
	if !IsInZone(zone, 50, 50) {
		t.Error("中心点应在矩形区域内")
	}
}

func TestIsInZone_Rect_OnEdge(t *testing.T) {
	zone := &ZoneConfig{CenterX: 50, CenterY: 50, Width: 100, Height: 60, Shape: "rect"}
	// 右边界: |100-50| = 50 = 100/2 → 在边界上
	if !IsInZone(zone, 100, 50) {
		t.Error("右边界点应在矩形区域内（含边界）")
	}
	// 上边界: |80-50| = 30 = 60/2
	if !IsInZone(zone, 50, 80) {
		t.Error("上边界点应在矩形区域内（含边界）")
	}
}

func TestIsInZone_Rect_Outside(t *testing.T) {
	zone := &ZoneConfig{CenterX: 50, CenterY: 50, Width: 100, Height: 60, Shape: "rect"}
	if IsInZone(zone, 101, 50) {
		t.Error("超出宽度的点不应在矩形区域内")
	}
	if IsInZone(zone, 50, 81) {
		t.Error("超出高度的点不应在矩形区域内")
	}
}

func TestIsInZone_Rect_Corner(t *testing.T) {
	zone := &ZoneConfig{CenterX: 0, CenterY: 0, Width: 200, Height: 100, Shape: "rect"}
	// 角点 (100, 50): |100| <= 100 && |50| <= 50 → 在边界上
	if !IsInZone(zone, 100, 50) {
		t.Error("角点应在矩形区域内（含边界）")
	}
	// 角外 (101, 50)
	if IsInZone(zone, 101, 50) {
		t.Error("角外点不应在矩形区域内")
	}
}

// ---------- IsInZone: unknown shape ----------

func TestIsInZone_UnknownShape(t *testing.T) {
	zone := &ZoneConfig{CenterX: 0, CenterY: 0, Radius: 100, Shape: "triangle"}
	if IsInZone(zone, 0, 0) {
		t.Error("未知形状应返回 false")
	}
}

func TestIsInZone_EmptyShape(t *testing.T) {
	zone := &ZoneConfig{CenterX: 0, CenterY: 0, Radius: 100, Shape: ""}
	if IsInZone(zone, 0, 0) {
		t.Error("空形状应返回 false")
	}
}
