#!/usr/bin/env python3
"""Throwaway GLAUx Anabasis Book 1 import proof (#2050).

Research code, not a production adapter. It reads pinned inputs from an
input directory (see fetch_inputs.sh), never from the network, and writes
deterministic manifests. Stdlib only; run with `python3 -I`.

Stages, each separately versioned:

1. Text extraction (EXTRACT_VERSION): Perseus TEI Book 1 -> exact document text
   with per-character section and editorial-markup provenance.
2. Producer decoding and binding (BIND_VERSION): every GLAUx node of Book 1 is
   classified and bound to a half-open code-point span of that text, or to
   no span with an explicit reason.
3. Canonical mapping (MAP_VERSION): AGDT-style postag -> UPOS via a crosswalk
   observed in the published UD Ancient Greek-Perseus conversion; citation
   headword and Mouseion `ancient-greek` v1 canonical key (#2033).
4. Complete-sentence segmentation (SEGMENT_VERSION) over the exact text,
   separate from native GLAUx segments.
5. One-Book 3/15 selection model (mirrors internal/selection; parity is
   checked against the real Go code by selectionprobe).
6. Deterministic representative-sentence/card-input eligibility (#2034).
"""

from __future__ import annotations

import argparse
import bisect
import collections
import csv
import difflib
import glob
import io
import json
import os
import re
import sys
import unicodedata
import xml.etree.ElementTree as ET
from dataclasses import dataclass, field

EXTRACT_VERSION = "perseus-tei-book-extract/1"
BIND_VERSION = "glaux-perseus-bind/1"
MAP_VERSION = "glaux-agdt-upos-crosswalk/1"
PROFILE_VERSION = "ancient-greek/1"
SEGMENT_VERSION = "oct-grc-terminal/1"
CARD_INPUT_VERSION = "grc-card-input-eligibility/1"

TEI = "{http://www.tei-c.org/ns/1.0}"
BOOK = "1"
SOURCE_DOCUMENT_ID = "perseus-grc2@6fc2f2a1:tlg0032.tlg006:book1"
ELISION_GLAUX = "’"  # RIGHT SINGLE QUOTATION MARK, as GLAUx writes elision
ELISION_SOURCE = "ʼ"  # MODIFIER LETTER APOSTROPHE, as Perseus writes elision
TERMINALS = {".", ";", ";", "!", "?"}
CLOSERS = {")", "]", "’", "”", "»"}
MAX_UNITS = 50
MAX_CODE_POINTS = 400
ALLOWED_POS = {"NOUN", "VERB", "ADJ", "ADV"}  # selection.DefaultConfig
RECURRING_MIN = 3  # selection.DefaultRecurringMinOccurrences
ACROSS_BOOKS_MIN = 15  # selection.DefaultAcrossBooksMinOccurrences


def nfc(value: str) -> str:
    return unicodedata.normalize("NFC", value)


def canonical_key(lemma: str) -> str:
    """Mouseion `ancient-greek` v1 profile (#2033): NFC, lowercase, final sigma, NFC."""
    return nfc(nfc(lemma).lower().replace("ς", "σ"))


def is_lemma(value: str) -> bool:
    """Mirror of internal/lexical.IsLemma: at least one Unicode letter."""
    return any(unicodedata.category(ch).startswith("L") for ch in value)


# ---------------------------------------------------------------------------
# Stage 1: text extraction
# ---------------------------------------------------------------------------


@dataclass
class Extracted:
    text: str
    section: list[str]  # per code point: section ref ("1.1.1")
    markup: list[str]  # per code point: "", "del", "add", "sic"
    sections: list[tuple[str, int, int]]  # (ref, start, end)
    regions: list[tuple[str, int, int, str]]  # (kind, start, end, section)
    paragraphs: list[int]  # offsets of <milestone unit="para"/>
    heading: str


PARA = "\x00"  # sentinel for <milestone unit="para"/>; never emitted


def _collapse(chunks: list[tuple[str, str]]) -> tuple[list[tuple[str, str]], list[int]]:
    """Collapse XML whitespace runs to one space and trim the ends, keeping
    each character's markup label. TEI whitespace is not significant.
    Returns the characters and the offsets of paragraph milestones."""
    out: list[tuple[str, str]] = []
    paras: list[int] = []
    for ch, label in chunks:
        if ch == PARA:
            paras.append(len(out))
            continue
        if ch.isspace():
            if out and out[-1][0] != " ":
                out.append((" ", ""))
            continue
        out.append((ch, label))
    while out and out[0][0] == " ":
        out.pop(0)
        paras = [max(0, p - 1) for p in paras]
    while out and out[-1][0] == " ":
        out.pop()
    return out, paras


def extract_book(tei_path: str, book: str = BOOK) -> Extracted:
    root = ET.parse(tei_path).getroot()
    books = [
        d
        for d in root.iter(TEI + "div")
        if d.get("subtype") == "book" and d.get("n") == book
    ]
    if len(books) != 1:
        raise ValueError(f"expected one book {book}, found {len(books)}")
    div = books[0]
    heading = ""
    text: list[str] = []
    section: list[str] = []
    markup: list[str] = []
    sections: list[tuple[str, int, int]] = []
    regions: list[tuple[str, int, int, str]] = []
    paragraphs: list[int] = []
    for chapter in div:
        if chapter.tag == TEI + "head":
            heading = " ".join("".join(chapter.itertext()).split())
            continue
        if chapter.get("subtype") != "chapter":
            raise ValueError(f"unexpected book child {chapter.tag}")
        for sec in chapter:
            if sec.get("subtype") != "section":
                raise ValueError(f"unexpected chapter child {sec.tag}")
            ref = f"{book}.{chapter.get('n')}.{sec.get('n')}"
            chunks: list[tuple[str, str]] = []

            def walk(elem: ET.Element, label: str) -> None:
                tag = elem.tag.replace(TEI, "")
                if tag in ("del", "add", "sic"):
                    if label:
                        raise ValueError(f"nested editorial markup in {ref}")
                    label = tag
                elif tag == "milestone":
                    if elem.get("unit") == "para":
                        chunks.append((PARA, ""))
                elif tag not in ("div", "p"):
                    raise ValueError(f"unsupported element {tag} in {ref}")
                if elem.text:
                    chunks.extend((ch, label) for ch in elem.text)
                for child in elem:
                    walk(child, label)
                    if child.tail:
                        chunks.extend((ch, label) for ch in child.tail)

            walk(sec, "")
            body, marks = _collapse(chunks)
            if text:
                text.append(" ")
                section.append("")
                markup.append("")
            start = len(text)
            paragraphs.extend(start + m for m in marks)
            for ch, label in body:
                text.append(ch)
                section.append(ref)
                markup.append(label)
            sections.append((ref, start, len(text)))
    # Editorial regions as maximal runs; spaces inside a run belong to it.
    i = 0
    while i < len(text):
        if markup[i]:
            j = i
            while j < len(text) and (
                markup[j] == markup[i]
                or (text[j] == " " and j + 1 < len(text) and markup[j + 1] == markup[i])
            ):
                j += 1
            regions.append((markup[i], i, j, section[i]))
            i = j
        else:
            i += 1
    return Extracted(
        "".join(text), section, markup, sections, regions, paragraphs, heading
    )


# ---------------------------------------------------------------------------
# Stage 2: native nodes, decoding and binding
# ---------------------------------------------------------------------------

NODE_FIELDS = (
    "id",
    "form",
    "lemma",
    "postag",
    "head",
    "relation",
    "artificial",
    "form_original",
    "animacy",
    "sense",
)


@dataclass
class Node:
    order: int
    sentence_id: str
    struct_id: str
    analysis: str
    attrs: dict[str, str]
    div_book: str
    div_chapter: str
    div_section: str
    kind: str = ""  # see classify()
    start: int | None = None
    end: int | None = None
    bind_rule: str = ""
    bind_status: str = ""
    bind_note: str = ""
    upos: str = ""
    upos_candidates: str = ""
    map_rule: str = ""
    map_status: str = ""
    headword: str = ""
    key: str = ""
    vocab: str = ""  # eligible | non_vocab | unprinted | secluded | unresolved
    vocab_reason: str = ""
    complete_sentence: int | None = None
    flags: list[str] = field(default_factory=list)

    @property
    def nid(self) -> str:
        return self.attrs["id"]

    @property
    def form(self) -> str:
        return self.attrs.get("form", "")


