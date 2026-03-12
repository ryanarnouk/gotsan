package report

import (
	"fmt"
	"go/token"
	"sort"
)

type Diagnostic struct {
	Pos     token.Pos
	File    string
	Line    int
	Column  int
	Message string
}

type Reporter struct {
	Findings []Diagnostic // all analyzer findings (including unresolved annotation targets)
}

// Warn records an analysis finding.
func (r *Reporter) Warn(d Diagnostic) {
	r.Findings = append(r.Findings, d)
}

// WarnAnnotation records annotation/contract issues in the same stream.
func (r *Reporter) WarnAnnotation(d Diagnostic) {
	r.Findings = append(r.Findings, d)
}

func (r *Reporter) Print() {
	sortDiagnostics(r.Findings)

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
