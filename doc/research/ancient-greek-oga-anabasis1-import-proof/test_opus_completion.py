"""Focused tests for opus_completion.py (#2052 fresh-owner pass, v2 revision).

Synthetic fixtures are invented to exercise one rule each; they are not source
observations. Temporary files go to an owned scratch directory, not /tmp. The
last class compares a full rebuild with the committed v2 summary and is skipped
when the private inputs are absent.
"""

from __future__ import annotations

import collections
import json
import shutil
import tempfile
import unicodedata
import unittest
from pathlib import Path

import mapping_completion_v2 as v2
import opus_completion as oc

SCRATCH = Path.home() / ".cache/scratch/greek-oga-anabasis1-import-proof/opus-completion-tests"


class OwnedScratch:
    """A fresh directory under the owned scratch root, removed on exit."""

    def __enter__(self) -> Path:
        SCRATCH.mkdir(parents=True, exist_ok=True)
        self.path = Path(tempfile.mkdtemp(prefix="t-", dir=SCRATCH))
        return self.path

    def __exit__(self, *exc) -> None:
        shutil.rmtree(self.path, ignore_errors=True)


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

    def test_compound_notation_needs_the_same_numbered_base(self):
        record = ("vbs.cmp.ml", "6459", "compound", "sulle/gw", "su/n-le/gw2")
        self.assertEqual(lemma("σύνλέγω3", record)["status"], "unresolved")
        self.assertEqual(lemma("σύνλέγω", record)["status"], "unresolved")

    def test_ambiguous_compound_notation_is_not_decoded(self):
        got = lemma("σύνλέγω2", ("c", "1", "compound", "sulle/gw", "su/n-le/gw2"),
                    ("c", "2", "compound", "sunle/gw", "su/n-le/gw2"))
        self.assertEqual(got["status"], "unresolved")

    def test_exact_key_wins_over_compound_notation(self):
        got = lemma("σύνλέγω2", ("x", "1", "lemlist", "su/nle/gw2", ""),
                    ("c", "2", "compound", "sulle/gw", "su/n-le/gw2"))
        self.assertEqual(got["rule"], "I-NUM-MORPHEUS-KEY")

    def test_key_from_two_bases_is_qualified_not_rejected(self):
        got = lemma("πρόσειμι1", ("c", "1", "compound", "pro/seimi1", "pro/s-ei)=mi"),
                    ("c", "2", "compound", "pro/seimi1", "pro/s-ei)mi/"))
        self.assertEqual(got["status"], "resolved")
        self.assertEqual(got["declared"]["compound_bases"], ["εἰμί", "εἶμι"])
        self.assertEqual(got["qualifications"], ["scheme-derivation-ambiguous"])

    def test_unattested_anomalous_plain_form_is_unsupported(self):
        for raw in ("ἐνεἰμί", "σύλλέγω", "σύνεἶδον", "κατάκαθέζομαι"):
            got = lemma(raw, self.LEGW3)
            self.assertEqual(got["rule"], "I-UNSUPPORTED-CITATION-FORM", raw)
            self.assertEqual(got["predicted_form"], raw)
            self.assertIsNone(got["headword"])

    def test_no_grave_acute_equivalence(self):
        got = lemma("ἑπτὰ", ("lemlist", "1", "lemlist", "e(pta/", ""))
        self.assertEqual(got["rule"], "I-UNSUPPORTED-CITATION-FORM")
        self.assertEqual(lemma("ἑπτά", ("lemlist", "1", "lemlist", "e(pta/", ""))["rule"], "I-PLAIN")

    def test_attested_anomalous_form_stays_usable(self):
        got = lemma("οἶκόνδε", ("lemlist", "1", "lemlist", "oi)=ko/nde", ""))
        self.assertEqual(got["rule"], "I-PLAIN")
        self.assertTrue(got["attested_key"])
        self.assertEqual(got["raw_flags"], ["multiple-accents"])

    def test_unattested_well_formed_plain_form_stays_usable(self):
        got = lemma("κῦρος", self.LEGW3)
        self.assertEqual((got["rule"], got["status"], got["attested_key"]), ("I-PLAIN", "resolved", False))

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
        self.assertEqual(oc.vocabulary_status(lemma("ἐνεἰμί", self.LEGW3),
                                              {"status": "resolved", "upos": "VERB"}), "unresolved-identity")


