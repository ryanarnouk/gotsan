# Go Thread Safety Analysis

A static analysis tool for Go that supports expressing lock dependency rules and verifies lock-related preconditions, postconditions, invariants, and side effects in shared-memory concurrent programs.

## Overview

TODO (copy from proposal/paper)

Concurrency bugs such as data races and deadlocks in shared-memory systems using mutual exclusion locks arise only under particular runtime timing conditions. In languages like Go, programs do not encode lock ordering or the programmer's intended synchronization patterns, so these properties cannot be checked statically.

By allowing programmers to provide lightweight annotations, lock dependencies and patterns can be verified statically. This is similar to a function signature enforcing a contract at compile time. This enables earlier detection of deadlocks and race-prone locking patterns, before deployment and runtime.

## Setup

### Run Analysis

```bash
go run main.go -file <path to file>
```

or

```bash
go run main.go -pkg <path to pkg>
```

Use `-v` flag for verbose logging features.

### Run Analysis Using go/analysis Analyzer

```bash
go run ./cmd/gotsan-analyzer <path to file or package>
```

This uses `pipeline.GoAnalysisAnalyzer` as a thin adapter over the existing parse + SSA engine.

## Project Structure
- `/analyzer`: SSA and CFG analysis
- `/ir`: internal representation for the analysis tool after the parser completes 
- `/parse`: parse annotations from the source file or package
