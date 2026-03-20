# GoBench Samples

Credit: each test in the folder are modified bug kernels from *GoBench: A Benchmark Suite of Real-World Go Concurrency Bugs*.

We filtered the benchmark test suite Goker (from Gobench) to only consider bugs solely caused by Mutex-related constructs (i.e., no mixed bugs). The changes to each test are minimal to those in Goker, with only annotations added to the files to test our tool Gotsan.

Citation:

Yuan, T., Li, G., Lu, J., Liu, C., Li, L., & Xue, J. (2021, February). Gobench: A benchmark suite of real-world go concurrency bugs. In 2021 IEEE/ACM International Symposium on Code Generation and Optimization (CGO) (pp. 187-199). IEEE.

Repository link:

https://github.com/timmyyuan/gobench
