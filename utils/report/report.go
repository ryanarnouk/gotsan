package report

import (
	"fmt"
	"go/token"
	"sort"
	"strconv"
	"strings"
)

type Diagnostic struct {
	Pos     token.Pos
	File    string
	Line    int
	Column  int
	Message string
}

type Reporter struct {
	Findings                 []Diagnostic
	Warnings                 []Diagnostic
	IgnoreMissingAnnotations bool
	// seen holds diagnostics that have already been reported; used to avoid duplicates.
	seen         map[string]struct{}
	seenWarnings map[string]struct{}
}

// NewReporter constructs a Reporter with internal deduplication state initialized.
func NewReporter() *Reporter {
	return &Reporter{
		seen:         make(map[string]struct{}),
		seenWarnings: make(map[string]struct{}),
	}
}

// Warn records an analysis finding.
func (r *Reporter) Warn(d Diagnostic) {
	if r == nil {
		return
	}
	if r.seen == nil {
		// if the reporter was constructed manually without NewReporter, lazily allocate
		r.seen = make(map[string]struct{})
	}
	key := diagnosticKey(d)
	if _, ok := r.seen[key]; ok {
		return
	}
	r.seen[key] = struct{}{}
	r.Findings = append(r.Findings, d)
}

// WarnHeuristic records an advisory warning that should be printed separately
// from core analysis findings.
func (r *Reporter) WarnHeuristic(d Diagnostic) {
	if r == nil {
		return
	}
	if r.IgnoreMissingAnnotations {
		return
	}
	if r.seenWarnings == nil {
		r.seenWarnings = make(map[string]struct{})
	}

	key := diagnosticKey(d)
	if _, ok := r.seenWarnings[key]; ok {
		return
	}

	r.seenWarnings[key] = struct{}{}
	r.Warnings = append(r.Warnings, d)
}

func diagnosticKey(d Diagnostic) string {
	var b strings.Builder
	b.Grow(len(d.File) + len(d.Message) + 32)
	b.WriteString(d.File)
	b.WriteString(":")
	b.WriteString(strconv.Itoa(d.Line))
	b.WriteString(":")
	b.WriteString(strconv.Itoa(d.Column))
	b.WriteString(":")
	b.WriteString(d.Message)
	return b.String()
}

func (r *Reporter) Print() {
	sortDiagnostics(r.Findings)
	sortDiagnostics(r.Warnings)

	if len(r.Findings) > 0 {
		fmt.Println()
		fmt.Println("============================================================")
		fmt.Printf("GOTSAN REPORT - %d finding(s)\n", len(r.Findings))
		fmt.Println("============================================================")
		for _, d := range r.Findings {
			fmt.Printf("%s:%d:%d: %s\n", d.File, d.Line, d.Column, d.Message)
		}
		fmt.Println("============================================================")
	}

	if len(r.Warnings) > 0 {
		fmt.Println()
		fmt.Println("============================================================")
		fmt.Printf("MISSING ANNOTATION ADVISORY WARNINGS - %d warning(s)\n", len(r.Warnings))
		fmt.Println("============================================================")
		for _, d := range r.Warnings {
			fmt.Printf("%s:%d:%d: %s\n", d.File, d.Line, d.Column, d.Message)
		}
		fmt.Println("============================================================")
	}
}

func sortDiagnostics(diags []Diagnostic) {
	sort.Slice(diags, func(i, j int) bool {
		a := diags[i]
		b := diags[j]

		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.Column != b.Column {
			return a.Column < b.Column
		}
		return a.Message < b.Message
	})
}
