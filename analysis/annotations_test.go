package analysis

import (
	"go/token"
	"reflect"
	"testing"
)

func TestParseAnnotation(t *testing.T) {
	tests := []struct {
		name       string
		comment    string
		wantKind   AnnotationKind
		wantParams []string
	}{
		{
			name:       "Simple Requires",
			comment:    "//@requires(mu)",
			wantKind:   Requires,
			wantParams: []string{"mu"},
		},
		{
			name:       "Multiple Params",
			comment:    "//@requires(mu1, mu2)",
			wantKind:   Requires,
			wantParams: []string{"mu1", "mu2"},
		},
		{
			name:       "Spaces in Params",
			comment:    "/*@acquires( lock_a , lock_b ) */",
			wantKind:   Acquires,
			wantParams: []string{"lock_a", "lock_b"},
		},
		{
			name:       "Guarded By",
			comment:    "//@guarded_by(state_mu)",
			wantKind:   GuardedBy,
			wantParams: []string{"state_mu"},
		},
		{
			name:       "Mixed Case Keyword",
			comment:    "// @Requires(mu)",
			wantKind:   Requires,
			wantParams: []string{"mu"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Using token.NoPos for tests as we aren't verifying line numbers here
			actual, err := ParseAnnotation(tt.comment, token.NoPos)
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
