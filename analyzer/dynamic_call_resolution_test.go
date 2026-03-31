package analyzer

import (
	"go/token"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

func buildTestSSAPackageFromFile(t *testing.T, absPath string) *ssa.Package {
	t.Helper()

	fset := token.NewFileSet()
	cfg := &packages.Config{Mode: packages.LoadSyntax, Fset: fset, Tests: true}
	pkgs, err := packages.Load(cfg, absPath)
	if err != nil {
		t.Fatalf("packages.Load failed: %v", err)
	}
	if packages.PrintErrors(pkgs) > 0 {
		t.Fatalf("failed to load package from file: %s", absPath)
	}
	if len(pkgs) == 0 {
		t.Fatalf("no packages loaded for path %s", absPath)
	}

	prog, ssaPkgs := ssautil.Packages(pkgs, ssa.BuilderMode(0))
	prog.Build()

	if len(ssaPkgs) == 0 {
		t.Fatalf("failed to build SSA package for %s", absPath)
	}

	var best *ssa.Package
	bestMembers := -1
	for _, ssaPkg := range ssaPkgs {
		if ssaPkg == nil {
			continue
		}
		if count := len(ssaPkg.Members); count > bestMembers {
			bestMembers = count
			best = ssaPkg
		}
	}
	if best != nil {
		return best
	}

	t.Fatalf("no non-nil SSA package built for %s", absPath)
	return nil
}

func mustRepoRoot(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	root := filepath.Clean(filepath.Join(wd, ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("failed to locate repo root from %s: %v", wd, err)
	}

	return root
}

func dynamicDispatchFixturePath(t *testing.T) string {
	t.Helper()

	return filepath.Join(mustRepoRoot(t), "tests", "testdata", "dynamic_dispatch")
}

func serving3148FixturePath(t *testing.T) string {
	t.Helper()

	return filepath.Join(mustRepoRoot(t), "evaluation", "gobench_samples", "nonblocking", "data_race", "serving", "3148")
}

func findFunctionByName(pkg *ssa.Package, name string) *ssa.Function {
	if pkg == nil {
		return nil
	}

	for fn := range collectPackageFunctions(pkg) {
		if fn != nil && fn.Name() == name {
			return fn
		}
	}

	return nil
}

func firstDynamicCallInFunction(fn *ssa.Function) *ssa.Call {
	if fn == nil {
		return nil
	}

	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			callInstr, ok := instr.(*ssa.Call)
			if !ok {
				continue
			}

			if callInstr.Call.StaticCallee() == nil {
				return callInstr
			}
		}
	}

	return nil
}

func hasTargetByName(targets []*ssa.Function, name string) bool {
	for _, fn := range targets {
		if fn != nil && fn.Name() == name {
			return true
		}
	}
	return false
}

func TestResolveDynamicCallTargets_FunctionParameterBinding(t *testing.T) {
	pkg := buildTestSSAPackageFromFile(t, dynamicDispatchFixturePath(t))
	caller := findFunctionByName(pkg, "callThroughParam")
	if caller == nil {
		t.Fatal("expected callThroughParam in tests/testdata/dynamicdispatch")
	}

	dynCall := firstDynamicCallInFunction(caller)
	if dynCall == nil {
		t.Fatal("expected dynamic call in callThroughParam")
	}

	targets := resolveDynamicCallTargets(caller, dynCall)
	if !hasTargetByName(targets, "targetOne") {
		t.Fatalf("expected targetOne in dynamic targets, got %d targets", len(targets))
	}
}

func TestResolveDynamicCallTargets_InterfaceParameterBinding(t *testing.T) {
	pkg := buildTestSSAPackageFromFile(t, dynamicDispatchFixturePath(t))
	caller := findFunctionByName(pkg, "callThroughInterface")
	if caller == nil {
		t.Fatal("expected callThroughInterface in tests/testdata/dynamicdispatch")
	}

	dynCall := firstDynamicCallInFunction(caller)
	if dynCall == nil {
		t.Fatal("expected dynamic interface call in callThroughInterface")
	}

	targets := resolveDynamicCallTargets(caller, dynCall)
	if !hasTargetByName(targets, "Do") {
		t.Fatalf("expected worker.Do in dynamic interface targets, got %d targets", len(targets))
	}
}

func TestResolveDynamicCallTargets_InterfaceParameterBindingViaGo(t *testing.T) {
	pkg := buildTestSSAPackageFromFile(t, dynamicDispatchFixturePath(t))
	caller := findFunctionByName(pkg, "runDoerInGoroutine")
	if caller == nil {
		t.Fatal("expected runDoerInGoroutine in tests/testdata/dynamicdispatch")
	}

	dynCall := firstDynamicCallInFunction(caller)
	if dynCall == nil {
		t.Fatal("expected dynamic interface call in runDoerInGoroutine")
	}

	targets := resolveDynamicCallTargets(caller, dynCall)
	if !hasTargetByName(targets, "Do") {
		t.Fatalf("expected worker.Do in dynamic interface targets via go callsite, got %d targets", len(targets))
	}
}

func TestBuildRecursionGraph_UsesDynamicTargets(t *testing.T) {
	pkg := buildTestSSAPackageFromFile(t, dynamicDispatchFixturePath(t))
	graph := buildRecursionGraph(pkg)

	caller := findFunctionByName(pkg, "dynamicTrampoline")
	target := findFunctionByName(pkg, "recursiveDriver")
	if caller == nil || target == nil {
		t.Fatal("expected dynamicTrampoline and recursiveDriver functions")
	}

	if !graph.isRecursiveEdge(target, caller) {
		t.Fatal("expected recursiveDriver->dynamicTrampoline to be recursive via dynamic target edge")
	}
}

func TestCollectDeferredCallLockEffects_Serving3148_NoStackOverflow(t *testing.T) {
	pkg := buildTestSSAPackageFromFile(t, serving3148FixturePath(t))
	runFn := findFunctionByName(pkg, "Run")
	if runFn == nil {
		t.Fatal("expected Run method in serving3148 fixture")
	}

	foundDefer := false
	for _, block := range runFn.Blocks {
		for _, instr := range block.Instrs {
			deferInstr, ok := instr.(*ssa.Defer)
			if !ok {
				continue
			}

			foundDefer = true
			locks, unlocks := collectDeferredCallLockEffects(&deferInstr.Call)
			if locks == nil || unlocks == nil {
				t.Fatal("expected non-nil lock effect sets")
			}
		}
	}

	if !foundDefer {
		t.Fatal("expected at least one defer instruction in Run")
	}
}
