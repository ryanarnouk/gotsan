#!/usr/bin/env python3
"""
Run analyzer against gobench samples and save results to JSON.

Supports flexible selectors:
  - all: run all test cases
  - blocking: run all blocking tests
  - nonblocking/non_blocking: run all nonblocking tests
  - subtype (abba_deadlock, double_locking, etc): run by bug type
  - test name (cockroach7504): run specific test by name
"""

import argparse
import json
import os
import re
import subprocess
import sys
from datetime import datetime, timezone
from pathlib import Path


def get_script_dir():
    """Get the directory where this script is located."""
    return Path(__file__).parent.resolve()


def get_repo_root():
    """Get the repository root (two levels up from script)."""
    return get_script_dir().parent.parent


def strip_path(full_path: str, repo_root: Path) -> str:
    """Convert absolute path to repo-relative path."""
    try:
        return str(Path(full_path).relative_to(repo_root))
    except ValueError:
        return str(Path(full_path).name)


def normalize_selector(sel: str) -> str:
    """Normalize selector for comparison (lowercase, alphanumeric only)."""
    return re.sub(r'[^a-z0-9]', '', sel.lower())


def canonical_selector(raw_sel: str) -> str:
    """Convert selector aliases to canonical form."""
    normalized = normalize_selector(raw_sel)
    
    if normalized in ('nonblocking', 'nonblock'):
        return 'nonblocking'
    if normalized in ('doubledeadlock', 'doublelocking'):
        return 'double_locking'
    
    return raw_sel


def collect_test_files(script_dir: Path) -> list:
    """Recursively find all .go test files."""
    return sorted(script_dir.glob('**/*.go'))


def get_relative_path(file_path: Path, script_dir: Path) -> str:
    """Get path relative to script directory."""
    try:
        return str(file_path.relative_to(script_dir))
    except ValueError:
        return str(file_path)


def matches_selector(selector_normalized: str, rel_path: str) -> bool:
    """Check if a file matches the given selector."""
    parts = rel_path.split('/')
    
    # Extract components
    category = parts[0] if len(parts) > 0 else ''
    subtype = parts[1] if len(parts) > 1 else ''
    filename = Path(rel_path).name
    base_stem = filename.rsplit('.', 1)[0].replace('_test', '')
    
    if selector_normalized == 'all':
        return True
    
    if selector_normalized == normalize_selector(category):
        return True
    
    if selector_normalized == normalize_selector(subtype):
        return True
    
    if selector_normalized == normalize_selector(base_stem):
        return True
    
    return False


def strip_path_from_output(output: str, repo_root: Path) -> str:
    """Remove absolute paths from analyzer output."""
    # Replace absolute paths with repo-relative paths
    repo_root_str = str(repo_root)
    
    def replace_path(match):
        full_path = match.group(1)
        try:
            rel_path = Path(full_path).relative_to(repo_root_str)
            return str(rel_path)
        except ValueError:
            return full_path
    
    # Match file paths (sequences of path separators and alphanumeric chars)
    pattern = r'(' + re.escape(repo_root_str) + r'[/\\][^\s:]+)'
    output = re.sub(pattern, replace_path, output)
    
    return output


def run_test_case(
    script_dir: Path,
    repo_root: Path,
    rel_file: str,
    analyzer_flags: list,
) -> tuple:
    """
    Run analyzer on a single test file.
    
    Returns: (exit_code, output)
    """
    cmd = [
        'go', 'run', 'main.go',
        '-file', rel_file,
        *analyzer_flags
    ]
    
    try:
        result = subprocess.run(
            cmd,
            cwd=str(repo_root),
            capture_output=True,
            text=True,
            timeout=60
        )
        
        output = result.stdout + result.stderr
        # Strip absolute paths from output
        output = strip_path_from_output(output, repo_root)
        
        return result.returncode, output
    except subprocess.TimeoutExpired:
        return 1, '[timeout after 60 seconds]'
    except Exception as e:
        return 1, f'[error running analyzer: {e}]'


