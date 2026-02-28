package parse

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"gotsan/ir"
	"gotsan/utils"
	"strings"
)

// Implements the ast.Visitor interface
type Visitor struct {
	Fset     *token.FileSet
	Registry *ir.ContractRegistry
}

// Given a CommentGroup AST type, loop through and return all discovered
// annotations
func (v *Visitor) parseAnnotations(groups ...*ast.CommentGroup) []Annotation {
	var discovered []Annotation
	for _, group := range groups {
		if group == nil {
			continue
		}
		for _, c := range group.List {
			ann, err := ParseAnnotation(c.Text)
			if err == nil && ann.Kind != ir.None {
				discovered = append(discovered, ann)
			}
		}
	}
	return discovered
}

// Register a list of annotations as data invariants
func (v *Visitor) registerDataInvariants(annotations []Annotation, names []*ast.Ident, prefix string, pos token.Pos) {
	for _, ann := range annotations {
		if ann.Kind != ir.GuardedBy {
			fmt.Printf("[WARNING]: unexpected annotation %v at %s\n", ann.Kind.String(), utils.FormatPos(v.Fset, pos))
			continue
		}

		for _, nameIdent := range names {
			key := nameIdent.Name
			if prefix != "" {
				key = prefix + "." + key
			}

			for _, param := range ann.Params {
				v.Registry.Data[key] = &ir.DataInvariant{
					MutexName: param,
					Pos:       pos,
				}
			}
		}
	}
}

func (v *Visitor) handleFuncDecl(n *ast.FuncDecl) *ir.FunctionContract {
	contract := &ir.FunctionContract{
		Pos: n.Pos(),
	}

	// Doc refers to function documentation comments
	for _, annotation := range v.parseAnnotations(n.Doc) {
		for _, param := range annotation.Params {
			req := ir.Requirement{
				Kind:   annotation.Kind,
				Target: strings.TrimSpace(param),
			}
			contract.Expectations = append(contract.Expectations, req)
		}
	}

	return contract
}

func receiverTypeName(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 {
		return ""
	}

	recvType := types.ExprString(recv.List[0].Type)
	return ir.NormalizeTypeName(recvType)
}

// Specification for a variable definition
func (v *Visitor) handleValueSpecs(node *ast.GenDecl, specs []ast.Spec) {
	for _, spec := range specs {
		vSpec, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}

		annotations := v.parseAnnotations(node.Doc, vSpec.Comment)
		v.registerDataInvariants(annotations, vSpec.Names, "", node.Pos())
	}
}

// Field specification within a struct
func (v *Visitor) handleTypeSpecs(specs []ast.Spec) {
	for _, spec := range specs {
		tSpec, ok := spec.(*ast.TypeSpec)
		if !ok {
			continue
		}

		structType, ok := tSpec.Type.(*ast.StructType)
		if !ok || structType.Fields == nil {
			// if error or struct does not contain any fields
			// no possible fields to guard
			continue
		}

		for _, field := range structType.Fields.List {
			annotations := v.parseAnnotations(field.Doc, field.Comment)
			v.registerDataInvariants(annotations, field.Names, tSpec.Name.Name, field.Pos())
		}
	}
}

// Handle variable declarations and struct fields with a "guarded_by" annotation
func (v *Visitor) handleDataInvariantDecl(n *ast.GenDecl) {
	switch n.Tok {
	case token.VAR:
		v.handleValueSpecs(n, n.Specs)
	case token.TYPE:
		v.handleTypeSpecs(n.Specs)
	}
}

func (v *Visitor) Visit(node ast.Node) ast.Visitor {
	switch n := node.(type) {
	case *ast.FuncDecl:
		// Add the function to the registry
		contract := v.handleFuncDecl(n)
		key := ir.MakeFunctionKey(n.Name.Name, receiverTypeName(n.Recv))
		v.Registry.Functions[key] = contract
		if _, exists := v.Registry.Functions[n.Name.Name]; !exists {
			v.Registry.Functions[n.Name.Name] = contract
		}
	case *ast.GenDecl:
		v.handleDataInvariantDecl(n)
	}
	return v
}
