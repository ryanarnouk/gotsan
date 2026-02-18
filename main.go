package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"golang.org/x/tools/go/packages"
	"gotsan/analysis"
	"log"
	"os"
)

func main() {
	// Parse the command line arg
	filePath := flag.String("file", "", "path to Go source file to analyze")
	pkgPattern := flag.String("pkg", "", "Go package to analyze")
	flag.Parse()

	if *filePath == "" && *pkgPattern == "" {
		fmt.Println("Usage:")
		fmt.Println("   gotsan -file <path-to-go-file>")
		fmt.Println("   gotsan -pkg <path-to-go-pkg>")
		os.Exit(1)
	}

	// Single file
	if *filePath != "" {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, *filePath, nil, parser.ParseComments)
		if err != nil {
			log.Fatalf("failed to parse file: %v", err)
		}

		// Initialize the registry/store containing
		// each function and field concurrency contract
		registry := analysis.NewContractRegistry()

		v := &analysis.Visitor{
			Fset:     fset,
			Registry: registry,
		}

		ast.Walk(v, file)

		v.Registry.PrintContractRegistry(fset)
		return
	}

	// Whole package/module
	fset := token.NewFileSet()

	cfg := &packages.Config{
		Mode: packages.NeedName |
			packages.NeedFiles |
			packages.NeedSyntax |
			packages.NeedTypes |
			packages.NeedTypesInfo,
		Fset: fset,
	}

	pkgs, err := packages.Load(cfg, *pkgPattern)
	if err != nil {
		log.Fatalf("failed to load packages: %v", err)
	}

	// packages.Load can return packages with Errors set
	var hadErrors bool
	for _, pkg := range pkgs {
		for _, e := range pkg.Errors {
			hadErrors = true
			fmt.Fprintf(os.Stderr, "load error: %s\n", e)
		}
	}
	if hadErrors {
		os.Exit(1)
	}

	// One registry for the whole run (you could also do one per package)
	registry := analysis.NewContractRegistry()

	// Walk every file in every loaded package
	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			v := &analysis.Visitor{
				Fset:     fset,
				Registry: registry,
			}
			ast.Walk(v, file)
		}
	}

	registry.PrintContractRegistry(fset)
}
