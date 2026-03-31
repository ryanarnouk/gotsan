#!/usr/bin/env python3
"""Unified CSV-first metrics workflow for GoBench experiments.

Subcommands:
- table: compute precision/recall/F1 table from count CSV (scope,tp,fn[,fp,untested])
- triage: parse run_all.txt, remove advisories, and generate TP/FP labeling artifacts

Examples:
  python3 calc_metrics.py table --input metrics_input.csv --out computed_metrics.csv
  python3 calc_metrics.py triage --input run_all.txt --outdir triage
"""

from __future__ import annotations

import argparse
import csv
import math
import re
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Optional


def safe_div(numerator: int, denominator: int) -> Optional[float]:
    if denominator == 0:
        return None
    return numerator / denominator


def format_score(value: Optional[float]) -> str:
    if value is None or math.isnan(value):
        return "N/A"
    return f"{value:.3f}"


def parse_nonnegative_int(raw: str, field_name: str, row_num: int, allow_blank: bool = False) -> Optional[int]:
    value = (raw or "").strip()
    if value == "":
        if allow_blank:
            return None
        raise ValueError(f"row {row_num}: missing required field '{field_name}'")
    try:
        parsed = int(value)
    except ValueError as exc:
        raise ValueError(f"row {row_num}: '{field_name}' must be an integer, got '{value}'") from exc
    if parsed < 0:
        raise ValueError(f"row {row_num}: '{field_name}' must be non-negative, got {parsed}")
    return parsed


def write_csv_rows(out_path: Optional[Path], fieldnames: list[str], rows: list[dict[str, str]]) -> None:
    if out_path is None:
        writer = csv.DictWriter(sys.stdout, fieldnames=fieldnames)
        writer.writeheader()
        for row in rows:
            writer.writerow(row)
        return

    with out_path.open("w", newline="", encoding="utf-8") as handle:
        writer = csv.DictWriter(handle, fieldnames=fieldnames)
        writer.writeheader()
        for row in rows:
            writer.writerow(row)


# -----------------------------
# table mode
# -----------------------------


@dataclass
class MetricRow:
    scope: str
    tp: int
    fn: int
    fp: Optional[int]
    untested: Optional[int]


def load_metric_rows(input_path: Path) -> list[MetricRow]:
    with input_path.open("r", newline="", encoding="utf-8") as handle:
        reader = csv.DictReader(handle)
        required = {"scope", "tp", "fn"}
        missing = required - set(reader.fieldnames or [])
        if missing:
            needed = ", ".join(sorted(missing))
            raise ValueError(f"missing required CSV columns: {needed}")

        rows: list[MetricRow] = []
        for row_num, raw in enumerate(reader, start=2):
            scope = (raw.get("scope") or "").strip()
            if not scope:
                raise ValueError(f"row {row_num}: 'scope' cannot be empty")

            tp = parse_nonnegative_int(raw.get("tp", ""), "tp", row_num)  # type: ignore[arg-type]
            fn = parse_nonnegative_int(raw.get("fn", ""), "fn", row_num)  # type: ignore[arg-type]
            fp = parse_nonnegative_int(raw.get("fp", ""), "fp", row_num, allow_blank=True)
            untested = parse_nonnegative_int(raw.get("untested", ""), "untested", row_num, allow_blank=True)
            rows.append(MetricRow(scope=scope, tp=tp, fn=fn, fp=fp, untested=untested))

        if not rows:
            raise ValueError("input CSV has no data rows")
        return rows


