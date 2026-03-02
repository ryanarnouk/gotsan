package analyzer

import (
	"go/types"
	"strings"

	"golang.org/x/tools/go/ssa"
)

// resolvePathInScope handles names with dots (e.g., "a.mu" or "mu.lock")
func resolvePathInScope(fn *ssa.Function, parts []string) types.Object {
	first := parts[0]

	// 1. Check if the first part is a parameter (includes receiver)
	for _, p := range fn.Params {
		if p.Name() == first {
			return findFieldPathInType(p.Type(), parts[1:])
		}
	}

	// 2. Implicit receiver check (the first part might be a field of the receiver)
	if len(fn.Params) > 0 {
		recv := fn.Params[0]
		// Case: "field.subfield" where "field" is on the receiver
		return findFieldPathInType(recv.Type(), parts)
	}

	return nil
}

// resolveSingleName handles simple identifiers (e.g., "mu")
func resolveSingleName(fn *ssa.Function, name string) types.Object {
	// 1. Check Parameters
	if obj := findInParams(fn, name); obj != nil {
		return obj
	}

	// 2. Check Implicit Receiver Fields
	if obj := findInReceiverFields(fn, name); obj != nil {
		return obj
	}

	// 3. Check Package Globals
	return findInPackageGlobals(fn, name)
}

func findInParams(fn *ssa.Function, name string) types.Object {
	for _, p := range fn.Params {
		if p.Name() == name {
			return p.Object()
		}
	}
	return nil
}

func findInReceiverFields(fn *ssa.Function, name string) types.Object {
	if len(fn.Params) == 0 {
		return nil
	}

	// Use a helper to peel away pointers and find the underlying struct
	strct, ok := getUnderlyingStruct(fn.Params[0].Type())
	if !ok {
		return nil
	}

	for i := 0; i < strct.NumFields(); i++ {
		field := strct.Field(i)
		if field.Name() == name {
			return field
		}
	}
	return nil
}

func findInPackageGlobals(fn *ssa.Function, name string) types.Object {
	if fn.Pkg != nil {
		if member, ok := fn.Pkg.Members[name]; ok {
			return member.Object()
		}
	}
	return nil
}

// Helper to handle the pointer/struct traversal logic
func getUnderlyingStruct(t types.Type) (*types.Struct, bool) {
	curr := t.Underlying()
	if ptr, ok := curr.(*types.Pointer); ok {
		curr = ptr.Elem().Underlying()
	}
	strct, ok := curr.(*types.Struct)
	return strct, ok
}

func findFieldPathInType(typ types.Type, fieldPath []string) types.Object {
	if len(fieldPath) == 0 {
		return nil
	}

	var found types.Object
	current := typ

	for _, fieldName := range fieldPath {
		if ptr, ok := current.Underlying().(*types.Pointer); ok {
			current = ptr.Elem()
		}

		strct, ok := current.Underlying().(*types.Struct)
		if !ok {
			return nil
		}

		matched := false
		for i := 0; i < strct.NumFields(); i++ {
			field := strct.Field(i)
			if field.Name() == fieldName {
				found = field
				current = field.Type()
				matched = true
				break
			}
		}

		if !matched {
			return nil
		}
	}

	return found
}

func findFieldPathInValue(val ssa.Value, fieldPath []string) types.Object {
	if len(fieldPath) == 0 {
		return traceToObject(val)
	}

	typ := val.Type()
	var found types.Object

	for _, fieldName := range fieldPath {
		if ptr, ok := typ.Underlying().(*types.Pointer); ok {
			typ = ptr.Elem()
		}

		strct, ok := typ.Underlying().(*types.Struct)
		if !ok {
			return nil
		}

		matched := false
		for i := 0; i < strct.NumFields(); i++ {
			field := strct.Field(i)
			if field.Name() == fieldName {
				found = field
				typ = field.Type()
				matched = true
				break
			}
		}

		if !matched {
			return nil
		}
	}

	return found
}

func traceToObject(val ssa.Value) types.Object {
	for {
		switch v := val.(type) {
		case *ssa.FieldAddr:
			// This is the common case: &a.mu
			// We get the struct field object directly
			ptr := v.X.Type().Underlying().(*types.Pointer)
			strct := ptr.Elem().Underlying().(*types.Struct)
			return strct.Field(v.Field)
		case *ssa.UnOp:
			// If it's a pointer dereference (*ptr), look at the pointer
			// This handles pointers to pointers by not returning and continuing the loop
			val = v.X
		case *ssa.IndexAddr:
			// If it's a mutex in a slice (locks[i]) trace as slice
			return traceToObject(v.X)
		case *ssa.Parameter:
			// If it was passed in as an argument
			return v.Object()
		case *ssa.Global:
			// If it's a global mutex
			return v.Object()
		default:
			// If we hit something we don't recognize, stop
			return nil
		}
	}
}

// Resolve a mutex variable name in an annotation to the corresponding assignment
// in the SSA blocks
func resolveObjectInScope(fn *ssa.Function, targetName string) types.Object {
	if targetName == "" {
		return nil
	}

	parts := strings.Split(targetName, ".")
	if len(parts) > 1 {
		return resolvePathInScope(fn, parts)
	}

	// Handle single-name resolution
	return resolveSingleName(fn, targetName)
}

func resolveObjectAtCallSite(call *ssa.Call, targetName string) types.Object {
	callee := call.Call.StaticCallee()
	if callee == nil || targetName == "" {
		return nil
	}

	parts := strings.Split(targetName, ".")
	if len(parts) == 0 {
		return nil
	}

	// Try mapping explicit parameter targets first (e.g., from.mu, to.mu, a.mu)
	first := parts[0]
	for i, p := range callee.Params {
		if p.Name() == first && i < len(call.Call.Args) {
			if len(parts) == 1 {
				return traceToObject(call.Call.Args[i])
			}
			return findFieldPathInValue(call.Call.Args[i], parts[1:])
		}
	}

	// Fallback: interpret target relative to receiver
	if len(call.Call.Args) > 0 {
		receiver := call.Call.Args[0]
		if len(parts) == 1 {
			return findFieldPathInValue(receiver, parts)
		}

		// Handles receiver-qualified targets like b.mu or b.vault.mu
		if len(callee.Params) > 0 && callee.Params[0].Name() == first {
			return findFieldPathInValue(receiver, parts[1:])
		}
		return findFieldPathInValue(receiver, parts)
	}
	return nil
}
