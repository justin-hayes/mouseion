"""THROWAWAY: verified Book 1 inputs and a small attributed raw-Kaikki subset.

Usage: python3 prepare_inputs.py SCRATCH CACHED_RAW_KAIKKI_GZIP
Never reads gold labels. No pretrained resource or quoted examples are used.
"""

import gzip
import hashlib
import json
import re
import runpy
import sys
import unicodedata as U
from pathlib import Path
from urllib.parse import quote

root = Path(__file__).resolve().parent
scratch = Path(sys.argv[1]).expanduser()
dictionary = Path(sys.argv[2]).expanduser()
# Reuse the already import-checked extraction; its proposals are not inputs.
source = runpy.run_path(str(root / "prepare_review.py"))
sentences, sections = source["sentences"], source["sections"]
(scratch / "corpus.json").write_text(json.dumps({
    "sentences": sentences, "sections": sections, "pins": source["PINS"],
}, ensure_ascii=False))


def key(word):
    return U.normalize("NFC", re.sub(r"\d+$", "", word)).lower().replace("ς", "σ")


needed = {key(t["oga_lemma"]) for s in sentences for t in s["tokens"]}
entries = []
count = 0
with gzip.open(dictionary, "rb") as handle:
    for line in handle:
        if b'"grc"' not in line:
            continue
        row = json.loads(line)
        if row.get("lang_code") != "grc":
            continue
        count += 1
        if key(row.get("word", "")) not in needed:
            continue
        senses = []
        for sense in row.get("senses", []):
            # Exclude quotation/example/translation objects, which can carry
            # different rights. Retain only dictionary glosses and metadata.
            senses.append({k: sense[k] for k in ("glosses", "tags", "form_of") if k in sense})
        word = row["word"]
        entries.append({
            "word": word, "pos": row.get("pos"),
            "etymology_number": row.get("etymology_number"), "senses": senses,
            "source_row_sha256": hashlib.sha256(line).hexdigest(),
            "url": "https://en.wiktionary.org/wiki/" + quote(word) + "#Ancient_Greek",
            "history": "https://en.wiktionary.org/w/index.php?title=" + quote(word) + "&action=history",
        })
with dictionary.open("rb") as handle:
    digest = hashlib.file_digest(handle, "sha256").hexdigest()
subset = {
    "schema": "anabasis-dictionary-anchor-input/1",
    "origin": "Local raw-Kaikki gzip cache used by the existing Mouseion dictionary derivation; default upstream download URL below. Exact original dump date/extractor revision not recorded in this cache, so not inferred from today's website.",
    "upstream_url": "https://kaikki.org/dictionary/raw-wiktextract-data.jsonl.gz",
    "compressed_source_sha256": digest, "compressed_source_bytes": dictionary.stat().st_size,
    "source_grc_rows": count, "selected_rows": len(entries),
    "selection": "All exact NFC/lower/final-sigma-normalised OGA Book 1 lemma headwords; trailing OGA homograph number removed for candidate lookup only. All entries retained separately; no numbered sense binding claimed.",
    "licence": "English Wiktionary contributors; CC BY-SA 4.0 and GFDL. This subset uses the CC BY-SA 4.0 route, attributes each page and contributor history, and is shared under CC BY-SA 4.0. Extracted by Kaikki/Wiktextract (Tatu Ylonen and contributors); reduced to glosses/metadata for this prototype. No quotations, media or external examples retained.",
    "licence_sources": [
        "https://en.wiktionary.org/w/index.php?title=Wiktionary:Copyrights&oldid=87545419",
        "https://kaikki.org/dictionary/rawdata.html",
        "https://creativecommons.org/licenses/by-sa/4.0/",
    ],
    "entries": entries,
}
(root / "dictionary-subset.json").write_text(json.dumps(subset, ensure_ascii=False, indent=2) + "\n")
print(json.dumps({k: v for k, v in subset.items() if k not in {"entries", "licence", "origin", "selection", "licence_sources"}}, indent=2))