def computed_metric_rows(rows: list[MetricRow]) -> list[dict[str, str]]:
    out: list[dict[str, str]] = []
    for row in rows:
        recall = safe_div(row.tp, row.tp + row.fn)

        if row.fp is None:
            precision = None
            f1 = None
            precision_status = "missing_fp"
            fp_str = ""
        else:
            precision = safe_div(row.tp, row.tp + row.fp)
            if precision is None or recall is None or (precision + recall) == 0:
                f1 = None
            else:
                f1 = 2 * precision * recall / (precision + recall)
            precision_status = "ok"
            fp_str = str(row.fp)

        out.append(
            {
                "scope": row.scope,
                "tp": str(row.tp),
                "fn": str(row.fn),
                "fp": fp_str,
                "untested": "" if row.untested is None else str(row.untested),
                "precision": format_score(precision),
                "recall": format_score(recall),
                "f1": format_score(f1),
                "precision_status": precision_status,
            }
        )

    return out


def cmd_table(args: argparse.Namespace) -> int:
    input_path = Path(args.input)
    if not input_path.exists():
        print(f"error: input file not found: {input_path}", file=sys.stderr)
        return 2

    try:
        rows = load_metric_rows(input_path)
        computed_rows = computed_metric_rows(rows)
    except ValueError as err:
        print(f"error: {err}", file=sys.stderr)
        return 2

    fieldnames = ["scope", "tp", "fn", "fp", "untested", "precision", "recall", "f1", "precision_status"]
    out_path = Path(args.out) if args.out else None
    write_csv_rows(out_path, fieldnames, computed_rows)
    if out_path is not None:
        print(f"wrote {out_path}")
    return 0


# -----------------------------
# triage mode
# -----------------------------


CASE_HEADER_RE = re.compile(r"^\[all\s+(\d+)/(\d+)\]\s+(.+)$")
REPORT_HEADER_RE = re.compile(r"^GOTSAN REPORT\s+-\s+(\d+) finding\(s\)$")
FINDING_RE = re.compile(r"^(.+?):(\d+):(\d+):\s+(.*)$")


@dataclass
class Finding:
    case_index: int
    case_total: int
    case_path: str
    source_path: str
    line: int
    column: int
    message: str


@dataclass
class CaseResult:
    case_index: int
    case_total: int
    case_path: str
    report_count_declared: int
    findings: list[Finding]


def parse_run_all(text: str) -> list[CaseResult]:
    lines = text.splitlines()
    cases: list[CaseResult] = []

    i = 0
    while i < len(lines):
        m = CASE_HEADER_RE.match(lines[i].strip())
        if not m:
            i += 1
            continue

        case_index = int(m.group(1))
        case_total = int(m.group(2))
        case_path = m.group(3).strip()

        i += 1
        report_count_declared = 0
        findings: list[Finding] = []

        while i < len(lines):
            nxt = lines[i].strip()
            if CASE_HEADER_RE.match(nxt):
                break

            report_m = REPORT_HEADER_RE.match(nxt)
            if report_m:
                report_count_declared = int(report_m.group(1))
                i += 1
                saw_report_content = False

                while i < len(lines):
                    row = lines[i].rstrip("\n")
                    trimmed = row.strip()
                    if CASE_HEADER_RE.match(trimmed):
                        break
                    if not trimmed:
                        i += 1
                        continue
                    if trimmed.startswith("===="):
                        if saw_report_content:
                            break
                        i += 1
                        continue

                    f = FINDING_RE.match(trimmed)
                    if f:
                        saw_report_content = True
                        findings.append(
                            Finding(
                                case_index=case_index,
                                case_total=case_total,
                                case_path=case_path,
                                source_path=f.group(1),
                                line=int(f.group(2)),
                                column=int(f.group(3)),
                                message=f.group(4).strip(),
                            )
                        )
                    i += 1
                continue

            i += 1

        cases.append(
            CaseResult(
                case_index=case_index,
                case_total=case_total,
                case_path=case_path,
                report_count_declared=report_count_declared,
                findings=findings,
            )
        )

    return cases


def split_case_path(case_path: str) -> tuple[str, str, str, str, str]:
    parts = case_path.split("/")
    category = "/".join(parts[:2]) if len(parts) >= 2 else ""
    project = parts[2] if len(parts) >= 3 else ""
    bug_id = parts[3] if len(parts) >= 4 else ""
    file_name = parts[4] if len(parts) >= 5 else ""
    case_id = file_name.removesuffix("_test.go") if file_name else case_path.replace("/", "__")
    return category, project, bug_id, file_name, case_id


