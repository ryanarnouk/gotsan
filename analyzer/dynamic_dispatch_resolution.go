package analyzer

import (
	"go/types"

	"golang.org/x/tools/go/ssa"
)

func resolveFunctionFromValue(v ssa.Value) *ssa.Function {
	switch n := v.(type) {
	case *ssa.Function:
		return n
	case *ssa.MakeClosure:
		if n.Fn == nil {
			return nil
		}
		fn, _ := n.Fn.(*ssa.Function)
		return fn
	case *ssa.ChangeType:
		return resolveFunctionFromValue(n.X)
	case *ssa.ChangeInterface:
		return resolveFunctionFromValue(n.X)
	case *ssa.UnOp:
		return resolveFunctionFromValue(n.X)
	case *ssa.Alloc:
		refs := n.Referrers()
		if refs == nil {
			return nil
		}
		for _, ref := range *refs {
			store, ok := ref.(*ssa.Store)
			if !ok {
				continue
			}
			if fn := resolveFunctionFromValue(store.Val); fn != nil {
				return fn
			}
		}
	}

	return nil
}

func resolveParameterFromValue(v ssa.Value) *ssa.Parameter {
	if v == nil {
		return nil
	}

	switch n := v.(type) {
	case *ssa.Parameter:
		return n
	case *ssa.ChangeType:
		return resolveParameterFromValue(n.X)
	case *ssa.ChangeInterface:
		return resolveParameterFromValue(n.X)
	case *ssa.UnOp:
		return resolveParameterFromValue(n.X)
	default:
		return nil
	}
}

func resolveConcreteTypesFromValue(v ssa.Value) []types.Type {
	if v == nil {
		return nil
	}

	switch n := v.(type) {
	case *ssa.MakeInterface:
		if n.X == nil {
			return nil
		}
		return []types.Type{n.X.Type()}
	case *ssa.Call:
		callee := n.Call.StaticCallee()
		if callee == nil {
			return nil
		}
		return inferConcreteReturnTypes(callee)
	case *ssa.ChangeType:
		return resolveConcreteTypesFromValue(n.X)
	case *ssa.ChangeInterface:
		return resolveConcreteTypesFromValue(n.X)
	case *ssa.UnOp:
		return resolveConcreteTypesFromValue(n.X)
	default:
		if t := v.Type(); t != nil {
			return []types.Type{t}
		}
	}

	return nil
}

func inferConcreteReturnTypes(fn *ssa.Function) []types.Type {
	if fn == nil || len(fn.Blocks) == 0 {
		return nil
	}

	out := make([]types.Type, 0)
	seen := make(map[string]bool)

	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			ret, ok := instr.(*ssa.Return)
			if !ok {
				continue
			}

			for _, result := range ret.Results {
				for _, t := range resolveConcreteTypesFromValue(result) {
					if t == nil {
						continue
					}
					key := t.String()
					if seen[key] {
						continue
					}
					seen[key] = true
					out = append(out, t)
				}
			}
		}
	}

	return out
}

func resolveMethodTargetsForType(pkg *ssa.Package, recvType types.Type, methodName string) []*ssa.Function {
	if pkg == nil || recvType == nil || methodName == "" {
		return nil
	}

	seen := make(map[*ssa.Function]bool)
	out := make([]*ssa.Function, 0)

	addFromMethodSet := func(t types.Type) {
		if t == nil {
			return
		}

		methodSet := pkg.Prog.MethodSets.MethodSet(t)
		for i := 0; i < methodSet.Len(); i++ {
			sel := methodSet.At(i)
			if sel == nil || sel.Obj() == nil || sel.Obj().Name() != methodName {
				continue
			}

			fn := pkg.Prog.MethodValue(sel)
			if fn == nil || seen[fn] {
				continue
			}

			seen[fn] = true
			out = append(out, fn)
		}
	}

	addFromMethodSet(recvType)
	if _, ok := recvType.Underlying().(*types.Pointer); !ok {
		addFromMethodSet(types.NewPointer(recvType))
	}

	return out
}

func appendUniqueFunction(targets []*ssa.Function, fn *ssa.Function, seen map[*ssa.Function]bool) []*ssa.Function {
	if fn == nil || seen[fn] {
		return targets
	}
	seen[fn] = true
	return append(targets, fn)
}

