#!/usr/bin/env python3
"""Tests for import_proof.py (#2050).

Synthetic tests always run. Full-input tests run when GLAUX_PROOF_INPUTS
names a directory populated by fetch_inputs.sh; they regenerate every
manifest and require byte-identical output, and check Go parity when `go`
is on PATH.

    python3 -I -m unittest discover -s scripts -p 'test_*.py'
"""

from __future__ import annotations

import collections
import os
import shutil
import subprocess
import sys
import tempfile
import unicodedata
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import import_proof as ip  # noqa: E402

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(HERE)
REPO = os.path.abspath(os.path.join(ROOT, "..", "..", ".."))

TEI_HEAD = '<TEI xmlns="http://www.tei-c.org/ns/1.0"><text><body><div type="edition">'
TEI_TAIL = "</div></body></text></TEI>"


def tei(book_body: str) -> str:
    return f'{TEI_HEAD}<div type="textpart" subtype="book" n="1"><head>Κεφ Α</head>{book_body}</div>{TEI_TAIL}'


def section(ch: str, n: str, body: str) -> str:
    return f'<div type="textpart" subtype="chapter" n="{ch}"><div type="textpart" subtype="section" n="{n}"><p>{body}</p></div></div>'


def write(tmp: str, name: str, content: str) -> str:
    path = os.path.join(tmp, name)
    with open(path, "w", encoding="utf-8") as fh:
        fh.write(content)
    return path


def glaux(words: list[dict], book: str = "1") -> str:
    def attrs(w: dict) -> str:
        base = {"div_book": book, "div_chapter": "1.1", "div_section": "1.1.1"}
        base.update(w)
        return " ".join(f'{k}="{v}"' for k, v in base.items())

    body = "".join(f"<word {attrs(w)}/>" for w in words)
    return f'<treebank version="2" xml:lang="grc"><sentence struct_id="1" id="1" document_id="x" analysis="auto">{body}</sentence></treebank>'


def nfd(value: str) -> str:
    return unicodedata.normalize("NFD", value)


def word(
    i: int, form: str, lemma: str = "λ", postag: str = "n-s---mn-", **extra
) -> dict:
    w = {
        "id": str(i),
        "form": nfd(form),
        "lemma": nfd(lemma),
        "postag": postag,
        "head": "0",
        "relation": "SBJ",
    }
    w.update(extra)
    return w


class Fixture(unittest.TestCase):
    def setUp(self) -> None:
        self.tmp = tempfile.mkdtemp()

    def tearDown(self) -> None:
        shutil.rmtree(self.tmp)

    def load(self, body: str, words: list[dict]):
        doc = ip.extract_book(write(self.tmp, "t.xml", tei(body)))
        nodes, _ = ip.read_nodes(write(self.tmp, "g.xml", glaux(words)))
        return doc, nodes


class ExtractionTest(Fixture):
    def test_whitespace_markup_and_heading(self) -> None:
        body = section(
            "1",
            "1",
            "  ἔχει   <del>καὶ\n  φύλακας</del> <sic>ἐπὶ ξύλου</sic> <add>τὸ</add>. ",
        ) + section("2", "1", "δεύτερον.")
        doc = ip.extract_book(write(self.tmp, "t.xml", tei(body)))
        self.assertEqual(doc.text, "ἔχει καὶ φύλακας ἐπὶ ξύλου τὸ. δεύτερον.")
        self.assertEqual(doc.heading, "Κεφ Α")
        self.assertEqual(
            [(k, doc.text[s:e]) for k, s, e, _ in doc.regions],
            [("del", "καὶ φύλακας"), ("sic", "ἐπὶ ξύλου"), ("add", "τὸ")],
        )
        self.assertEqual([r for r, _, _ in doc.sections], ["1.1.1", "1.2.1"])
        self.assertEqual(doc.section[doc.text.index("δεύτερον")], "1.2.1")
        self.assertEqual(doc.section[doc.text.index(" δεύτερον")], "")

    def test_paragraph_milestone_offset_inside_section(self) -> None:
        body = section("1", "1", 'α β. <milestone unit="para" ed="P"/>γ δ.')
        doc = ip.extract_book(write(self.tmp, "t.xml", tei(body)))
        self.assertEqual(doc.text[doc.paragraphs[0] :], "γ δ.")

    def test_unsupported_element_is_rejected(self) -> None:
        body = section("1", "1", "α <choice><sic>ἦν</sic><corr>ἢν</corr></choice>.")
        with self.assertRaises(ValueError):
            ip.extract_book(write(self.tmp, "t.xml", tei(body)))


