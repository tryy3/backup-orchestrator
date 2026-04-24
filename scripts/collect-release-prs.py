#!/usr/bin/env python3
"""Collect PR data from a GitHub release draft for downstream AI summarization.

Parses the release body produced by Release Drafter, extracts PR references,
fetches each PR's full description via gh CLI, and writes a structured JSON
file with extracted sections (release-note block, breaking changes, etc.).

Usage:
  python3 scripts/collect-release-prs.py v0.4.0
  python3 scripts/collect-release-prs.py v0.4.0 --output pr-data.json
  python3 scripts/collect-release-prs.py v0.4.0 --force

Requirements: gh (GitHub CLI, authenticated)
"""

import argparse
import json
import os
import re
import subprocess
import sys
import tempfile
from pathlib import Path
from typing import Optional

# Default cache directory relative to repo root.
CACHE_DIR = Path("docs/version-drafts")
RELEASE_FIELDS = "body,tagName,name,isDraft"
PR_FIELDS = "number,title,body,labels,url,mergedAt,state"


class GhCommandError(RuntimeError):
    """Raised when a gh CLI command fails."""

    def __init__(self, cmd: list[str], stderr: str):
        self.cmd = cmd
        self.stderr = stderr
        super().__init__(f"Error running: {' '.join(cmd)}\n{stderr}")


def run_gh(args: list[str]) -> str:
    """Run a gh CLI command and return stdout."""
    cmd = ["gh"] + args
    result = subprocess.run(cmd, capture_output=True, text=True)
    if result.returncode != 0:
        raise GhCommandError(cmd, result.stderr)
    return result.stdout.strip()


def fetch_release(version: str) -> dict:
    """Fetch release data by tag name."""
    raw = run_gh(["release", "view", version, "--json", RELEASE_FIELDS])
    return json.loads(raw)


def parse_pr_numbers(body: str) -> list[int]:
    """Extract unique PR numbers from a release body.

    Looks for patterns like (#123) which is how Release Drafter formats PR refs.
    Also matches standalone #123 references. Deduplicates and sorts.
    """
    refs = re.findall(r"#(\d+)", body)
    numbers = sorted(set(int(r) for r in refs))
    return numbers


def fetch_pr(number: int) -> dict:
    """Fetch a single PR's data via gh CLI."""
    raw = run_gh([
        "pr", "view", str(number),
        "--json", PR_FIELDS,
    ])
    return json.loads(raw)


def extract_release_note(body: str) -> Optional[str]:
    """Extract the ```release-note fenced block content."""
    match = re.search(r"```release-note\s*\n(.*?)\n```", body or "", re.DOTALL)
    if not match:
        return None
    note = match.group(1).strip()
    if not note or note.upper() == "NONE":
        return None
    return note


def extract_section(body: str, heading: str) -> Optional[str]:
    """Extract content under a markdown ### heading until the next heading."""
    pattern = rf"### {re.escape(heading)}\s*\n(.*?)(?=\n### |\Z)"
    match = re.search(pattern, body or "", re.DOTALL)
    if not match:
        return None
    content = match.group(1).strip()
    # Strip HTML comments
    content = re.sub(r"<!--.*?-->", "", content, flags=re.DOTALL).strip()
    if not content:
        return None
    # Ignore template placeholder text
    if content.startswith("If this PR introduces breaking changes"):
        return None
    # Ignore explicit "no breaking changes" responses
    lower = content.lower().rstrip(".")
    if lower in ("none", "n/a", "no", "no breaking changes"):
        return None
    # Also filter out multi-line responses that start with negation
    first_line = content.split("\n")[0].lower().strip().rstrip(".")
    if first_line.startswith(("this pr does not introduce", "no breaking", "none")):
        return None
    return content


def process_pr(pr_data: dict) -> dict:
    """Process raw PR data into the structured format."""
    body = pr_data.get("body") or ""
    labels = [label["name"] for label in pr_data.get("labels", [])]

    return {
        "number": pr_data["number"],
        "title": pr_data["title"],
        "url": pr_data["url"],
        "labels": labels,
        "mergedAt": pr_data.get("mergedAt"),
        "state": pr_data.get("state"),
        "releaseNote": extract_release_note(body),
        "breakingChanges": extract_section(body, "Breaking changes"),
        "whatThisPrDoes": extract_section(body, "What this PR does"),
        "body": body,
    }


def repo_root() -> Path:
    """Find the git repository root."""
    result = subprocess.run(
        ["git", "rev-parse", "--show-toplevel"],
        capture_output=True, text=True,
    )
    if result.returncode != 0:
        return Path(".")
    return Path(result.stdout.strip())


def default_output_path(version: str) -> Path:
    """Return the default cache path for a version."""
    return repo_root() / CACHE_DIR / f"{version}-pr-data.json"


def release_metadata(release: dict, version: str) -> dict:
    """Build the release metadata stored in the output document."""
    return {
        "tag": release.get("tagName", version),
        "name": release.get("name", ""),
        "isDraft": release.get("isDraft", False),
        "body": release.get("body", ""),
    }


