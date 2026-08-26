package rules

import "math"

// FixedScale is the scale factor applied to every fixed-point quality value.
// A raw value of x represents x/FixedScale in real units (e.g. milligrams or
// permille depending on the metric). All arithmetic below keeps integers so
// that concurrent and boundary tests can reproduce results exactly.
const FixedScale int64 = 1000

// HalfAwayFromZero rounds a numerator/denominator quotient to the nearest
// integer, rounding halves away from zero as required by the spec.
func HalfAwayFromZero(num, den int64) (int64, error) {
	if den == 0 {
		return 0, New(CodeDivideByZero, "", "denominator is zero")
	}
	if num == 0 {
		return 0, nil
	}
	q := num / den
	r := num % den
	// Double the remainder to decide rounding; compare against |den| to
	// avoid overflow on den == math.MinInt64.
	if abs64(r)*2 >= abs64(den) {
		if (num > 0 && den > 0) || (num < 0 && den < 0) {
			q++
		} else {
			q--
		}
	}
	return q, nil
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

// CheckedMul multiplies two int64 values and reports overflow.
func CheckedMul(a, b int64) (int64, error) {
	if a == 0 || b == 0 {
		return 0, nil
	}
	if a == math.MinInt64 && b == -1 || b == math.MinInt64 && a == -1 {
		return 0, New(CodeFixedOverflow, "", "multiplication overflow")
	}
	r := a * b
	if r/b != a {
		return 0, New(CodeFixedOverflow, "", "multiplication overflow")
	}
	return r, nil
}

// CheckedAdd adds two int64 values and reports overflow.
func CheckedAdd(a, b int64) (int64, error) {
	r := a + b
	if (b > 0 && r < a) || (b < 0 && r > a) {
		return 0, New(CodeFixedOverflow, "", "addition overflow")
	}
	return r, nil
}

// ScaledDiv expresses the quotient a/b in fixed point (permille), computing
// round(a*FixedScale/b) with overflow and divide-by-zero checks and
// half-away-from-zero rounding. This is the core primitive for densities,
// ratios, strengths and deviations.
func ScaledDiv(a, b int64) (int64, error) {
	if b == 0 {
		return 0, New(CodeDivideByZero, "", "denominator is zero")
	}
	num, err := CheckedMul(a, FixedScale)
	if err != nil {
		return 0, err
	}
	return HalfAwayFromZero(num, b)
}

// PercentOf computes (part/total)*100 in fixed point (permille scale), used
// for moisture content and erosion loss rate. The result is
// round(part*100*FixedScale/total) with overflow and divide-by-zero checks.
func PercentOf(part, total int64) (int64, error) {
	if total == 0 {
		return 0, New(CodeDivideByZero, "", "total is zero")
	}
	if part < 0 || total < 0 {
		return 0, New(CodeInvalidSign, "", "negative mass in ratio")
	}
	num, err := CheckedMul(part, 100*FixedScale)
	if err != nil {
		return 0, err
	}
	return HalfAwayFromZero(num, total)
}
