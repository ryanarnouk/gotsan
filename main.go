package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"gotsan/analysis"
	"log"
	"os"
)

func main() {
	// Parse the command line arg
	path := flag.String("file", "", "path to Go source file to analyze")
	flag.Parse()

	if *path == "" {
		fmt.Println("Usage: gotsan -file <path-to-go-file>")
		os.Exit(1)
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, *path, nil, parser.ParseComments)
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
}
