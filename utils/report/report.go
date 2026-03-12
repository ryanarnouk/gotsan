package report

import (
	"fmt"
	"go/token"
)

type Diagnostic struct {
	Pos      token.Pos
	File     string
	Line     int
	Column   int
	Severity string // "warning", "error"
	Message  string
}

type Reporter struct {
	Diagnostics []Diagnostic
	// seen holds messages that have already been reported; used to avoid duplicates.
	seen map[string]struct{}
}

// NewReporter constructs a Reporter with internal deduplication state initialized.
func NewReporter() *Reporter {
	return &Reporter{seen: make(map[string]struct{})}
}

func (r *Reporter) Warn(d Diagnostic) {
	if r == nil {
		return
	}
	if r.seen == nil {
		// if the reporter was constructed manually without NewReporter, lazily allocate
		r.seen = make(map[string]struct{})
	}
	// key by message only (dedupe identical error text)
	if _, ok := r.seen[d.Message]; ok {
		return
	}
	r.seen[d.Message] = struct{}{}
	r.Diagnostics = append(r.Diagnostics, d)
}

func (r *Reporter) Print() {
	for _, d := range r.Diagnostics {
		fmt.Printf("%s:%d:%d: %s\n",
			d.File, d.Line, d.Column, d.Message)
	}
}
