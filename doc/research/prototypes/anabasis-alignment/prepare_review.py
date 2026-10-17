"""THROWAWAY: prepare candidates, never human labels or evaluation results.

Run after import_probe.go; uses only the standard library and pinned local files.
"""

import collections
import hashlib
import json
import re
import sys
import xml.etree.ElementTree as ET
from pathlib import Path

HERE = Path(__file__).resolve().parent
SCRATCH = Path(sys.argv[1]).expanduser()
PINS = {
    "oga.conllu": {
        "url": "https://raw.githubusercontent.com/mgbilby/oga_src/223d2258745ecf35e7659191080f93640fe900c1/data/tlg0032.tlg006.perseus-grc2_oga020_src.conllu",
        "sha256": "c51bba2df6e80e2eb8dd2543d00cf6d6e6b8c65cc9ed03c7d27176fed063eb61",
    },
    "brownson.xml": {
        "url": "https://raw.githubusercontent.com/PerseusDL/canonical-greekLit/0189b9c8d8d1f148133f2f0af309b4ee90ea4332/data/tlg0032/tlg006/tlg0032.tlg006.perseus-eng2.xml",
        "sha256": "f4c88203196a4f2ed315d47642d10f9515c568374139ba1e25e7934312586ea7",
    },
}
raw = {}
for name, pin in PINS.items():
    raw[name] = (SCRATCH / name).read_bytes()
    assert hashlib.sha256(raw[name]).hexdigest() == pin["sha256"]

T = "{http://www.tei-c.org/ns/1.0}"


def english_text(element):
    if element.tag in {T + "note", T + "head"}:
        return ""
    if element.tag == T + "milestone":
        return " "  # Preserve a word boundary, including possible.<milestone/>It.
    return (element.text or "") + "".join(
        english_text(child) + (child.tail or "") for child in element
    )


book = next(
    d for d in ET.fromstring(raw["brownson.xml"]).iter(T + "div")
    if d.get("subtype") == "book" and d.get("n") == "1"
)
sections = {
    f"1.{chapter.get('n')}.{section.get('n')}": " ".join(english_text(section).split())
    for chapter in book.findall(T + "div") for section in chapter.findall(T + "div")
}
sentences = []
for block in raw["oga.conllu"].decode().strip().split("\n\n"):
    lines = block.splitlines()
    rows = [line.split("\t") for line in lines if line and not line.startswith("#")]
    if not rows:
        continue
    refs = [row[9].rsplit("_", 1)[1] for row in rows]
    if not any(ref.startswith("1.") for ref in refs):
        continue
    assert all(ref.startswith("1.") for ref in refs)
    text, tokens = "", []
    for row, ref in zip(rows, refs):
        if row[3] in {"-", "x"}:
            continue
        if text and row[1][0] not in ",.;\u00b7\u037e\u0387":
            text += " "
        start = len(text)
        text += row[1]
        tokens.append({
            "source_token_id": row[0], "surface": row[1], "oga_lemma": row[2],
            "native_pos": row[3], "section": ref, "span": [start, len(text)],
        })
    sentences.append({
        "source_sentence_id": next(line.split(" = ", 1)[1] for line in lines if line.startswith("# sent_id")),
        "book_sentence_ordinal": len(sentences) + 1,
        "sections": list(dict.fromkeys(refs)), "greek": text, "tokens": tokens,
        "dropped_ellipsis": len(rows) - len(tokens),
    })

# Bind every reconstructed sentence, retained token and codepoint offset to the
# real import seam, without a database or modifications to the production code.
imported = json.loads((SCRATCH / "import.json").read_text())
assert len(imported["Sentences"]) == len(sentences)
for sentence, actual in zip(sentences, imported["Sentences"]):
    assert sentence["greek"] == actual["Text"]
    assert len(sentence["tokens"]) == len(actual["Tokens"])
    offset = actual["Location"]["StartOffset"]
    for token, actual_token in zip(sentence["tokens"], actual["Tokens"]):
        assert token["surface"] == actual_token["Surface"]
        assert token["oga_lemma"] == actual_token["RawLemma"]
        assert token["span"] == [
            actual_token["Location"]["StartOffset"] - offset,
            actual_token["Location"]["EndOffset"] - offset,
        ]
        token["import_upos"] = actual_token["UPOS"]

