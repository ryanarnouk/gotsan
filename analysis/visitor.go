package analysis

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"
)

// Implements the ast.Visitor interface
type Visitor struct {
	Fset     *token.FileSet
	Registry *ContractRegistry
}

func (v *Visitor) handleFuncDecl(n *ast.FuncDecl) *FunctionContract {
	contract := &FunctionContract{
		Pos: n.Pos(),
	}

	// Doc refers to function documentation comments
	if n.Doc != nil {
		for _, comment := range n.Doc.List {
			annotation, err := ParseAnnotation(comment.Text)
			if err != nil {
				fmt.Errorf("Could not parse annotation line, %d\n", comment.Pos())
				continue
			}

			for _, param := range annotation.Params {
				req := Requirement{
					Kind:   annotation.Kind,
					Target: strings.TrimSpace(param),
				}
				contract.Expectations = append(contract.Expectations, req)
			}

		}
	}

	return contract
}

func (v *Visitor) Visit(node ast.Node) ast.Visitor {
	switch n := node.(type) {
	case *ast.FuncDecl:
		// Add the function to the registry
		v.Registry.Functions[n.Name.Name] = v.handleFuncDecl(n)
		// TODO: GenDecl for struct fields and global variables
	}
	return v
}