def read_nodes(glaux_path: str, book: str = BOOK) -> tuple[list[Node], dict]:
    root = ET.parse(glaux_path).getroot()
    nodes: list[Node] = []
    other_books: collections.Counter = collections.Counter()
    sentences_by_book: collections.Counter = collections.Counter()
    order = 0
    for sent in root:
        books = {w.get("div_book") for w in sent}
        if len(books) != 1:
            raise ValueError(f"sentence {sent.get('id')} spans books {books}")
        (sb,) = books
        sentences_by_book[sb] += 1
        for word in sent:
            if sb != book:
                other_books[sb] += 1
                continue
            unknown = (
                set(word.attrib)
                - set(NODE_FIELDS)
                - {"div_book", "div_chapter", "div_section"}
            )
            if unknown:
                raise ValueError(
                    f"unknown node attributes {unknown} on {word.get('id')}"
                )
            nodes.append(
                Node(
                    order=order,
                    sentence_id=sent.get("id"),
                    struct_id=sent.get("struct_id"),
                    analysis=sent.get("analysis"),
                    attrs={
                        k: word.get(k) for k in NODE_FIELDS if word.get(k) is not None
                    },
                    div_book=word.get("div_book"),
                    div_chapter=word.get("div_chapter"),
                    div_section=word.get("div_section"),
                )
            )
            order += 1
    ids = [n.nid for n in nodes]
    if len(ids) != len(set(ids)):
        raise ValueError("duplicate native word ids")
    info = {
        "root_attributes": dict(
            sorted((k.split("}")[-1], v) for k, v in root.attrib.items())
        ),
        "sentences_by_book": dict(sorted(sentences_by_book.items())),
        "nodes_by_other_book": dict(sorted(other_books.items())),
    }
    return nodes, info


def classify(node: Node) -> str:
    """Producer-decoding classes. Conventions are inferred from the file and
    each one is validated against the exact text during binding."""
    form = node.form
    original = node.attrs.get("form_original")
    if node.attrs.get("artificial"):
        return "unprinted"
    if original is not None and original.startswith("{") and original.endswith("}"):
        if form:
            raise ValueError(f"secluded node {node.nid} has a form")
        return "secluded"
    if not form:
        raise ValueError(f"empty form without convention on {node.nid}")
    if original is not None and original.startswith("<") and original.endswith(">"):
        if original[1:-1] != form:
            raise ValueError(f"supplement form mismatch on {node.nid}")
        return "supplied"
    if form == "†":
        return "crux"
    if form.startswith("-") and len(form) > 1:
        return "suffix_component"
    if form.endswith("-") and len(form) > 1:
        return "crasis_fragment"
    return "word"


def decode_surface(form: str) -> str:
    """GLAUx surface -> expected source spelling: NFC (GLAUx is NFD) and the
    producer's elision mark U+2019 -> the source's U+02BC."""
    return nfc(form).replace(ELISION_GLAUX, ELISION_SOURCE)


def bind(nodes: list[Node], doc: Extracted) -> list[dict]:
    """Sequentially bind nodes to the exact text. Every node is bound,
    anchored, or rejected; a rejection is recorded, never guessed."""
    text = doc.text
    sic_starts = {s for kind, s, _, _ in doc.regions if kind == "sic"}
    sic_ends = {e for kind, _, e, _ in doc.regions if kind == "sic"}
    cursor = 0
    prev_end: int | None = None
    exceptions: list[dict] = []
    for idx, node in enumerate(nodes):
        node.kind = classify(node)
        if node.kind == "unprinted":
            node.bind_status, node.bind_rule = "no_span", "B-UNPRINTED"
            node.bind_note = f"artificial={node.attrs['artificial']}"
            continue
        if node.kind == "crasis_fragment":
            continue  # bound after its host (shared span)
        if node.kind == "suffix_component":
            expected = decode_surface(node.form[1:])
            start = cursor
            if prev_end is None or start != prev_end:
                raise ValueError(
                    f"suffix component {node.nid} not adjacent to its host"
                )
            rule = "B-SUFFIX-COMPONENT"
        elif node.kind == "crux":
            while cursor < len(text) and text[cursor] == " ":
                cursor += 1
            # A crux marks a <sic> boundary: the next character opens a sic
            # region, or the previous bound span closed one.
            opens = cursor in sic_starts
            closes = prev_end in sic_ends
            if opens == closes:
                exceptions.append(
                    {
                        "node": node.nid,
                        "kind": node.kind,
                        "offset": cursor,
                        "problem": "crux not at a single sic boundary",
                    }
                )
                node.bind_status, node.bind_rule = "failed", "B-CRUX-ANCHOR"
                continue
            node.start = node.end = cursor if opens else prev_end
            node.bind_status, node.bind_rule = "anchor", "B-CRUX-ANCHOR"
            node.bind_note = "opens sic" if opens else "closes sic"
            continue
        else:
            while cursor < len(text) and text[cursor] == " ":
                cursor += 1
            start = cursor
            surface = (
                node.attrs.get("form_original", "")[1:-1]
                if node.kind == "secluded"
                else node.form
            )
            expected = decode_surface(surface)
            rule = {
                "word": "B-EXACT",
                "secluded": "B-SECLUDED",
                "supplied": "B-SUPPLIED",
            }[node.kind]
        end = start + len(expected)
        actual = text[start:end]
        if actual != expected:
            exceptions.append(
                {
                    "node": node.nid,
                    "kind": node.kind,
                    "offset": start,
                    "problem": f"expected {expected!r} found {actual!r}",
                }
            )
            node.bind_status, node.bind_rule = "failed", rule
            raise ValueError(
                f"binding failed at node {node.nid}: expected {expected!r} at {start}, found {actual!r}"
            )
        labels = set(doc.markup[start:end])
        want = {
            "word": {""},
            "secluded": {"del"},
            "supplied": {"add"},
            "suffix_component": None,
        }[node.kind]
        if node.kind == "word" and labels == {"sic"}:
            node.flags.append("in_crux")
        elif want is not None and labels != want:
            raise ValueError(f"node {node.nid} {node.kind} spans markup {labels}")
        if node.kind == "suffix_component" and labels - {"", "sic"}:
            raise ValueError(f"component {node.nid} spans markup {labels}")
        node.start, node.end = start, end
        node.bind_status, node.bind_rule = "bound", rule
        if (
            nfc(surface if node.kind != "suffix_component" else node.form[1:])
            != expected
        ):
            node.bind_note = "elision U+2019->U+02BC"
        cursor = prev_end = end
        if node.kind == "word" or node.kind == "supplied":
            # Crasis fragments immediately before this host share its span.
            back = idx - 1
            while (
                back >= 0
                and nodes[back].kind == "crasis_fragment"
                and nodes[back].start is None
            ):
                frag = nodes[back]
                frag_base = _base_letters(frag.form[:-1])
                if not _base_letters(actual).startswith(frag_base[:1]):
                    raise ValueError(
                        f"crasis fragment {frag.nid} does not share its host's initial"
                    )
                frag.start, frag.end = start, end
                frag.bind_status, frag.bind_rule = "shared", "B-CRASIS-SHARED"
                frag.bind_note = f"shares span with host {node.nid}"
                node.flags.append("crasis_host")
                back -= 1
    for node in nodes:
        if node.kind == "crasis_fragment" and node.start is None:
            raise ValueError(f"crasis fragment {node.nid} has no host")
    return exceptions


def _base_letters(value: str) -> str:
    return "".join(
        ch
        for ch in unicodedata.normalize("NFD", value.lower())
        if not unicodedata.combining(ch)
    )


def coverage(nodes: list[Node], doc: Extracted) -> dict:
    covered = [False] * len(doc.text)
    for node in nodes:
        if node.start is not None and node.end is not None:
            for i in range(node.start, node.end):
                covered[i] = True
    uncovered = [
        (i, doc.text[i])
        for i in range(len(doc.text))
        if not covered[i] and doc.text[i] != " "
    ]
    section_mismatch = [
        n.nid
        for n in nodes
        if n.start is not None
        and n.end > n.start
        and doc.section[n.start] != n.div_section
    ]
    # A bound span may abut a letter only where a suffix component evidences
    # the join; otherwise a prefix match could hide a misalignment.
    joined = {n.start for n in nodes if n.kind == "suffix_component"}
    boundary = []
    for n in nodes:
        if n.start is None or n.end == n.start or n.kind == "crasis_fragment":
            continue
        text = doc.text
        if (
            n.start > 0
            and is_lemma(text[n.start - 1])
            and is_lemma(text[n.start])
            and n.start not in joined
        ):
            boundary.append(n.nid)
        if (
            n.end < len(text)
            and is_lemma(text[n.end])
            and is_lemma(text[n.end - 1])
            and n.end not in joined
        ):
            boundary.append(n.nid)
    return {
        "uncovered_non_space": uncovered,
        "section_mismatches": section_mismatch,
        "word_boundary_violations": boundary,
    }


