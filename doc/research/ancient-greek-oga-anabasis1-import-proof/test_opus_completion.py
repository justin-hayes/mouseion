"""Focused tests for opus_completion.py (#2052 fresh-owner pass).

Synthetic fixtures are invented to exercise one rule each; they are not source
observations. The last class compares a full rebuild with the committed summary
and is skipped when the private inputs are absent.
"""

from __future__ import annotations

import collections
import json
import tempfile
import unittest
import unicodedata
from pathlib import Path

import mapping_completion_v2 as v2
import opus_completion as oc


def index_of(*records: tuple[str, str, str, str, str]) -> tuple[dict, dict, list[dict]]:
    """Index maps from (file, line, class, key, derivation) tuples."""
    rows = [v2.parse_row(f, int(n), c, k, d) for f, n, c, k, d in records]
    by_head, _ = v2.index_maps(rows)
    return by_head, oc.compound_notation_index(rows), rows


STEMS = {"stems": [":le:le/gw3", ":de:leg reg_conj", "", ":le:h)/1", ":wd:h)/ conj", ""]}


def lemma(raw: str, *records) -> dict:
    by_head, compounds, _ = index_of(*records)
    return oc.interpret_lemma(raw, by_head, compounds, STEMS)


def node(node_id, start, end, surface, sentence=0, flags=()):
    return {"id": node_id, "reading_span": [start, end], "surface": surface, "sentence": sentence,
            "citation_flags": list(flags)}


class IdentityRules(unittest.TestCase):
    LEGW3 = ("stems", "1", "le-block", "le/gw3", "")

    def test_numbered_label_with_exact_key_keeps_scheme_discriminator(self):
        got = lemma("λέγω3", self.LEGW3)
        self.assertEqual(got["rule"], "I-NUM-MORPHEUS-KEY")
        self.assertEqual(got["headword"], "λέγω")
        self.assertEqual(got["discriminator"], {"scheme": "morpheus", "value": "3"})
        self.assertEqual(oc.identity_string(got), "λέγω#morpheus:3")
        self.assertEqual(got["declared"]["stem_classes"], ["de"])

    def test_missing_digit_mapping_stays_unresolved(self):
        got = lemma("λέγω4", self.LEGW3)
        self.assertEqual(got["status"], "unresolved")
        self.assertIsNone(got["headword"])

    def test_significant_diacritics_never_match(self):
        self.assertEqual(lemma("λεγω3", self.LEGW3)["status"], "unresolved")
        self.assertEqual(lemma("λέγὼ3", self.LEGW3)["status"], "unresolved")
        self.assertNotEqual(oc.profile_key("εἰμί"), oc.profile_key("εἶμι"))

    def test_plain_label_is_not_a_wildcard_for_numbered(self):
        plain = lemma("λέγω", self.LEGW3)
        self.assertEqual(plain["rule"], "I-PLAIN")
        self.assertIsNone(plain["discriminator"])
        self.assertNotEqual(oc.identity_string(plain), oc.identity_string(lemma("λέγω3", self.LEGW3)))

    def test_compound_record_decodes_hyphenless_notation(self):
        got = lemma("σύνλέγω2", ("vbs.cmp.ml", "6459", "compound", "sulle/gw", "su/n-le/gw2"))
        self.assertEqual(got["rule"], "I-COMPOUND-RECORD")
        self.assertEqual(got["headword"], "συλλέγω")
        self.assertIsNone(got["discriminator"])
        self.assertEqual(got["raw_flags"], ["multiple-accents"])
        self.assertEqual(got["citation_flags"], [])

    def test_ambiguous_compound_notation_is_not_decoded(self):
        got = lemma("σύνλέγω2", ("c", "1", "compound", "sulle/gw", "su/n-le/gw2"),
                    ("c", "2", "compound", "sunle/gw", "su/n-le/gw2"))
        self.assertEqual(got["status"], "unresolved")

    def test_exact_key_wins_over_compound_notation(self):
        got = lemma("σύνλέγω2", ("x", "1", "lemlist", "su/nle/gw2", ""),
                    ("c", "2", "compound", "sulle/gw", "su/n-le/gw2"))
        self.assertEqual(got["rule"], "I-NUM-MORPHEUS-KEY")

    def test_key_from_two_bases_is_qualified(self):
        got = lemma("πρόσειμι1", ("c", "1", "compound", "pro/seimi1", "pro/s-ei)=mi"),
                    ("c", "2", "compound", "pro/seimi1", "pro/s-ei)mi/"))
        self.assertEqual(got["declared"]["compound_bases"], ["εἰμί", "εἶμι"])
        self.assertEqual(got["qualifications"], ["scheme-derivation-ambiguous"])

    def test_missing_stem_source_fails_closed(self):
        by_head, compounds, _ = index_of(("absent-file", "1", "le-block", "le/gw3", ""))
        with self.assertRaises(SystemExit):
            oc.interpret_lemma("λέγω3", by_head, compounds, STEMS)

    def test_citation_form_flags(self):
        self.assertEqual(oc.citation_form_flags("ἐνεἰμί"), ["medial-breathing"])
        self.assertEqual(oc.citation_form_flags("σύλλέγω"), ["multiple-accents"])
        self.assertEqual(oc.citation_form_flags("ἑπτὰ"), ["grave-accent"])
        for good in ("λέγω", "εἰμί", "εὐθύς", "ῥήτωρ", "ὁ", "Ἀρταξέρξης"):
            self.assertEqual(oc.citation_form_flags(good), [], good)
        self.assertIn("stray-combining-mark", oc.citation_form_flags("̔"))
        self.assertIn("non-greek-character", oc.citation_form_flags("lego"))

    def test_profile_key(self):
        nfd = unicodedata.normalize("NFD", "Κῦρος")
        self.assertEqual(oc.profile_key(nfd), "κῦροσ")
        self.assertEqual(oc.profile_key("ᾠδή"), "ᾠδή")  # iota subscript kept


