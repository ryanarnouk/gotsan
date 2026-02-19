package parse

import (
	"gotsan/ir"
	"reflect"
	"testing"
)

func TestParseAnnotation(t *testing.T) {
	tests := []struct {
		name       string
		comment    string
		wantKind   ir.AnnotationKind
		wantParams []string
	}{
		{
			name:       "Simple Requires",
			comment:    "//@requires(mu)",
			wantKind:   ir.Requires,
			wantParams: []string{"mu"},
		},
		{
			name:       "Multiple Params",
			comment:    "//@requires(mu1, mu2)",
			wantKind:   ir.Requires,
			wantParams: []string{"mu1", "mu2"},
		},
		{
			name:       "Spaces in Params",
			comment:    "/*@acquires( lock_a , lock_b ) */",
			wantKind:   ir.Acquires,
			wantParams: []string{"lock_a", "lock_b"},
		},
		{
			name:       "Guarded By",
			comment:    "//@guarded_by(state_mu)",
			wantKind:   ir.GuardedBy,
			wantParams: []string{"state_mu"},
		},
		{
			name:       "Mixed Case Keyword",
			comment:    "// @requires(mu)",
			wantKind:   ir.Requires,
			wantParams: []string{"mu"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual, err := ParseAnnotation(tt.comment)
			if err != nil {
				t.Errorf("ParseAnnotation() error = %v", err)
				return
			}

			if actual.Kind != tt.wantKind {
				t.Errorf("Kind = %v, want %v", actual.Kind, tt.wantKind)
			}

			if !reflect.DeepEqual(actual.Params, tt.wantParams) {
				t.Errorf("Params = %v, want %v", actual.Params, tt.wantParams)
			}
		})
	}
}