# ---------------------------------------------------------------------------
# Stage 3: canonical mapping
# ---------------------------------------------------------------------------


def load_crosswalk(ud_dir: str) -> tuple[dict, dict]:
    """(AGDT letter, NFC lemma) -> Counter(UPOS) and letter -> Counter(UPOS),
    from UD Ancient Greek-Perseus, whose XPOS keeps the native AGDT postag."""
    by_lemma: dict = collections.defaultdict(collections.Counter)
    by_letter: dict = collections.defaultdict(collections.Counter)
    for path in sorted(glob.glob(os.path.join(ud_dir, "grc_perseus-ud-*.conllu"))):
        with open(path, encoding="utf-8") as fh:
            for line in fh:
                if not line.strip() or line.startswith("#"):
                    continue
                cols = line.rstrip("\n").split("\t")
                if "-" in cols[0] or "." in cols[0] or not cols[4] or cols[4] == "_":
                    continue
                by_lemma[(cols[4][0], nfc(cols[2]))][cols[3]] += 1
                by_letter[cols[4][0]][cols[3]] += 1
    return by_lemma, by_letter


# Letter rules: the single UPOS UD Perseus assigns to every token with that
# AGDT letter. `c` and `v` need lemma or context rules (see map_upos).
LETTER_UPOS = {
    "a": "ADJ",
    "d": "ADV",
    "g": "PART",
    "i": "INTJ",
    "l": "DET",
    "m": "NUM",
    "n": "NOUN",
    "p": "PRON",
    "r": "ADP",
    "u": "PUNCT",
}


def map_upos(
    letter: str, lemma: str, relation: str, by_lemma: dict, by_letter: dict
) -> tuple[str, str, str, str]:
    """Return (upos, candidates, rule, status). status: resolved | ambiguous."""
    if letter in LETTER_UPOS:
        observed = by_letter.get(letter, collections.Counter())
        if set(observed) != {LETTER_UPOS[letter]}:
            raise ValueError(f"crosswalk letter {letter} not unanimous: {observed}")
        return (
            LETTER_UPOS[letter],
            LETTER_UPOS[letter],
            f"M-LETTER-{letter}",
            "resolved",
        )
    if letter == "v":
        if relation == "AuxV":
            return "AUX", "AUX", "M-V-AUXV", "resolved"
        if lemma == "εἰμί":
            return "AUX", "AUX", "M-V-EIMI", "resolved"
        return "VERB", "VERB", "M-V-DEFAULT", "resolved"
    if letter == "c":
        observed = by_lemma.get(("c", lemma))
        if observed and len(observed) == 1:
            (upos,) = observed
            return upos, upos, "M-C-LEMMA", "resolved"
        candidates = "|".join(sorted(observed)) if observed else "CCONJ|SCONJ"
        return (
            "",
            candidates,
            "M-C-UNATTESTED" if not observed else "M-C-SPLIT",
            "ambiguous",
        )
    raise ValueError(f"no mapping rule for postag letter {letter!r}")


def apply_mapping(nodes: list[Node], by_lemma: dict, by_letter: dict) -> None:
    for node in nodes:
        if node.kind == "unprinted":
            node.vocab, node.vocab_reason = (
                "unprinted",
                "artificial node; not a printed occurrence",
            )
            continue
        if node.kind == "secluded":
            node.vocab, node.vocab_reason = (
                "secluded",
                "editorially deleted text; producer supplies no annotation",
            )
            continue
        if node.kind == "crux":
            node.vocab, node.vocab_reason = (
                "non_vocab",
                "crux sign (punctuation-class editorial mark)",
            )
            node.upos, node.upos_candidates, node.map_rule, node.map_status = (
                "PUNCT",
                "PUNCT",
                "M-LETTER-u",
                "resolved",
            )
            continue
        lemma_raw = node.attrs.get("lemma")
        postag = node.attrs.get("postag", "")
        if not lemma_raw or not postag:
            node.vocab, node.vocab_reason = (
                "unresolved",
                "printed node without lemma or postag",
            )
            continue
        lemma = nfc(lemma_raw)
        upos, candidates, rule, status = map_upos(
            postag[0], lemma, node.attrs.get("relation", ""), by_lemma, by_letter
        )
        node.upos, node.upos_candidates, node.map_rule, node.map_status = (
            upos,
            candidates,
            rule,
            status,
        )
        node.headword = lemma if is_lemma(lemma) else ""
        node.key = canonical_key(lemma) if node.headword else ""
        if status == "ambiguous":
            if all(c not in ALLOWED_POS for c in candidates.split("|")):
                node.vocab, node.vocab_reason = (
                    "non_vocab",
                    f"every UPOS candidate ({candidates}) is outside AllowedPOS",
                )
            else:
                node.vocab, node.vocab_reason = (
                    "unresolved",
                    f"ambiguous UPOS {candidates}",
                )
            continue
        if upos not in ALLOWED_POS:
            node.vocab, node.vocab_reason = (
                "non_vocab",
                f"UPOS {upos} outside AllowedPOS",
            )
        elif not node.headword:
            node.vocab, node.vocab_reason = "unresolved", "lemma has no letter"
        else:
            node.vocab, node.vocab_reason = "eligible", ""
        if (
            node.headword
            and node.headword[:1] != node.headword[:1].lower()
            and upos == "NOUN"
        ):
            node.flags.append("capitalized_noun_lemma")


# ---------------------------------------------------------------------------
# Stage 4: complete sentences
# ---------------------------------------------------------------------------


@dataclass
class Sentence:
    index: int
    start: int
    end: int
    text: str
    units: int
    code_points: int
    sections: str
    native_segments: list[str]
    markup: list[str]
    boundary: str
    lexical_nodes: int = 0
    eligible_nodes: int = 0


def segment(doc: Extracted) -> list[tuple[int, int, str]]:
    """Split exact text at terminals (ano teleia is medial). A terminal plus
    any immediately following closers ends a sentence. The end of the unit
    is a boundary only if the text ends with a terminal."""
    text = doc.text
    spans: list[tuple[int, int, str]] = []
    start = 0
    i = 0
    while i < len(text):
        if text[i] in TERMINALS:
            j = i + 1
            while j < len(text) and text[j] in CLOSERS:
                j += 1
            if j == len(text) or text[j] == " ":
                spans.append((start, j, f"terminal {text[i]!r}"))
                start = j + 1 if j < len(text) else j
                i = j
                continue
        i += 1
    if start < len(text):
        spans.append((start, len(text), "unit end without terminal"))
    return spans


def build_sentences(nodes: list[Node], doc: Extracted) -> list[Sentence]:
    out: list[Sentence] = []
    for index, (start, end, boundary) in enumerate(segment(doc)):
        text = doc.text[start:end].strip()
        markup = sorted({m for m in doc.markup[start:end] if m})
        refs = [doc.section[i] for i in range(start, end) if doc.section[i]]
        sections = refs[0] if refs[0] == refs[-1] else f"{refs[0]}-{refs[-1]}"
        out.append(
            Sentence(
                index,
                start,
                end,
                text,
                len(text.split()),
                len(text),
                sections,
                [],
                markup,
                boundary,
            )
        )
    # Attach nodes. Unprinted nodes join the sentence of their native segment's
    # preceding printed node; they never define boundaries.
    starts = [s.start for s in out]
    last_sentence: dict[str, int] = {}
    for node in nodes:
        if node.start is not None:
            si = bisect.bisect_right(starts, node.start) - 1
            if not (out[si].start <= node.start <= out[si].end):
                raise ValueError(f"node {node.nid} outside sentences")
            node.complete_sentence = si
            last_sentence[node.sentence_id] = si
    for node in nodes:
        if node.start is None:
            node.complete_sentence = last_sentence.get(node.sentence_id)
    for node in nodes:
        s = out[node.complete_sentence]
        if node.sentence_id not in s.native_segments:
            s.native_segments.append(node.sentence_id)
        if node.kind not in ("unprinted", "crux") and node.upos != "PUNCT":
            s.lexical_nodes += 1
        if node.vocab == "eligible":
            s.eligible_nodes += 1
    return out


