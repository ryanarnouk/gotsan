# Metrics Calculation Script

Use a single script (`calc_metrics.py`) for both count-table metrics and run-all triage.

### 1) Count Table (TP/FN/FP by scope)

- Input: `metrics_input.csv`
- Command:

```bash
python3 calc_metrics.py table --input metrics_input.csv --out computed_metrics.csv
```

If `fp` is blank, precision/F1 are marked unavailable.

### 2) Triage From run_all.txt (advisory-free)

- Command:

```bash
python3 calc_metrics.py triage --input run_all.txt --outdir triage
```

- Generated files:

`triage/report_only.txt` (advisory-free findings)
`triage/report_findings.csv` (label each finding as `TP` or `FP`)
`triage/case_summary.csv` (per-case detection summary)
`triage/metrics_summary.csv` (live precision/recall snapshot)
`triage/metrics_by_category.csv` (same metrics grouped by subcategory, e.g. `blocking/double_locking`)

Both metrics CSVs now include case-level false-positive fields:

- `fp_case_derived`: predicted-positive cases that are not TP cases
- `false_positive_rate_per_test_case_level`: `fp_case_derived / included_cases`

`report_findings.csv` label values:

- `TP`: counts toward true positives
- `FP`: counts toward false positives
- `DUPLICATE` (or `DUP`): excluded from TP/FP scoring; tracked separately as duplicate findings

Re-running triage preserves existing labels and notes.

To exclude invalid benchmark cases (for example, non-mutex data-race kernels), set
`include_in_metrics=0` for those rows in `triage/case_summary.csv`.

`metrics_summary.csv` will then compute precision/recall/F1 using only included cases.