EVIDENCE = {"pairs": {("c", "ἤ"): collections.Counter({"CCONJ": 3}),
                      ("c", "ὡσ"): collections.Counter({"SCONJ": 2, "CCONJ": 1})}}


class PosRules(unittest.TestCase):
    def test_unknown_native_tag_fails_closed(self):
        got = oc.interpret_pos("x", "λέγω", "PRED", False, EVIDENCE)
        self.assertEqual(got["status"], "unresolved")
        self.assertIsNone(got["upos"])
        self.assertEqual(oc.vocabulary_status({"status": "resolved"}, got), "unresolved-pos")

    def test_letters_follow_the_conversion(self):
        self.assertEqual(oc.interpret_pos("n", "κῦροσ", "SBJ", False, EVIDENCE)["upos"], "NOUN")
        self.assertEqual(oc.interpret_pos("n", "ὁδόσ", "ADV", False, EVIDENCE)["upos"], "NOUN")
        self.assertEqual(oc.interpret_pos("a", "πολύσ", "ADV", False, EVIDENCE)["upos"], "ADJ")
        self.assertEqual(oc.interpret_pos("d", "δέ", "AuxY", False, EVIDENCE)["upos"], "ADV")

    def test_conjunction_pairs(self):
        self.assertEqual(oc.interpret_pos("c", "ἤ", "COORD", False, EVIDENCE)["upos"], "CCONJ")
        undecided = oc.interpret_pos("c", "ὡσ", "AuxC", False, EVIDENCE)
        self.assertEqual(undecided["status"], "ambiguous-outside-allowed")
        self.assertEqual(oc.vocabulary_status({"status": "unresolved"}, undecided), "non-vocabulary")

    def test_eimi_context_and_diacritics(self):
        cop = oc.interpret_pos("v", oc.profile_key("εἰμί"), "PRED", True, EVIDENCE)
        other = oc.interpret_pos("v", oc.profile_key("εἰμί"), "PRED", False, EVIDENCE)
        go = oc.interpret_pos("v", oc.profile_key("εἶμι"), "PRED", False, EVIDENCE)
        self.assertEqual((cop["upos"], cop["rule"]), ("AUX", "P-V-EIMI-PNOM"))
        self.assertEqual((other["upos"], other["rule"]), ("AUX", "P-V-EIMI-OTHER"))
        self.assertEqual(go["upos"], "VERB")

    def test_auxv_on_other_verb_is_unmapped(self):
        self.assertEqual(oc.interpret_pos("v", "ἔχω", "AuxV", False, EVIDENCE)["status"], "unresolved")

    def test_unresolved_identity_with_eligible_pos(self):
        pos = oc.interpret_pos("v", "λέγω", "PRED", False, EVIDENCE)
        self.assertEqual(oc.vocabulary_status({"status": "unresolved"}, pos), "unresolved-identity")

    def test_pnom_through_coordination(self):
        verb = ["1", "ἦν", "εἰμί", "v", "", "", "0", "PRED", "_", "t_1"]
        coord = ["2", "καί", "καί", "c", "", "", "1", "COORD", "_", "t_2"]
        pnom = ["3", "x", "x", "a", "", "", "2", "PNOM", "_", "t_3"]
        children = {"1": [coord], "2": [pnom]}
        self.assertTrue(oc.pnom_reachable(verb, children))
        self.assertFalse(oc.pnom_reachable(verb, {"1": [["2", "", "", "", "", "", "1", "AuxZ"]]}))

    def test_conversion_evidence_fails_closed_when_letter_splits(self):
        rows = ["1\tα\tα\tADV\td--------\t_\t0\troot\t_\t_", "2\tβ\tβ\tPART\td--------\t_\t1\tdiscourse\t_\t_"]
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "x.conllu"
            path.write_text("\n".join(rows) + "\n\n", encoding="utf-8")
            with self.assertRaises(SystemExit):
                oc.ud_perseus_evidence([path])