def parse_args():
    """Custom argument parsing to handle --save with optional path."""
    args = sys.argv[1:]
    
    if not args or args[0] in ('-h', '--help'):
        print(__doc__)
        parser = argparse.ArgumentParser(description='Run analyzer against gobench samples')
        parser.print_help()
        sys.exit(0 if args and args[0] in ('-h', '--help') else 1)
    
    save_flag = False
    save_path = None
    selector = None
    analyzer_flags = []
    
    i = 0
    while i < len(args):
        arg = args[i]
        
        if arg == '--save':
            save_flag = True
            # Check if next arg looks like a path
            if i + 1 < len(args):
                next_arg = args[i + 1]
                if next_arg.endswith('.json') or '/' in next_arg:
                    save_path = next_arg
                    i += 2
                    continue
            i += 1
            continue
        
        if selector is None:
            selector = arg
            i += 1
            continue
        
        # Everything else is analyzer flags
        analyzer_flags.append(arg)
        i += 1
    
    if not selector:
        print("Error: selector is required", file=sys.stderr)
        sys.exit(1)
    
    return save_flag, save_path, selector, analyzer_flags


def main():
    save_flag, save_path, selector, analyzer_flags = parse_args()
    
    script_dir = get_script_dir()
    repo_root = get_repo_root()
    
    # Determine report path
    report_path = None
    if save_flag:
        if save_path is None:
            results_dir = script_dir / 'results'
            results_dir.mkdir(exist_ok=True)
            timestamp = datetime.now().strftime('%Y%m%d_%H%M%S')
            report_path = results_dir / f'run_{timestamp}.json'
        else:
            # Convert relative paths to absolute (relative to script dir, not cwd)
            report_path = Path(save_path)
            if not report_path.is_absolute():
                report_path = script_dir / report_path
    
    # Collect and filter test files
    all_files = collect_test_files(script_dir)
    selector_canonical = canonical_selector(selector)
    selector_normalized = normalize_selector(selector_canonical)
    
    filtered_files = []
    for file_path in all_files:
        # For display: relative to script_dir
        display_rel_path = get_relative_path(file_path, script_dir)
        if matches_selector(selector_normalized, display_rel_path):
            # For analyzer: relative to repo_root
            analyzer_rel_path = str(file_path.relative_to(repo_root))
            filtered_files.append((file_path, display_rel_path, analyzer_rel_path))
    
    if not filtered_files:
        print(f"No matching .go files found for selector '{selector}'", file=sys.stderr)
        return 1
    cases = []
    print()
    
    for idx, (file_path, display_rel_path, analyzer_rel_path) in enumerate(filtered_files, 1):
        print('=' * 60)
        print(f'[{selector} {idx}/{len(filtered_files)}] {display_rel_path}')
        print('=' * 60)
        
        exit_code, output = run_test_case(
            script_dir,
            repo_root,
            analyzer_rel_path,
            analyzer_flags
        )
        
        # Display output line by line
        if output.strip():
            for line in output.rstrip().split('\n'):
                print(line)
        else:
            print('[no output from analyzer]')
        
        print(f'-- exit code: {exit_code}')
        print()
        
        cases.append({
            'file': display_rel_path,
            'exit_code': exit_code,
            'output': output
        })
    
    # Summary
    print(f"[{selector}] completed {len(filtered_files)} case(s)")
    
    # Save report if requested
    if report_path:
        report_path.parent.mkdir(parents=True, exist_ok=True)
        
        report_data = {
            'selector': selector,
            'generated_at': datetime.now(timezone.utc).isoformat().replace('+00:00', 'Z'),
            'cases': cases,
            'summary': {
                'total': len(filtered_files),
                'passed': sum(1 for c in cases if c['exit_code'] == 0),
                'failed': sum(1 for c in cases if c['exit_code'] != 0),
            }
        }
        
        with open(report_path, 'w') as f:
            json.dump(report_data, f, indent=2)
        
        # Try to show relative path, fall back to absolute
        try:
            rel_report_path = report_path.relative_to(repo_root)
            print(f"[{selector}] report written to: {rel_report_path}")
        except ValueError:
            print(f"[{selector}] report written to: {report_path}")
    
    return 0


if __name__ == '__main__':
    sys.exit(main())
