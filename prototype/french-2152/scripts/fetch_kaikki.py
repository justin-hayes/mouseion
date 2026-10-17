"""Fetch bounded per-word Kaikki French JSONL pages (post-processed) for the agreed sample.

Usage: python3 -I fetch_kaikki.py <out_dir>
Resolves each title through Kaikki's own search index, then downloads only that page's JSONL.
"""

import hashlib
import json
import sys
import urllib.parse
import urllib.request
from pathlib import Path

WORDS = [
    # pronominal identity
    "laver", "se laver", "habiller", "s'habiller", "aller", "s'en aller", "en aller",
    "apercevoir", "s'apercevoir", "souvenir", "se souvenir", "falloir", "lever", "se lever",
    # function-word contractions and elided forms (must not become content vocabulary)
    "du", "au", "aux", "des", "le", "de", "l'", "d'accord",
    # common nouns: gender, plural, h status
    "homme", "chemise", "marché", "pomme", "sœur", "main", "porte", "café", "prix", "œil",
    "cheval", "travail", "ciel", "héros", "hache", "haricot", "hôtel", "hasard", "huile",
    "petit déjeuner", "petit-déjeuner",
    # accents that change meaning
    "sûr", "sur", "côte", "côté", "dû",
    # verb forms / irregulars
    "venir", "faire", "boire", "revenir",
]
BASE = "https://kaikki.org/dictionary/"


def get(url: str) -> bytes:
    request = urllib.request.Request(url, headers={"User-Agent": "mouseion-issue-2152-probe"})
    with urllib.request.urlopen(request, timeout=60) as response:  # noqa: S310 - fixed https host
        return response.read()


def search_index(prefix: str) -> list:
    name = urllib.parse.quote(prefix.lower()[:2]) + ".json"
    data = json.loads(get(BASE + "search/start/" + name))
    if data[0]:  # three-character index
        name = urllib.parse.quote(prefix.lower()[:3]) + ".json"
        data = json.loads(get(BASE + "search/start/" + name))
    return data[1]


out = Path(sys.argv[1])
out.mkdir(parents=True, exist_ok=True)
manifest = []
for word in WORDS:
    record = {"word": word}
    try:
        hits = [h for h in search_index(word) if h[0] == word]
        # The search index points at "All languages combined" pages; the French
        # dictionary uses the same path segments under /French/.
        french = [[h[0], h[1].replace("/All languages combined/", "/French/")] for h in hits]
        record["search_hits"] = [h[1] for h in hits]
        if not french:
            record["status"] = "no French page in search index"
        else:
            url = urllib.parse.quote(french[0][1], safe=":/")
            if url.endswith(".html"):
                url = url[: -len(".html")] + ".jsonl"
            body = get(url)
            path = out / (hashlib.sha1(word.encode()).hexdigest()[:10] + ".jsonl")
            path.write_bytes(body)
            record.update(
                status="fetched", url=url, file=path.name, bytes=len(body),
                sha256=hashlib.sha256(body).hexdigest(),
                entries=sum(1 for line in body.splitlines() if line.strip()),
            )
    except Exception as error:  # record and continue; this is a probe
        record["status"] = f"error: {error}"
    manifest.append(record)
    print(record["word"], record["status"], record.get("entries", ""))
(out / "manifest.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=1))
