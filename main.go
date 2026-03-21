package main

import (
	"flag"
	"fmt"
	"go/token"
	"gotsan/ir"
	"gotsan/pipeline"
	"gotsan/utils/logger"
	"gotsan/utils/report"
	"log"
	"os"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

func main() {
	// Parse the command line arg
	filePath := flag.String("file", "", "path to Go source file to analyze")
	pkgPattern := flag.String("pkg", "", "Go package to analyze")
	lenient := flag.Bool("l", false, "lenient mode: only detect deadlocks involving goroutines")
	strict := flag.Bool("s", false, "strict mode: detect deadlocks in single-threaded code as well (default)")
	verbose := flag.Bool("v", false, "enable debug logs")
	ignoreMissingAnnotations := flag.Bool("ignore-missing-annotations", false, "suppress heuristic missing annotation advisory warnings")
	includeTestFiles := flag.Bool("include-tests", true, "include test files in analysis (default: true)")
	flag.Parse()

	if *lenient && *strict {
		fmt.Println("cannot specify both -l and -s flags")
		os.Exit(1)
	}

	if *verbose {
		logger.SetLevel(logger.Debug)
	}

	if *filePath == "" && *pkgPattern == "" {
		fmt.Println("Usage:")
		fmt.Println("   gotsan -file <path-to-go-file>")
		fmt.Println("   gotsan -pkg <path-to-go-pkg>")
		fmt.Println("")
		fmt.Println("Flags:")
		fmt.Println("   -file <path>              path to Go source file to analyze")
		fmt.Println("   -pkg <pattern>            Go package pattern to analyze")
		fmt.Println("   -l                        lenient mode: detect deadlocks in concurrent code only")
		fmt.Println("   -s                        strict mode: detect deadlocks in single-threaded code (default)")
		fmt.Println("   -v                        verbose logging")
		fmt.Println("   -include-tests            include test files in analysis (default: true)")
		fmt.Println("   -ignore-missing-annotations suppress missing annotation advisory warnings")
		os.Exit(1)
	}

	fset := token.NewFileSet()
	cfg := &packages.Config{
		Mode:  packages.LoadSyntax,
		Fset:  fset,
		Tests: *includeTestFiles,
	}

	pattern := *pkgPattern
	if *filePath != "" {
		// If the user specified a single file, make that the pattern for the package
		pattern = *filePath
	}

	pkgs, err := packages.Load(cfg, pattern)
	if err != nil {
		log.Fatalf("failed to load packages: %v", err)
	}

	if packages.PrintErrors(pkgs) > 0 {
		os.Exit(1)
	}

	// 1. Annotation Discovery Phase (AST)
	// One registry is used for the entire run
	registry := ir.NewContractRegistry()

	// Walk every file in every loaded package
	for _, pkg := range pkgs {
		pipeline.PopulateRegistryFromFiles(registry, pkg.Syntax, fset)
	}

	if logger.IsVerbose() {
		registry.PrintContractRegistry(fset)
	}

	// 2. Analysis Phase
	prog, ssaPkgs := ssautil.Packages(pkgs, ssa.BuilderMode(0))
	prog.Build()

	reporter := report.NewReporter()
	reporter.IgnoreMissingAnnotations = *ignoreMissingAnnotations

	strictMode := true
	if *lenient {
		strictMode = false
	} else if *strict {
		strictMode = true
	}

	for _, ssaPkg := range ssaPkgs {
		if ssaPkg == nil {
			continue
		}

		pipeline.AnalyzeSSAPackage(ssaPkg, registry, reporter, fset, strictMode)
	}

	reporter.Print()
}
