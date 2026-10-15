"""Invented fixtures for the v2 review revision: nothing here needs private data."""

import json
import tempfile
import unittest
import unicodedata
from pathlib import Path

import mapping_completion_v2 as mc

SCRATCH = Path.home() / ".cache/scratch/greek-oga-anabasis1-import-proof/mapping-completion"


def acc(base, *marks):
    """Compose a Greek letter string from explicit combining marks (NFC for comparison)."""
    return unicodedata.normalize("NFC", base + "".join(marks))


SMOOTH, ROUGH, ACUTE, GRAVE, PERISPOMENI = "̓", "̔", "́", "̀", "͂"


def rows_for(*specs):
    """specs: (key, class, derivation, line, file)."""
    return [mc.parse_row(f, n, c, k, d) for k, c, d, n, f in specs]


def decide(raw, rows, occurrences=("t_1",)):
    by_head, by_bare = mc.index_maps(rows)
    return mc.identity_decision(mc.label_for(raw, occurrences), by_head, by_bare, rows)


class RegressionIdentity(unittest.TestCase):
    """Fixtures that the eb2e9f70 evaluator resolved wrongly by erasing diacritics."""

    def test_rough_smooth_difference_does_not_resolve(self):
        # ἦ (eta + smooth + perispomeni) must not reach Morpheus h)/1 (ἤ, smooth + oxia).
        rows = rows_for(("h)/1", "lemlist", "", 1, "lemlist"))
        label_word = acc("η", SMOOTH, PERISPOMENI) + "1"
        decision = decide(label_word, rows)
        self.assertNotEqual(decision["decision"], "candidate-match-under-unvalidated-hypothesis")
        self.assertEqual(decision["exact_witnesses"], [])

    def test_grave_versus_acute_does_not_resolve(self):
        # εὒθύς2 (grave on the first upsilon) must not reach Morpheus eu)qu/s2 (acute).
        rows = rows_for(("eu)qu/s2", "lemlist", "", 1, "lemlist"))
        label_word = acc("ε", "") + acc("υ", SMOOTH, GRAVE) + "θύς" + "2"
        decision = decide(label_word, rows)
        self.assertNotEqual(decision["decision"], "candidate-match-under-unvalidated-hypothesis")
        self.assertEqual(decision["exact_witnesses"], [])

    def test_exact_precomposed_and_decomposed_spellings_match(self):
        rows = rows_for(("h)/1", "lemlist", "", 1, "lemlist"))
        exact = acc("η", SMOOTH, ACUTE) + "1"
        self.assertEqual(decide(exact, rows)["decision"], "candidate-match-under-unvalidated-hypothesis")
        self.assertEqual(decide(unicodedata.normalize("NFD", exact), rows)["decision"],
                         "candidate-match-under-unvalidated-hypothesis")

    def test_sigma_folding_is_the_only_equivalence(self):
        rows = rows_for(("ce/nos1", "lemlist", "", 1, "lemlist"))
        self.assertEqual(decide("ξένοσ1", rows)["decision"], "candidate-match-under-unvalidated-hypothesis")

    def test_accent_erased_witness_is_listed_but_not_approved(self):
        rows = rows_for(("h)/1", "lemlist", "", 1, "lemlist"))
        decision = decide(acc("η", SMOOTH, PERISPOMENI) + "1", rows)
        self.assertEqual(decision["decision"], "no-exact-source-key")
        self.assertGreaterEqual(decision["accent_erased_near_witness_count"], 1)

    def test_source_diacritic_mutation_changes_decision(self):
        original = rows_for(("h)/1", "lemlist", "", 1, "lemlist"))
        mutated = rows_for(("h)=1", "lemlist", "", 1, "lemlist"))
        label_word = acc("η", SMOOTH, ACUTE) + "1"
        self.assertEqual(decide(label_word, original)["decision"], "candidate-match-under-unvalidated-hypothesis")
        self.assertNotEqual(decide(label_word, mutated)["decision"],
                            "candidate-match-under-unvalidated-hypothesis")

    def test_shared_key_from_two_derivations_is_collision_not_homograph_proof(self):
        rows = rows_for(
            ("pro/seimi1", "compound", "pro/s-ei)=mi", 5639, "vbs.cmp.ml"),
            ("pro/seimi1", "compound", "pro/s-ei)mi/", 5643, "vbs.cmp.ml"),
        )
        decision = decide("πρόσειμι1", rows)
        self.assertEqual(decision["decision"], "source-key-collision")
        self.assertEqual(len(decision["collision_derivations"]), 2)
        self.assertEqual(decision["canonical_identity"], "not-adopted")

    def test_loose_record_never_approves(self):
        # "le/gw3-" is undecodable exactly; the loose retry reads it as le/gw3.
        rows = rows_for(("le/gw3-", "lemlist", "", 1, "lemlist"))
        self.assertTrue(rows[0]["loose"])
        self.assertEqual(decide("λέγω3", rows)["decision"], "loose-record-only-no-approval")

    def test_no_decision_is_called_resolved(self):
        rows = rows_for(("le/gw3", "lemlist", "", 1, "lemlist"))
        self.assertNotIn("resolved", decide("λέγω3", rows)["decision"])