first = collections.Counter(s["sections"][0] for s in sentences)
touch = collections.Counter(ref for s in sentences for ref in s["sections"])
assert set(sections) == set(touch)
baseline = {
    "sentences": len(sentences), "sections": len(touch),
    "printed_tokens": sum(len(s["tokens"]) for s in sentences),
    "ellipsis_dropped": sum(s["dropped_ellipsis"] for s in sentences),
    "sections_by_first_sentence_count": dict(sorted(collections.Counter(first.values()).items())),
    "sections_by_touching_sentence_count": dict(sorted(collections.Counter(touch.values()).items())),
    "cross_section_sentences": [
        {"id": s["source_sentence_id"], "sections": s["sections"]}
        for s in sentences if len(s["sections"]) > 1
    ],
    "ten_sentence_sections": [r for r, n in first.items() if n == 10],
    "import_correspondence": "607 sentence texts, 10462 token surfaces/lemmas/offsets checked against real import seam; no database writes",
    "analysis": imported["Analysis"],
}
assert (baseline["sentences"], baseline["printed_tokens"], baseline["ellipsis_dropped"]) == (607, 10462, 110)

# Agent-drafted proposals: these are NOT algorithm predictions or gold labels.
# Each span is copied verbatim from the extracted Brownson text, not translated.
proposals = [
    (1, "Two-sentence section; straightforward opening with discontinuous verb",
     [("1.1.1", "Darius and Parysatis had two sons born to them, of whom the elder was Artaxerxes and the younger Cyrus.")],
     [("γίγνονται", ["had", "born to them"]), ("παῖδες", ["sons"]), ("πρεσβύτερος", ["elder"])]),
    (3, "Very short sentence inside a three-sentence section; English semicolon",
     [("1.1.2", "The elder, as it chanced, was with him already;")],
     [("πρεσβύτερος", ["elder"]), ("παρὼν", ["was with him already"]), ("ἐτύγχανε", ["as it chanced"])]),
    (15, "Medium sentence inside a four-sentence section; multiword verbal links",
     [("1.1.6", "In the first place, he sent orders to the commanders of all the garrisons he had in the cities to enlist as many Peloponnesian soldiers of the best sort as they severally could, on the plea that Tissaphernes had designs upon their cities.")],
     [("παρήγγειλε", ["sent orders"]), ("φυλακὰς", ["garrisons"]), ("ἐπιβουλεύοντος", ["had designs upon"])]),
    (177, "Longer sentence comprising an entire one-sentence section",
     [("1.3.13", sections["1.3.13"])],
     [("ἀπορία", ["difficulty"]), ("ἐγκέλευστοι", ["at the instigation of"]), ("ἀπιέναι", ["departing"])]),
    (238, "Short sentence in the ten-sentence section; clause within an English sentence",
     [("1.4.8", "for I have men-of-war with which I can overtake their craft.")],
     [("τριήρεις", ["men-of-war"]), ("ἑλεῖν", ["overtake"]), ("πλοῖον", ["craft"])]),
    (103, "Long cross-section sentence; final English clause belongs to next Greek sentence",
     [("1.2.17", "And then, as they went on faster and faster, at length with a shout the troops broke into a run of their own accord, in the direction of the camp."),
      ("1.2.18", "As for the barbarians, they were terribly frightened; the Cilician queen took to flight in her carriage, and the people in the market left their wares behind and took to their heels;")],
     [("φόβος", ["frightened"]), ("ἔφυγεν", ["took to flight"]), ("ὤνια", ["wares"])]),
    (428, "Editorial mismatch: following English canals passage translates Greek absent from OGA",
     [("1.7.15", "This trench extended up through the plain for a distance of twelve parasangs, reaching to the wall of Media,")],
     [("παρετέτατο", ["extended"]), ("τάφρος", ["trench"]), ("πεδίου", ["plain"])]),
    (536, "Two-sentence section with an existing Brownson-attributed Ugarit record; no Ugarit links used yet",
     [("1.9.17", "For generals and captains who came overseas to serve him for the sake of money judged that loyal obedience to Cyrus was worth more to them than their mere monthly pay.")],
     [("κερδαλεώτερον", ["worth more"]), ("πειθαρχεῖν", ["obedience"]), ("κέρδος", ["pay"])]),
]


