package main

import (
	"gotsan/pipeline"

	"golang.org/x/tools/go/analysis/singlechecker"
)

// Executable that uses the go/analysis/singlechecker
// package
func main() {
	singlechecker.Main(pipeline.GoAnalysisAnalyzer)
}
