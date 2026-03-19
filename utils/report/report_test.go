package report

import (
	"testing"
)

func TestReporterDeduplication(t *testing.T) {
	r := NewReporter()
	if r == nil {
		t.Fatal("NewReporter returned nil")
	}

	d1 := Diagnostic{File: "f.go", Line: 10, Column: 2, Message: "duplicate error"}
	d2 := Diagnostic{File: "f.go", Line: 20, Column: 4, Message: "duplicate error"}
	d2Dup := Diagnostic{File: "f.go", Line: 20, Column: 4, Message: "duplicate error"}
	d3 := Diagnostic{File: "f.go", Line: 30, Column: 6, Message: "another error"}

	r.Warn(d1)
	r.Warn(d2)    // same message at different location should be preserved
	r.Warn(d2Dup) // exact duplicate should be suppressed
	r.Warn(d3)

	if len(r.Findings) != 3 {
		t.Errorf("expected 3 findings after dedup, got %d: %v", len(r.Findings), r.Findings)
	}
	if r.Findings[0] != d1 {
		t.Errorf("first finding wrong: %v", r.Findings[0])
	}
	if r.Findings[1] != d2 {
		t.Errorf("second finding wrong: %v", r.Findings[1])
	}
	if r.Findings[2] != d3 {
		t.Errorf("third finding wrong: %v", r.Findings[2])
	}
}

func TestReporterHeuristicWarningDeduplication(t *testing.T) {
	r := NewReporter()
	if r == nil {
		t.Fatal("NewReporter returned nil")
	}

	d1 := Diagnostic{File: "f.go", Line: 10, Column: 2, Message: "heuristic warning"}
	d1Dup := Diagnostic{File: "f.go", Line: 10, Column: 2, Message: "heuristic warning"}
	d2 := Diagnostic{File: "f.go", Line: 20, Column: 3, Message: "heuristic warning"}

	r.WarnHeuristic(d1)
	r.WarnHeuristic(d1Dup) // exact duplicate should be suppressed
	r.WarnHeuristic(d2)

	if len(r.Warnings) != 2 {
		t.Fatalf("expected 2 warnings after dedup, got %d: %v", len(r.Warnings), r.Warnings)
	}

	if len(r.Findings) != 0 {
		t.Fatalf("expected no core findings, got %d", len(r.Findings))
	}
}

func TestReporterIgnoreMissingAnnotations(t *testing.T) {
	r := NewReporter()
	r.IgnoreMissingAnnotations = true

	r.WarnHeuristic(Diagnostic{File: "f.go", Line: 10, Column: 1, Message: "heuristic warning"})

	if len(r.Warnings) != 0 {
		t.Fatalf("expected warnings to be suppressed, got %d", len(r.Warnings))
	}
}
