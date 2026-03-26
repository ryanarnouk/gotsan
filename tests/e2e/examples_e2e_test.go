package examples_test

import (
	"flag"
	"fmt"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gotsan/ir"
	"gotsan/pipeline"
	"gotsan/utils/report"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

// This suite intentionally lives under tests/e2e:
// - examples communicates the pedagogical purpose (how to use the analyzer)
// - expected-output snapshots communicate the verification purpose (lock behavior regression checks)
//
// Each example is analyzed end-to-end and compared against an expected report snapshot.
// This makes examples executable documentation and a stable conformance suite.
var update = flag.Bool("update", false, "update expected snapshot files")

func TestExamples_LenientMode_ExpectedFindings(t *testing.T) {
	runExpectedSuite(t, false)
}

func TestExamples_StrictMode_ExpectedFindings(t *testing.T) {
	runExpectedSuite(t, true)
}

func runExpectedSuite(t *testing.T, strict bool) {
	t.Helper()

	repoRoot := mustRepoRoot(t)
	exampleFiles := mustExampleFiles(t, repoRoot)

	for _, absPath := range exampleFiles {
		relPath, err := filepath.Rel(repoRoot, absPath)
		if err != nil {
			t.Fatalf("failed to compute relative path for %s: %v", absPath, err)
		}

		name := strings.TrimSuffix(relPath, ".go")
		name = strings.ReplaceAll(name, string(os.PathSeparator), "__")
		mode := "lenient"
		if strict {
			mode = "strict"
		}
		expectedPath := filepath.Join(repoRoot, "tests", "e2e", "testdata", name+"."+mode+".expected")

		t.Run(relPath+"/"+mode, func(t *testing.T) {
			findings := analyzeFile(t, absPath, strict)
			actual := strings.Join(findings, "\n")
			if len(findings) > 0 {
				actual += "\n"
			}

			if *update {
				if err := os.MkdirAll(filepath.Dir(expectedPath), 0o755); err != nil {
					t.Fatalf("failed to create testdata dir: %v", err)
				}
				if err := os.WriteFile(expectedPath, []byte(actual), 0o644); err != nil {
					t.Fatalf("failed writing expected snapshot %s: %v", expectedPath, err)
				}
			}

			expectedBytes, err := os.ReadFile(expectedPath)
			if err != nil {
				t.Fatalf("failed reading expected snapshot %s: %v (run: go test ./tests/e2e -run Expected -update)", expectedPath, err)
			}

			expected := string(expectedBytes)
			if actual != expected {
				t.Fatalf("unexpected findings for %s in %s mode\n\nexpected:\n%s\nactual:\n%s", relPath, mode, expected, actual)
			}
		})
	}
}

func analyzeFile(t *testing.T, absPath string, strict bool) []string {
	t.Helper()

	fset := token.NewFileSet()
	cfg := &packages.Config{Mode: packages.LoadSyntax, Fset: fset}

	pkgs, err := packages.Load(cfg, absPath)
	if err != nil {
		t.Fatalf("failed to load package for %s: %v", absPath, err)
	}
	if packages.PrintErrors(pkgs) > 0 {
		t.Fatalf("package load errors for %s", absPath)
	}

	registry := ir.NewContractRegistry()
	for _, pkg := range pkgs {
		pipeline.PopulateRegistryFromFiles(registry, pkg.Syntax, fset)
	}

	prog, ssaPkgs := ssautil.Packages(pkgs, ssa.BuilderMode(0))
	prog.Build()

	reporter := report.NewReporter()
	for _, ssaPkg := range ssaPkgs {
		if ssaPkg == nil {
			continue
		}
		pipeline.AnalyzeSSAPackage(ssaPkg, registry, reporter, fset, strict)
	}

	findings := make([]string, 0, len(reporter.Findings))
	for _, d := range reporter.Findings {
		relFile, err := filepath.Rel(mustRepoRoot(t), d.File)
		if err != nil {
			relFile = d.File
		}
		findings = append(findings, fmt.Sprintf("%s:%d:%d: %s", filepath.ToSlash(relFile), d.Line, d.Column, d.Message))
	}
	slices.Sort(findings)
	return findings
}

func mustExampleFiles(t *testing.T, repoRoot string) []string {
	t.Helper()

	pattern := filepath.Join(repoRoot, "examples", "*", "*.go")
	files, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("failed globbing example files: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("no example files found under %s", pattern)
	}
	slices.Sort(files)
	return files
}

func mustRepoRoot(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	root := filepath.Clean(filepath.Join(wd, "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("failed to locate repo root from %s: %v", wd, err)
	}
	return root
}
