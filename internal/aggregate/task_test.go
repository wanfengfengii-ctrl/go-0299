package aggregate

import (
	"testing"

	"rammed-earth-roof-beam-clearance/internal/domain"
	"rammed-earth-roof-beam-clearance/internal/rules"
)

func TestCheckGenerationStale(t *testing.T) {
	task := NewTask("t1", "area", "digest", domain.DirectionRising, 1)
	if err := task.CheckGeneration(2, "op"); err == nil || !rules.Is(err, rules.CodeGenerationStale) {
		t.Fatalf("want GENERATION_STALE, got %v", err)
	}
	if err := task.CheckGeneration(1, "op"); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}

func TestCheckDigestStale(t *testing.T) {
	task := NewTask("t1", "area", "digest", domain.DirectionRising, 1)
	if err := task.CheckDigest("other", "op"); err == nil || !rules.Is(err, rules.CodeDesignDigestStale) {
		t.Fatalf("want DESIGN_DIGEST_STALE, got %v", err)
	}
	if err := task.CheckDigest("digest", "op"); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}

func TestAppendCellContinuousPrefix(t *testing.T) {
	task := NewTask("t1", "area", "digest", domain.DirectionRising, 1)
	if err := task.AppendCell(1, 0, "op"); err != nil {
		t.Fatalf("append 0: %v", err)
	}
	// Skipping seq 1 breaks the continuous prefix.
	if err := task.AppendCell(1, 2, "op"); err == nil || !rules.Is(err, rules.CodeOutOfOrder) {
		t.Fatalf("want OUT_OF_ORDER, got %v", err)
	}
	// Duplicate seq 0 also breaks it.
	if err := task.AppendCell(1, 0, "op"); err == nil || !rules.Is(err, rules.CodeOutOfOrder) {
		t.Fatalf("want OUT_OF_ORDER for duplicate, got %v", err)
	}
	// Correct next prefix cell succeeds.
	if err := task.AppendCell(1, 1, "op"); err != nil {
		t.Fatalf("append 1: %v", err)
	}
}

func TestClockRegression(t *testing.T) {
	task := NewTask("t1", "area", "digest", domain.DirectionRising, 1)
	if err := task.AdvanceClock(10, "op"); err != nil {
		t.Fatalf("advance: %v", err)
	}
	if err := task.AdvanceClock(10, "op"); err == nil || !rules.Is(err, rules.CodeClockRegression) {
		t.Fatalf("want CLOCK_REGRESSION, got %v", err)
	}
	if err := task.AdvanceClock(5, "op"); err == nil || !rules.Is(err, rules.CodeClockRegression) {
		t.Fatalf("want CLOCK_REGRESSION, got %v", err)
	}
}

func TestOpenGeneration(t *testing.T) {
	task := NewTask("t1", "area", "digest", domain.DirectionRising, 1)
	if err := task.OpenGeneration("op"); err != nil {
		t.Fatalf("open: %v", err)
	}
	if got := task.Header().Generation; got != 2 {
		t.Fatalf("generation=%d want 2", got)
	}
}