def write_report_only(out_path: Path, cases: list[CaseResult]) -> None:
    chunks: list[str] = []
    sep = "=" * 60

    for case in cases:
        chunks.append(sep)
        chunks.append(f"[all {case.case_index}/{case.case_total}] {case.case_path}")
        chunks.append(sep)
        chunks.append("")

        count = len(case.findings)
        chunks.append(sep)
        chunks.append(f"GOTSAN REPORT - {count} finding(s)")
        chunks.append(sep)
        if count == 0:
            chunks.append("(no report findings)")
        else:
            for finding in case.findings:
                chunks.append(f"{finding.source_path}:{finding.line}:{finding.column}: {finding.message}")
        chunks.append(sep)
        chunks.append("")

    out_path.write_text("\n".join(chunks) + "\n", encoding="utf-8")


def write_findings_csv(out_path: Path, cases: list[CaseResult]) -> None:
    fieldnames = [
        "case_index",
        "case_total",
        "case_path",
        "category",
        "project",
        "bug_id",
        "case_id",
        "source_path",
        "line",
        "column",
        "message",
        "label",
        "notes",
    ]

    existing_labels: dict[str, tuple[str, str]] = {}
    if out_path.exists():
        with out_path.open("r", newline="", encoding="utf-8") as handle:
            reader = csv.DictReader(handle)
            for row in reader:
                key = "|".join(
                    [
                        (row.get("case_path") or ""),
                        (row.get("source_path") or ""),
                        (row.get("line") or ""),
                        (row.get("column") or ""),
                        (row.get("message") or ""),
                    ]
                )
                existing_labels[key] = ((row.get("label") or "").strip(), (row.get("notes") or "").strip())

    with out_path.open("w", newline="", encoding="utf-8") as handle:
        writer = csv.DictWriter(handle, fieldnames=fieldnames)
        writer.writeheader()

        for case in cases:
            category, project, bug_id, _, case_id = split_case_path(case.case_path)
            for finding in case.findings:
                key = "|".join(
                    [
                        case.case_path,
                        finding.source_path,
                        str(finding.line),
                        str(finding.column),
                        finding.message,
                    ]
                )
                prior_label, prior_notes = existing_labels.get(key, ("", ""))
                writer.writerow(
                    {
                        "case_index": case.case_index,
                        "case_total": case.case_total,
                        "case_path": case.case_path,
                        "category": category,
                        "project": project,
                        "bug_id": bug_id,
                        "case_id": case_id,
                        "source_path": finding.source_path,
                        "line": finding.line,
                        "column": finding.column,
                        "message": finding.message,
                        "label": prior_label,
                        "notes": prior_notes,
                    }
                )


def write_case_summary_csv(out_path: Path, cases: list[CaseResult]) -> None:
    fieldnames = [
        "case_index",
        "case_total",
        "case_path",
        "category",
        "project",
        "bug_id",
        "case_id",
        "finding_count",
        "include_in_metrics",
        "case_label",
        "notes",
    ]

    existing_labels: dict[str, tuple[str, str, str]] = {}
    if out_path.exists():
        with out_path.open("r", newline="", encoding="utf-8") as handle:
            reader = csv.DictReader(handle)
            for row in reader:
                key = (row.get("case_path") or "").strip()
                include = (row.get("include_in_metrics") or "").strip()
                if include == "":
                    include = "1"
                existing_labels[key] = (
                    include,
                    (row.get("case_label") or "").strip(),
                    (row.get("notes") or "").strip(),
                )

    with out_path.open("w", newline="", encoding="utf-8") as handle:
        writer = csv.DictWriter(handle, fieldnames=fieldnames)
        writer.writeheader()

        for case in cases:
            category, project, bug_id, _, case_id = split_case_path(case.case_path)
            prior_include, prior_label, prior_notes = existing_labels.get(case.case_path, ("1", "", ""))
            writer.writerow(
                {
                    "case_index": case.case_index,
                    "case_total": case.case_total,
                    "case_path": case.case_path,
                    "category": category,
                    "project": project,
                    "bug_id": bug_id,
                    "case_id": case_id,
                    "finding_count": len(case.findings),
                    "include_in_metrics": prior_include,
                    "case_label": prior_label,
                    "notes": prior_notes,
                }
            )