def native_vs_complete(
    nodes: list[Node], sentences: list[Sentence], doc: Extracted
) -> dict:
    """Relate native GLAUx segments to complete sentences."""
    seg_sentences: dict[str, set] = collections.defaultdict(set)
    seg_last: dict[str, Node] = {}
    for node in nodes:
        if node.start is not None:
            seg_sentences[node.sentence_id].add(node.complete_sentence)
            seg_last[node.sentence_id] = node
    split_segments = sorted(
        (k for k, v in seg_sentences.items() if len(v) > 1), key=int
    )
    end_char = collections.Counter()
    for seg, node in seg_last.items():
        ch = (
            doc.text[node.end - 1] if node.end and node.end > node.start else "(anchor)"
        )
        end_char[ch if ch in TERMINALS | {"·", ",", ")"} else "word"] += 1
    per_sentence = collections.Counter(len(s.native_segments) for s in sentences)
    return {
        "native_segments": len(seg_sentences),
        "native_segments_spanning_complete_sentences": split_segments,
        "native_segment_final_printed_character": dict(sorted(end_char.items())),
        "native_segments_per_complete_sentence": dict(sorted(per_sentence.items())),
    }


# ---------------------------------------------------------------------------
# Stage 5: selection model (one analyzed Book, empty Known/Reserved)
# ---------------------------------------------------------------------------


def project(nodes: list[Node]) -> dict[tuple[str, str], list[Node]]:
    """Mirror of selection.Project with DefaultConfig: count eligible
    occurrences per (canonical key, UPOS)."""
    groups: dict[tuple[str, str], list[Node]] = collections.defaultdict(list)
    for node in sorted(
        (n for n in nodes if n.vocab == "eligible"), key=lambda n: (n.start, n.order)
    ):
        groups[(node.key, node.upos)].append(node)
    return groups


def select(groups: dict[tuple[str, str], list[Node]]) -> dict[tuple[str, str], str]:
    """Mirror of eligibleCurrentReadingCandidates for a library containing
    only this Book: the across-Books total equals the in-Book count."""
    out = {}
    for ident, occ in groups.items():
        in_book = len(occ)
        across = in_book if in_book < RECURRING_MIN else 0
        if in_book >= RECURRING_MIN:
            out[ident] = "selected: in-Book count >= 3"
        elif in_book >= 1 and across >= ACROSS_BOOKS_MIN:
            out[ident] = "selected: across-Books count >= 15"
        else:
            out[ident] = (
                f"not selected: in-Book {in_book} < 3 and across-Books {across} < 15"
            )
    return out


# ---------------------------------------------------------------------------
# Stage 6: deterministic representative-sentence eligibility
# ---------------------------------------------------------------------------


def occurrence_rejection(node: Node, sentence: Sentence) -> str:
    if sentence.boundary.startswith("unit end"):
        return "sentence boundary incomplete"
    if sentence.units > MAX_UNITS:
        return "sentence over 50 whitespace units"
    if sentence.code_points > MAX_CODE_POINTS:
        return "sentence over 400 code points"
    if node.start is None or node.end is None or node.end <= node.start:
        return "target has no verified span"
    if not node.headword:
        return "no usable citation headword"
    return ""


def card_inputs(groups, selection, sentences, text: str) -> list[dict]:
    rows = []
    for ident in sorted(
        i for i, why in selection.items() if why.startswith("selected")
    ):
        occ = groups[ident]
        reasons: collections.Counter = collections.Counter()
        chosen = None
        for node in occ:
            why = occurrence_rejection(node, sentences[node.complete_sentence])
            if why:
                reasons[why] += 1
                continue
            chosen = node
            break
        row = {
            "key": ident[0],
            "upos": ident[1],
            "headword": occ[0].headword,
            "occurrences": len(occ),
            "earlier_rejections": "; ".join(
                f"{k} x{v}" for k, v in sorted(reasons.items())
            ),
        }
        if chosen is None:
            row.update(
                {
                    "status": "omitted",
                    "reason": "no qualifying occurrence: " + ", ".join(sorted(reasons)),
                }
            )
        else:
            s = sentences[chosen.complete_sentence]
            flags = list(chosen.flags)
            if s.markup:
                flags.append("sentence_markup=" + "+".join(s.markup))
            same = [n for n in occ if n.complete_sentence == chosen.complete_sentence]
            if len(same) > 1:
                flags.append(f"target_identity_repeats_in_sentence={len(same)}")
            row.update(
                {
                    "status": "eligible",
                    "reason": "",
                    "node": chosen.nid,
                    "sentence": s.index,
                    "target_start": chosen.start,
                    "target_end": chosen.end,
                    "target_surface": text[chosen.start : chosen.end],
                    "units": s.units,
                    "code_points": s.code_points,
                    "flags": ",".join(flags),
                }
            )
        rows.append(row)
    return rows


# ---------------------------------------------------------------------------
# Evidence: text revision and printed-edition point checks
# ---------------------------------------------------------------------------

REVISIONS = [
    ("6ca53fa904", "2016-11-15"),
    ("c73fc5af20", "2016-11-15"),
    ("4cbaae5064", "2016-12-06"),
    ("0bb4112b41", "2017-08-16"),
    ("d7ee42566e", "2018-03-20"),
    ("bd7b1c7a36", "2019-09-09"),
    ("84a6f3e545", "2022-12-09"),
    ("257c4b7b68", "2024-09-28"),
    ("0c75c60094", "2026-03-29"),
    ("299a8af276", "2026-05-04"),
    ("92e51fcc3f", "2026-05-20"),
    ("31bce58d6d", "2026-07-16"),
    ("cb252e6d7a", "2026-07-17"),
    ("dd96fe31bc", "2026-08-26"),
    ("70427323b6", "2026-08-26"),
]
_TOKEN = re.compile(r"[^\s,.·;:()]+|[,.·;:()]")


def glaux_token_stream(nodes: list[Node]) -> list[str]:
    """The printed token sequence GLAUx encodes, decoded to source spelling:
    components rejoined to their hosts, crasis fragments and cruces dropped."""
    out: list[str] = []
    for node in nodes:
        if node.kind in ("unprinted", "crux", "crasis_fragment"):
            continue
        if node.kind == "suffix_component":
            out[-1] += decode_surface(node.form[1:])
            continue
        surface = (
            node.attrs["form_original"][1:-1] if node.kind == "secluded" else node.form
        )
        out.append(decode_surface(surface))
    return out


def lenient_book_text(tei_path: str) -> str:
    """Book 1 text of any revision: all text content except the heading, so
    <choice> keeps both sic and corr readings visible to the comparison."""
    root = ET.parse(tei_path).getroot()
    (div,) = [
        d
        for d in root.iter(TEI + "div")
        if d.get("subtype") == "book" and d.get("n") == BOOK
    ]
    parts = []
    for child in div:
        if child.tag != TEI + "head":
            parts.append(" ".join(child.itertext()))
    return nfc(" ".join(parts))


def revision_comparison(nodes: list[Node], perseus_dir: str) -> list[list]:
    glaux = glaux_token_stream(nodes)
    rows = []
    for rev, date in REVISIONS:
        path = os.path.join(perseus_dir, f"grc2-{rev}.xml")
        source = _TOKEN.findall(lenient_book_text(path))
        ops = [
            op
            for op in difflib.SequenceMatcher(
                None, glaux, source, autojunk=False
            ).get_opcodes()
            if op[0] != "equal"
        ]
        first = "; ".join(
            f"{tag} glaux={glaux[i1:i2]} source={source[j1:j2]}"
            for tag, i1, i2, j1, j2 in ops[:3]
        )
        rows.append(
            [
                rev,
                date,
                len(glaux),
                len(source),
                len(ops),
                sum(i2 - i1 for _, i1, i2, _, _ in ops),
                sum(j2 - j1 for _, _, _, j1, j2 in ops),
                first,
            ]
        )
    return rows


def point_checks(fixture: str, ocr: str) -> list[list]:
    with open(ocr, encoding="utf-8") as fh:
        lines = fh.read().split("\n")
    rows = []
    with open(fixture, encoding="utf-8") as fh:
        for rec in csv.DictReader(fh, delimiter="\t"):
            line = lines[int(rec["ocr_line"]) - 1]
            rows.append(
                [
                    rec["check"],
                    rec["section"],
                    rec["ocr_line"],
                    rec["ocr_expected"],
                    "found" if rec["ocr_expected"] in line else "NOT FOUND",
                    line.strip(),
                ]
            )
    return rows


# ---------------------------------------------------------------------------
# Output
# ---------------------------------------------------------------------------


