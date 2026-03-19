#!/usr/bin/env bash
set -uo pipefail

usage() {
  cat <<'EOF'
Run analyzer against gobench sample groups and print output to stdout.

Usage:
  evaluation/gobench_samples/run_cases.sh <failure|success|all> [analyzer flags...]

Examples:
  evaluation/gobench_samples/run_cases.sh failure
  evaluation/gobench_samples/run_cases.sh success -l
  evaluation/gobench_samples/run_cases.sh all -s
EOF
}

if [[ $# -lt 1 ]]; then
  usage
  exit 1
fi

mode="$1"
shift || true

case "$mode" in
  failure|success|all) ;;
  -h|--help)
    usage
    exit 0
    ;;
  *)
    echo "Error: invalid mode '$mode'. Expected failure, success, or all." >&2
    usage
    exit 1
    ;;
esac

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

collect_group_files() {
  local group="$1"
  find "$SCRIPT_DIR/$group" -type f -name '*.go' -print0 | xargs -0 -I{} printf '%s\n' "{}" | sort
}

run_group() {
  local group="$1"
  local -a files=()

  while IFS= read -r file; do
    [[ -n "$file" ]] && files+=("$file")
  done < <(collect_group_files "$group")

  if [[ ${#files[@]} -eq 0 ]]; then
    echo "No .go files found under $SCRIPT_DIR/$group"
    return 0
  fi

  local index=0
  local failed=0

  for file in "${files[@]}"; do
    index=$((index + 1))
    echo ""
    echo "============================================================"
    echo "[$group $index/${#files[@]}] $file"
    echo "============================================================"

    (
      cd "$REPO_ROOT" || exit 1
      go run main.go -file "$file" "$@"
    )
    local status=$?

    echo "-- exit code: $status"
    if [[ $status -ne 0 ]]; then
      failed=$((failed + 1))
    fi
  done

  echo ""
  echo "[$group] completed ${#files[@]} case(s), failures: $failed"

  if [[ $failed -ne 0 ]]; then
    return 1
  fi
  return 0
}

overall_status=0

if [[ "$mode" == "failure" || "$mode" == "all" ]]; then
  run_group failure "$@" || overall_status=1
fi

if [[ "$mode" == "success" || "$mode" == "all" ]]; then
  run_group success "$@" || overall_status=1
fi

exit $overall_status
