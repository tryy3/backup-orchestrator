#!/usr/bin/env python3
"""Add an AI-generated summary paragraph to a structured release note draft.

Reads the structured markdown produced by scripts/release-notes.py, calls the
GitHub Models API (gpt-4o) to write a short human-readable summary, then
prepends it to the existing categorized PR list.

The AI is only allowed to summarise what is already in the notes. It must not
invent features, fixes, or claims that are not present in the input.

Usage:
  # Pipe from release-notes.py
  python3 scripts/release-notes.py | python3 scripts/ai-polish.py

  # Read from a file
  python3 scripts/ai-polish.py --input notes.md

  # Write to a file
  python3 scripts/ai-polish.py --input notes.md --output polished.md

  # In CI — GITHUB_TOKEN is set automatically
  python3 scripts/ai-polish.py --input notes.md

Environment:
  GITHUB_TOKEN  Required. Used to authenticate with the GitHub Models API.
                Locally: set via `export GITHUB_TOKEN=$(gh auth token)`.
                In Actions: set via `env: GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}`.
"""

import argparse
import json
import os
import sys
import urllib.error
import urllib.request


GITHUB_MODELS_URL = "https://models.inference.ai.azure.com/chat/completions"
MODEL = "gpt-4o"

# The system prompt defines what the AI is allowed to do.
# Safety constraint: only summarise what is in the notes, no inventions.
SYSTEM_PROMPT = """\
You are a technical writer helping maintainers of an open-source project \
(a backup orchestration system) write release notes.

Your task: given a structured list of merged pull requests grouped by category, \
write a short summary paragraph (3-6 sentences) that gives a human-readable \
overview of what changed in this release.

Rules:
- Only describe changes that are explicitly listed in the input. Do not invent \
  features, fixes, or improvements that are not mentioned.
- If there are breaking changes or security fixes, mention them prominently.
- Keep the tone neutral and informative, not marketing-speak.
- Do not repeat each PR individually — the full list follows the summary.
- Do not include a heading. Output only the paragraph text.
- If the input contains no user-facing changes (only maintenance), say so briefly.
"""


def call_github_models(token: str, notes: str) -> str:
    payload = {
        "model": MODEL,
        "messages": [
            {"role": "system", "content": SYSTEM_PROMPT},
            {
                "role": "user",
                "content": (
                    "Here are the release notes for this version. "
                    "Write a short summary paragraph:\n\n"
                    + notes
                ),
            },
        ],
        "max_tokens": 400,
        "temperature": 0.3,
    }

    req = urllib.request.Request(
        GITHUB_MODELS_URL,
        data=json.dumps(payload).encode("utf-8"),
        headers={
            "Authorization": f"Bearer {token}",
            "Content-Type": "application/json",
        },
        method="POST",
    )

    try:
        with urllib.request.urlopen(req, timeout=60) as resp:
            body = json.loads(resp.read().decode("utf-8"))
    except urllib.error.HTTPError as exc:
        error_body = exc.read().decode("utf-8", errors="replace")
        print(
            f"GitHub Models API error {exc.code}: {error_body}",
            file=sys.stderr,
        )
        sys.exit(1)

    return body["choices"][0]["message"]["content"].strip()


def inject_summary(notes: str, summary: str) -> str:
    """Insert the summary paragraph right after the '## What's Changed' heading."""
    marker = "## What's Changed"
    if marker in notes:
        return notes.replace(
            marker,
            f"{marker}\n\n{summary}\n",
            1,
        )
    # Fallback: prepend to the whole document.
    return f"{summary}\n\n{notes}"


def main():
    parser = argparse.ArgumentParser(
        description="Add an AI summary paragraph to a release note draft."
    )
    parser.add_argument(
        "--input",
        metavar="FILE",
        help="Read notes from FILE instead of stdin.",
    )
    parser.add_argument(
        "--output",
        metavar="FILE",
        help="Write polished notes to FILE instead of stdout.",
    )
    args = parser.parse_args()

    token = os.environ.get("GITHUB_TOKEN")
    if not token:
        # Try to get it from gh CLI when running locally.
        import subprocess
        result = subprocess.run(
            ["gh", "auth", "token"],
            capture_output=True,
            text=True,
        )
        if result.returncode == 0:
            token = result.stdout.strip()
        else:
            print(
                "Error: GITHUB_TOKEN is not set and `gh auth token` failed.\n"
                "Run `gh auth login` or set GITHUB_TOKEN.",
                file=sys.stderr,
            )
            sys.exit(1)

    if args.input:
        with open(args.input) as f:
            notes = f.read()
    else:
        notes = sys.stdin.read()

    if not notes.strip():
        print("Error: empty input — nothing to summarise.", file=sys.stderr)
        sys.exit(1)

    print("Calling GitHub Models API (gpt-4o)...", file=sys.stderr)
    summary = call_github_models(token, notes)
    polished = inject_summary(notes, summary)

    if args.output:
        with open(args.output, "w") as f:
            f.write(polished)
        print(f"Written to {args.output}", file=sys.stderr)
    else:
        print(polished)


if __name__ == "__main__":
    main()
