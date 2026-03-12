package pipeline

import (
	"flag"
	"go/token"
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
	Flags:    flag.FlagSet{},
}

func init() {
	GoAnalysisAnalyzer.Flags.Init("gotsan", flag.ExitOnError)
	GoAnalysisAnalyzer.Flags.Bool("l", false, "lenient mode: only detect deadlocks involving goroutines")
	GoAnalysisAnalyzer.Flags.Bool("s", false, "strict mode: detect deadlocks in single-threaded code as well")
}

func runGoAnalysis(pass *analysis.Pass) (any, error) {
	// Get flag values
	lenientFlag := pass.Analyzer.Flags.Lookup("l")
	strictFlag := pass.Analyzer.Flags.Lookup("s")

	strict := false
	if strictFlag != nil {
		if bv, ok := strictFlag.Value.(flag.Getter); ok {
			if v, _ := bv.Get().(bool); v {
				strict = true
			}
		}
	}

	if lenientFlag != nil && strictFlag != nil {
		if bvL, okL := lenientFlag.Value.(flag.Getter); okL {
			if bvS, okS := strictFlag.Value.(flag.Getter); okS {
				if vL, _ := bvL.Get().(bool); vL {
					if vS, _ := bvS.Get().(bool); vS {
						pass.Report(analysis.Diagnostic{
							Pos:      token.NoPos,
							Message:  "cannot specify both -l and -s flags",
							Category: "error",
						})
						return nil, nil
					}
				}
			}
		}
	}

	registry := ir.NewContractRegistry()
	PopulateRegistryFromFiles(registry, pass.Files, pass.Fset)

	ssaResult := pass.ResultOf[buildssa.Analyzer].(*buildssa.SSA)
	if ssaResult == nil || ssaResult.Pkg == nil {
		return nil, nil
	}

	reporter := report.NewReporter()
	AnalyzeSSAPackage(ssaResult.Pkg, registry, reporter, pass.Fset, strict)

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
