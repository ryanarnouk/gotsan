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
	reporter := &report.Reporter{}
	PopulateRegistryFromFiles(registry, pass.Files, pass.Fset)

	ssaResult := pass.ResultOf[buildssa.Analyzer].(*buildssa.SSA)
	if ssaResult == nil || ssaResult.Pkg == nil {
		return nil, nil
	}

	AnalyzeSSAPackage(ssaResult.Pkg, registry, reporter, pass.Fset)

	for _, d := range reporter.Findings {
		if d.Pos == 0 {
			continue
		}
		pass.Report(analysis.Diagnostic{
			Pos:      d.Pos,
			Message:  d.Message,
			Category: "analysis",
		})
	}

	return nil, nil
}