func resolveParameterBindingTargets(fn *ssa.Function, param *ssa.Parameter, pkg *ssa.Package) []*ssa.Function {
	if fn == nil || param == nil || pkg == nil {
		return nil
	}

	idx := -1
	for i, p := range fn.Params {
		if p == param {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil
	}

	seen := make(map[*ssa.Function]bool)
	out := make([]*ssa.Function, 0)

	for caller := range collectPackageFunctions(pkg) {
		for _, block := range caller.Blocks {
			for _, instr := range block.Instrs {
				switch callLike := instr.(type) {
				case *ssa.Call:
					if callLike.Call.StaticCallee() != fn {
						continue
					}

					if idx >= len(callLike.Call.Args) {
						continue
					}

					target := resolveFunctionFromValue(callLike.Call.Args[idx])
					if target == nil || seen[target] {
						continue
					}

					seen[target] = true
					out = append(out, target)
				case *ssa.Go:
					if callLike.Call.StaticCallee() != fn {
						continue
					}

					if idx >= len(callLike.Call.Args) {
						continue
					}

					target := resolveFunctionFromValue(callLike.Call.Args[idx])
					if target == nil || seen[target] {
						continue
					}

					seen[target] = true
					out = append(out, target)
				}
			}
		}
	}

	return out
}

func resolveParameterBindingMethodTargets(
	fn *ssa.Function,
	param *ssa.Parameter,
	pkg *ssa.Package,
	methodName string,
) []*ssa.Function {
	if fn == nil || param == nil || pkg == nil || methodName == "" {
		return nil
	}

	idx := -1
	for i, p := range fn.Params {
		if p == param {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil
	}

	seen := make(map[*ssa.Function]bool)
	out := make([]*ssa.Function, 0)

	for caller := range collectPackageFunctions(pkg) {
		for _, block := range caller.Blocks {
			for _, instr := range block.Instrs {
				switch callLike := instr.(type) {
				case *ssa.Call:
					if callLike.Call.StaticCallee() != fn {
						continue
					}

					if idx >= len(callLike.Call.Args) {
						continue
					}

					arg := callLike.Call.Args[idx]
					for _, recvType := range resolveConcreteTypesFromValue(arg) {
						for _, target := range resolveMethodTargetsForType(pkg, recvType, methodName) {
							if target == nil || seen[target] {
								continue
							}

							seen[target] = true
							out = append(out, target)
						}
					}
				case *ssa.Go:
					if callLike.Call.StaticCallee() != fn {
						continue
					}

					if idx >= len(callLike.Call.Args) {
						continue
					}

					arg := callLike.Call.Args[idx]
					for _, recvType := range resolveConcreteTypesFromValue(arg) {
						for _, target := range resolveMethodTargetsForType(pkg, recvType, methodName) {
							if target == nil || seen[target] {
								continue
							}

							seen[target] = true
							out = append(out, target)
						}
					}
				}
			}
		}
	}

	return out
}

func resolveDynamicCallTargets(callerFn *ssa.Function, msg *ssa.Call) []*ssa.Function {
	if callerFn == nil || msg == nil {
		return nil
	}

	targets := make([]*ssa.Function, 0)
	seen := make(map[*ssa.Function]bool)

	if direct := resolveFunctionFromValue(msg.Call.Value); direct != nil {
		targets = appendUniqueFunction(targets, direct, seen)
	}

	if callerFn.Pkg != nil {
		if param := resolveParameterFromValue(msg.Call.Value); param != nil {
			for _, bound := range resolveParameterBindingTargets(callerFn, param, callerFn.Pkg) {
				targets = appendUniqueFunction(targets, bound, seen)
			}

			if msg.Call.Method != nil {
				for _, bound := range resolveParameterBindingMethodTargets(callerFn, param, callerFn.Pkg, msg.Call.Method.Name()) {
					targets = appendUniqueFunction(targets, bound, seen)
				}
			}
		}
	}

	if msg.Call.Method != nil {
		for _, recvType := range resolveConcreteTypesFromValue(msg.Call.Value) {
			for _, target := range resolveMethodTargetsForType(callerFn.Pkg, recvType, msg.Call.Method.Name()) {
				targets = appendUniqueFunction(targets, target, seen)
			}
		}
	}

	unop, ok := msg.Call.Value.(*ssa.UnOp)
	if !ok {
		return targets
	}

	fieldAddr, ok := unop.X.(*ssa.FieldAddr)
	if !ok || callerFn.Pkg == nil {
		return targets
	}

	for fn := range collectPackageFunctions(callerFn.Pkg) {
		for _, block := range fn.Blocks {
			for _, instr := range block.Instrs {
				store, ok := instr.(*ssa.Store)
				if !ok {
					continue
				}

				storeFieldAddr, ok := store.Addr.(*ssa.FieldAddr)
				if !ok {
					continue
				}

				if storeFieldAddr.Field != fieldAddr.Field {
					continue
				}

				if storeFieldAddr.X.Type().String() != fieldAddr.X.Type().String() {
					continue
				}

				target := resolveFunctionFromValue(store.Val)
				targets = appendUniqueFunction(targets, target, seen)

				if target == nil {
					param := resolveParameterFromValue(store.Val)
					if param != nil {
						for _, bound := range resolveParameterBindingTargets(fn, param, callerFn.Pkg) {
							targets = appendUniqueFunction(targets, bound, seen)
						}

						if msg.Call.Method != nil {
							for _, bound := range resolveParameterBindingMethodTargets(fn, param, callerFn.Pkg, msg.Call.Method.Name()) {
								targets = appendUniqueFunction(targets, bound, seen)
							}
						}
					}
				}
			}
		}
	}

	return targets
}
