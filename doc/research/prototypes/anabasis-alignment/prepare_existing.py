"""THROWAWAY: bind one digest-pinned, Brownson-attributed Ugarit record.

Usage: python3 prepare_existing.py SCRATCH
No gold labels used. Site-level CC BY-SA/provenance limits remain explicit.
"""

import hashlib
import json
import re
import sys
import unicodedata as U
from html.parser import HTMLParser
from pathlib import Path

root = Path(__file__).resolve().parent
scratch = Path(sys.argv[1]).expanduser()
raw = (scratch / "ugarit-34674.html").read_bytes()
digest = "e5d891142483c7243fbf1869896cb4668eed8ceb9f57ca8c75d62025a36e0995"
assert hashlib.sha256(raw).hexdigest() == digest


class Tokens(HTMLParser):
    def __init__(self):
        super().__init__()
        self.current = None
        self.tokens = []

    def handle_starttag(self, tag, attributes):
        attrs = dict(attributes)
        if tag == "span" and attrs.get("id", "").startswith("token_"):
            self.current = {"id": attrs["id"], "group": attrs.get("data-ref"), "text": ""}

    def handle_data(self, text):
        if self.current is not None:
            self.current["text"] += text

    def handle_endtag(self, tag):
        if tag == "span" and self.current is not None:
            self.tokens.append(self.current)
            self.current = None


parser = Tokens()
parser.feed(raw.decode())
grc = [t for t in parser.tokens if t["id"].startswith("token_1_")]
eng = [t for t in parser.tokens if t["id"].startswith("token_2_")]
assert (len(grc), len(eng)) == (129, 220)
corpus = json.loads((scratch / "corpus.json").read_text())
refs = ["1.9.17", "1.9.18", "1.9.19"]
sentences = [s for s in corpus["sentences"] if set(s["sections"]) <= set(refs)]


def stream(items):
    return [
        (ch, item["locator"])
        for item in items
        for ch in U.normalize("NFC", item["text"]).lower().replace("ς", "σ")
        if (ch.isalpha() and ch != "\u02bc") or ch.isdigit()
    ]


def bind(original, actual):
    left = stream([{"text": t["text"], "locator": t["id"]} for t in original])
    right = stream(actual)
    # Retains Greek accents/breathings/iota subscripts; ignores only punctuation,
    # whitespace and token boundaries, not letter differences.
    assert [a for a, _ in left] == [a for a, _ in right]
    mapping = {}
    for (_, native), (_, target) in zip(left, right):
        mapping.setdefault(native, set()).add(tuple(target))
    for token in original:
        token["bound_occurrences"] = sorted(mapping.get(token["id"], set()))


bind(grc, [
    {"text": t["surface"], "locator": [s["source_sentence_id"], t["source_token_id"]]}
    for s in sentences for t in s["tokens"]
])
# Same fixed tokenisation as the gold's source-section token identities.
eng_tokens = {
    ref: [{"text": m.group(), "span": [m.start(), m.end()], "index": i}
          for i, m in enumerate(re.finditer(r"\w+(?:['’\-]\w+)*|[^\w\s]", corpus["sections"][ref]))]
    for ref in refs
}
bind(eng, [
    {"text": token["text"], "locator": [ref, token["index"]]}
    for ref in refs for token in eng_tokens[ref]
])
asset = {
    "schema": "anabasis-existing-alignment/1", "url": "https://www.ugarit.ialigner.com/text.php?id=34674",
    "html_sha256": digest, "sections": refs,
    "creator": "Jacob Schwartz, 2022-12-23; translation attributed to Carleton L. Brownson",
    "licence": "Ugarit site-level CC BY-SA 4.0; retain attribution to Jacob Schwartz, Brownson and Perseus. Derived extraction shared under CC BY-SA 4.0. Site notice is not independent proof of contributor authority. Used only as a bounded candidate comparison, not blanket source acceptance.",
    "licence_url": "https://creativecommons.org/licenses/by-sa/4.0/",
    "binding": "All Greek and English NFC letter/digit streams match the pinned OGA/Brownson sections, retaining Greek diacritics and treating U+02BC as an elision mark rather than a lexical letter (Ugarit has U+1FBD at the corresponding location). Native token IDs map by occurrence, punctuation excluded from links. Group correspondence expands many-to-many; it is not a hand-checked gold set.",
    "greek": grc, "english": eng,
}
(root / "existing-ugarit.json").write_text(json.dumps(asset, ensure_ascii=False, indent=2) + "\n")
print("Bound Ugarit record: 129 Greek / 220 English tokens across 1.9.17–19; no gold labels used.")