EVIDENCE = {"pairs": {("c", "ἤ"): collections.Counter({"CCONJ": 3}),
                      ("c", "ὡσ"): collections.Counter({"SCONJ": 2, "CCONJ": 1})}}
NONE = {"pnom": False, "pp": False, "adverbial": False, "infinitive_subject": False}


def ctx(**kw) -> dict:
    return {**NONE, **kw}


class PosRules(unittest.TestCase):
    def test_unknown_native_tag_fails_closed(self):
        got = oc.interpret_pos("x", "λέγω", "PRED", NONE, EVIDENCE)
        self.assertEqual(got["status"], "unresolved")
        self.assertIsNone(got["upos"])
        self.assertEqual(oc.vocabulary_status({"status": "resolved"}, got), "unresolved-pos")

    def test_letters_follow_the_conversion(self):
        self.assertEqual(oc.interpret_pos("n", "κῦροσ", "SBJ", NONE, EVIDENCE)["upos"], "NOUN")
        self.assertEqual(oc.interpret_pos("n", "ὁδόσ", "ADV", NONE, EVIDENCE)["upos"], "NOUN")
        self.assertEqual(oc.interpret_pos("a", "πολύσ", "ADV", NONE, EVIDENCE)["upos"], "ADJ")
        self.assertEqual(oc.interpret_pos("d", "δέ", "AuxY", NONE, EVIDENCE)["upos"], "ADV")

    def test_conjunction_pairs(self):
        self.assertEqual(oc.interpret_pos("c", "ἤ", "COORD", NONE, EVIDENCE)["upos"], "CCONJ")
        undecided = oc.interpret_pos("c", "ὡσ", "AuxC", NONE, EVIDENCE)
        self.assertEqual(undecided["status"], "ambiguous-outside-allowed")
        self.assertEqual(oc.vocabulary_status({"status": "unresolved"}, undecided), "non-vocabulary")

    def test_eimi_rules(self):
        eimi = oc.profile_key("εἰμί")
        cases = [
            (ctx(pnom=True, infinitive_subject=True), "AUX", "P-V-EIMI-COP-PNOM"),
            (ctx(pp=True), "AUX", "P-V-EIMI-COP-ADVERBIAL-PREDICATE"),
            (ctx(adverbial=True), "AUX", "P-V-EIMI-COP-ADVERBIAL-PREDICATE"),
            (ctx(infinitive_subject=True, adverbial=True), "VERB", "P-V-EIMI-IMPERSONAL-INFINITIVE"),
            (ctx(infinitive_subject=True, pp=True), "AUX", "P-V-EIMI-COP-ADVERBIAL-PREDICATE"),
            (NONE, "VERB", "P-V-EIMI-EXISTENTIAL"),
        ]
        for context, upos, rule in cases:
            got = oc.interpret_pos("v", eimi, "PRED", context, EVIDENCE)
            self.assertEqual((got["upos"], got["rule"]), (upos, rule), context)

    def test_eimi_and_eimi_go_are_distinct(self):
        go = oc.interpret_pos("v", oc.profile_key("εἶμι"), "PRED", ctx(pnom=True), EVIDENCE)
        self.assertEqual((go["upos"], go["rule"]), ("VERB", "P-V-VERB"))

    def test_auxv_on_other_verb_is_unmapped(self):
        self.assertEqual(oc.interpret_pos("v", "ἔχω", "AuxV", NONE, EVIDENCE)["status"], "unresolved")

    def test_eimi_context_from_native_rows(self):
        verb = ["1", "ἦν", "εἰμί", "v", "", "", "0", "PRED", "_", "t_1"]
        coord = ["2", "καί", "καί", "c", "", "", "1", "COORD", "_", "t_2"]
        pnom = ["3", "x", "x", "a", "", "", "2", "PNOM", "_", "t_3"]
        inf = ["4", "λαβεῖν", "λαμβάνω", "v", "", "Mood=n|Tense=a", "1", "SBJ", "_", "t_4"]
        self.assertTrue(oc.eimi_context(verb, {"1": [coord], "2": [pnom]})["pnom"])
        got = oc.eimi_context(verb, {"1": [inf]})
        self.assertEqual(got, ctx(infinitive_subject=True))

    def test_conversion_evidence_fails_closed_when_letter_splits(self):
        rows = ["1\tα\tα\tADV\td--------\t_\t0\troot\t_\t_", "2\tβ\tβ\tPART\td--------\t_\t1\tdiscourse\t_\t_"]
        with OwnedScratch() as tmp:
            path = tmp / "x.conllu"
            path.write_text("\n".join(rows) + "\n\n", encoding="utf-8")
            with self.assertRaises(SystemExit):
                oc.ud_perseus_evidence([path])


