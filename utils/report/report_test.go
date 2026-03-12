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
	d3 := Diagnostic{File: "f.go", Line: 30, Column: 6, Message: "another error"}

	r.Warn(d1)
	r.Warn(d2) // same message, should be suppressed
	r.Warn(d3)

	if len(r.Diagnostics) != 2 {
		t.Errorf("expected 2 diagnostics after dedup, got %d: %v", len(r.Diagnostics), r.Diagnostics)
	}
	if r.Diagnostics[0].Message != d1.Message {
		t.Errorf("first message wrong: %v", r.Diagnostics[0])
	}
	if r.Diagnostics[1].Message != d3.Message {
		t.Errorf("second message wrong: %v", r.Diagnostics[1])
	}
}
