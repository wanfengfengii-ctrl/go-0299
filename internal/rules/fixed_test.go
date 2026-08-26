package rules

import (
	"math"
	"testing"
)

func TestHalfAwayFromZero(t *testing.T) {
	cases := []struct {
		name        string
		num, den    int64
		want        int64
		wantErr     bool
		wantErrCode Code
	}{
		{"exact", 10, 2, 5, false, ""},
		{"round half up positive", 5, 2, 3, false, ""},
		{"round half down positive", 4, 2, 2, false, ""},
		{"round half up negative", -5, 2, -3, false, ""},
		{"round half down negative", -4, 2, -2, false, ""},
		{"negative numerator", -3, 2, -2, false, ""},
		{"zero numerator", 0, 7, 0, false, ""},
		{"divide by zero", 1, 0, 0, true, CodeDivideByZero},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := HalfAwayFromZero(c.num, c.den)
			if c.wantErr {
				if err == nil {
					t.Fatalf("want error, got %d", got)
				}
				if !Is(err, c.wantErrCode) {
					t.Fatalf("want code %s, got %v", c.wantErrCode, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != c.want {
				t.Fatalf("HalfAwayFromZero(%d,%d)=%d want %d", c.num, c.den, got, c.want)
			}
		})
	}
}

func TestCheckedMulOverflow(t *testing.T) {
	if _, err := CheckedMul(math.MaxInt64, 2); err == nil {
		t.Fatal("want overflow error")
	} else if !Is(err, CodeFixedOverflow) {
		t.Fatalf("want FIXED_OVERFLOW, got %v", err)
	}
	if _, err := CheckedMul(math.MinInt64, -1); err == nil {
		t.Fatal("want overflow error for MinInt64 * -1")
	}
	if got, err := CheckedMul(3, 4); err != nil || got != 12 {
		t.Fatalf("CheckedMul(3,4)=%d,%v want 12,nil", got, err)
	}
}

func TestCheckedAddOverflow(t *testing.T) {
	if _, err := CheckedAdd(math.MaxInt64, 1); err == nil {
		t.Fatal("want overflow error")
	} else if !Is(err, CodeFixedOverflow) {
		t.Fatalf("want FIXED_OVERFLOW, got %v", err)
	}
	if got, err := CheckedAdd(2, 3); err != nil || got != 5 {
		t.Fatalf("CheckedAdd(2,3)=%d,%v want 5,nil", got, err)
	}
}

func TestPercentOf(t *testing.T) {
	// water/dryMass percentage: 10g water in 90g dry -> 11.111% = 11111 permille
	got, err := PercentOf(10, 90)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 11111 {
		t.Fatalf("PercentOf(10,90)=%d want 11111", got)
	}
	if _, err := PercentOf(1, 0); err == nil || !Is(err, CodeDivideByZero) {
		t.Fatalf("want DIVIDE_BY_ZERO, got %v", err)
	}
	if _, err := PercentOf(-1, 2); err == nil || !Is(err, CodeInvalidSign) {
		t.Fatalf("want INVALID_SIGN, got %v", err)
	}
}

func TestScaledDivOverflow(t *testing.T) {
	if _, err := ScaledDiv(math.MaxInt64, 1); err == nil {
		t.Fatal("want overflow error scaling MaxInt64")
	}
	if _, err := ScaledDiv(1, 0); err == nil || !Is(err, CodeDivideByZero) {
		t.Fatalf("want DIVIDE_BY_ZERO, got %v", err)
	}
}
