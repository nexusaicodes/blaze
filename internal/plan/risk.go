package plan

import (
	"fmt"

	"github.com/sakshamsaxena/blaze/internal/dodge"
)

// Risk factor weights and thresholds (step 6 of 05-plan.md). These are
// starting values; 10-validation.md describes how to tune them.
const (
	// Factor A: outside callers, status modified.
	OutsideCallersFewPoints  = 10 // 1-2 outside callers
	OutsideCallersSomePoints = 20 // 3-9 outside callers
	OutsideCallersManyPoints = 30 // 10 or more outside callers

	// Factor B: other packages, status modified.
	OtherPackagesFewPoints  = 10 // 1-3 other packages
	OtherPackagesManyPoints = 20 // 4 or more other packages

	// Factor C: exported, status modified.
	ExportedPoints = 10

	// Factor D: signature or declaration changed.
	SignaturePoints = 10

	// Factor E: tests, kind func or method.
	NoTestsPoints        = 15 // no test references this symbol
	UnchangedTestsPoints = 5  // tests exist but none changed in this PR

	// Factor F: size of the change.
	MediumChangeLines  = 50
	MediumChangePoints = 10
	LargeChangeLines   = 150
	LargeChangePoints  = 15

	// Score cap and the thresholds of Risk.Level.
	RiskMaxScore    = 100
	RiskHighScore   = 50
	RiskMediumScore = 25
)

// risk implements step 6 for item: a score and reasons for a symbol item
// whose role is contract, logic or wiring and whose Blast is set; every
// other item gets Risk{Score: 0, Level: "none"}.
func risk(item *Item) Risk {
	if !riskEligible(item) {
		return Risk{Level: "none", Reasons: []string{}}
	}

	score := 0
	var reasons []string
	add := func(points int, reason string) {
		score += points
		reasons = append(reasons, reason)
	}

	b := item.Blast
	sym := item.Symbol

	if item.Status == ChangeModified {
		if points, reason := outsideCallersFactor(b.OutsideCount); points > 0 {
			add(points, reason)
		}
		if points, reason := otherPackagesFactor(b.OtherPackages); points > 0 {
			add(points, reason)
		}
		if sym.Exported {
			add(ExportedPoints, "exported")
		}
	}

	if item.SignatureChanged {
		reason := "signature changed"
		if sym.Kind == dodge.KindType {
			reason = "declaration changed"
		}
		add(SignaturePoints, reason)
	}

	if sym.Kind == dodge.KindFunc || sym.Kind == dodge.KindMethod {
		switch {
		case b.TestCount == 0:
			add(NoTestsPoints, "no test references this")
		case b.ChangedTests == 0:
			add(UnchangedTestsPoints, "tests exist but none changed in this PR")
		}
	}

	if lines := item.Added + item.Deleted; lines >= LargeChangeLines {
		add(LargeChangePoints, fmt.Sprintf("large change (%d lines)", lines))
	} else if lines >= MediumChangeLines {
		add(MediumChangePoints, fmt.Sprintf("large change (%d lines)", lines))
	}

	if score > RiskMaxScore {
		score = RiskMaxScore
	}
	return Risk{Score: score, Level: riskLevel(score), Reasons: nonNil(reasons)}
}

// riskEligible reports whether item is scored per step 6: a symbol item
// whose role is contract, logic or wiring and whose Blast is set.
func riskEligible(item *Item) bool {
	if item.Kind != ItemSymbol || item.Blast == nil {
		return false
	}
	switch item.Role {
	case RoleContract, RoleLogic, RoleWiring:
		return true
	default:
		return false
	}
}

// outsideCallersFactor is risk factor A.
func outsideCallersFactor(n int) (int, string) {
	reason := fmt.Sprintf("%d callers outside this PR", n)
	switch {
	case n == 0:
		return 0, ""
	case n <= 2:
		return OutsideCallersFewPoints, reason
	case n <= 9:
		return OutsideCallersSomePoints, reason
	default:
		return OutsideCallersManyPoints, reason
	}
}

// otherPackagesFactor is risk factor B.
func otherPackagesFactor(p int) (int, string) {
	reason := fmt.Sprintf("used from %d other packages", p)
	switch {
	case p == 0:
		return 0, ""
	case p <= 3:
		return OtherPackagesFewPoints, reason
	default:
		return OtherPackagesManyPoints, reason
	}
}

// riskLevel maps a score to its Risk.Level.
func riskLevel(score int) string {
	switch {
	case score >= RiskHighScore:
		return "high"
	case score >= RiskMediumScore:
		return "medium"
	default:
		return "low"
	}
}

// threadRisk is the thread aggregate of step 6: the Risk of the
// highest-scoring item among orderedItems (already in the thread's review
// order), with each reason prefixed by that item's Title and ": ". A tie
// goes to the item that comes first in orderedItems. A thread with no
// scored item (every item at Level "none") gets Risk{0, "none", nil}.
func threadRisk(orderedItems []*Item) Risk {
	var best *Item
	for _, it := range orderedItems {
		if it.Risk.Level == "none" {
			continue
		}
		if best == nil || it.Risk.Score > best.Risk.Score {
			best = it
		}
	}
	if best == nil {
		return Risk{Level: "none", Reasons: []string{}}
	}

	reasons := make([]string, len(best.Risk.Reasons))
	for i, r := range best.Risk.Reasons {
		reasons[i] = best.Title + ": " + r
	}
	return Risk{Score: best.Risk.Score, Level: best.Risk.Level, Reasons: nonNil(reasons)}
}