class DecodingTest(unittest.TestCase):
    def test_decode_surface_only_normalizes_and_maps_elision(self) -> None:
        self.assertEqual(ip.decode_surface(nfd("δ’")), "δʼ")
        self.assertEqual(ip.decode_surface(nfd("ᾐσθάνετο")), "ᾐσθάνετο")
        self.assertNotEqual(ip.decode_surface(nfd("ἦν")), "ἢν")

    def test_classify(self) -> None:
        mk = lambda **a: ip.Node(0, "1", "1", "auto", a, "1", "1.1", "1.1.1")  # noqa: E731
        self.assertEqual(
            ip.classify(mk(id="1", form="E", artificial="elliptic")), "unprinted"
        )
        self.assertEqual(
            ip.classify(mk(id="1", form="", form_original="{καὶ}")), "secluded"
        )
        self.assertEqual(
            ip.classify(mk(id="1", form="τὸ", form_original="<τὸ>")), "supplied"
        )
        self.assertEqual(ip.classify(mk(id="1", form="†")), "crux")
        self.assertEqual(ip.classify(mk(id="1", form="-δὲ")), "suffix_component")
        self.assertEqual(ip.classify(mk(id="1", form="κα-")), "crasis_fragment")
        self.assertEqual(ip.classify(mk(id="1", form="λόγος")), "word")
        with self.assertRaises(ValueError):
            ip.classify(mk(id="1", form="τὸ", form_original="<τὰ>"))
        with self.assertRaises(ValueError):
            ip.classify(mk(id="1", form="καὶ", form_original="{καὶ}"))
        with self.assertRaises(ValueError):
            ip.classify(mk(id="1", form=""))


class BindingTest(Fixture):
    def test_components_crasis_crux_seclusion_and_elision(self) -> None:
        body = section("1", "1", "μηδὲ κἂν <sic>ἐπὶ</sic> <del>καὶ</del> δʼ ἔχει.")
        words = [
            word(1, "μη", "μή", "d--------"),
            word(2, "-δὲ", "δέ", "c--------"),
            word(3, "κα-", "καί", "c--------"),
            word(4, "κἂν", "ἐάν", "c--------"),
            word(5, "†", "†", "u--------"),
            word(6, "ἐπὶ", "ἐπί", "r--------"),
            word(7, "†", "†", "u--------"),
            {"id": "8", "form": "", "form_original": "{καὶ}"},
            word(9, "δ’", "δέ", "c--------"),
            word(10, "ἔχει", "ἔχω", "v3spia---"),
            word(11, ".", ".", "u--------"),
            {
                "id": "12",
                "form": "E",
                "lemma": "E",
                "postag": "z--------",
                "head": "0",
                "relation": "PRED",
                "artificial": "elliptic",
            },
        ]
        doc, nodes = self.load(body, words)
        self.assertEqual(ip.bind(nodes, doc), [])
        span = {n.nid: (n.start, n.end, n.bind_status) for n in nodes}
        t = doc.text
        self.assertEqual(t[span["1"][0] : span["1"][1]], "μη")
        self.assertEqual(t[span["2"][0] : span["2"][1]], "δὲ")
        self.assertEqual(span["3"][:2], span["4"][:2])
        self.assertEqual(span["3"][2], "shared")
        self.assertEqual(t[span["4"][0] : span["4"][1]], "κἂν")
        self.assertEqual(span["5"][0], span["5"][1])
        self.assertEqual(span["5"][0], t.index("ἐπὶ"))
        self.assertEqual(span["7"][0], t.index("ἐπὶ") + 3)
        self.assertEqual(t[span["8"][0] : span["8"][1]], "καὶ")
        self.assertEqual(t[span["9"][0] : span["9"][1]], "δʼ")
        self.assertEqual(span["12"], (None, None, "no_span"))
        cov = ip.coverage(nodes, doc)
        self.assertEqual(cov["uncovered_non_space"], [])
        self.assertEqual(cov["word_boundary_violations"], [])

    def test_mismatch_is_rejected_not_guessed(self) -> None:
        doc, nodes = self.load(
            section("1", "1", "λόγος."),
            [word(1, "λόγου"), word(2, ".", ".", "u--------")],
        )
        with self.assertRaises(ValueError):
            ip.bind(nodes, doc)

    def test_component_must_be_adjacent(self) -> None:
        doc, nodes = self.load(
            section("1", "1", "μη δὲ."),
            [
                word(1, "μη", "μή", "d--------"),
                word(2, "-δὲ", "δέ", "c--------"),
                word(3, ".", ".", "u--------"),
            ],
        )
        with self.assertRaises(ValueError):
            ip.bind(nodes, doc)

    def test_secluded_node_must_lie_in_deletion(self) -> None:
        doc, nodes = self.load(
            section("1", "1", "καὶ λόγος."),
            [
                {"id": "1", "form": "", "form_original": "{καὶ}"},
                word(2, "λόγος"),
                word(3, ".", ".", "u--------"),
            ],
        )
        with self.assertRaises(ValueError):
            ip.bind(nodes, doc)

    def test_prefix_match_is_reported_as_boundary_violation(self) -> None:
        doc, nodes = self.load(
            section("1", "1", "λόγοσ."),
            [
                word(1, "λόγο"),
                word(2, "σ", "σ", "u--------"),
                word(3, ".", ".", "u--------"),
            ],
        )
        ip.bind(nodes, doc)
        self.assertEqual(
            ip.coverage(nodes, doc)["word_boundary_violations"], ["1", "2"]
        )

    def test_uncovered_characters_are_reported(self) -> None:
        doc, nodes = self.load(
            section("1", "1", "λόγος ἔχει."),
            [word(1, "λόγος"), word(2, ".", ".", "u--------")],
        )
        with self.assertRaises(ValueError):
            ip.bind(nodes, doc)
        doc, nodes = self.load(section("1", "1", "λόγος ;"), [word(1, "λόγος")])
        ip.bind(nodes, doc)
        self.assertEqual(ip.coverage(nodes, doc)["uncovered_non_space"], [(6, ";")])

    def test_section_label_mismatch_is_reported(self) -> None:
        doc, nodes = self.load(
            section("1", "2", "λόγος."),
            [word(1, "λόγος"), word(2, ".", ".", "u--------")],
        )
        ip.bind(nodes, doc)
        self.assertEqual(ip.coverage(nodes, doc)["section_mismatches"], ["1", "2"])


