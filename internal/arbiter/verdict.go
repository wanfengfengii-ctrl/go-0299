package arbiter

import (
	"rammed-earth-roof-beam-clearance/internal/domain"
	"rammed-earth-roof-beam-clearance/internal/rules"
)

// Reviews tracks independent qualified-person reviews for a task and decides
// whether the terminal-verdict precondition is met (two different qualified
// reviewers, domain rule and acceptance 7).
type Reviews struct {
	reviews []domain.Review
}

// NewReviews creates an empty review set.
func NewReviews() *Reviews { return &Reviews{} }

// NewReviewsFrom reconstructs a review set from persisted reviews.
func NewReviewsFrom(reviews []domain.Review) *Reviews { return &Reviews{reviews: reviews} }

// Reviews returns the recorded reviews in insertion order.
func (r *Reviews) Reviews() []domain.Review { return r.reviews }

// Add records an independent review. A reviewer may not submit twice for the
// same task (deterministic rejection, test scenario 10).
func (r *Reviews) Add(rev domain.Review, opID domain.OperationID) error {
	if !rev.Qualified {
		return rules.New(rules.CodeInvalidSign, string(opID), "reviewer is not qualified")
	}
	for _, existing := range r.reviews {
		if existing.Reviewer == rev.Reviewer {
			return rules.New(rules.CodeIdempotencyConflict, string(opID), "reviewer already submitted")
		}
	}
	r.reviews = append(r.reviews, rev)
	return nil
}

// CanFinalize reports whether two different qualified reviewers have both
// submitted independent conclusions.
func (r *Reviews) CanFinalize() bool {
	qualified := make(map[string]bool)
	for _, rev := range r.reviews {
		if rev.Qualified {
			qualified[rev.Reviewer] = true
		}
	}
	return len(qualified) >= 2
}

// VerdictBarrier is the single-write terminal verdict. Only one verdict may
// exist per task; a second write returns FINAL_CONFLICT (failure boundary 7).
type VerdictBarrier struct {
	verdict *domain.FinalVerdict
}

// NewVerdictBarrier creates an empty verdict barrier.
func NewVerdictBarrier() *VerdictBarrier { return &VerdictBarrier{} }

// NewVerdictBarrierFrom reconstructs a verdict barrier from a persisted verdict.
func NewVerdictBarrierFrom(v *domain.FinalVerdict) *VerdictBarrier {
	b := NewVerdictBarrier()
	if v != nil {
		cp := *v
		b.verdict = &cp
	}
	return b
}

// Write attempts the single terminal write. The first write wins; subsequent
// writes read the existing verdict or receive FINAL_CONFLICT.
func (b *VerdictBarrier) Write(v domain.FinalVerdict, opID domain.OperationID) (*domain.FinalVerdict, error) {
	if b.verdict != nil {
		if b.verdict.Kind == v.Kind {
			return b.verdict, nil
		}
		return b.verdict, rules.New(rules.CodeFinalConflict, string(opID), "a different terminal verdict already exists")
	}
	v.WriteVer = 1
	b.verdict = &v
	return b.verdict, nil
}

// Current returns the existing terminal verdict, if any.
func (b *VerdictBarrier) Current() *domain.FinalVerdict { return b.verdict }
