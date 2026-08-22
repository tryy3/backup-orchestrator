#!/usr/bin/env python3
"""Resolve GitHub draft releases for release-note automation.

Release Drafter usually creates drafts with semver tag names (for example
``v0.4.0``). GitHub can also expose drafts as ``untagged-*`` while the release
``name`` still carries the intended semver. This module treats both shapes as
valid release candidates and picks the highest semantic version when asked to
auto-select a draft.

Usage:
  python3 scripts/draft_release.py
  python3 scripts/draft_release.py --version v0.4.0
  python3 scripts/draft_release.py --release-id 123456
  python3 scripts/draft_release.py --json
"""

from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys
import urllib.error
import urllib.request
from dataclasses import asdict, dataclass
from typing import Any, Optional


SEMVER_PREFIXED = re.compile(
    r"^v(?P<major>\d+)\.(?P<minor>\d+)\.(?P<patch>\d+)"
    r"(?:-(?P<prerelease>[0-9A-Za-z.-]+))?"
    r"(?:\+(?P<build>[0-9A-Za-z.-]+))?$"
)


@dataclass(frozen=True)
class DraftRelease:
    id: int
    tag_name: str
    name: str
    is_draft: bool
    effective_version: str

    @property
    def edit_tag(self) -> str:
        """Tag name to pass to ``gh release edit``."""
        return self.tag_name


def _field(release: dict[str, Any], *keys: str) -> str:
    for key in keys:
        value = release.get(key)
        if value:
            return str(value)
    return ""


def parse_semver(value: str) -> Optional[tuple[int, int, int, str]]:
    match = SEMVER_PREFIXED.match(value.strip())
    if not match:
        return None
    return (
        int(match.group("major")),
        int(match.group("minor")),
        int(match.group("patch")),
        match.group("prerelease") or "",
    )


def effective_version(release: dict[str, Any]) -> Optional[str]:
    tag_name = _field(release, "tagName", "tag_name")
    name = _field(release, "name")

    if parse_semver(tag_name):
        return tag_name
    if tag_name.startswith("untagged-") and parse_semver(name):
        return name
    if parse_semver(name):
        return name
    return None


def is_release_candidate(release: dict[str, Any]) -> bool:
    return effective_version(release) is not None


def to_draft_release(release: dict[str, Any]) -> Optional[DraftRelease]:
    version = effective_version(release)
    if not version:
        return None
    tag_name = _field(release, "tagName", "tag_name")
    if not tag_name:
        return None
    release_id = release.get("id")
    if release_id is None:
        return None
    return DraftRelease(
        id=int(release_id),
        tag_name=tag_name,
        name=_field(release, "name"),
        is_draft=bool(release.get("isDraft", release.get("draft", False))),
        effective_version=version,
    )


def compare_semver(left: str, right: str) -> int:
    """Return negative/zero/positive when left is older/equal/newer than right."""
    left_parts = parse_semver(left)
    right_parts = parse_semver(right)
    if left_parts is None or right_parts is None:
        raise ValueError("compare_semver requires semver-prefixed values")
    if left_parts[:3] != right_parts[:3]:
        return (left_parts[:3] > right_parts[:3]) - (left_parts[:3] < right_parts[:3])
    left_pre, right_pre = left_parts[3], right_parts[3]
    if left_pre == right_pre:
        return 0
    if not left_pre:
        return 1
    if not right_pre:
        return -1
    return (left_pre > right_pre) - (left_pre < right_pre)


def select_highest_draft(releases: list[dict[str, Any]]) -> Optional[DraftRelease]:
    candidates: list[DraftRelease] = []
    for release in releases:
        if not release.get("isDraft", release.get("draft", False)):
            continue
        draft = to_draft_release(release)
        if draft:
            candidates.append(draft)
    if not candidates:
        return None
    return max(candidates, key=lambda draft: parse_semver(draft.effective_version) or (0, 0, 0, ""))