class MappingTest(unittest.TestCase):
    letters = {k: collections.Counter({v: 1}) for k, v in ip.LETTER_UPOS.items()}

    def test_profile_key(self) -> None:
        self.assertEqual(ip.canonical_key("Κῦρος"), "κῦροσ")
        self.assertEqual(ip.canonical_key(nfd("Κῦρος")), ip.canonical_key("Κῦρος"))
        self.assertNotEqual(ip.canonical_key("εἰμί"), ip.canonical_key("εἶμι"))
        self.assertIn("ͅ", nfd(ip.canonical_key("ᾠδή")))
        self.assertNotEqual(ip.canonical_key("ἄν"), ip.canonical_key("ἀν"))

    def test_letter_and_verb_rules(self) -> None:
        self.assertEqual(
            ip.map_upos("n", "Κῦρος", "SBJ", {}, self.letters)[:1], ("NOUN",)
        )
        self.assertEqual(ip.map_upos("d", "οὐ", "AuxZ", {}, self.letters)[:1], ("ADV",))
        self.assertEqual(
            ip.map_upos("g", "μέν", "AuxY", {}, self.letters)[:1], ("PART",)
        )
        self.assertEqual(
            ip.map_upos("v", "εἰμί", "PRED", {}, self.letters)[:3],
            ("AUX", "AUX", "M-V-EIMI"),
        )
        self.assertEqual(
            ip.map_upos("v", "ἔχω", "AuxV", {}, self.letters)[:1], ("AUX",)
        )
        self.assertEqual(
            ip.map_upos("v", "ἔχω", "PRED", {}, self.letters)[:1], ("VERB",)
        )
        with self.assertRaises(ValueError):
            ip.map_upos("q", "x", "", {}, self.letters)
        with self.assertRaises(ValueError):
            ip.map_upos(
                "n", "x", "", {}, {"n": collections.Counter({"NOUN": 5, "PROPN": 1})}
            )

    def test_conjunctions(self) -> None:
        by_lemma = {
            ("c", "καί"): collections.Counter({"CCONJ": 9}),
            ("c", "ὡς"): collections.Counter({"SCONJ": 3, "CCONJ": 1}),
        }
        self.assertEqual(
            ip.map_upos("c", "καί", "COORD", by_lemma, self.letters),
            ("CCONJ", "CCONJ", "M-C-LEMMA", "resolved"),
        )
        self.assertEqual(
            ip.map_upos("c", "ὡς", "AuxC", by_lemma, self.letters)[1:],
            ("CCONJ|SCONJ", "M-C-SPLIT", "ambiguous"),
        )
        self.assertEqual(
            ip.map_upos("c", "ἐπειδή", "AuxC", by_lemma, self.letters)[1:],
            ("CCONJ|SCONJ", "M-C-UNATTESTED", "ambiguous"),
        )

    def node(self, **attrs) -> ip.Node:
        n = ip.Node(0, "1", "1", "auto", attrs, "1", "1.1", "1.1.1")
        n.kind = ip.classify(n)
        return n

    def test_vocabulary_status_never_silently_drops(self) -> None:
        nodes = [
            self.node(id="1", form="ἐπειδή", lemma="ἐπειδή", postag="c--------"),
            self.node(id="2", form="οὕτω", lemma="οὕτω", postag="c--------"),
            self.node(id="3", form="", form_original="{καὶ}"),
            self.node(
                id="4", form="E", lemma="E", postag="z--------", artificial="elliptic"
            ),
            self.node(id="5", form="λόγος", lemma="λόγος", postag="n-s---mn-"),
            self.node(id="6", form="λόγος"),
        ]
        by_lemma = {("c", "οὕτω"): collections.Counter({"ADV": 2, "CCONJ": 1})}
        ip.apply_mapping(nodes, by_lemma, self.letters)
        self.assertEqual(
            [n.vocab for n in nodes],
            [
                "non_vocab",
                "unresolved",
                "secluded",
                "unprinted",
                "eligible",
                "unresolved",
            ],
        )


