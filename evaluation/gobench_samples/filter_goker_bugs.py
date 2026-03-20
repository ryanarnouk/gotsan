#!/usr/bin/env python3
"""A utility script to copy tests from the GoBench project's GoKer bug folders by config Type/Subtype and copy matches.

For use in the GoBench repository.

Usage example:
  python3 filter_goker_bugs.py \
    --subtype "Resource Deadlock" \
    --subsubtype "RWR deadlock" \
      --destination /tmp/goker-selection \
      --dry-run
"""

from __future__ import annotations

import argparse
import json
import shutil
import sys
from pathlib import Path


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description=(
            "Scan GoKer config files, filter by Type/Subtype, "
            "and copy matching bug folders to a separate destination."
        )
    )
    parser.add_argument(
        "--goker-root",
        type=Path,
        default=Path("gobench/goker"),
        help="Path to GoKer root (default: gobench/goker)",
    )
    parser.add_argument(
        "--config-root",
        type=Path,
        default=Path("gobench/configures/goker"),
        help="Path to GoKer config root (default: gobench/configures/goker)",
    )
    parser.add_argument(
        "--subtype",
        required=True,
        help="Desired SubType value (maps to config 'type' field; case-insensitive exact match)",
    )
    parser.add_argument(
        "--subsubtype",
        required=True,
        help="Desired SubsubType value (maps to config 'subtype' field; case-insensitive exact match)",
    )
    parser.add_argument(
        "--destination",
        type=Path,
        required=True,
        help="Directory where matching bug folders will be copied",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Only print matching folders without copying",
    )
    parser.add_argument(
        "--overwrite",
        action="store_true",
        help="Replace destination folders if they already exist",
    )
    return parser.parse_args()


def normalize(value: str) -> str:
    return " ".join(value.strip().casefold().split())


def parse_bug_key(bug_key: str) -> tuple[str, str] | None:
    """Parse config keys like project_bugid into (project, bug_id)."""
    if "_" not in bug_key:
        return None

    project, bug_id = bug_key.rsplit("_", 1)
    if not project or not bug_id:
        return None
    return project, bug_id


def load_config(path: Path) -> dict[str, dict[str, str]]:
    data = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(data, dict):
        raise ValueError(f"Config must be a JSON object: {path}")
    return data


def copy_bug_folder(src_bug_dir: Path, dst_bug_dir: Path, overwrite: bool) -> None:
    if dst_bug_dir.exists():
        if not overwrite:
            raise FileExistsError(f"Destination already exists: {dst_bug_dir}")
        shutil.rmtree(dst_bug_dir)

    dst_bug_dir.parent.mkdir(parents=True, exist_ok=True)
    shutil.copytree(src_bug_dir, dst_bug_dir)


def main() -> int:
    args = parse_args()
    goker_root = args.goker_root.resolve()
    config_root = args.config_root.resolve()
    destination = args.destination.resolve()
    wanted_type = normalize(args.subtype)
    wanted_subtype = normalize(args.subsubtype)

    if not config_root.exists() or not config_root.is_dir():
        print(f"error: config root not found or not a directory: {config_root}", file=sys.stderr)
        return 2

    blocking_config = config_root / "blocking.json"
    nonblocking_config = config_root / "nonblocking.json"
    if not blocking_config.exists() or not nonblocking_config.exists():
        print(
            "error: expected blocking.json and nonblocking.json under "
            f"{config_root}",
            file=sys.stderr,
        )
        return 2

    try:
        config_maps = {
            "blocking": load_config(blocking_config),
            "nonblocking": load_config(nonblocking_config),
        }
    except (OSError, json.JSONDecodeError, ValueError) as exc:
        print(f"error: failed loading config files: {exc}", file=sys.stderr)
        return 2

    matched_bug_dirs: list[Path] = []
    skipped_invalid_keys = 0
    missing_bug_dirs = 0
    total_config_entries = 0

    for bug_class, bug_map in config_maps.items():
        for bug_key, bug_info in bug_map.items():
            total_config_entries += 1

            if not isinstance(bug_info, dict):
                continue

            cfg_type = normalize(str(bug_info.get("type", "")))
            cfg_subtype = normalize(str(bug_info.get("subtype", "")))
            if cfg_type != wanted_type or cfg_subtype != wanted_subtype:
                continue

            parsed = parse_bug_key(bug_key)
            if parsed is None:
                skipped_invalid_keys += 1
                continue

            project, bug_id = parsed
            bug_dir = goker_root / bug_class / project / bug_id
            if not bug_dir.exists() or not bug_dir.is_dir():
                missing_bug_dirs += 1
                continue

            matched_bug_dirs.append(bug_dir)

    if not matched_bug_dirs:
        print("No matching bugs found.")
        print(f"Checked config entries: {total_config_entries}")
        print(f"Invalid bug keys skipped: {skipped_invalid_keys}")
        print(f"Missing bug folders skipped: {missing_bug_dirs}")
        return 0

    print(f"Matching bugs: {len(matched_bug_dirs)}")
    print(f"Checked config entries: {total_config_entries}")
    print(f"Invalid bug keys skipped: {skipped_invalid_keys}")
    print(f"Missing bug folders skipped: {missing_bug_dirs}")

    for bug_dir in matched_bug_dirs:
        rel_bug_dir = bug_dir.relative_to(goker_root)
        rel_parts = rel_bug_dir.parts
        # rel_bug_dir looks like: blocking/<project>/<bug_id> or nonblocking/<project>/<bug_id>
        # Drop the first segment so destination does not create blocking/nonblocking folders.
        stripped_rel_bug_dir = Path(*rel_parts[1:]) if len(rel_parts) > 1 else rel_bug_dir
        dst_bug_dir = destination / stripped_rel_bug_dir

        if args.dry_run:
            print(f"[dry-run] {bug_dir} -> {dst_bug_dir}")
            continue

        copy_bug_folder(bug_dir, dst_bug_dir, args.overwrite)
        print(f"Copied: {bug_dir} -> {dst_bug_dir}")

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