def write_tsv(path: str, header: list[str], rows: list[list]) -> None:
    buf = io.StringIO()
    w = csv.writer(buf, delimiter="\t", lineterminator="\n", quoting=csv.QUOTE_MINIMAL)
    w.writerow(header)
    for row in rows:
        w.writerow(["" if v is None else v for v in row])
    with open(path, "w", encoding="utf-8", newline="") as fh:
        fh.write(buf.getvalue())


def write_json(path: str, data) -> None:
    with open(path, "w", encoding="utf-8") as fh:
        json.dump(data, fh, ensure_ascii=False, indent=1, sort_keys=True)
        fh.write("\n")


def run(inputs: str, out: str) -> dict:
    glaux = os.path.join(inputs, "glaux", "xml", "0032-006.xml")
    tei = os.path.join(inputs, "perseus", "grc2-bd7b1c7a36.xml")
    ud = os.path.join(inputs, "udperseus")
    os.makedirs(out, exist_ok=True)

    doc = extract_book(tei)
    nodes, native_info = read_nodes(glaux)
    exceptions = bind(nodes, doc)
    cov = coverage(nodes, doc)
    by_lemma, by_letter = load_crosswalk(ud)
    apply_mapping(nodes, by_lemma, by_letter)
    sentences = build_sentences(nodes, doc)
    seg_info = native_vs_complete(nodes, sentences, doc)
    groups = project(nodes)
    selection = select(groups)
    cards = card_inputs(groups, selection, sentences, doc.text)

    with open(os.path.join(out, "book1.txt"), "w", encoding="utf-8") as fh:
        fh.write(doc.text + "\n")
    write_tsv(
        os.path.join(out, "sections.tsv"),
        ["section", "start", "end", "paragraph_start"],
        [[r, s, e, "yes" if s in doc.paragraphs else ""] for r, s, e in doc.sections],
    )
    write_tsv(
        os.path.join(out, "editorial_regions.tsv"),
        ["kind", "start", "end", "section", "text"],
        [[k, s, e, sec, doc.text[s:e]] for k, s, e, sec in doc.regions],
    )
    write_tsv(
        os.path.join(out, "nodes.tsv"),
        [
            "order",
            "native_sentence",
            "struct_id",
            "analysis",
            "node_id",
            "div_section",
            "kind",
            "form_nfc",
            "form_original",
            "lemma_nfc",
            "postag",
            "head",
            "relation",
            "artificial",
            "animacy",
            "sense",
            "bind_status",
            "bind_rule",
            "start",
            "end",
            "source_text",
            "bind_note",
            "upos",
            "upos_candidates",
            "map_rule",
            "map_status",
            "headword",
            "canonical_key",
            "vocabulary",
            "vocabulary_reason",
            "complete_sentence",
            "flags",
        ],
        [
            [
                n.order,
                n.sentence_id,
                n.struct_id,
                n.analysis,
                n.nid,
                n.div_section,
                n.kind,
                nfc(n.form),
                n.attrs.get("form_original"),
                nfc(n.attrs.get("lemma", "")),
                n.attrs.get("postag"),
                n.attrs.get("head"),
                n.attrs.get("relation"),
                n.attrs.get("artificial"),
                n.attrs.get("animacy"),
                n.attrs.get("sense"),
                n.bind_status,
                n.bind_rule,
                n.start,
                n.end,
                doc.text[n.start : n.end] if n.start is not None else "",
                n.bind_note,
                n.upos,
                n.upos_candidates,
                n.map_rule,
                n.map_status,
                n.headword,
                n.key,
                n.vocab,
                n.vocab_reason,
                n.complete_sentence,
                ",".join(n.flags),
            ]
            for n in nodes
        ],
    )
    write_tsv(
        os.path.join(out, "sentences.tsv"),
        [
            "sentence",
            "start",
            "end",
            "sections",
            "boundary",
            "units",
            "code_points",
            "over_cap",
            "native_segments",
            "editorial_markup",
            "lexical_nodes",
            "eligible_nodes",
            "text",
        ],
        [
            [
                s.index,
                s.start,
                s.end,
                s.sections,
                s.boundary,
                s.units,
                s.code_points,
                "yes" if s.units > MAX_UNITS or s.code_points > MAX_CODE_POINTS else "",
                " ".join(s.native_segments),
                "+".join(s.markup),
                s.lexical_nodes,
                s.eligible_nodes,
                s.text,
            ]
            for s in sentences
        ],
    )
    ident_rows = []
    for ident in sorted(groups, key=lambda i: (-len(groups[i]), i)):
        occ = groups[ident]
        ident_rows.append(
            [
                ident[0],
                ident[1],
                occ[0].headword,
                len(occ),
                "absent_in_source",
                selection[ident],
                " ".join(sorted({doc.text[n.start : n.end] for n in occ})),
                ",".join(
                    sorted(
                        {
                            f
                            for n in occ
                            for f in n.flags
                            if f == "capitalized_noun_lemma"
                        }
                    )
                ),
            ]
        )
    write_tsv(
        os.path.join(out, "identities.tsv"),
        [
            "canonical_key",
            "upos",
            "headword",
            "occurrences",
            "discriminator",
            "selection",
            "observed_forms",
            "flags",
        ],
        ident_rows,
    )
    card_header = [
        "key",
        "upos",
        "headword",
        "occurrences",
        "status",
        "reason",
        "earlier_rejections",
        "node",
        "sentence",
        "target_start",
        "target_end",
        "target_surface",
        "units",
        "code_points",
        "flags",
    ]
    write_tsv(
        os.path.join(out, "card_inputs.tsv"),
        card_header,
        [[r.get(h, "") for h in card_header] for r in cards],
    )
    crosswalk_rows = []
    for (letter, lemma), counts in sorted(by_lemma.items()):
        if letter in ("c", "v") and any(
            n.attrs.get("postag", "")[:1] == letter
            and nfc(n.attrs.get("lemma", "")) == lemma
            for n in nodes
        ):
            crosswalk_rows.append(
                [letter, lemma, " ".join(f"{k}:{v}" for k, v in counts.most_common())]
            )
    write_tsv(
        os.path.join(out, "crosswalk_lemma_c_v.tsv"),
        ["agdt_letter", "lemma", "ud_perseus_upos_counts"],
        crosswalk_rows,
    )
    write_tsv(
        os.path.join(out, "crosswalk_letters.tsv"),
        ["agdt_letter", "ud_perseus_upos_counts"],
        [
            [k, " ".join(f"{u}:{c}" for u, c in v.most_common())]
            for k, v in sorted(by_letter.items())
        ],
    )
    # Analyzer-shaped input for the Go selection probe.
    analysis = {
        "Language": "grc",
        "Sentences": [
            {
                "Text": s.text,
                "Tokens": [
                    {
                        "Surface": doc.text[n.start : n.end],
                        "RawLemma": nfc(n.attrs.get("lemma", "")),
                        "CanonicalLemma": n.key,
                        "UPOS": n.upos,
                        "Dependency": "",
                        "Location": {
                            "SourceDocumentID": SOURCE_DOCUMENT_ID,
                            "Section": n.div_section,
                            "StartOffset": n.start,
                            "EndOffset": n.end,
                        },
                    }
                    for n in sorted(
                        nodes,
                        key=lambda n: (n.start if n.start is not None else -1, n.order),
                    )
                    if n.complete_sentence == s.index
                    and n.start is not None
                    and n.vocab in ("eligible", "non_vocab")
                ],
            }
            for s in sentences
        ],
    }
    with open(os.path.join(out, "analysis.json"), "w", encoding="utf-8") as fh:
        json.dump(
            analysis, fh, ensure_ascii=False, sort_keys=True, separators=(",", ":")
        )
        fh.write("\n")
    write_tsv(
        os.path.join(out, "selection_model.tsv"),
        ["canonical_key", "upos", "occurrences", "selected"],
        [
            [
                k,
                u,
                len(groups[(k, u)]),
                "yes" if selection[(k, u)].startswith("selected") else "no",
            ]
            for k, u in sorted(groups)
        ],
    )

    revisions = revision_comparison(nodes, os.path.join(inputs, "perseus"))
    write_tsv(
        os.path.join(out, "text_revisions.tsv"),
        [
            "perseus_commit",
            "date",
            "glaux_tokens",
            "source_tokens",
            "diff_ops",
            "glaux_tokens_unmatched",
            "source_tokens_unmatched",
            "first_differences",
        ],
        revisions,
    )
    fixture = os.path.join(
        os.path.dirname(os.path.abspath(__file__)),
        "..",
        "fixtures",
        "marchant-point-checks.tsv",
    )
    checks = point_checks(fixture, os.path.join(inputs, "marchant-ia", "djvu.txt"))
    write_tsv(
        os.path.join(out, "marchant_point_checks.tsv"),
        ["check", "section", "ocr_line", "ocr_expected", "result", "ocr_line_text"],
        checks,
    )

    summary = summarize(
        nodes,
        doc,
        cov,
        exceptions,
        native_info,
        seg_info,
        sentences,
        groups,
        selection,
        cards,
    )
    summary["sensitivity"] = sensitivity(nodes, groups)
    summary["text_revisions"] = {
        r[0]: {"glaux_tokens_unmatched": r[5], "source_tokens_unmatched": r[6]}
        for r in revisions
    }
    summary["marchant_point_checks"] = dict(
        sorted(collections.Counter(r[4] for r in checks).items())
    )
    write_json(os.path.join(out, "summary.json"), summary)
    review = review_set(nodes, doc, sentences, groups, cards)
    review_header = [
        "case",
        "identity",
        "node",
        "native_sentence",
        "section",
        "sentence",
        "units",
        "code_points",
        "target_surface",
        "target_code_points_nfd_glaux",
        "status",
        "note",
        "sentence_text",
    ]
    write_tsv(
        os.path.join(out, "review_set.tsv"),
        review_header,
        [[r.get(h, "") for h in review_header] for r in review],
    )
    write_json(os.path.join(out, "ledger.json"), ledger(summary))
    write_json(os.path.join(out, "catalog_example.json"), catalog_example(summary))
    return summary