class PosProposal(unittest.TestCase):
    def test_unknown_native_tags_are_unsupported_not_uppercased(self):
        for tag in ("g", "x", "M"):
            result = mc.propose(tag, "ADV", "λόγος", False, "absent")
            self.assertEqual(result["status"], "unsupported-native-tag", tag)
            self.assertEqual(result["candidates"], [], tag)

    def test_known_native_tags_yield_valid_upos_candidates(self):
        for tag in sorted(mc.KNOWN_NATIVE):
            for deprel in ("ADV", "OBJ", "SBJ", "COORD", "AuxY"):
                result = mc.propose(tag, deprel, "λόγος", False, "absent")
                self.assertNotEqual(result["status"], "unsupported-native-tag", (tag, deprel))
                self.assertTrue(set(result["candidates"]) <= mc.UPOS, (tag, deprel))

    def test_noun_with_adverb_relation_is_unresolved_proposal_not_contradiction(self):
        result = mc.propose("n", "ADV", "λόγος", False, "absent")
        self.assertEqual(result["status"], "proposal-multi-candidate")
        self.assertNotIn("contradiction", result["note"])

    def test_proper_evidence_requires_exact_key(self):
        rows = rows_for(("*(/ellhn", "lemlist", "", 1, "nom01"))
        by_head, _ = mc.index_maps(rows)
        self.assertEqual(mc.exact_evidence(acc("ἕλλην", ""), by_head), "exact-starred-only")
        self.assertEqual(mc.exact_evidence("Ελλην", by_head), "absent")

    def test_copula_proposal_needs_pnom_child(self):
        self.assertEqual(mc.propose("v", "PRED", "εἰμί", True, "absent")["candidates"], ["AUX"])
        self.assertEqual(mc.propose("v", "PRED", "εἰμί", False, "absent")["candidates"], ["VERB"])


class Alignment(unittest.TestCase):
    def node(self, nid, surface, lemma, pos, empty=False):
        return {"id": nid, "surface": surface, "raw_lemma": lemma, "empty": empty,
                "native_morphology": {"pos": pos}}

    def col(self, nid, surface, lemma, pos):
        return [nid, surface, lemma, pos] + ["_"] * 5 + [nid]

    def test_empty_nodes_are_counted_separately_from_printed(self):
        nodes = [self.node("t_1", "α", "α", "n"), self.node("t_2", "", None, "", empty=True),
                 self.node("t_3", "β", "β", "v")]
        conllu = {"t_1": self.col("t_1", "α", "α", "n"), "t_3": self.col("t_3", "β", "β", "v")}
        result = mc.align(nodes, conllu)
        self.assertEqual((result["printed"], result["printed_aligned"], result["empty"]), (2, 2, 1))
        self.assertEqual(result["mismatches"], [])

    def test_lemma_mismatch_is_reported(self):
        nodes = [self.node("t_1", "α", "α", "n")]
        conllu = {"t_1": self.col("t_1", "α", "γ", "n")}
        self.assertEqual(mc.align(nodes, conllu)["mismatches"], ["t_1"])


class Reproduction(unittest.TestCase):
    def scratch(self):
        SCRATCH.mkdir(parents=True, exist_ok=True)
        return tempfile.TemporaryDirectory(dir=SCRATCH)

    def test_check_pin_detects_mutation(self):
        with self.scratch() as tmp:
            path = Path(tmp) / "x.txt"
            path.write_text("changed", encoding="utf-8")
            with self.assertRaises(SystemExit):
                mc.check_pin(path, "0" * 64)
            mc.check_pin(path, mc.sha256(path))

    def test_identical_reproduction_has_no_problems(self):
        with self.scratch() as tmp:
            out = Path(tmp) / "out"
            out.mkdir()
            (out / "led.json").write_text("[]\n", encoding="utf-8")
            summary = json.dumps({"a": 1}, sort_keys=True) + "\n"
            self.assertEqual(mc.reproduction_problems(summary, summary, {"led.json": "[]\n"}, out), [])

    def test_summary_mutation_is_reported_by_key(self):
        with self.scratch() as tmp:
            committed = json.dumps({"a": 1, "b": 2}, sort_keys=True) + "\n"
            computed = json.dumps({"a": 1, "b": 3}, sort_keys=True) + "\n"
            self.assertEqual(mc.diff_keys(committed, computed), ["b"])
            problems = mc.reproduction_problems(committed, computed, {}, Path(tmp))
            self.assertTrue(any("summary differs" in p for p in problems))

    def test_private_output_mutation_is_reported(self):
        with self.scratch() as tmp:
            out = Path(tmp)
            (out / "led.json").write_text("tampered", encoding="utf-8")
            summary = "{}\n"
            problems = mc.reproduction_problems(summary, summary, {"led.json": "[]\n"}, out)
            self.assertTrue(any("led.json" in p for p in problems))

    def test_missing_committed_summary_is_reported(self):
        with self.scratch() as tmp:
            self.assertTrue(mc.reproduction_problems(None, "{}\n", {}, Path(tmp)))

    def test_write_guard_refuses_v1_and_import_paths(self):
        with self.assertRaises(SystemExit):
            mc.guard_write_targets([mc.V1_DIR / "morpheus-index.tsv"])
        with self.assertRaises(SystemExit):
            mc.guard_write_targets([mc.PRIVATE / "import" / "native-nodes.json"])
        with self.assertRaises(SystemExit):
            mc.guard_write_targets([mc.HERE / "mapping-completion-summary.json"])
        mc.guard_write_targets([mc.OUT_DIR / "mapping-ledger-v2.json", mc.SUMMARY])


if __name__ == "__main__":
    unittest.main()
