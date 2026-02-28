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
	verbose := flag.Bool("v", false, "enable debug logs")
	flag.Parse()

	if *verbose {
		logger.SetLevel(logger.Debug)
	}

	if *filePath == "" && *pkgPattern == "" {
		fmt.Println("Usage:")
		fmt.Println("   gotsan -file <path-to-go-file>")
		fmt.Println("   gotsan -pkg <path-to-go-pkg>")
		os.Exit(1)
	}

	fset := token.NewFileSet()
	cfg := &packages.Config{
		Mode: packages.LoadSyntax,
		Fset: fset,
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
	reporter := &report.Reporter{}

	for _, ssaPkg := range ssaPkgs {
		if ssaPkg == nil {
			continue
		}

		pipeline.AnalyzeSSAPackage(ssaPkg, registry, reporter, fset)
	}

	reporter.Print()
}