def is_truthy_flag(value: str) -> bool:
    return value.strip().lower() in {"1", "true", "t", "yes", "y"}


def normalize_finding_label(value: str) -> str:
    normalized = value.strip().upper()
    if normalized in {"TP", "FP", "DUPLICATE", "DUP"}:
        if normalized == "DUP":
            return "DUPLICATE"
        return normalized
    return ""


METRICS_FIELDNAMES = [
    "scope",
    "total_cases",
    "included_cases",
    "excluded_cases",
    "tp_labeled_findings",
    "fp_labeled_findings",
    "duplicate_labeled_findings",
    "unlabeled_findings",
    "tp_case",
    "fn_case_derived",
    "fp_case_derived",
    "predicted_positive_cases",
    "false_positive_rate_per_test_case_level",
    "precision_finding_level",
    "precision_case_level",
    "recall_case_level",
    "f1_case_level",
]


def new_counter() -> dict[str, int]:
    return {
        "total_cases": 0,
        "included_cases": 0,
        "excluded_cases": 0,
        "tp_labeled_findings": 0,
        "fp_labeled_findings": 0,
        "duplicate_labeled_findings": 0,
        "unlabeled_findings": 0,
        "tp_case_from_case_label": 0,
        "predicted_positive_cases": 0,
    }


def build_metrics_row(scope: str, counts: dict[str, int], tp_case_from_findings: int) -> dict[str, str]:
    tp_case = max(counts["tp_case_from_case_label"], tp_case_from_findings)
    fn_case = max(counts["included_cases"] - tp_case, 0)
    fp_case = max(counts["predicted_positive_cases"] - tp_case, 0)

    precision_finding = safe_div(counts["tp_labeled_findings"], counts["tp_labeled_findings"] + counts["fp_labeled_findings"])
    precision_case = safe_div(tp_case, counts["predicted_positive_cases"])
    recall_case = safe_div(tp_case, counts["included_cases"])
    false_positive_rate_per_test = safe_div(fp_case, counts["included_cases"])
    if precision_case is None or recall_case is None or (precision_case + recall_case) == 0:
        f1_case = None
    else:
        f1_case = 2 * precision_case * recall_case / (precision_case + recall_case)

    return {
        "scope": scope,
        "total_cases": str(counts["total_cases"]),
        "included_cases": str(counts["included_cases"]),
        "excluded_cases": str(counts["excluded_cases"]),
        "tp_labeled_findings": str(counts["tp_labeled_findings"]),
        "fp_labeled_findings": str(counts["fp_labeled_findings"]),
        "duplicate_labeled_findings": str(counts["duplicate_labeled_findings"]),
        "unlabeled_findings": str(counts["unlabeled_findings"]),
        "tp_case": str(tp_case),
        "fn_case_derived": str(fn_case),
        "fp_case_derived": str(fp_case),
        "predicted_positive_cases": str(counts["predicted_positive_cases"]),
        "false_positive_rate_per_test_case_level": format_score(false_positive_rate_per_test),
        "precision_finding_level": format_score(precision_finding),
        "precision_case_level": format_score(precision_case),
        "recall_case_level": format_score(recall_case),
        "f1_case_level": format_score(f1_case),
    }


