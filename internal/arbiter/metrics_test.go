package arbiter

import (
	"math"
	"testing"

	"rammed-earth-roof-beam-clearance/internal/rules"
)

func TestMoistureContent(t *testing.T) {
	// 10g water in 90g dry -> 11.111% = 11111 permille
	got, err := MoistureContent(10, 90)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 11111 {
		t.Fatalf("MoistureContent=%d want 11111", got)
	}
	if _, err := MoistureContent(10, 0); err == nil || !rules.Is(err, rules.CodeDivideByZero) {
		t.Fatalf("want DIVIDE_BY_ZERO, got %v", err)
	}
}

func TestDryDensity(t *testing.T) {
	// 1800g in 1 litre -> 1800.000 g/L
	got, err := DryDensity(1800, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 1800*rules.FixedScale {
		t.Fatalf("DryDensity=%d want %d", got, 1800*rules.FixedScale)
	}
	if _, err := DryDensity(1800, 0); err == nil || !rules.Is(err, rules.CodeDivideByZero) {
		t.Fatalf("want DIVIDE_BY_ZERO, got %v", err)
	}
}

func TestCompactiveEnergy(t *testing.T) {
	// weight=10, fall=1, blows=100, volume=1 -> 1000
	got, err := CompactiveEnergy(10, 1, 100, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 1000 {
		t.Fatalf("CompactiveEnergy=%d want 1000", got)
	}
	if _, err := CompactiveEnergy(math.MaxInt64, math.MaxInt64, 2, 1); err == nil || !rules.Is(err, rules.CodeFixedOverflow) {
		t.Fatalf("want FIXED_OVERFLOW, got %v", err)
	}
	if _, err := CompactiveEnergy(10, 1, 100, 0); err == nil || !rules.Is(err, rules.CodeDivideByZero) {
		t.Fatalf("want DIVIDE_BY_ZERO, got %v", err)
	}
}

func TestErosionLossRate(t *testing.T) {
	// 5g lost from 100g -> 5% = 5000 permille
	got, err := ErosionLossRate(5, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 5000 {
		t.Fatalf("ErosionLossRate=%d want 5000", got)
	}
}

func TestVerticalDeviation(t *testing.T) {
	// 3mm deviation over 1000mm height -> 0.003 = 3 permille
	got, err := VerticalDeviation(3, 1000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 3 {
		t.Fatalf("VerticalDeviation=%d want 3", got)
	}
	if _, err := VerticalDeviation(3, 0); err == nil || !rules.Is(err, rules.CodeDivideByZero) {
		t.Fatalf("want DIVIDE_BY_ZERO, got %v", err)
	}
}
