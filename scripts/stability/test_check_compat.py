#!/usr/bin/env python3
"""Tests for check_compat.py. Run: python3 -m unittest discover -s scripts/stability"""

import os
import sys
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import check_compat as cc  # noqa: E402


def manifest(*rows):
    return "# header\n\n## Surface\n\n```\n" + "\n".join(rows) + "\n```\n"


def findings(base_rows, head_rows, base_version, head_version):
    return cc.since_findings(
        cc.surface(manifest(*base_rows)),
        cc.surface(manifest(*head_rows)),
        cc.parse_version(base_version),
        cc.parse_version(head_version),
    )


class SurfaceTest(unittest.TestCase):
    def test_parses_since_and_deprecation(self):
        s = cc.surface(manifest(
            "get widgets                   stable",
            "  --old                       stable  deprecated 0.38.0 → remove 0.40.0",
            "exec widget                   experimental  since 0.39.0",
            "  --wait                      experimental  since 0.40.0",
        ))
        self.assertEqual(s["get widgets"], ("stable", False, None))
        self.assertEqual(s["get widgets --old"], ("stable", True, None))
        self.assertEqual(s["exec widget"], ("experimental", False, "0.39.0"))
        self.assertEqual(s["exec widget --wait"], ("experimental", False, "0.40.0"))


class SinceTest(unittest.TestCase):
    RELEASED = [
        "exec widget                   experimental  since 0.39.0",
        "  --wait                      experimental  since 0.40.0",
    ]

    def test_released_since_versions_survive_the_next_release(self):
        # The break this exists to fix: after 0.40.0 ships, declarations that
        # shipped in 0.39.0 and 0.40.0 are true and must stay green.
        self.assertEqual(findings(self.RELEASED, self.RELEASED, "0.40.0", "0.40.0"), [])

    def test_new_declaration_names_the_next_release(self):
        for since in ("0.40.1", "0.41.0"):
            head = self.RELEASED + ["get gadgets                   experimental  since " + since]
            self.assertEqual(findings(self.RELEASED, head, "0.40.0", "0.40.0"), [], since)

    def test_new_declaration_naming_a_shipped_release_is_refused(self):
        # 0.40.0 is pkg/version on main, i.e. it already shipped without it.
        for since in ("0.39.0", "0.40.0"):
            head = self.RELEASED + ["get gadgets                   experimental  since " + since]
            out = findings(self.RELEASED, head, "0.40.0", "0.40.0")
            self.assertEqual([k for k, _ in out], ["get gadgets"], since)
            self.assertIn("already shipped", out[0][1])
            self.assertIn("expected one of 0.40.1, 0.41.0", out[0][1])

    def test_demotion_of_existing_entry_naming_a_shipped_release_is_refused(self):
        base = self.RELEASED + ["get gadgets                   stable"]
        head = self.RELEASED + ["get gadgets                   experimental  since 0.38.0"]
        self.assertEqual([k for k, _ in findings(base, head, "0.40.0", "0.40.0")], ["get gadgets"])

    def test_rewriting_a_released_since_version_is_refused(self):
        head = [
            "exec widget                   experimental  since 0.38.0",
            "  --wait                      experimental  since 0.40.0",
        ]
        out = findings(self.RELEASED, head, "0.40.0", "0.40.0")
        self.assertEqual([k for k, _ in out], ["exec widget"])
        self.assertIn("rewritten", out[0][1])

    def test_rewriting_a_released_since_version_forward_is_refused(self):
        # On the release PR 0.41.0 is an allowed value, but 0.40.0 shipped.
        head = [
            "exec widget                   experimental  since 0.39.0",
            "  --wait                      experimental  since 0.41.0",
        ]
        out = findings(self.RELEASED, head, "0.40.0", "0.41.0")
        self.assertEqual([k for k, _ in out], ["exec widget --wait"])
        self.assertIn("rewritten to 0.41.0", out[0][1])

    def test_removing_a_released_since_version_is_refused(self):
        head = [
            "exec widget                   experimental",
            "  --wait                      experimental  since 0.40.0",
        ]
        out = findings(self.RELEASED, head, "0.40.0", "0.40.0")
        self.assertEqual([k for k, _ in out], ["exec widget"])
        self.assertIn("removed", out[0][1])

    def test_promotion_drops_the_since_version(self):
        head = [
            "exec widget                   stable",
            "  --wait                      stable",
        ]
        self.assertEqual(findings(self.RELEASED, head, "0.40.0", "0.40.0"), [])

    def test_unreleased_since_version_may_be_corrected(self):
        base = self.RELEASED + ["get gadgets                   experimental  since 0.40.1"]
        head = self.RELEASED + ["get gadgets                   experimental  since 0.41.0"]
        self.assertEqual(findings(base, head, "0.40.0", "0.40.0"), [])

    def test_since_beyond_the_next_release_is_refused(self):
        for since in ("0.40.2", "0.42.0", "1.0.0"):
            head = self.RELEASED + ["get gadgets                   experimental  since " + since]
            out = findings(self.RELEASED, head, "0.40.0", "0.40.0")
            self.assertEqual([k for k, _ in out], ["get gadgets"], since)

    def test_release_pr_numbered_differently_is_refused(self):
        # Declared on main for the next patch; release-please then cuts a minor.
        main = self.RELEASED + ["get gadgets                   experimental  since 0.40.1"]
        out = findings(main, main, "0.40.0", "0.41.0")
        self.assertEqual([k for k, _ in out], ["get gadgets"])

    def test_release_pr_numbered_as_declared_passes(self):
        main = self.RELEASED + ["get gadgets                   experimental  since 0.41.0"]
        self.assertEqual(findings(main, main, "0.40.0", "0.41.0"), [])

    def test_not_a_version(self):
        head = self.RELEASED + ["get gadgets                   experimental  since soon"]
        self.assertEqual([k for k, _ in findings(self.RELEASED, head, "0.40.0", "0.40.0")],
                         ["get gadgets"])


class ParseVersionTest(unittest.TestCase):
    def test_accepts_prefix_and_prerelease(self):
        self.assertEqual(cc.parse_version("v0.39.0-rc.1"), (0, 39, 0))

    def test_rejects_non_versions(self):
        for s in ("0.39", "x.y.z", ""):
            with self.assertRaises(ValueError):
                cc.parse_version(s)


if __name__ == "__main__":
    unittest.main()
