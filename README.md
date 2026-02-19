# Go Thread Safety Analysis

A static analysis tool for the Go that supports expressing lock dependency rules and verifies lock-related preconditions and invariants in shared-memory concurrent programs.

## Overview

TODO (copy from proposal)

Concurrency bugs such as data races and deadlocks in shared-memory systems using mutual exclusion locks arise only under particular runtime timing conditions. In languages like Go, programs do not encode lock ordering or the programmer's intended synchronization patterns, so these properties cannot be checked statically.

By allowing programmers to provide lightweight annotations, lock dependencies and patterns can be verified statically. This is similar to a function signature enforcing a contract at compile time. This enables earlier detection of deadlocks and race-prone locking patterns, before deployment and runtime.

## Proposed Structure
- `/analyzer`: SSA and CFG analysis
- `/ir`: internal representation for the analysis tool after the parser completes 
- `/parse`: parse annotations from the source file or package
