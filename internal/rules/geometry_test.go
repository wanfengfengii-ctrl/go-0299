package rules

import (
	"math"
	"testing"

	"rammed-earth-roof-beam-clearance/internal/domain"
)

func rect(x, y, w, h int64) domain.Rect { return domain.Rect{X: x, Y: y, W: w, H: h} }

func TestRectArea(t *testing.T) {
	if got, err := RectArea(rect(0, 0, 10, 5)); err != nil || got != 50 {
		t.Fatalf("RectArea=%d,%v want 50,nil", got, err)
	}
	if _, err := RectArea(rect(0, 0, -1, 5)); err == nil || !Is(err, CodeGeometryNegative) {
		t.Fatalf("want GEOMETRY_NEGATIVE, got %v", err)
	}
	if _, err := RectArea(rect(0, 0, 0, 5)); err == nil || !Is(err, CodeGeometryDegenerate) {
		t.Fatalf("want GEOMETRY_DEGENERATE, got %v", err)
	}
	if _, err := RectArea(rect(0, 0, math.MaxInt64, 2)); err == nil || !Is(err, CodeGeometryOverflow) {
		t.Fatalf("want GEOMETRY_OVERFLOW, got %v", err)
	}
}

func TestValidateGeometryValid(t *testing.T) {
	g := domain.WallGeometry{
		Wall: rect(0, 0, 1000, 2000),
		Layers: []domain.Layer{
			{Number: 1, Rect: rect(0, 0, 1000, 1000)},
			{Number: 2, Rect: rect(0, 1000, 1000, 1000)},
		},
		Cells: []domain.Cell{
			{Layer: 1, Seq: 0, Rect: rect(0, 0, 1000, 1000)},
			{Layer: 2, Seq: 0, Rect: rect(0, 1000, 1000, 1000)},
		},
	}
	if err := ValidateGeometry(g); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}

func TestValidateGeometryOverlap(t *testing.T) {
	g := domain.WallGeometry{
		Wall: rect(0, 0, 1000, 2000),
		Cells: []domain.Cell{
			{Layer: 1, Seq: 0, Rect: rect(0, 0, 600, 1000)},
			{Layer: 1, Seq: 1, Rect: rect(500, 0, 600, 1000)},
		},
	}
	err := ValidateGeometry(g)
	if err == nil || err.Code != CodeGeometryOverlap {
		t.Fatalf("want GEOMETRY_OVERLAP, got %v", err)
	}
}

func TestValidateGeometryForbiddenZone(t *testing.T) {
	g := domain.WallGeometry{
		Wall: rect(0, 0, 1000, 2000),
		Openings: []domain.Opening{
			{Name: "window", Rect: rect(400, 400, 200, 200)},
		},
		Cells: []domain.Cell{
			{Layer: 1, Seq: 0, Rect: rect(0, 0, 1000, 1000)},
		},
	}
	err := ValidateGeometry(g)
	if err == nil || err.Code != CodeForbiddenZone {
		t.Fatalf("want FORBIDDEN_ZONE, got %v", err)
	}
}

func TestValidateGeometryLayerOutOfRange(t *testing.T) {
	g := domain.WallGeometry{
		Wall: rect(0, 0, 1000, 1000),
		Layers: []domain.Layer{
			{Number: 1, Rect: rect(0, 0, 1000, 2000)},
		},
	}
	err := ValidateGeometry(g)
	if err == nil || err.Code != CodeLayerOutOfRange {
		t.Fatalf("want LAYER_OUT_OF_RANGE, got %v", err)
	}
}