def unique_span(text, quote):
    assert text.count(quote) == 1, quote
    start = text.index(quote)
    return [start, start + len(quote)]


samples = []
for ordinal, reason, spans, links in proposals:
    sample = dict(sentences[ordinal - 1])
    sample.update({"reason": reason, "human_review": None, "candidate_provenance": "agent-drafted; not evaluated algorithm output"})
    sample["section_sizes_first"] = {r: first[r] for r in sample["sections"]}
    sample["section_sizes_touching"] = {r: touch[r] for r in sample["sections"]}
    sample["english_context"] = {r: sections[r] for r in sample["sections"]}
    sample["candidate_spans"] = [
        {"section": ref, "span": unique_span(sections[ref], quote), "quote": quote}
        for ref, quote in spans
    ]
    sample["english_tokens"] = {
        ref: [{"index": i, "surface": m.group(), "span": [m.start(), m.end()]}
              for i, m in enumerate(re.finditer(r"\w+(?:['’\-]\w+)*|[^\w\s]", text))]
        for ref, text in sample["english_context"].items()
    }
    sample["candidate_word_links"] = []
    for surface, quotes in links:
        matches = [t for t in sample["tokens"] if t["surface"] == surface]
        assert len(matches) == 1, surface
        token = matches[0]
        assert token["import_upos"] in {"NOUN", "VERB", "ADJ", "ADV"}
        destinations = []
        for quote in quotes:
            matches = [s for s in sample["candidate_spans"] if quote in s["quote"]]
            assert len(matches) == 1, quote
            parent = matches[0]
            ref = parent["section"]
            relative_start, relative_end = unique_span(parent["quote"], quote)
            start, end = parent["span"][0] + relative_start, parent["span"][0] + relative_end
            destinations.append({
                "section": ref, "span": [start, end], "quote": quote,
                "english_token_indices": [t["index"] for t in sample["english_tokens"][ref]
                                          if t["span"][0] < end and t["span"][1] > start],
            })
        sample["candidate_word_links"].append({
            "source_token_id": token["source_token_id"], "surface": surface,
            "oga_lemma": token["oga_lemma"], "destinations": destinations,
            "status": "proposed_not_reviewed",
        })
    samples.append(sample)

asset = {
    "schema": "anabasis-alignment-review/1", "is_gold": False,
    "status": "awaiting_live_human_review", "reviewer": None,
    "sources": PINS, "baseline": baseline,
    "offset_convention": "zero-based half-open Unicode codepoints; English relative to whitespace-collapsed, note-free section with milestone boundaries preserved; Greek relative to import sentence",
    "english_token_convention": "zero-based indices within section; regex word (internal apostrophe/hyphen allowed) or single non-whitespace punctuation",
    "link_scope": "24 selected vocabulary occurrences only; not all words. Confirm every English realization or explicitly label NULL/uncertain. Do not infer dictionary-sense labels.",
    "licence": "Greek: OGA v0.2.0 / oga_src / Perseus, CC BY-SA 4.0. English: Xenophon, translated by Carleton L. Brownson (1921–22), digitisation Perseus/Tufts, CC BY-SA 4.0. Extraction and unreviewed proposed correspondences by OpenAI assistant for Justin, 2026-10-17; adapted data shared under CC BY-SA 4.0 (https://creativecommons.org/licenses/by-sa/4.0/). Code remains under repository licence. No pretrained weights or extra corpora used.",
    "limitations": "Purposive 8-sentence difficulty sample, not random or representative; biased proposals shown to reviewer; no held-out set; word links not dictionary senses; no measured reliability or production acceptance.",
    "samples": samples,
}
(HERE / "review-candidates.json").write_text(json.dumps(asset, ensure_ascii=False, indent=2) + "\n")
template = (HERE / "review-template.html").read_text()
(HERE / "review.html").write_text(template.replace("/*CANDIDATES*/null", json.dumps(asset, ensure_ascii=False).replace("<", "\\u003c")))
print(json.dumps({"status": asset["status"], "sentences": len(samples), "target_occurrences": sum(len(s["candidate_word_links"]) for s in samples), "baseline": baseline}, indent=2))
