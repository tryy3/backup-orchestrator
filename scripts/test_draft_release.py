#!/usr/bin/env python3
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from draft_release import (
    compare_semver,
    effective_version,
    select_highest_draft,
    to_draft_release,
)


class DraftReleaseTests(unittest.TestCase):
    def test_effective_version_prefers_semver_tag(self):
        release = {
            "id": 1,
            "tag_name": "v0.4.0",
            "name": "v0.4.0",
            "draft": True,
        }
        self.assertEqual(effective_version(release), "v0.4.0")

    def test_effective_version_from_untagged_name(self):
        release = {
            "id": 2,
            "tag_name": "untagged-abc123",
            "name": "v0.4.1",
            "draft": True,
        }
        self.assertEqual(effective_version(release), "v0.4.1")

    def test_select_highest_draft_ignores_non_semver(self):
        releases = [
            {
                "id": 1,
                "tag_name": "v0.3.0",
                "name": "v0.3.0",
                "draft": True,
            },
            {
                "id": 2,
                "tag_name": "untagged-abc123",
                "name": "v0.4.0",
                "draft": True,
            },
            {
                "id": 3,
                "tag_name": "random-tag",
                "name": "not-a-version",
                "draft": True,
            },
        ]
        selected = select_highest_draft(releases)
        self.assertIsNotNone(selected)
        assert selected is not None
        self.assertEqual(selected.effective_version, "v0.4.0")
        self.assertEqual(selected.tag_name, "untagged-abc123")

    def test_compare_semver_orders_prereleases(self):
        self.assertGreater(compare_semver("v1.0.0", "v0.9.9"), 0)
        self.assertGreater(compare_semver("v1.0.0", "v1.0.0-rc.1"), 0)
        self.assertLess(compare_semver("v1.0.0-rc.1", "v1.0.0"), 0)

    def test_to_draft_release_requires_id_and_tag(self):
        draft = to_draft_release(
            {
                "id": 9,
                "tag_name": "untagged-xyz",
                "name": "v0.5.0",
                "draft": True,
            }
        )
        self.assertIsNotNone(draft)
        assert draft is not None
        self.assertEqual(draft.edit_tag, "untagged-xyz")
        self.assertEqual(draft.effective_version, "v0.5.0")


if __name__ == "__main__":
    unittest.main()
