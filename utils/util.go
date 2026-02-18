package utils

import (
	"fmt"
	"go/token"
)

func FormatPos(fset *token.FileSet, pos token.Pos) string {
	if fset == nil || pos == token.NoPos {
		return ""
	}
	p := fset.Position(pos)
	// file:line:col
	return fmt.Sprintf("%s:%d:%d", p.Filename, p.Line, p.Column)
}