def load_cached(path: Path) -> Optional[dict]:
    """Load an existing cached JSON file, or None if it doesn't exist."""
    if not path.exists():
        return None
    with open(path, encoding="utf-8") as f:
        return json.load(f)


def write_json_atomic(path: Path, data: dict) -> None:
    """Write JSON to a file atomically."""
    tmp_name = ""
    try:
        path.parent.mkdir(parents=True, exist_ok=True)
        with tempfile.NamedTemporaryFile(
            "w",
            encoding="utf-8",
            dir=path.parent,
            prefix=f".{path.name}.",
            suffix=".tmp",
            delete=False,
        ) as f:
            tmp_name = f.name
            json.dump(data, f, indent=2, ensure_ascii=False)
            f.write("\n")
        os.replace(tmp_name, path)
    except OSError as err:
        if tmp_name:
            try:
                Path(tmp_name).unlink(missing_ok=True)
            except OSError:
                pass
        print(f"Error writing {path}: {err}", file=sys.stderr)
        sys.exit(1)


def main():
    parser = argparse.ArgumentParser(
        description="Collect PR data from a GitHub release for AI summarization."
    )
    parser.add_argument(
        "version",
        help="Release version/tag to collect from (e.g. v0.4.0).",
    )
    parser.add_argument(
        "--output", "-o",
        metavar="FILE",
        default=None,
        help="Output JSON file (default: docs/version-drafts/<version>-pr-data.json).",
    )
    parser.add_argument(
        "--force", "-f",
        action="store_true",
        help="Force re-fetch all PRs, ignoring cached data.",
    )
    args = parser.parse_args()
    version = args.version

    # Resolve output path
    output_path = Path(args.output) if args.output else default_output_path(version)

    # Fetch the release body
    print(f"Fetching release {version}...", file=sys.stderr)
    try:
        release = fetch_release(version)
    except GhCommandError as err:
        print(err, file=sys.stderr)
        sys.exit(1)
    if not release.get("isDraft"):
        print(
            f"Error: release {version} exists but is not a draft release.",
            file=sys.stderr,
        )
        sys.exit(1)

    # Parse PR numbers from release body
    pr_numbers = parse_pr_numbers(release.get("body", ""))
    current_set = set(pr_numbers)
    print(f"Found {len(pr_numbers)} PR references: {pr_numbers}", file=sys.stderr)

    if not pr_numbers:
        print("Warning: no PR references found in release body.", file=sys.stderr)

    # Check for cached data
    cached = None if args.force else load_cached(output_path)
    cached_by_number: dict[int, dict] = {}
    if cached:
        cached_by_number = {pr["number"]: pr for pr in cached.get("prs", [])}
        cached_set = set(cached_by_number.keys())

        added = current_set - cached_set
        removed = cached_set - current_set
        unchanged = current_set & cached_set

        print(f"Cache: {len(cached_set)} PRs cached, "
              f"{len(added)} new, {len(removed)} removed, "
              f"{len(unchanged)} unchanged.", file=sys.stderr)

        if not added and not removed:
            print("Nothing changed — using cached data.", file=sys.stderr)
            # Still update the release body (it may have changed)
            cached["release"] = release_metadata(release, version)
            write_json_atomic(output_path, cached)
            print(f"Updated release body in {output_path}", file=sys.stderr)
            return

        if removed:
            print(f"  Removing stale PRs: {sorted(removed)}", file=sys.stderr)

        to_fetch = sorted(added)
    else:
        if not args.force:
            print("No cached data found — fetching all PRs.", file=sys.stderr)
        else:
            print("Force mode — fetching all PRs.", file=sys.stderr)
        to_fetch = pr_numbers

    # Fetch only the PRs we need
    new_prs: dict[int, dict] = {}
    failed_prs: list[int] = []
    for i, number in enumerate(to_fetch, 1):
        print(f"  [{i}/{len(to_fetch)}] Fetching PR #{number}...", file=sys.stderr)
        try:
            pr_data = fetch_pr(number)
            new_prs[number] = process_pr(pr_data)
        except GhCommandError as err:
            failed_prs.append(number)
            print(f"  Error fetching PR #{number}:", file=sys.stderr)
            print(err.stderr, file=sys.stderr)

    if failed_prs:
        print(
            f"Error: failed to fetch {len(failed_prs)} PR(s): {failed_prs}. "
            "Output was not written.",
            file=sys.stderr,
        )
        sys.exit(1)

    # Merge: keep cached PRs that are still in the release, add new ones
    merged_prs = {}
    for num in pr_numbers:
        if num in new_prs:
            merged_prs[num] = new_prs[num]
        elif num in cached_by_number:
            merged_prs[num] = cached_by_number[num]
    # Preserve the order from the release body
    prs = [merged_prs[num] for num in pr_numbers if num in merged_prs]

    # Build output document
    output = {
        "release": release_metadata(release, version),
        "prs": prs,
    }

    # Write output
    write_json_atomic(output_path, output)
    print(f"\nWritten {len(prs)} PRs to {output_path}", file=sys.stderr)


if __name__ == "__main__":
    main()
