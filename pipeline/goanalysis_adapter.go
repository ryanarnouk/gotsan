package pipeline

import (
	"gotsan/ir"
	"gotsan/utils/report"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
)

var GoAnalysisAnalyzer = &analysis.Analyzer{
	Name:     "gotsan",
	Doc:      "verifies lock-related preconditions and invariants from gotsan annotations",
	Requires: []*analysis.Analyzer{buildssa.Analyzer},
	Run:      runGoAnalysis,
}

func runGoAnalysis(pass *analysis.Pass) (any, error) {
	registry := ir.NewContractRegistry()
	PopulateRegistryFromFiles(registry, pass.Files, pass.Fset)

	ssaResult := pass.ResultOf[buildssa.Analyzer].(*buildssa.SSA)
	if ssaResult == nil || ssaResult.Pkg == nil {
		return nil, nil
	}

	reporter := &report.Reporter{}
	AnalyzeSSAPackage(ssaResult.Pkg, registry, reporter, pass.Fset)

	for _, d := range reporter.Diagnostics {
		if d.Pos == 0 {
			continue
		}
		pass.Report(analysis.Diagnostic{
			Pos:      d.Pos,
			Message:  d.Message,
			Category: d.Severity,
		})
	}

	return nil, nil
}
