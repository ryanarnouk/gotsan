# Gobench Annotation Checklist

Updated: 2026-03-21T00:42:06.744525Z

Legend: [x] annotated, [ ] needs annotation.
test=pass|untested|fail

## blocking

### abba_deadlock

#### cockroach
- [x] 10214/cockroach10214_test.go | test=pass
- [x] 7504/cockroach7504_test.go | test=pass

#### hugo
- [x] 3251/hugo3251_test.go | test=fail

Locks are objects in a map which is only known dynamically at runtime. Hard to reason about this bug statically without resolving the lock values in the hash map.

#### kubernetes

- [x] 13135/kubernetes13135_test.go | test=pass
- [x] 30872/kubernetes30872_test.go | test=fail

#### moby
- [x] 4951/moby4951_test.go | test=pass

### double_locking

#### cockroach
- [x] 584/cockroach584_test.go | test=pass
- [x] 9935/cockroach9935_test.go | test=pass

#### etcd
- [x] 10492/etcd10492_test.go | test=pass

Was not working at first due to the function that double locks 
being called in a dynamic context. Added some code to apply heuristics to recognize dynamic function context calls and resolve, and now this seems to work. Not sure how generalizable this solution is.

- [x] 5509/etcd5509_test.go | test=untested
- [x] 6708/etcd6708_test.go | test=untested

#### grpc
- [x] 3017/grpc3017_test.go | test=untested
- [x] 795/grpc795_test.go | test=untested

#### hugo
- [ ] 5379/hugo5379_test.go | test=untested

#### moby
- [x] 17176/moby17176_test.go | test=pass
- [ ] 36114/moby36114_test.go | test=untested
- [ ] 7559/moby7559_test.go | test=untested

#### syncthing
- [ ] 4829/syncthing4829_test.go | test=untested

### rwr_deadlock

#### cockroach
- [x] 16167/cockroach16167_test.go | test=pass
- [x] 3710/cockroach3710_test.go | test=pass
- [x] 6181/cockroach6181_test.go | test=fail

#### kubernetes
- [x] 58107/kubernetes58107_test.go | test=pass
- [x] 62464/kubernetes62464_test.go | test=pass

## nonblocking

### data_race

#### etcd
- [ ] 4876/etcd4876_test.go | test=untested
- [ ] 8194/etcd8194_test.go | test=untested
- [ ] 9446/etcd9446_test.go | test=untested

#### grpc
- [ ] 1748/grpc1748_test.go | test=untested
- [ ] 3090/grpc3090_test.go | test=untested

#### istio
- [ ] 16742/istio16742_test.go | test=untested
- [ ] 8144/istio8144_test.go | test=untested
- [ ] 8214/istio8214_test.go | test=untested

#### kubernetes
- [x] 49404/kubernetes49404_test.go | test=fail

WaitGroup, not mutex.

- [x] 77796/kubernetes77796_test.go | test=pass

Early example runs for -race flag vs. static analysis (using `time` command):
Race flag dynamic analysis:  0.25s user 0.29s system 124% cpu 0.434 total
Static analysis:   0.02s user 0.01s system 86
% cpu 0.031 total

- [x] 79631/kubernetes79631_test.go | test=pass
- [x] 80284/kubernetes80284_test.go | test=pass

Also required the addition of `mu` (of type Sync.Mutex) on the Authenticator struct. The original Kubernetes source code contained this type, and it was reduced in the simplified bug kernel. We need to re-add it to tag the type with the `@guarded_by` tag and allow Gotsan to flag the missing lock/unlock operations.

Added the guarded_by flag and adding the lock/unlock in the corresponding location changed the data race to pass when run through the `-race` flag.

- [ ] 81091/kubernetes81091_test.go | test=untested
- [ ] 81148/kubernetes81148_test.go | test=untested
- [ ] 82239/kubernetes82239_test.go | test=untested
- [ ] 82550/kubernetes82550_test.go | test=untested
- [ ] 88331/kubernetes88331_test.go | test=untested
- [ ] 89164/kubernetes89164_test.go | test=untested

#### serving
- [x] 3148/serving3148_test.go | test=fail

Caused by WaitGroup, not a mutex.

- [x] 6472/serving6472_test.go | test=fail

Caused by Sync.Cond, not a straightforward mutex.
