"""Compare raw Wiktextract French records (from the capped prefix) with Kaikki's
post-processed per-word JSONL for the same words, on the fields the derivation reads.

Usage: python3 -I compare_raw_post.py <raw_fr_records.json> <out_json> word...
"""

import json
import sys
import urllib.parse
import urllib.request

FIELDS = ("pos", "tags", "categories", "forms", "sounds", "head_templates")
raw_path, out_path, words = sys.argv[1], sys.argv[2], sys.argv[3:]
raw = json.load(open(raw_path, encoding="utf-8"))


def post_records(word: str) -> list[dict]:
    url = "https://kaikki.org/dictionary/French/meaning/{}/{}/{}.jsonl".format(
        *(urllib.parse.quote(part) for part in (word[0], word[:2], word))
    )
    request = urllib.request.Request(url, headers={"User-Agent": "mouseion-issue-2152-probe"})
    with urllib.request.urlopen(request, timeout=60) as response:  # noqa: S310
        return [json.loads(line) for line in response.read().splitlines() if line.strip()]


def summary(record: dict) -> dict:
    senses = record.get("senses", [])
    return {
        "keys": sorted(record),
        "pos": record.get("pos"),
        "tags": record.get("tags"),
        "gender_categories": [c for c in record.get("categories", []) if isinstance(c, str) and ("masculine" in c or "feminine" in c)],
        "category_count": len(record.get("categories", [])),
        "glosses": [s.get("glosses", [None])[0] for s in senses],
        "sense_tags": [s.get("tags") for s in senses],
        "plural_forms": [f["form"] for f in record.get("forms", []) if "plural" in f.get("tags", [])],
        "form_count": len(record.get("forms", [])),
        "ipa": [(s.get("ipa"), s.get("tags")) for s in record.get("sounds", []) if s.get("ipa")],
        "head_expansions": [t.get("expansion") for t in record.get("head_templates", [])],
    }


report = {}
for word in words:
    raws = [summary(r) for r in raw if r.get("word") == word]
    posts = [summary(r) for r in post_records(word)]
    report[word] = {"raw": raws, "post": posts}
json.dump(report, open(out_path, "w", encoding="utf-8"), ensure_ascii=False, indent=1)
for word, sides in report.items():
    print(f"== {word}: raw {len(sides['raw'])} records, post {len(sides['post'])} records")
    for r, p in zip(sides["raw"], sides["post"]):
        for key in r:
            if r[key] != p[key]:
                print(f"  DIFF {key}:\n    raw:  {r[key]}\n    post: {p[key]}")
