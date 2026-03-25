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

Use `-l` for lenient mode (utilizes heuristics to lower warnings emitted based on observable goroutine paths in the code):

```bash
go run main.go -pkg <path to pkg> -l
```

Use `-s` for strict mode (does not assume anything about the thread the code is running in):

```bash
go run main.go -pkg <path to pkg> -s
```

`-l` and `-s` are mutually exclusive.

## Project Structure
- `/analyzer`: SSA and CFG analysis
- `/ir`: internal representation for the analysis tool after the parser completes 
- `/parse`: parse annotations from the source file or package

### Tests

Run all tests from repo root:

```bash
go test ./...
```

Run examples end-to-end suite only:

```bash
go test ./tests/e2e
```

Regenerate expected snapshot files after intentional analyzer output changes:

```bash
go test ./tests/e2e -run Expected -update
```

Golden snapshot naming convention:

- `*.lenient.expected`: expected findings for lenient mode (non-strict run)
- `*.strict.expected`: expected findings for strict mode

Example:

- `tests/e2e/testdata/examples__simple__simple.lenient.expected`
- `tests/e2e/testdata/examples__simple__simple.strict.expected`
