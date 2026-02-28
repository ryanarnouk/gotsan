package pipeline

import (
	"go/ast"
	"go/token"
	"gotsan/analyzer"
	"gotsan/ir"
	"gotsan/parse"
	"gotsan/utils/report"

	"golang.org/x/tools/go/ssa"
)

func PopulateRegistryFromFiles(registry *ir.ContractRegistry, files []*ast.File, fset *token.FileSet) {
	if registry == nil || fset == nil {
		return
	}

	visitor := &parse.Visitor{
		Fset:     fset,
		Registry: registry,
	}

	for _, file := range files {
		ast.Walk(visitor, file)
	}
}

func AnalyzeSSAPackage(ssaPkg *ssa.Package, registry *ir.ContractRegistry, reporter *report.Reporter, fset *token.FileSet) {
	if ssaPkg == nil {
		return
	}

	analyzer.Run(ssaPkg, registry, reporter, fset)
}