def write_metrics_summary_csv(out_path: Path, out_by_category_path: Path, findings_csv: Path, case_csv: Path) -> None:
    all_counts = new_counter()
    by_category_counts: dict[str, dict[str, int]] = {}
    included_case_paths: set[str] = set()
    case_path_to_category: dict[str, str] = {}

    with case_csv.open("r", newline="", encoding="utf-8") as handle:
        reader = csv.DictReader(handle)
        for row in reader:
            case_path = (row.get("case_path") or "").strip()
            category = (row.get("category") or "").strip() or "unknown"
            include_flag = (row.get("include_in_metrics") or "1").strip()

            cat_counts = by_category_counts.setdefault(category, new_counter())
            all_counts["total_cases"] += 1
            cat_counts["total_cases"] += 1

            if not is_truthy_flag(include_flag):
                all_counts["excluded_cases"] += 1
                cat_counts["excluded_cases"] += 1
                continue

            all_counts["included_cases"] += 1
            cat_counts["included_cases"] += 1
            if case_path:
                included_case_paths.add(case_path)
                case_path_to_category[case_path] = category

            try:
                count = int((row.get("finding_count") or "0").strip())
            except ValueError:
                count = 0
            if count > 0:
                all_counts["predicted_positive_cases"] += 1
                cat_counts["predicted_positive_cases"] += 1

            label = (row.get("case_label") or "").strip().upper()
            if label == "TP":
                all_counts["tp_case_from_case_label"] += 1
                cat_counts["tp_case_from_case_label"] += 1

    tp_cases_from_findings_all: set[str] = set()
    tp_cases_from_findings_by_category: dict[str, set[str]] = {}

    with findings_csv.open("r", newline="", encoding="utf-8") as handle:
        reader = csv.DictReader(handle)
        for row in reader:
            case_path = (row.get("case_path") or "").strip()
            if case_path not in included_case_paths:
                continue

            category = case_path_to_category.get(case_path, (row.get("category") or "").strip() or "unknown")
            cat_counts = by_category_counts.setdefault(category, new_counter())

            label = normalize_finding_label((row.get("label") or ""))
            if label == "TP":
                all_counts["tp_labeled_findings"] += 1
                cat_counts["tp_labeled_findings"] += 1
                tp_cases_from_findings_all.add(case_path)
                tp_cases_from_findings_by_category.setdefault(category, set()).add(case_path)
            elif label == "FP":
                all_counts["fp_labeled_findings"] += 1
                cat_counts["fp_labeled_findings"] += 1
            elif label == "DUPLICATE":
                all_counts["duplicate_labeled_findings"] += 1
                cat_counts["duplicate_labeled_findings"] += 1
            else:
                all_counts["unlabeled_findings"] += 1
                cat_counts["unlabeled_findings"] += 1

    overall_row = build_metrics_row("overall", all_counts, len(tp_cases_from_findings_all))
    write_csv_rows(out_path, METRICS_FIELDNAMES, [overall_row])

    category_rows: list[dict[str, str]] = []
    for category in sorted(by_category_counts):
        category_rows.append(
            build_metrics_row(
                category,
                by_category_counts[category],
                len(tp_cases_from_findings_by_category.get(category, set())),
            )
        )
    category_rows.append(overall_row)
    write_csv_rows(out_by_category_path, METRICS_FIELDNAMES, category_rows)