class Sentences(unittest.TestCase):
    def test_terminals_closers_and_medial_ano_teleia(self):
        text = "α β · γ . δ ; ε ( ζ . ) η ."
        got = oc.segment(text)
        self.assertEqual([text[s["start"]:s["end"]] for s in got], ["α β · γ .", "δ ;", "ε ( ζ . )", "η ."])
        self.assertEqual(got[0]["units"], 5)
        self.assertEqual(got[0]["punctuation_units"], 2)

    def test_unit_must_end_on_terminal(self):
        with self.assertRaises(SystemExit):
            oc.segment("α β .  γ")

    def test_unreviewed_terminal_fails_closed(self):
        with self.assertRaises(SystemExit):
            oc.segment("α ! β .")

    def test_node_crossing_boundary_fails(self):
        sentences = oc.segment("α . β .")
        with self.assertRaises(SystemExit):
            oc.sentence_of([{"id": "t_1", "reading_span": [0, 4]}], sentences)


class SelectionAndInputs(unittest.TestCase):
    def test_three_in_book_with_one_book(self):
        rows = oc.selection_model({("β", "NOUN"): ["a", "b", "c"], ("α", "VERB"): ["d", "e"]})
        self.assertEqual([(r["key"], r["selected"]) for r in rows], [("α", False), ("β", True)])
        self.assertTrue(oc.selection_tsv(rows).startswith("canonical_key\tupos\toccurrences\tselected\n"))

    def test_earliest_capped_input_never_shortened(self):
        long_sentence = " ".join(["λ"] * 51) + " ."
        text = long_sentence + " α β ."
        sentences = oc.segment(text)
        first = node("t_1", 0, 1, "λ", 0)
        later = node("t_2", len(long_sentence) + 1, len(long_sentence) + 2, "α", 1)
        ledger = {"t_1": first, "t_2": later}
        got = oc.card_inputs([{"key": "k", "upos": "NOUN"}], {("k", "NOUN"): ["t_2", "t_1"]}, ledger, sentences, text)
        self.assertEqual(got[0]["chosen"], "t_2")
        self.assertEqual(got[0]["earlier_rejections"], [{"id": "t_1", "reasons": ["over-unit-cap"]}])
        self.assertEqual(sentences[0]["units"], 52)

    def test_omission_reasons_and_unverified_span(self):
        text = "α β ."
        sentences = oc.segment(text)
        ledger = {"t_1": node("t_1", 0, 1, "γ", 0), "t_2": node("t_2", 2, 3, "β", 0, ["medial-breathing"])}
        got = oc.card_inputs([{"key": "k", "upos": "VERB"}], {("k", "VERB"): ["t_1", "t_2"]}, ledger, sentences, text)
        self.assertIsNone(got[0]["chosen"])
        self.assertEqual(got[0]["omission"], ["headword-unusable:medial-breathing", "target-span-unverified"])


class ReproductionGuards(unittest.TestCase):
    def test_hash_mismatch_fails(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "input"
            path.write_text("x", encoding="utf-8")
            with self.assertRaises(SystemExit):
                oc.check_pin(path, "0" * 64)
            with self.assertRaises(SystemExit):
                oc.check_pin(Path(tmp) / "missing", "0" * 64)

    def test_summary_and_private_output_mismatch(self):
        with tempfile.TemporaryDirectory() as tmp:
            (Path(tmp) / "a.json").write_text("old", encoding="utf-8")
            problems = oc.reproduction_problems('{"x": 1}\n', '{"x": 2}\n', {"a.json": "new", "b.tsv": "z"}, Path(tmp))
            self.assertEqual(problems, ["summary differs in keys ['x']", "private output differs from computed: a.json",
                                        "private output missing: b.tsv"])
            self.assertEqual(oc.reproduction_problems(None, "{}", {}, Path(tmp))[0][:24], "committed summary missin")

    def test_write_guard(self):
        oc.guard_write_targets([oc.SUMMARY, oc.OUT_DIR / "x.json"])
        for bad in (oc.HERE / "mapping-completion-v2-summary.json", oc.PRIVATE / "mapping-completion-v2" / "x.json",
                    oc.PRIVATE / "import" / "native-nodes.json"):
            with self.assertRaises(SystemExit):
                oc.guard_write_targets([bad])


@unittest.skipUnless(oc.NATIVE_NODES.is_file() and oc.SUMMARY.is_file(), "private inputs not present")
class FullBuild(unittest.TestCase):
    def test_rebuild_matches_committed_summary(self):
        summary, private = oc.build()
        text = json.dumps(summary, ensure_ascii=False, indent=1, sort_keys=True) + "\n"
        self.assertEqual(oc.reproduction_problems(oc.SUMMARY.read_text(encoding="utf-8"), text, private, oc.OUT_DIR), [])
        self.assertEqual(summary["denominators"]["unresolved_ids"], [])
        self.assertEqual(summary["identity_layer"]["numbered_occurrences"], 142)
        self.assertEqual(summary["denominators"]["printed"], 10462)


if __name__ == "__main__":
    unittest.main()