def sensitivity(nodes: list[Node], groups) -> dict:
    """Measured effects of alternatives that are NOT adopted here."""
    base = {i for i, occ in groups.items() if len(occ) >= RECURRING_MIN}
    propn = {
        i
        for i in base
        if groups[i][0].headword[:1] != groups[i][0].headword[:1].lower()
        and i[1] == "NOUN"
    }
    eimi = [n for n in nodes if n.map_rule == "M-V-EIMI"]
    secluded_lexical = [
        n for n in nodes if n.kind == "secluded" and is_lemma(n.attrs["form_original"])
    ]
    components = [n for n in nodes if n.kind == "suffix_component"]
    hosts = collections.Counter()
    for i, n in enumerate(nodes):
        if n.kind == "suffix_component":
            host = nodes[i - 1]
            hosts[
                f"{nfc(host.attrs.get('lemma', ''))}+{nfc(n.attrs.get('lemma', ''))}"
            ] += 1
    return {
        "capitalized_noun_lemma_as_PROPN": {
            "rule": "not adopted: UD Perseus emits no PROPN; capitalization is a GLAUx lemma convention, not a native tag",
            "selected_identities_removed": len(propn),
            "selected_occurrences_removed": sum(len(groups[i]) for i in propn),
            "selected_identities_remaining": len(base) - len(propn),
        },
        "eimi_as_VERB": {
            "rule": "not adopted: UD Perseus tags 1864 of 1866 εἰμί AUX",
            "occurrences": len(eimi),
            "would_add_selected_identity": len(eimi) >= RECURRING_MIN,
        },
        "secluded_text_as_required_vocabulary": {
            "rule": "if edition-secluded words must carry vocabulary identities, these nodes have no producer annotation and block vocabulary-ready publication",
            "lexical_nodes": len(secluded_lexical),
            "node_ids": [n.nid for n in secluded_lexical],
        },
        "suffix_component_lemma_pairs": dict(sorted(hosts.items())),
        "suffix_components": len(components),
    }


def review_set(nodes, doc, sentences, groups, cards) -> list[dict]:
    """A small deterministic set of attributed source examples; each case is
    chosen by a stated rule, and absent cases are reported as absent."""
    by_id = {n.nid: n for n in nodes}
    rows: list[dict] = []

    def add(case: str, node: Node, note: str, status: str = "") -> None:
        s = sentences[node.complete_sentence]
        rows.append(
            {
                "case": case,
                "identity": f"{node.key}|{node.upos}" if node.key else "",
                "node": node.nid,
                "native_sentence": node.sentence_id,
                "section": node.div_section,
                "sentence": s.index,
                "units": s.units,
                "code_points": s.code_points,
                "target_surface": doc.text[node.start : node.end]
                if node.start is not None
                else "",
                "target_code_points_nfd_glaux": " ".join(
                    f"U+{ord(c):04X}" for c in node.form
                ),
                "status": status,
                "note": note,
                "sentence_text": s.text,
            }
        )

    eligible = [c for c in cards if c["status"] == "eligible"]
    plain = sorted(
        (c for c in eligible if not c["flags"] and not c["earlier_rejections"]),
        key=lambda c: (-c["occurrences"], c["key"]),
    )
    for c in plain[:3]:
        add(
            "ordinary",
            by_id[c["node"]],
            f"most frequent unflagged selected identity ({c['occurrences']} occurrences)",
            "eligible",
        )
    short = sorted(
        (c for c in eligible if c["units"] <= 5),
        key=lambda c: (c["sentence"], c["key"]),
    )
    if short:
        add(
            "short_complete_sentence",
            by_id[short[0]["node"]],
            "complete sentence of at most five whitespace units; the six-unit minimum is removed under #2034",
            "eligible",
        )
    later = sorted(
        (
            c
            for c in eligible
            if "over 50" in c["earlier_rejections"]
            or "over 400" in c["earlier_rejections"]
        ),
        key=lambda c: (-c["occurrences"], c["key"]),
    )
    if later:
        c = later[0]
        first = groups[(c["key"], c["upos"])][0]
        add(
            "over_cap_rejected",
            first,
            "earliest occurrence; its complete sentence exceeds a cap and is not truncated",
            "rejected",
        )
        add(
            "over_cap_next_qualifying",
            by_id[c["node"]],
            "next qualifying occurrence chosen instead",
            "eligible",
        )
    for c in cards:
        if c["status"] == "omitted":
            add(
                "omitted_all_over_cap",
                groups[(c["key"], c["upos"])][0],
                c["reason"],
                "omitted",
            )
    repeats = sorted(
        (c for c in eligible if "target_identity_repeats" in c["flags"]),
        key=lambda c: (-c["occurrences"], c["key"]),
    )
    if repeats:
        add(
            "repeated_target",
            by_id[repeats[0]["node"]],
            f"identity occurs more than once in this sentence; the target is node {repeats[0]['node']} at [{repeats[0]['target_start']},{repeats[0]['target_end']})",
            "eligible",
        )
    subscript = sorted(
        (
            c
            for c in eligible
            if "ͅ" in unicodedata.normalize("NFD", c["target_surface"])
            and "̓" in unicodedata.normalize("NFD", c["target_surface"])
            or "ͅ" in unicodedata.normalize("NFD", c["target_surface"])
            and "̔" in unicodedata.normalize("NFD", c["target_surface"])
        ),
        key=lambda c: (-c["occurrences"], c["key"]),
    )
    if subscript:
        add(
            "polytonic_nfd_nfc",
            by_id[subscript[0]["node"]],
            "GLAUx form is NFD (code points shown); the bound source span is NFC; breathing and iota subscript retained",
            "eligible",
        )
    comp = [n for n in nodes if n.kind == "suffix_component"]
    if comp:
        i = nodes.index(comp[0])
        add(
            "component_host",
            nodes[i - 1],
            "first part of a producer-split word; source-derived span",
            "",
        )
        add(
            "component_suffix",
            comp[0],
            "suffix component bound adjacent to its host with no whitespace",
            "",
        )
    crasis = [n for n in nodes if n.kind == "crasis_fragment"]
    if crasis:
        i = nodes.index(crasis[0])
        add(
            "crasis_shared_span_fragment",
            crasis[0],
            f"fragment form {crasis[0].form!r} is not a source substring; shares its host span",
            "",
        )
        add(
            "crasis_shared_span_host",
            nodes[i + 1],
            "host node carries the printed word",
            "",
        )
    elided = [
        n for n in nodes if n.bind_note.startswith("elision") and n.vocab == "eligible"
    ]
    if elided:
        add(
            "elision_mark_decoding",
            elided[0],
            "GLAUx writes U+2019; the source has U+02BC; the citation lemma is not reconstructed from the form",
            "",
        )
    typo = [n for n in nodes if n.div_section == "1.3.14" and nfc(n.form) == "ἦν"]
    if typo:
        add(
            "text_error_propagated",
            typo[0],
            "Perseus revision 2019 text reads ἦν; Perseus 84a6f3e5 encodes sic ἦν / corr ἢν; Marchant OCR reads ἣν (accent misread). GLAUx annotates the typo as εἰμί.",
            "",
        )
    crux = [n for n in nodes if "in_crux" in n.flags and n.vocab == "eligible"]
    if crux:
        add(
            "crux_target",
            crux[0],
            "word inside Marchant's obelized (corrupt) passage; eligible by current rules",
            "",
        )
    sec = [
        n for n in nodes if n.kind == "secluded" and is_lemma(n.attrs["form_original"])
    ]
    if sec:
        add(
            "secluded_unannotated",
            sec[0],
            "edition-deleted word; GLAUx supplies no lemma or postag",
            "",
        )
    names = sorted(
        (c for c in eligible if "capitalized_noun_lemma" in c["flags"]),
        key=lambda c: (-c["occurrences"], c["key"]),
    )
    if names:
        add(
            "proper_name_as_noun",
            by_id[names[0]["node"]],
            "UD Perseus maps every AGDT noun to NOUN, so names are vocabulary candidates",
            "eligible",
        )
    adv = sorted(
        (c for c in eligible if c["upos"] == "ADV" and (c["key"], "ADJ") in groups),
        key=lambda c: (-c["occurrences"], c["key"]),
    )
    if adv:
        add(
            "adverb_under_adjective_headword",
            by_id[adv[0]["node"]],
            "AGDT lemma convention: the adverb's headword is the adjective citation form; a distinct ADV identity",
            "eligible",
        )
    frag = [s for s in sentences if s.units <= 3 and s.text.startswith("ἔφη")]
    if frag:
        n = next(
            n
            for n in nodes
            if n.complete_sentence == frag[0].index and n.start is not None
        )
        add(
            "question_mark_attribution_fragment",
            n,
            "terminal ';' inside direct speech leaves the speech attribution as its own 'sentence'",
            "",
        )
    homograph = [
        n
        for n in nodes
        if n.vocab == "eligible" and nfc(n.attrs.get("lemma", "")) == "εἶμι"
    ]
    if not homograph:
        rows.append(
            {
                "case": "lexical_homograph_discriminator",
                "status": "absent",
                "note": "GLAUx lemmas carry no homograph numbers in this file, and εἶμι does not occur in Book 1; no discriminator case exists to illustrate",
            }
        )
    return rows


