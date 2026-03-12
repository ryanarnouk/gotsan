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