def cmd_triage(args: argparse.Namespace) -> int:
    input_path = Path(args.input)
    if not input_path.exists():
        print(f"error: input file not found: {input_path}", file=sys.stderr)
        return 2

    text = input_path.read_text(encoding="utf-8")
    cases = parse_run_all(text)
    if not cases:
        print("error: no [all i/n] case sections found in input", file=sys.stderr)
        return 2

    outdir = Path(args.outdir)
    if not outdir.is_absolute():
        outdir = input_path.parent / outdir
    outdir.mkdir(parents=True, exist_ok=True)

    report_only = outdir / "report_only.txt"
    findings_csv = outdir / "report_findings.csv"
    case_csv = outdir / "case_summary.csv"
    metrics_csv = outdir / "metrics_summary.csv"
    metrics_by_category_csv = outdir / "metrics_by_category.csv"
    case_details_csv = outdir / "case_details.csv"

    write_report_only(report_only, cases)
    write_findings_csv(findings_csv, cases)
    write_case_summary_csv(case_csv, cases)
    write_metrics_summary_csv(metrics_csv, metrics_by_category_csv, findings_csv, case_csv)

    # Write per-case details
    write_case_details_csv(case_details_csv, findings_csv, case_csv)

    print(f"parsed cases: {len(cases)}")
    print(f"wrote: {report_only}")
    print(f"wrote: {findings_csv}")
    print(f"wrote: {case_csv}")
    print(f"wrote: {metrics_csv}")
    print(f"wrote: {metrics_by_category_csv}")
    print(f"wrote: {case_details_csv}")
    return 0


def write_case_details_csv(out_path: Path, findings_csv: Path, case_csv: Path) -> None:
    # Load findings by case_path
    from collections import defaultdict
    findings_by_case = defaultdict(list)
    with findings_csv.open("r", newline="", encoding="utf-8") as handle:
        reader = csv.DictReader(handle)
        for row in reader:
            findings_by_case[row["case_path"]].append(row)

    # Load case summary
    cases = []
    with case_csv.open("r", newline="", encoding="utf-8") as handle:
        reader = csv.DictReader(handle)
        for row in reader:
            cases.append(row)

    # Prepare output
    fieldnames = [
        "case_path",
        "finding_count",
        "tp_count",
        "fp_count",
        "duplicate_count",
        "unlabeled_count",
        "precision",
        "recall",
        "include_in_metrics",
        "case_label",
        "notes",
    ]
    def norm_label(label):
        l = (label or "").strip().upper()
        if l == "DUP":
            return "DUPLICATE"
        return l
    rows = []
    for case in cases:
        case_path = case["case_path"]
        findings = findings_by_case.get(case_path, [])
        tp = sum(1 for f in findings if norm_label(f["label"]) == "TP")
        fp = sum(1 for f in findings if norm_label(f["label"]) == "FP")
        dup = sum(1 for f in findings if norm_label(f["label"]) == "DUPLICATE")
        unlabeled = sum(1 for f in findings if not (f["label"] or "").strip())
        precision = safe_div(tp, tp + fp)
        recall = safe_div(tp, len(findings))
        rows.append({
            "case_path": case_path,
            "finding_count": str(len(findings)),
            "tp_count": str(tp),
            "fp_count": str(fp),
            "duplicate_count": str(dup),
            "unlabeled_count": str(unlabeled),
            "precision": format_score(precision),
            "recall": format_score(recall),
            "include_in_metrics": case.get("include_in_metrics", ""),
            "case_label": case.get("case_label", ""),
            "notes": case.get("notes", ""),
        })
    write_csv_rows(out_path, fieldnames, rows)


# -----------------------------
# CLI
# -----------------------------


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Unified CSV metrics workflow (table + triage)")
    sub = parser.add_subparsers(dest="command")

    table = sub.add_parser("table", help="Compute CSV precision/recall/F1 table from count CSV")
    table.add_argument("--input", "-i", required=True, help="CSV with scope,tp,fn and optional fp,untested")
    table.add_argument("--out", "-o", default="", help="Optional output CSV file (prints CSV to stdout if omitted)")
    table.set_defaults(handler=cmd_table)

    triage = sub.add_parser("triage", help="Parse run_all.txt into advisory-free triage artifacts")
    triage.add_argument("--input", "-i", required=True, help="Path to run_all.txt")
    triage.add_argument("--outdir", "-o", default="triage", help="Output directory (default: triage)")
    triage.set_defaults(handler=cmd_triage)

    return parser


def main(argv: Optional[list[str]] = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)

    if not getattr(args, "command", None):
        parser.print_help()
        return 2

    return args.handler(args)


if __name__ == "__main__":
    raise SystemExit(main())