def _github_request(path: str, token: str) -> Any:
    request = urllib.request.Request(
        f"https://api.github.com{path}",
        headers={
            "Authorization": f"Bearer {token}",
            "Accept": "application/vnd.github+json",
            "X-GitHub-Api-Version": "2022-11-28",
        },
    )
    try:
        with urllib.request.urlopen(request, timeout=60) as response:
            return json.loads(response.read().decode("utf-8"))
    except urllib.error.HTTPError as exc:
        body = exc.read().decode("utf-8", errors="replace")
        raise RuntimeError(f"GitHub API error {exc.code} for {path}: {body}") from exc


def github_token() -> str:
    token = os.environ.get("GITHUB_TOKEN") or os.environ.get("GH_TOKEN")
    if token:
        return token
    result = subprocess.run(
        ["gh", "auth", "token"],
        capture_output=True,
        text=True,
    )
    if result.returncode == 0 and result.stdout.strip():
        return result.stdout.strip()
    raise RuntimeError(
        "GITHUB_TOKEN or GH_TOKEN is not set and `gh auth token` failed."
    )


def detect_repo() -> str:
    result = subprocess.run(
        ["gh", "repo", "view", "--json", "nameWithOwner", "-q", ".nameWithOwner"],
        capture_output=True,
        text=True,
    )
    if result.returncode == 0 and result.stdout.strip():
        return result.stdout.strip()
    remote = subprocess.run(
        ["git", "remote", "get-url", "origin"],
        capture_output=True,
        text=True,
    ).stdout.strip()
    slug = re.sub(r".*github\.com[:/]", "", remote)
    return re.sub(r"\.git$", "", slug)


def list_draft_releases(repo: Optional[str] = None, token: Optional[str] = None) -> list[dict[str, Any]]:
    repo = repo or detect_repo()
    token = token or github_token()
    releases = _github_request(f"/repos/{repo}/releases?per_page=100", token)
    return [release for release in releases if release.get("draft") is True]


def fetch_release_by_id(
    release_id: int,
    repo: Optional[str] = None,
    token: Optional[str] = None,
) -> dict[str, Any]:
    repo = repo or detect_repo()
    token = token or github_token()
    return _github_request(f"/repos/{repo}/releases/{release_id}", token)


def resolve_draft(
    *,
    version: Optional[str] = None,
    release_id: Optional[int] = None,
    repo: Optional[str] = None,
    token: Optional[str] = None,
) -> DraftRelease:
    repo = repo or detect_repo()
    token = token or github_token()

    if release_id is not None:
        release = fetch_release_by_id(release_id, repo=repo, token=token)
        if not release.get("draft"):
            raise RuntimeError(f"Release {release_id} exists but is not a draft.")
        draft = to_draft_release(release)
        if draft is None:
            raise RuntimeError(
                f"Release {release_id} is a draft but has no semver tag or name."
            )
        return draft

    drafts = list_draft_releases(repo=repo, token=token)

    if version:
        normalized = version.strip()
        for release in drafts:
            draft = to_draft_release(release)
            if draft and (
                draft.tag_name == normalized
                or draft.effective_version == normalized
            ):
                return draft
        raise RuntimeError(
            f"No eligible draft release found for version/tag {normalized!r}."
        )

    selected = select_highest_draft(drafts)
    if selected is None:
        raise RuntimeError(
            "No eligible semver draft release found. "
            "Release Drafter must create a draft first."
        )
    return selected


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--version",
        metavar="TAG",
        help="Explicit draft version or tag to resolve (for example v0.4.0).",
    )
    parser.add_argument(
        "--release-id",
        type=int,
        metavar="ID",
        help="Explicit draft release ID to resolve.",
    )
    parser.add_argument(
        "--json",
        action="store_true",
        help="Print the resolved draft as JSON.",
    )
    parser.add_argument(
        "--list",
        action="store_true",
        help="List all eligible draft releases and exit.",
    )
    args = parser.parse_args()

    if args.list:
        drafts = []
        for release in list_draft_releases():
            draft = to_draft_release(release)
            if draft:
                drafts.append(asdict(draft))
        print(json.dumps(drafts, indent=2))
        return

    try:
        draft = resolve_draft(version=args.version, release_id=args.release_id)
    except RuntimeError as err:
        print(str(err), file=sys.stderr)
        sys.exit(1)

    if args.json:
        print(json.dumps(asdict(draft), indent=2))
        return

    print(draft.edit_tag)


if __name__ == "__main__":
    main()