def native(node_id, surface, lemma_, pos, start, end, empty=False):
    return {"id": node_id, "surface": surface, "raw_lemma": lemma_, "empty": empty,
            "native_morphology": {"pos": pos}, "reading_span": [start, end], "cts": "1_1_1"}


def row(node_id, surface, lemma_, pos, number="1", head="0"):
    return [number, surface, lemma_, pos, "", "_", head, "PRED", "_", node_id]


class IntegrityGates(unittest.TestCase):
    TEXT = "αβ γ ."

    def nodes(self):
        return [native("t_1", "αβ", "αβ", "n", 0, 2), native("t_2", "γ", "γ", "v", 3, 4),
                native("t_3", ".", ".", "u", 5, 6), native("e_1", "[0]", "[0]", "-", 0, 0, empty=True)]

    def groups(self):
        return [[row("t_1", "αβ", "αβ", "n"), row("t_2", "γ", "γ", "v", "2"), row("t_3", ".", ".", "u", "3"),
                 row("e_1", "[0]", "[0]", "-", "4")]]

    def test_valid_fixture_passes(self):
        self.assertEqual(oc.check_native_join(self.nodes(), self.groups())["printed_rows_joined"], 3)
        printed = [n for n in self.nodes() if not n["empty"]]
        self.assertEqual(oc.check_coverage(self.TEXT, printed)["covered_non_whitespace"], 4)
        self.assertTrue(oc.check_sentence_coverage(self.TEXT, oc.segment(self.TEXT))["tail_is_whitespace"])

    def test_missing_row_duplicate_join_and_duplicate_id(self):
        groups = self.groups()
        del groups[0][1]
        with self.assertRaises(SystemExit):
            oc.check_native_join(self.nodes(), groups)
        groups = self.groups()
        groups[0].append(row("t_2", "γ", "γ", "v", "5"))
        with self.assertRaises(SystemExit):
            oc.check_native_join(self.nodes(), groups)
        with self.assertRaises(SystemExit):
            oc.check_native_join(self.nodes() + [native("t_1", "αβ", "αβ", "n", 0, 2)], self.groups())

    def test_row_without_node(self):
        groups = self.groups()
        groups[0].append(row("t_9", "δ", "δ", "n", "5"))
        with self.assertRaises(SystemExit):
            oc.check_native_join(self.nodes(), groups)

    def test_surface_lemma_pos_and_diacritic_mismatch(self):
        for column, value in ((1, "αϐ"), (2, "ἀβ"), (3, "a")):
            groups = self.groups()
            groups[0][0][column] = value
            with self.assertRaises(SystemExit, msg=f"column {column}"):
                oc.check_native_join(self.nodes(), groups)

    def test_uncovered_overlapping_and_wrong_span(self):
        printed = [n for n in self.nodes() if not n["empty"]]
        with self.assertRaises(SystemExit):
            oc.check_coverage(self.TEXT, printed[:2])
        overlap = printed + [native("t_4", "β", "β", "n", 1, 2)]
        with self.assertRaises(SystemExit):
            oc.check_coverage(self.TEXT, overlap)
        wrong = [native("t_1", "αβ", "αβ", "n", 1, 3)] + printed[1:]
        with self.assertRaises(SystemExit):
            oc.check_coverage(self.TEXT, wrong)

    def test_group_crossing_fails(self):
        text = "α . β ."
        sentences = oc.segment(text)
        where = {"t_1": 0, "t_2": 1}
        groups = [[row("t_1", "α", "α", "n"), row("t_2", "β", "β", "n", "2")]]
        with self.assertRaises(SystemExit):
            oc.native_group_check(groups, where, {})
        self.assertEqual(len(sentences), 2)

    def test_sentence_tail_and_gap(self):
        sentences = oc.segment("α .")
        with self.assertRaises(SystemExit):
            oc.check_sentence_coverage("α . β", sentences)
        with self.assertRaises(SystemExit):
            oc.check_sentence_coverage("β α .", [{**sentences[0], "start": 2, "end": 5}])


