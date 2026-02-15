package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"strings"
)

// parseAnnotations scans a CommentGroup for a lock annotation
func parseAnnotations(cg *ast.CommentGroup) []string {
	if cg == nil {
		return nil
	}
	var annotations []string
	for _, c := range cg.List {
		text := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
		if strings.HasPrefix(text, "@") {
			annotations = append(annotations, text)
		}
	}
	return annotations
}

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

	// Walk declarations
	for _, decl := range file.Decls {
		switch d := decl.(type) {

		// look at function declaration
		case *ast.FuncDecl:
			annotations := parseAnnotations(d.Doc)
			if len(annotations) > 0 {
				fmt.Printf("Function %s has annotations: %v\n", d.Name.Name, annotations)
			}

		// look at type declarations (guarded_by syntax)
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				st, ok := ts.Type.(*ast.StructType)
				if !ok {
					continue
				}

				for _, field := range st.Fields.List {
					annotations := parseAnnotations(field.Doc)
					if len(annotations) > 0 {
						for _, name := range field.Names {
							fmt.Printf("Field %s has annotations: %v\n", name.Name, annotations)
						}
					}
				}
			}
		}
	}
}
