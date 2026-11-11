"""Tests for changelog.py. Run with: python3 -m unittest discover -s .github/scripts -p '*_test.py'"""

import json
import os
import unittest
from contextlib import redirect_stdout
from io import StringIO
from unittest import mock

import changelog


class ExtractTest(unittest.TestCase):
    def test_section(self):
        body = "## Summary\nx\n\n## Changelog\n\nAdd foo.\n\n## Notes\nzzz"
        self.assertEqual(changelog.extract(body), "Add foo.")

    def test_last_section(self):
        self.assertEqual(changelog.extract("## Changelog\n- a\n- b\n"), "- a\n- b")

    def test_heading_variants(self):
        for heading in ("## Changelog", "# changelog", "### Changelog ##", "  ## Changelog"):
            with self.subTest(heading=heading):
                self.assertEqual(changelog.extract(f"{heading}\nAdd foo"), "Add foo")

    def test_crlf(self):
        self.assertEqual(changelog.extract("## Changelog\r\nAdd foo.\r\n"), "Add foo.")

    def test_comments_stripped(self):
        body = "## Changelog\n<!--\nhint\n-->\nAdd foo <!-- inline -->\n"
        self.assertEqual(changelog.extract(body), "Add foo")

    def test_comment_only_is_empty(self):
        self.assertEqual(changelog.extract("## Changelog\n<!-- fill me -->\n"), "")

    def test_missing(self):
        for body in (None, "", "no section", "<!-- ## Changelog -->\nAdd foo", "## Changelog notes\nAdd foo"):
            with self.subTest(body=body):
                self.assertIsNone(changelog.extract(body))


class IsNoneTest(unittest.TestCase):
    def test_none(self):
        for entry in ("None", "none", "None.", "N/A", "  n/a  "):
            with self.subTest(entry=entry):
                self.assertTrue(changelog.is_none(entry))

    def test_not_none(self):
        for entry in ("Add foo", "None of the above changed", "- None"):
            with self.subTest(entry=entry):
                self.assertFalse(changelog.is_none(entry))


class BulletsTest(unittest.TestCase):
    def test_formats(self):
        cases = {
            "Add foo.": ["- Add foo. (#1)"],
            "Add foo\nwith bar.": ["- Add foo with bar. (#1)"],
            "- a\n* b\n+ c": ["- a (#1)", "- b (#1)", "- c (#1)"],
            "intro\n- a": ["- intro (#1)", "- a (#1)"],
            "- a\n  wrapped\n- b": ["- a wrapped (#1)", "- b (#1)"],
            "p1\n\np2": ["- p1 (#1)", "- p2 (#1)"],
        }
        for entry, want in cases.items():
            with self.subTest(entry=entry):
                self.assertEqual(changelog.bullets(entry, 1), want)


class CheckTest(unittest.TestCase):
    def check(self, body):
        with mock.patch.dict(os.environ, {"PR_BODY": body}), redirect_stdout(StringIO()):
            return changelog.check()

    def test_passes(self):
        self.assertEqual(self.check("## Changelog\nAdd foo."), 0)
        self.assertEqual(self.check("## Changelog\nNone"), 0)

    def test_fails(self):
        self.assertEqual(self.check(""), 1)
        self.assertEqual(self.check("## Summary\nx"), 1)
        self.assertEqual(self.check("## Changelog\n<!-- fill me -->\n"), 1)

    def test_template_fails_until_filled(self):
        path = os.path.join(os.path.dirname(__file__), "..", "pull_request_template.md")
        with open(path) as f:
            self.assertEqual(self.check(f.read()), 1)


def pr(number, title, body, bot=False):
    author = {"login": "app/dependabot", "is_bot": True} if bot else {"login": "someone", "is_bot": False}
    return {"title": f"{title} (#{number})", "body": body, "author": author}


class ReleaseNotesTest(unittest.TestCase):
    TAGS = ["v2.0.0", "v2.0.0-beta2", "v2.0.0-beta1", "v1.9.0"]

    def setUp(self):
        self.prs = {}
        self.log_ranges = []

    def fake_run(self, *args):
        if args[:2] == ("git", "tag"):
            return "\n".join(self.TAGS) + "\n"
        if args[:2] == ("git", "log"):
            self.log_ranges.append(args[-1])
            # Newest first, like git log; includes a direct push without a PR number.
            subjects = [self.prs[n]["title"] for n in sorted(self.prs, reverse=True)]
            return "\n".join(subjects + ["Bump provider version in examples to 1.9.0"]) + "\n"
        if args[:3] == ("gh", "pr", "view"):
            return json.dumps(self.prs[int(args[3])])
        raise AssertionError(f"unexpected command: {args}")

    def notes(self, tag, prev=None):
        with mock.patch.object(changelog, "run", self.fake_run):
            return changelog.release_notes(tag, prev)

    def test_grouping(self):
        self.prs = {
            10: pr(10, "feat(postgres): add ip access", "## Changelog\nAdd `ip_access`."),
            11: pr(11, "fix!: rename attr", "## Changelog\n- Rename `x` to `y`.\n- Drop `z`."),
            12: pr(12, "ci: tweak", "## Changelog\nNone"),
            13: pr(13, "chore(deps): bump x", "", bot=True),
            14: pr(14, "refactor: cleanup", "## Changelog\nFaster plan."),
            15: pr(15, "fix: crash on import", "## Changelog\nFix crash on import."),
        }
        self.assertEqual(self.notes("v2.0.0"), (
            "## 💥 Backward Incompatible Changes\n\n"
            "- Rename `x` to `y`. (#11)\n"
            "- Drop `z`. (#11)\n\n"
            "## ✨ New Features\n\n"
            "- Add `ip_access`. (#10)\n\n"
            "## 🐛 Bug Fixes\n\n"
            "- Fix crash on import. (#15)\n\n"
            "## 🔧 Other Changes\n\n"
            "- Faster plan. (#14)\n"
        ))

    def test_missing_entry_falls_back_to_title(self):
        self.prs = {
            20: pr(20, "feat(clickpipe): support BigQuery CDC", "no section"),
            21: pr(21, "docs: typo", ""),
        }
        notes = self.notes("v2.0.0")
        self.assertIn("## ✨ New Features\n\n- support BigQuery CDC (#20)\n", notes)
        self.assertNotIn("typo", notes)
        self.assertIn("review before publishing: #20, #21 -->", notes)

    def test_no_user_facing_changes(self):
        self.prs = {30: pr(30, "ci: tweak", "## Changelog\nNone")}
        self.assertEqual(self.notes("v2.0.0"), "No user-facing changes in this release.\n")

    def test_beta_callout(self):
        self.prs = {40: pr(40, "feat: thing", "## Changelog\nAdd thing.")}
        self.assertTrue(self.notes("v2.0.0-beta2").startswith("> ⚠️ **Beta release**"))
        self.assertNotIn("Beta release", self.notes("v2.0.0"))

    def test_previous_tag(self):
        self.notes("v2.0.0")  # stable skips pre-releases
        self.notes("v2.0.0-beta2")  # pre-release uses the previous tag of any kind
        self.notes("v1.9.0")  # first tag uses the whole history
        self.notes("v2.0.0", "v2.0.0-beta1")  # explicit previous tag wins
        self.assertEqual(self.log_ranges, ["v1.9.0..v2.0.0", "v2.0.0-beta1..v2.0.0-beta2", "v1.9.0", "v2.0.0-beta1..v2.0.0"])


if __name__ == "__main__":
    unittest.main()