def catalog_example(summary: dict) -> dict:
    """Minimal machine-readable discovery/acquisition example (#2028/ADR 0096).
    Illustrative research data, not a schema proposal."""
    return {
        "example_version": "glaux-anabasis1-catalog-example/1",
        "note": "Research illustration of a metadata-only catalogue offering; syncing it contributes no vocabulary counts.",
        "work": {
            "author": "Xenophon",
            "title": "Anabasis",
            "aliases": [
                {"scheme": "cts-work", "value": "urn:cts:greekLit:tlg0032.tlg006"},
                {"scheme": "glaux-tlg", "value": "0032-006"},
            ],
        },
        "edition": {
            "label": "Marchant, Xenophontis Opera Omnia vol. 3 (Oxford 1904)",
            "cts_version": "urn:cts:greekLit:tlg0032.tlg006.perseus-grc2",
            "evidence": "Perseus TEI sourceDesc; Marchant scan point checks",
        },
        "reading_units": [
            {
                "unit": "book 1",
                "bounds": {
                    "first": summary["text"]["first_section"],
                    "last": summary["text"]["last_section"],
                },
                "annotation_offer": "glaux@b077d8f6:xml/0032-006.xml div_book=1",
                "probe_status": "import proof complete; human acceptance pending (#2030)",
            },
            {
                "unit": "whole work (books 1-7)",
                "annotation_offer": "glaux@b077d8f6:xml/0032-006.xml",
                "probe_status": "not assessed; must not be presented as analyzed from Book 1 evidence",
            },
        ],
        "text_source": {
            "repository": "PerseusDL/canonical-greekLit",
            "commit": "bd7b1c7a36a031f5b320f522ca0e7b260109349c",
            "path": "data/tlg0032/tlg006/tlg0032.tlg006.perseus-grc2.xml",
            "git_blob": "6fc2f2a14a177cfc26b7ba822a71f55faf551d8a",
            "later_revisions": "84a6f3e5 adds sic/corr at 1.3.14; adopting it would be a new text revision requiring a new binding",
        },
        "annotation_source": {
            "repository": "alekkeersmaekers/glaux",
            "commit": "b077d8f6ff429a5c7245954bc16bb7d1d7948823",
            "path": "xml/0032-006.xml",
            "git_blob": "ad9a1c7745d3c14cfa3f14882dec2477bf300c13",
            "provenance": {
                "layer": "lemma, postag, syntax",
                "scope": "all 610 Book 1 segments",
                "label": "analysis=auto",
                "kind": "automatic",
            },
            "metadata_row": {
                "GLAUX_TEXT_ID": "405",
                "SOURCE": "Perseus",
                "SOURCE_LICENSE": "CC BY-SA 4.0",
                "TREEBANK_ANNOTATIONS": "NA",
            },
        },
        "component_rights": {
            "text": "CC BY-SA 3.0 US (repository README at bd7b1c7a); same Book 1 text declared CC BY-SA 4.0 at later revisions",
            "annotation": "CC BY-SA 4.0 (GLAUx README general licence; metadata row lists no restrictive treebank import)",
            "derived_manifests": "CC BY-SA 4.0 with attribution; crosswalk tables CC BY-NC-SA 2.5 (derived from UD Perseus)",
            "review_outcome": "evidence recorded; not approved by the agent",
        },
        "capabilities": {
            "vocabulary_identities": "lemma+UPOS via documented crosswalk; no homograph discriminators in source",
            "morphology": "native postag retained; FEATS not mapped",
            "syntax": "native heads/relations retained; not validated or mapped",
            "complete_sentences": f"{summary['sentences']['complete_sentences']} under {SEGMENT_VERSION}",
            "requires_stanza": False,
        },
        "acquisition": {
            "trigger": "To Read intent",
            "steps": [
                "fetch pinned text and annotation by commit",
                "verify sha256",
                "extract, bind, map, validate",
                "publish atomically or report blocked",
            ],
            "fallback": "none; another source or revision needs informed selection and fresh validation",
        },
    }