class SegmentationTest(unittest.TestCase):
    def doc(self, text: str) -> ip.Extracted:
        return ip.Extracted(
            text,
            ["1.1.1"] * len(text),
            [""] * len(text),
            [("1.1.1", 0, len(text))],
            [],
            [],
            "",
        )

    def test_ano_teleia_is_medial_and_question_mark_terminal(self) -> None:
        spans = ip.segment(self.doc("α β· γ δ. τί ποιεῖς; ἔφη."))
        self.assertEqual(
            [b for _, _, b in spans], ["terminal '.'", "terminal ';'", "terminal '.'"]
        )
        self.assertEqual(self.doc("").text, "")

    def test_closer_after_terminal_and_missing_final_terminal(self) -> None:
        text = "α (β.) γ δ"
        spans = ip.segment(self.doc(text))
        self.assertEqual([text[s:e] for s, e, _ in spans], ["α (β.)", "γ δ"])
        self.assertEqual(spans[-1][2], "unit end without terminal")


class SelectionAndCardTest(unittest.TestCase):
    def occurrence(
        self, i: int, sentence: int, key: str = "λόγοσ", upos: str = "NOUN"
    ) -> ip.Node:
        n = ip.Node(
            i, "1", "1", "auto", {"id": str(i), "form": "λόγος"}, "1", "1.1", "1.1.1"
        )
        n.kind, n.vocab, n.key, n.upos, n.headword = (
            "word",
            "eligible",
            key,
            upos,
            "λόγος",
        )
        n.start, n.end, n.complete_sentence = i * 10, i * 10 + 5, sentence
        return n

    def sentence(
        self, i: int, units: int, code_points: int, boundary: str = "terminal '.'"
    ) -> ip.Sentence:
        return ip.Sentence(i, 0, 0, "", units, code_points, "1.1.1", [], [], boundary)

    def test_three_occurrence_floor_with_one_book(self) -> None:
        nodes = [self.occurrence(i, 0) for i in range(3)] + [
            self.occurrence(5, 0, "ἄλλοσ"),
            self.occurrence(6, 0, "ἄλλοσ"),
        ]
        non_vocab = self.occurrence(7, 0, "μέν", "PART")
        non_vocab.vocab = "non_vocab"
        groups = ip.project(nodes + [non_vocab])
        sel = ip.select(groups)
        self.assertTrue(sel[("λόγοσ", "NOUN")].startswith("selected"))
        self.assertTrue(sel[("ἄλλοσ", "NOUN")].startswith("not selected"))
        self.assertNotIn(("μέν", "PART"), sel)

    def test_caps_and_earliest_qualifying_occurrence(self) -> None:
        nodes = [self.occurrence(i, i) for i in range(4)]
        sentences = [
            self.sentence(0, 51, 300),
            self.sentence(1, 50, 401),
            self.sentence(2, 50, 400),
            self.sentence(3, 5, 20),
        ]
        groups = ip.project(nodes)
        (row,) = ip.card_inputs(groups, ip.select(groups), sentences, "x" * 100)
        self.assertEqual((row["status"], row["sentence"]), ("eligible", 2))
        self.assertEqual(
            row["earlier_rejections"],
            "sentence over 400 code points x1; sentence over 50 whitespace units x1",
        )

    def test_omission_reason_and_incomplete_boundary(self) -> None:
        nodes = [self.occurrence(i, i) for i in range(3)]
        sentences = [
            self.sentence(0, 60, 300),
            self.sentence(1, 3, 20, "unit end without terminal"),
            self.sentence(2, 51, 30),
        ]
        groups = ip.project(nodes)
        (row,) = ip.card_inputs(groups, ip.select(groups), sentences, "x" * 100)
        self.assertEqual(row["status"], "omitted")
        self.assertEqual(
            row["reason"],
            "no qualifying occurrence: sentence boundary incomplete, sentence over 50 whitespace units",
        )


