package analyzer

import (
	"go/types"
	"strings"

	"golang.org/x/tools/go/ssa"
)

// Methods to resolve mutex references to an types.Object
// form, found in the SSA form

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

// Search through a struct (i.e., type) to find object (in SSA) corresponding to the field
func resolveNestedField(typ types.Type, fieldPath []string) types.Object {
	if len(fieldPath) == 0 {
		return nil
	}

	current := typ
	var found types.Object

	for _, fieldName := range fieldPath {
		// Dereference pointers
		if ptr, ok := current.Underlying().(*types.Pointer); ok {
			current = ptr.Elem()
		}

		// Must be a struct
		strct, ok := current.Underlying().(*types.Struct)
		if !ok {
			return nil
		}

		// Find the matching field
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

// Resolve a direct value (evaluated expression) to an object in SSA
func resolveValueToObject(val ssa.Value) types.Object {
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
			return resolveValueToObject(v.X)
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

// resolveIdentifier handles simple identifiers (e.g., "mu") that are not accessed through
// any structs
func resolveIdentifier(fn *ssa.Function, name string) types.Object {
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

// Returns the object of variables accessed through a parent struct.
// Or, more simply, are contained and accessed with a period (e.g., "a.mu" or "mu.lock")
func resolveMultiAccess(fn *ssa.Function, parts []string) types.Object {
	first := parts[0]

	// 1. Check if the first part is a parameter (includes receiver)
	for _, p := range fn.Params {
		if p.Name() == first {
			return resolveNestedField(p.Type(), parts[1:])
		}
	}

	// 2. Implicit receiver check (the first part might be a field of the receiver)
	if len(fn.Params) > 0 {
		recv := fn.Params[0]
		// Case: "field.subfield" where "field" is on the receiver
		return resolveNestedField(recv.Type(), parts)
	}

	return nil
}

func splitTarget(targetName string) []string {
	if targetName == "" {
		return nil
	}
	return strings.Split(targetName, ".")
}

func resolveValueField(val ssa.Value, fieldPath []string) types.Object {
	if len(fieldPath) == 0 {
		return resolveValueToObject(val)
	}
	return resolveNestedField(val.Type(), fieldPath)
}

// Resolve a mutex variable name in an annotation to the corresponding assignment
// in the SSA blocks
func resolveObjectInScope(fn *ssa.Function, targetName string) types.Object {
	parts := splitTarget(targetName)
	if len(parts) == 0 {
		return nil
	}

	if len(parts) == 1 {
		return resolveIdentifier(fn, targetName)
	}

	return resolveMultiAccess(fn, parts)
}

func resolveParamField(callee *ssa.Function, callArgs []ssa.Value, parts []string) types.Object {
	first := parts[0]
	for i, p := range callee.Params {
		if p.Name() == first && i < len(callArgs) {
			return resolveValueField(callArgs[i], parts[1:])
		}
	}
	return nil
}

func resolveObjectAtInvocation(callee *ssa.Function, callArgs []ssa.Value, targetName string) types.Object {
	if callee == nil || targetName == "" {
		return nil
	}

	parts := splitTarget(targetName)
	if len(parts) == 0 {
		return nil
	}

	// Try mapping to explicit parameters.
	obj := resolveParamField(callee, callArgs, parts)
	if obj != nil {
		return obj
	}

	// Fallback for package-level identifiers (e.g., @requires(mu) where mu is a global).
	if len(parts) == 1 {
		if obj := findInPackageGlobals(callee, targetName); obj != nil {
			return obj
		}
	}

	// Fallback: interpret relative to receiver (first argument).
	if len(callArgs) > 0 {
		receiver := callArgs[0]
		return resolveValueField(receiver, parts)
	}

	return nil
}

// Resolve object at the location of the call of a function
// This is used to resolve the mutex names in the annotations
// (of the callee function) to the SSA object and check the lockset
func resolveObjectAtCallSite(call *ssa.Call, targetName string) types.Object {
	callee := call.Call.StaticCallee()
	if callee == nil {
		return nil
	}

	return resolveObjectAtInvocation(callee, call.Call.Args, targetName)
}
