package rules

import (
	"sort"

	"rammed-earth-roof-beam-clearance/internal/domain"
)

// RectArea computes the integer-millimetre area of a rectangle, checking for
// negative or degenerate dimensions and for int64 overflow of the product.
func RectArea(r domain.Rect) (int64, *Error) {
	if r.W < 0 || r.H < 0 || r.X < 0 || r.Y < 0 {
		return 0, New(CodeGeometryNegative, "", "negative coordinate or size")
	}
	if r.W == 0 || r.H == 0 {
		return 0, New(CodeGeometryDegenerate, "", "zero-width or zero-height rectangle")
	}
	if _, err := CheckedMul(r.W, r.H); err != nil {
		return 0, New(CodeGeometryOverflow, "", "area overflow")
	}
	return r.W * r.H, nil
}

// RectOverlap reports whether two axis-aligned rectangles share any area.
func RectOverlap(a, b domain.Rect) bool {
	return a.X < b.X+b.W && b.X < a.X+a.W && a.Y < b.Y+b.H && b.Y < a.Y+a.H
}

// RectContains reports whether outer fully contains inner.
func RectContains(outer, inner domain.Rect) bool {
	return inner.X >= outer.X && inner.Y >= outer.Y &&
		inner.X+inner.W <= outer.X+outer.W &&
		inner.Y+inner.H <= outer.Y+outer.H
}

// issue records a detected geometry problem together with its stable code.
type issue struct {
	code   Code
	reason string
}

// priority orders issues so the reported primary code is deterministic:
// negative, degenerate, overflow, forbidden zone, overlap, out-of-range.
var priority = []Code{
	CodeGeometryNegative,
	CodeGeometryDegenerate,
	CodeGeometryOverflow,
	CodeForbiddenZone,
	CodeGeometryOverlap,
	CodeLayerOutOfRange,
	CodeGeometryGap,
}

// ValidateGeometry validates a locked wall geometry against the integer
// millimetre rules: non-negative and non-degenerate dimensions, area overflow,
// forbidden-zone avoidance, cell non-overlap and wall containment. It returns
// a single error whose primary code is the highest-priority problem found and
// whose reasons are deterministically sorted.
func ValidateGeometry(g domain.WallGeometry) *Error {
	var issues []issue

	if err := checkRect(g.Wall); err != nil {
		issues = append(issues, issue{err.Code, err.Reasons[0]})
	}

	for _, o := range g.Openings {
		if !RectContains(g.Wall, o.Rect) {
			issues = append(issues, issue{CodeGeometryOverlap, "opening " + o.Name + " outside wall"})
		}
		if err := checkRect(o.Rect); err != nil {
			issues = append(issues, issue{err.Code, "opening " + o.Name + ": " + err.Reasons[0]})
		}
	}

	for _, t := range g.Ties {
		if !RectContains(g.Wall, t.Reserve) {
			issues = append(issues, issue{CodeGeometryOverlap, "tie reserve " + t.Name + " outside wall"})
		}
	}

	for _, c := range g.Cells {
		if !RectContains(g.Wall, c.Rect) {
			issues = append(issues, issue{CodeGeometryOverlap, "cell outside wall"})
		}
		if err := checkRect(c.Rect); err != nil {
			issues = append(issues, issue{err.Code, "cell: " + err.Reasons[0]})
		}
		for _, o := range g.Openings {
			if RectOverlap(c.Rect, o.Rect) {
				issues = append(issues, issue{CodeForbiddenZone, "cell enters opening " + o.Name})
			}
		}
		for _, t := range g.Ties {
			if RectOverlap(c.Rect, t.Reserve) {
				issues = append(issues, issue{CodeForbiddenZone, "cell enters tie reserve " + t.Name})
			}
		}
	}

	for i := 0; i < len(g.Cells); i++ {
		for j := i + 1; j < len(g.Cells); j++ {
			if RectOverlap(g.Cells[i].Rect, g.Cells[j].Rect) {
				issues = append(issues, issue{CodeGeometryOverlap, "cells overlap"})
			}
		}
	}

	for _, l := range g.Layers {
		if !RectContains(g.Wall, l.Rect) {
			issues = append(issues, issue{CodeLayerOutOfRange, "layer outside wall"})
		}
	}

	if len(issues) == 0 {
		return nil
	}

	primary := issues[0].code
	for _, is := range issues {
		if rank(is.code) < rank(primary) {
			primary = is.code
		}
	}

	reasons := make([]string, 0, len(issues))
	for _, is := range issues {
		reasons = append(reasons, is.reason)
	}
	sort.Strings(reasons)
	return New(primary, "", reasons...)
}

func checkRect(r domain.Rect) *Error {
	if _, err := RectArea(r); err != nil {
		return err
	}
	return nil
}

func rank(c Code) int {
	for i, p := range priority {
		if p == c {
			return i
		}
	}
	return len(priority)
}
