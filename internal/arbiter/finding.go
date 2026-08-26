package arbiter

import "rammed-earth-roof-beam-clearance/internal/domain"

// Metric names accepted by numeric inspections. They map deterministically
// onto threshold comparisons and, on failure, onto a finding kind.
const (
	MetricDryDensity = "dry_density"
	MetricMoisture   = "moisture"
	MetricShear      = "shear"
	MetricErosion    = "erosion"
)

// Judge compares a measured fixed-point value against the locked thresholds
// and returns the finding kind (with ok=true) when the measurement fails, or
// ("", false) when it passes (acceptance 5). Only integer comparisons are
// used; no floating point participates in the verdict.
func Judge(metric string, value int64, th domain.Thresholds) (domain.FindingKind, bool) {
	switch metric {
	case MetricDryDensity:
		if value < th.MinDryDensity {
			return domain.FindingLowDensity, true
		}
	case MetricMoisture:
		if value < th.MinMoisture || value > th.MaxMoisture {
			return domain.FindingMoistureOut, true
		}
	case MetricShear:
		if value < th.MinShear {
			return domain.FindingLowShear, true
		}
	case MetricErosion:
		if value > th.MaxErosion {
			return domain.FindingErosion, true
		}
	}
	return "", false
}

// DefectKinds are the directly-observed (non-numeric) structural anomalies
// that force a rebuild: cold joint, layer crack and tie offset. They are
// reported by the scanner and do not participate in fixed-point arithmetic.
var DefectKinds = []domain.FindingKind{
	domain.FindingColdJoint,
	domain.FindingLayerCrack,
	domain.FindingTieOffset,
}

// IsDefect reports whether a finding kind is a directly-observed defect.
func IsDefect(k domain.FindingKind) bool {
	for _, d := range DefectKinds {
		if k == d {
			return true
		}
	}
	return false
}
