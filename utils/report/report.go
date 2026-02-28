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
}

func (r *Reporter) Warn(d Diagnostic) {
	r.Diagnostics = append(r.Diagnostics, d)
}

func (r *Reporter) Print() {
	for _, d := range r.Diagnostics {
		fmt.Printf("%s:%d:%d: %s\n",
			d.File, d.Line, d.Column, d.Message)
	}
}
