# Gobench Samples

Credit: each test in the folder is modified examples from https://github.com/timmyyuan/gobench.

The code was copied almost exactly for concurrency bugs in the benchmark repository that were determined to be related to shared-memory issues, which makes them useful for tagging and evaluating the static analysis tool. The biggest change in these samples was adding annotations into the test files and renaming the files.

Of the listed concurrency bugs, an LLM identified the subset below as shared-memory bugs. The "Bug Pattern" section details the LLMs description of the bug occurrence, with the following column describing the annotation of each file and whether the tool can determine successfully warn for a related issue.

Status Markers:

| Marker | Meaning |
| --- | --- |
| `[ ]` | Not started |
| `[x]` | Confirmed / done |
| `[-]` | Not applicable or not possible |

# Blocking Shared-Memory Cases

Use this table to record:

| Project | Issue | Bug Pattern | Annotated | Detection | Annotation Experience | Notes |
| --- | --- | --- | --- | --- | --- | --- |
| cockroach | 10214 | AB/BA deadlock between `coalescedMu` and `raftMu` across goroutines | `[x]` | `[ ]` |  | Wrote a patch using LLM query. Needs to be tested to see if this case has been fixed |
| cockroach | 1055 | Mutex held while waiting on `drain.Wait()` | `[x]` | `[-]` |  | Miscategorization, also relies on channels and WaitGroups to reproduce the bug |
| cockroach | 16167 | Recursive `RWMutex` deadlock on second `RLock` with pending writer | `[x]` | `[-]` |  | Unable to detect properly with RWMutex and RLock. Might be out of scope to try to implement for now |
| cockroach | 3710 | Double `RLock` deadlock with pending writer | `[ ]` | `[ ]` |  |  |
| cockroach | 584 | Missing `Unlock()` on `break` path | `[x]` | `[x]` |  | Easy addition and catch |
| cockroach | 6181 | Recursive `RLock` through `fmt.Printf`/`String()` | `[ ]` | `[ ]` |  |  |
| cockroach | 7504 | AB/BA deadlock between `LeaseState.mu` and `tableNameCache.mu` | `[ ]` | `[ ]` |  |  |
| cockroach | 9935 | Re-lock of same mutex through `exit()` | `[ ]` | `[ ]` |  |  |
| etcd | 10492 | Re-entrant lock through `checkpointer.Checkpoint()` | `[ ]` | `[ ]` |  |  |
| etcd | 5509 | Missing `RUnlock()` on early return path | `[ ]` | `[ ]` |  |  |
| etcd | 6708 | Write lock followed by nested read lock | `[ ]` | `[ ]` |  |  |
| grpc | 3017 | Early return without unlocking in timer callback | `[ ]` | `[ ]` |  |  |
| grpc | 795 | Double-lock or missing unlock in `GracefulStop()` | `[x]` | `[ ]` |  |  |
| hugo | 3251 | Write lock blocks later `RLock()` path | `[ ]` | `[ ]` |  |  |
| hugo | 5379 | `sync.Once` closure re-acquires same mutex | `[ ]` | `[ ]` |  |  |
| kubernetes | 13135 | Re-lock after callback chain from `startCaching()` | `[ ]` | `[ ]` |  |  |
| kubernetes | 30872 | AB/BA deadlock between `DeltaFIFO.lock` and `federatedInformerImpl.Lock()` | `[ ]` | `[ ]` |  |  |
| kubernetes | 58107 | `RLock()` held while blocked in `cond.Wait()` | `[ ]` | `[ ]` |  |  |
| kubernetes | 62464 | Recursive `RLock` with pending writer | `[ ]` | `[ ]` |  |  |
| moby | 17176 | Early return without releasing `devices.Lock()` | `[ ]` | `[ ]` |  |  |
| moby | 25384 | `WaitGroup` never fully decremented | `[ ]` | `[ ]` |  |  |
| moby | 27782 | `sync.Cond` wait never signaled for write events | `[ ]` | `[ ]` |  |  |
| moby | 29733 | `sync.Cond` wait with no state update or broadcast | `[ ]` | `[ ]` |  |  |
| moby | 30408 | `sync.Cond` wait with no manifest and no broadcast | `[ ]` | `[ ]` |  |  |
| moby | 36114 | Recursive mutex acquisition across helper call | `[ ]` | `[ ]` |  |  |
| moby | 4951 | AB/BA deadlock between `devices.Lock()` and `info.lock` | `[ ]` | `[ ]` |  |  |
| moby | 7559 | Error path continues without releasing lock | `[ ]` | `[ ]` |  |  |
| syncthing | 4829 | Write lock calls helper that takes read lock | `[ ]` | `[ ]` |  |  |

Summary: 28 pure shared-memory blocking bugs. These are the strongest candidates for annotation-based evaluation.

# Other Blocking Cases

These are still useful to keep listed, but they are less direct fits for lock-annotation evaluation.

## Mixed Bugs

| Project | Issues |
| --- | --- |
| etcd | 6873, 7443, 7492, 7902 |
| grpc | 1353, 1460 |
| istio | 17860 |
| kubernetes | 10182, 11298, 1321, 26980, 6632 |
| moby | 28462 |
| serving | 2137 |

## Channel / Message-Passing Only

| Project | Issues |
| --- | --- |
| cockroach | 10790, 13197, 13755, 1462, 18101, 2448, 24808, 25456, 35073, 35931 |
| etcd | 6857 |
| grpc | 1275, 1424, 660, 862 |
| istio | 16224, 18454 |
| kubernetes | 25331, 38669, 5316, 70277 |
| moby | 21233, 33293, 33781, 4395 |
| syncthing | 5795 |

Summary: 14 mixed bugs and 25 channel-only bugs, for 67 total blocking tests.

# Nonblocking

TODO