def ledger(summary: dict) -> dict:
    """Separate verdicts. Values: pass | conditional | fail | unassessed."""
    nodes, cov, sent, card = (
        summary["nodes"],
        summary["coverage"],
        summary["sentences"],
        summary["card_inputs"],
    )
    binding_ok = (
        not summary["binding_exceptions"]
        and not cov["uncovered_non_space_characters"]
        and not cov["section_mismatches"]
        and not cov["word_boundary_violations"]
    )
    unresolved = summary["nodes"]["by_vocabulary"].get("unresolved", 0)
    return {
        "scope_and_edition": {
            "verdict": "pass",
            "evidence": f"Book 1 complete: {summary['text']['sections']} sections {summary['text']['first_section']}-{summary['text']['last_section']}; every GLAUx Book 1 node bound; edition Marchant 1904 per Perseus header, point-checked against the scan OCR",
            "qualifications": [
                "text revision is the 2019 Perseus blob that GLAUx annotated, including the 1.3.14 typo later corrected upstream",
                "book heading excluded as ancillary; secluded, supplied and obelized text retained with markup provenance",
            ],
        },
        "rights_by_operation": {
            "framework": "#2035: evidence suffices | defer | narrow blocker; not legal advice",
            "local_private_noncommercial_analysis": "evidence suffices: text and annotation carry explicit BY-SA declarations",
            "direct_acquisition_from_upstream": "evidence suffices: public pinned commits, no gated access",
            "public_research_outputs": "evidence suffices with notices: text and annotation BY-SA (attribution, ShareAlike); UD-Perseus-derived crosswalk counts BY-NC-SA 2.5 (non-commercial research)",
            "bundled_or_distributed_data": "evidence suffices for #2035 exact-component review of the BY-SA text and annotation; acceptance is a human decision; vocabulary decisions need no NC-derived table at runtime",
            "open_points": [
                "Perseus README general Tufts copyright statement and request to offer modifications (recorded, not treated as a veto under #2035)",
                "Zenodo GLAUx archive declares CC BY 4.0 for a different release (not used)",
                "rights in GLAUx training data and models are not conflated with this output",
            ],
        },
        "binding_and_segmentation": {
            "verdict": "pass" if binding_ok else "fail",
            "evidence": f"{nodes['by_bind_status']}; 0 uncovered source characters; 0 section mismatches; {sent['complete_sentences']} complete sentences, all unions of native segments",
            "qualifications": [
                "one ';' boundary leaves a speech attribution fragment (sentence 226)",
                "terminal convention is research version oct-grc-terminal/1, not an accepted policy",
            ],
        },
        "required_lexical_identities": {
            "verdict": "conditional"
            if nodes["secluded_lexical_nodes"]
            else ("pass" if not unresolved else "fail"),
            "evidence": f"{summary['vocabulary']['eligible_occurrences']} eligible occurrences, {summary['vocabulary']['identities']} identities, {unresolved} unresolved printed nodes, 6 CCONJ|SCONJ-ambiguous nodes non-vocabulary under both candidates",
            "condition": f"{nodes['secluded_lexical_nodes']} edition-secluded lexical nodes have no producer annotation; vocabulary-ready only if #2030 accepts edition seclusion as a justified non-vocabulary classification",
        },
        "optional_capabilities": {
            "morphology": "unassessed (native postag retained, FEATS not mapped)",
            "syntax": "unassessed (native heads/relations and 96 elliptic nodes retained; no tree validation)",
            "dictionary_coverage": "unassessed (optional; the Kaikki dictionary index is deployment-supplied data with no grc build or test in the repository)",
            "lexical_homograph_discriminators": "absent in source",
        },
        "measured_output_quality": {
            "verdict": "conditional",
            "evidence": "see review_set.tsv and report: proper names as NOUN, negation/adverb lemmas, component splits, adverb headwords, εἰμί as AUX, text typo propagation, unbracketed editorial text in sentence strings",
        },
        "deterministic_card_inputs": {
            "verdict": "pass" if card["eligible"] else "fail",
            "evidence": f"{card['eligible']} of {card['selected_identities']} selected identities have a qualifying sentence; {card['omitted']} omitted ({card['omission_reasons']})",
            "qualification": f"{card['chosen_flags'].get('sentence_markup', 0)} chosen sentences contain edition deletions or supplements that the plain sentence string shows without brackets; presentation unresolved",
        },
        "unperformed_semantic_and_artifact_checks": {
            "verdict": "unassessed",
            "items": [
                "contextual Gloss/translation acceptance",
                "Anki rendering of polytonic text and emphasis",
                "dictionary meaning evidence",
                "human acceptance of examples and usefulness",
                "scholarly accuracy of automatic predictions",
            ],
        },
    }


def summarize(
    nodes,
    doc,
    cov,
    exceptions,
    native_info,
    seg_info,
    sentences,
    groups,
    selection,
    cards,
) -> dict:
    count = lambda it: dict(sorted(collections.Counter(it).items()))  # noqa: E731
    selected = [i for i, w in selection.items() if w.startswith("selected")]
    eligible_occ = sum(len(v) for v in groups.values())
    secluded = [n for n in nodes if n.kind == "secluded"]
    lexical_secluded = [n for n in secluded if is_lemma(n.attrs["form_original"])]
    units = [s.units for s in sentences]
    return {
        "versions": {
            "extract": EXTRACT_VERSION,
            "bind": BIND_VERSION,
            "map": MAP_VERSION,
            "profile": PROFILE_VERSION,
            "segment": SEGMENT_VERSION,
            "card_input": CARD_INPUT_VERSION,
        },
        "native": native_info,
        "text": {
            "code_points": len(doc.text),
            "sections": len(doc.sections),
            "first_section": doc.sections[0][0],
            "last_section": doc.sections[-1][0],
            "chapters": len({r.rsplit(".", 1)[0] for r, _, _ in doc.sections}),
            "heading_excluded": doc.heading,
            "editorial_regions": count(k for k, *_ in doc.regions),
            "paragraph_milestones": len(doc.paragraphs),
        },
        "nodes": {
            "total": len(nodes),
            "by_kind": count(n.kind for n in nodes),
            "by_bind_status": count(n.bind_status for n in nodes),
            "by_bind_rule": count(n.bind_rule for n in nodes),
            "elision_mark_decoded": sum(
                1 for n in nodes if n.bind_note.startswith("elision")
            ),
            "analysis_labels": count(n.analysis for n in nodes),
            "by_vocabulary": count(n.vocab for n in nodes),
            "by_upos": count(n.upos or "(none)" for n in nodes),
            "by_map_rule": count(n.map_rule or "(none)" for n in nodes),
            "ambiguous_upos": count(
                n.upos_candidates for n in nodes if n.map_status == "ambiguous"
            ),
            "secluded_nodes": len(secluded),
            "secluded_lexical_nodes": len(lexical_secluded),
            "crux_nodes_in_word_flag": sum(1 for n in nodes if "in_crux" in n.flags),
            "supplied_nodes": sum(1 for n in nodes if n.kind == "supplied"),
        },
        "binding_exceptions": exceptions,
        "coverage": {
            "uncovered_non_space_characters": [
                [i, c] for i, c in cov["uncovered_non_space"]
            ],
            "section_mismatches": cov["section_mismatches"],
            "word_boundary_violations": cov["word_boundary_violations"],
        },
        "sentences": {
            "complete_sentences": len(sentences),
            "boundaries": count(s.boundary for s in sentences),
            "units_le_5": sum(1 for u in units if u <= 5),
            "units_over_50": sum(1 for s in sentences if s.units > MAX_UNITS),
            "code_points_over_400": sum(
                1 for s in sentences if s.code_points > MAX_CODE_POINTS
            ),
            "over_either_cap": sum(
                1
                for s in sentences
                if s.units > MAX_UNITS or s.code_points > MAX_CODE_POINTS
            ),
            "median_units": sorted(units)[len(units) // 2],
            "max_units": max(units),
            "with_editorial_markup": count(
                "+".join(s.markup) for s in sentences if s.markup
            ),
            "without_eligible_vocabulary": sum(
                1 for s in sentences if s.eligible_nodes == 0
            ),
            "native_relation": seg_info,
        },
        "vocabulary": {
            "occurrence_denominators": {
                "native_nodes": len(nodes),
                "unprinted_nodes": sum(1 for n in nodes if n.kind == "unprinted"),
                "secluded_unannotated_nodes": len(secluded),
                "printed_punctuation_and_crux_nodes": sum(
                    1 for n in nodes if n.upos == "PUNCT"
                ),
                "printed_lexical_annotated_nodes": sum(
                    1
                    for n in nodes
                    if n.kind not in ("unprinted", "secluded") and n.upos != "PUNCT"
                ),
                "printed_lexical_non_vocabulary_by_upos": count(
                    n.upos or n.upos_candidates
                    for n in nodes
                    if n.vocab == "non_vocab" and n.upos != "PUNCT"
                ),
                "vocabulary_eligible": eligible_occ,
            },
            "eligible_occurrences": eligible_occ,
            "analyzable_token_denominator": eligible_occ,
            "identities": len(groups),
            "identities_by_upos": count(u for _, u in groups),
            "selected_identities": len(selected),
            "selected_by_upos": count(u for _, u in selected),
            "selected_occurrences": sum(len(groups[i]) for i in selected),
            "selected_capitalized_headwords_any_upos": sum(
                1
                for i in selected
                if groups[i][0].headword[:1] != groups[i][0].headword[:1].lower()
            ),
            "known_identities": 0,
            "reserved_identities": 0,
            "known_token_coverage": 0.0,
        },
        "card_inputs": {
            "selected_identities": len(cards),
            "eligible": sum(1 for c in cards if c["status"] == "eligible"),
            "omitted": sum(1 for c in cards if c["status"] == "omitted"),
            "omission_reasons": count(
                c["reason"] for c in cards if c["status"] == "omitted"
            ),
            "chosen_after_earlier_rejection": sum(
                1
                for c in cards
                if c["status"] == "eligible" and c["earlier_rejections"]
            ),
            "chosen_flags": count(
                f.split("=")[0]
                for c in cards
                if c.get("flags")
                for f in c["flags"].split(",")
            ),
        },
    }


def main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("inputs", help="directory populated by fetch_inputs.sh")
    parser.add_argument("out", help="manifest output directory")
    args = parser.parse_args(argv)
    summary = run(args.inputs, args.out)
    print(
        json.dumps(
            {k: summary[k] for k in ("nodes", "vocabulary", "card_inputs")},
            ensure_ascii=False,
            indent=1,
        )
    )
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
