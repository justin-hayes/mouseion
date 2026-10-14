"""Tests for completion_ledger.py. Run: python3 -m unittest -v test_completion_ledger.py (from scripts/)."""
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

import completion_ledger as cl  # noqa: E402

BASE_FIELDS = ["route", "node_key", "text_status", "text_rule", "vocab_class", "identity_status",
               "discriminator_status", "discriminator_origin", "readiness", "blockers"]


def row(route, node, readiness="vocab_ready", blockers="", text_status="exact"):
    return {"route": route, "node_key": node, "text_status": text_status, "text_rule": "rule",
            "vocab_class": "eligible", "identity_status": "resolved", "discriminator_status": "absent_in_source",
            "discriminator_origin": "", "readiness": readiness, "blockers": blockers}


class OverlayTests(unittest.TestCase):
    def test_sentence_11_gains_producer_blocker_and_keeps_existing(self):
        rows = [row("T1A2", "s011w026", "vocab_blocked", "BT-S011-ORDER;BI-ELIG-PARTICLE")]
        new, changes = cl.overlay(rows)
        self.assertEqual(new[0]["blockers"], "BT-S011-ORDER;BI-ELIG-PARTICLE;BP-PRODUCER-IDENTITY")
        self.assertEqual(new[0]["readiness"], "vocab_blocked")
        self.assertEqual(new[0]["completion_change"], "blocker_added:BP-PRODUCER-IDENTITY")
        self.assertIn("cg2:w0264", new[0]["completion_proposal"])
        self.assertEqual(len(changes), 1)

    def test_second_mu_te_pair_uses_its_own_canonical_token(self):
        new, _ = cl.overlay([row("T1A2", "s011w029", "vocab_blocked", "BT-S011-ORDER")])
        self.assertIn("cg2:w0266", new[0]["completion_proposal"])

    def test_s125_gains_route_decision_blocker_only(self):
        new, changes = cl.overlay([row("T1A2", "s125w015", "non_vocab_blocked", "BT-S125-VARIANT")])
        self.assertEqual(new[0]["blockers"], "BT-S125-VARIANT;BD-S125-ROUTE")
        self.assertTrue(new[0]["completion_proposal"].startswith("decision_required"))
        self.assertEqual(changes[0]["added_blocker"], "BD-S125-ROUTE")

    def test_unrelated_rows_are_unchanged(self):
        rows = [row("T1A2", "s001w001", "non_vocab_ok", "")]
        new, changes = cl.overlay(rows)
        self.assertEqual(new[0]["blockers"], "")
        self.assertEqual(new[0]["completion_change"], "unchanged")
        self.assertEqual(new[0]["completion_proposal"], "")
        self.assertEqual(changes, [])

    def test_existing_blocker_is_not_duplicated(self):
        rows = [row("T1A2", "s011w025", "non_vocab_blocked", "BP-PRODUCER-IDENTITY")]
        new, _ = cl.overlay(rows)
        self.assertEqual(new[0]["blockers"], "BP-PRODUCER-IDENTITY")

    def test_readiness_and_text_status_are_never_changed(self):
        rows = [row("T1A2", "s011w026", "vocab_blocked", "BT-S011-ORDER"),
                row("T1A2", "s125w015", "non_vocab_blocked", "BT-S125-VARIANT")]
        new, _ = cl.overlay(rows)
        for before, after in zip(rows, new):
            for field in ("text_status", "text_rule", "readiness", "vocab_class", "identity_status"):
                self.assertEqual(before[field], after[field])


class ChecksTests(unittest.TestCase):
    def _sentences(self):
        return [{"sentence_id": "11", "nodes": "2"}, {"sentence_id": "12", "nodes": "1"}]

    def test_ready_claim_is_detected(self):
        base = [row("T1A2", "s011w026", "vocab_blocked", "BT-S011-ORDER")]
        new = [dict(base[0], readiness="vocab_ready")]
        checks = cl.run_checks(base, new, [], [], [])
        self.assertEqual(checks["ready_claims_added"], 1)
        self.assertFalse(checks["all_checks_pass"])

    def test_dropped_blocker_is_detected(self):
        base = [row("T1A2", "s001w001", "non_vocab_blocked", "BT-X;BT-Y")]
        new = [dict(base[0], blockers="BT-X")]
        checks = cl.run_checks(base, new, [], [], [])
        self.assertFalse(checks["blockers_only_grow"])
        self.assertFalse(checks["all_checks_pass"])

    def test_changed_field_outside_overlay_is_detected(self):
        base = [row("T1A2", "s001w001", "non_vocab_ok", "")]
        new = [dict(base[0], text_status="own_surface_unspaced")]
        checks = cl.run_checks(base, new, [], [], [])
        self.assertFalse(checks["locked_fields_unchanged"])

    def test_sentence_overlay_reports_added_blockers(self):
        changes = [{"sentence_id": 11, "added_blocker": "BP-PRODUCER-IDENTITY"},
                   {"sentence_id": 11, "added_blocker": "BP-PRODUCER-IDENTITY"}]
        out = cl.sentence_overlay(self._sentences(), changes)
        self.assertEqual(out[0]["completion_changed_rows"], "2")
        self.assertEqual(out[0]["completion_added_blockers"], "BP-PRODUCER-IDENTITY")
        self.assertEqual(out[1]["completion_changed_rows"], "0")

    def test_end_to_end_on_a_small_fixture(self):
        with tempfile.TemporaryDirectory() as tmp:
            base_dir = Path(tmp) / "base"
            out_dir = Path(tmp) / "out"
            base_dir.mkdir()
            rows = [row("T1A2", "s001w001", "non_vocab_ok", ""),
                    row("T1A2", "s011w025", "non_vocab_blocked", "BT-S011-ORDER"),
                    row("T1A2", "s125w015", "non_vocab_blocked", "BT-S125-VARIANT")]
            cl.write_tsv(base_dir / "route_nodes.tsv", BASE_FIELDS, rows)
            cl.write_tsv(base_dir / "sentences.tsv", ["sentence_id", "nodes"],
                         [{"sentence_id": "1", "nodes": "1"}, {"sentence_id": "11", "nodes": "1"}])
            # The small fixture cannot satisfy the full-tree expectations; the run must report that.
            code = cl.main(["ledger", str(base_dir), str(out_dir)])
            self.assertEqual(code, 1)
            self.assertTrue((out_dir / "completion_route_nodes.tsv").exists())
            self.assertTrue((out_dir / "completion_checks.json").exists())


class RealManifestTests(unittest.TestCase):
    """Runs against the committed baseline when it is present in this worktree."""

    def test_committed_manifest_passes_all_checks(self):
        out = cl.DEFAULT_OUT
        if not (cl.DEFAULT_BASELINE / "route_nodes.tsv").exists():
            self.skipTest("baseline manifest not present")
        code = cl.main(["ledger", str(cl.DEFAULT_BASELINE), str(out)])
        self.assertEqual(code, 0)


if __name__ == "__main__":
    unittest.main()