class Sentences(unittest.TestCase):
    def test_terminals_closers_and_medial_ano_teleia(self):
        # Synthetic only: a terminal inside parentheses is not claimed for the source.
        text = "α β · γ . δ ; ε ( ζ . ) η ."
        got = oc.segment(text)
        self.assertEqual([text[s["start"]:s["end"]] for s in got], ["α β · γ .", "δ ;", "ε ( ζ . )", "η ."])
        self.assertEqual(got[0]["units"], 5)
        self.assertEqual(got[0]["punctuation_units"], 2)

    def test_unit_must_end_on_terminal(self):
        with self.assertRaises(SystemExit):
            oc.segment("α β .  γ")

    def test_unreviewed_terminal_fails_closed(self):
        for char in ("!", "?", ";", "·"):
            with self.assertRaises(SystemExit):
                oc.segment(f"α {char} β .")

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
        ledger = {"t_1": node("t_1", 0, 1, "λ", 0),
                  "t_2": node("t_2", len(long_sentence) + 1, len(long_sentence) + 2, "α", 1)}
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
        with OwnedScratch() as tmp:
            path = tmp / "input"
            path.write_text("x", encoding="utf-8")
            with self.assertRaises(SystemExit):
                oc.check_pin(path, "0" * 64)
            with self.assertRaises(SystemExit):
                oc.check_pin(tmp / "missing", "0" * 64)

    def test_summary_and_private_output_mismatch(self):
        with OwnedScratch() as tmp:
            (tmp / "a.json").write_text("old", encoding="utf-8")
            problems = oc.reproduction_problems('{"x": 1}\n', '{"x": 2}\n', {"a.json": "new", "b.tsv": "z"}, tmp)
            self.assertEqual(problems, ["summary differs in keys ['x']", "private output differs from computed: a.json",
                                        "private output missing: b.tsv"])
            self.assertEqual(oc.reproduction_problems(None, "{}", {}, tmp)[0][:24], "committed summary missin")

    def test_write_guard_protects_earlier_outputs(self):
        oc.guard_write_targets([oc.SUMMARY, oc.OUT_DIR / "x.json"])
        for bad in (oc.HERE / "opus-completion-summary.json", oc.OWN / "out" / "node-ledger.json",
                    oc.HERE / "mapping-completion-v2-summary.json", oc.PRIVATE / "mapping-completion-v2" / "x.json",
                    oc.PRIVATE / "import" / "native-nodes.json"):
            with self.assertRaises(SystemExit):
                oc.guard_write_targets([bad])


@unittest.skipUnless(oc.NATIVE_NODES.is_file() and oc.SUMMARY.is_file(), "private inputs not present")
class FullBuild(unittest.TestCase):
    def test_rebuild_matches_committed_summary_and_gates(self):
        summary, private = oc.build()
        text = json.dumps(summary, ensure_ascii=False, indent=1, sort_keys=True) + "\n"
        self.assertEqual(oc.reproduction_problems(oc.SUMMARY.read_text(encoding="utf-8"), text, private, oc.OUT_DIR), [])
        gates = summary["required_gates"]
        for name in ("native_join", "reading_coverage", "sentence_coverage", "native_groups_nest",
                     "canonical_pos_determinate"):
            self.assertTrue(gates[name]["pass"], name)
        self.assertEqual(gates["required_identity_usable"]["unsupported_ids"],
                         ["t_437", "t_4624", "t_4638", "t_4930", "t_5121", "t_5145", "t_5775"])
        self.assertFalse(gates["vocabulary_ready_accounting"]["pass"])
        self.assertEqual(summary["identity_layer"]["numbered_occurrences"], 142)


if __name__ == "__main__":
    unittest.main()