class TokenStreamTest(unittest.TestCase):
    def test_components_rejoin_and_signs_drop(self) -> None:
        mk = lambda **a: ip.Node(0, "1", "1", "auto", a, "1", "1.1", "1.1.1")  # noqa: E731
        nodes = [
            mk(id="1", form="οὐ"),
            mk(id="2", form="-δ’"),
            mk(id="3", form="κα-"),
            mk(id="4", form="κἂν"),
            mk(id="5", form="†"),
            mk(id="6", form="", form_original="{καὶ}"),
        ]
        for n in nodes:
            n.kind = ip.classify(n)
        self.assertEqual(ip.glaux_token_stream(nodes), ["οὐδʼ", "κἂν", "καὶ"])


INPUTS = os.environ.get("GLAUX_PROOF_INPUTS")


@unittest.skipUnless(INPUTS, "set GLAUX_PROOF_INPUTS to a fetch_inputs.sh directory")
class FullInputTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.out = tempfile.mkdtemp()
        cls.summary = ip.run(INPUTS, cls.out)

    @classmethod
    def tearDownClass(cls) -> None:
        shutil.rmtree(cls.out)

    def test_regeneration_is_byte_identical(self) -> None:
        committed = os.path.join(ROOT, "manifest")
        names = sorted(set(os.listdir(committed)) - {"selection_go.tsv"})
        self.assertIn("nodes.tsv", names)
        self.assertEqual(
            sorted(set(os.listdir(self.out)) - {"analysis.json", "selection_go.tsv"}),
            names,
        )
        for name in names:
            with (
                open(os.path.join(committed, name), "rb") as a,
                open(os.path.join(self.out, name), "rb") as b,
            ):
                self.assertEqual(a.read(), b.read(), name)

    def test_full_unit_accounting(self) -> None:
        s = self.summary
        self.assertEqual(s["nodes"]["total"], 10645)
        self.assertEqual(s["binding_exceptions"], [])
        self.assertEqual(
            s["coverage"],
            {
                "uncovered_non_space_characters": [],
                "section_mismatches": [],
                "word_boundary_violations": [],
            },
        )
        self.assertEqual(
            (
                s["text"]["first_section"],
                s["text"]["last_section"],
                s["text"]["sections"],
            ),
            ("1.1.1", "1.10.19", 205),
        )
        self.assertNotIn("unresolved", s["nodes"]["by_vocabulary"])
        self.assertEqual(
            s["text_revisions"]["bd7b1c7a36"],
            {"glaux_tokens_unmatched": 0, "source_tokens_unmatched": 0},
        )
        self.assertEqual(s["marchant_point_checks"], {"found": 12})
        self.assertEqual(
            s["sentences"]["native_relation"][
                "native_segments_spanning_complete_sentences"
            ],
            [],
        )

    @unittest.skipUnless(shutil.which("go"), "go not on PATH")
    def test_go_selection_parity(self) -> None:
        got = os.path.join(self.out, "selection_go.tsv")
        env = dict(
            os.environ,
            GOTMPDIR=os.environ.get(
                "GOTMPDIR", os.path.join(os.path.expanduser("~"), ".cache", "gotmp")
            ),
        )
        os.makedirs(env["GOTMPDIR"], exist_ok=True)
        subprocess.run(
            [
                "go",
                "run",
                "./doc/research/ancient-greek-glaux-anabasis1-import-proof/_selectionprobe",
                os.path.join(self.out, "analysis.json"),
                got,
            ],
            cwd=REPO,
            env=env,
            check=True,
            capture_output=True,
        )
        with (
            open(got, "rb") as a,
            open(os.path.join(self.out, "selection_model.tsv"), "rb") as b,
        ):
            self.assertEqual(a.read(), b.read())
        with (
            open(got, "rb") as a,
            open(os.path.join(ROOT, "manifest", "selection_go.tsv"), "rb") as b,
        ):
            self.assertEqual(a.read(), b.read())


if __name__ == "__main__":
    unittest.main()
