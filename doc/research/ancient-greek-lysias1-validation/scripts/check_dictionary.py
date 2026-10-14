#!/usr/bin/env python3
"""Measure public Ancient Greek dictionary entry availability for selected headwords.

Usage (stdlib only, isolated mode):
    python3 -I check_dictionary.py <detail_dir>

Reads <detail_dir>/selected_lemmas.json (raw AGDT lemma strings of selected identities)
and writes <detail_dir>/dictionary.json. One request per lemma, no retries: a failed
request is recorded as its HTTP status or error. Entry found means HTTP 200 and the
NFC lemma string appears in the page. Source: Kaikki.org Wiktionary extract for
Ancient Greek (public HTML; rights: see the report).
"""
import json
import sys
import unicodedata
import urllib.error
import urllib.parse
import urllib.request
from concurrent.futures import ThreadPoolExecutor

BASE = "https://kaikki.org/dictionary/Ancient%20Greek/meaning/{c1}/{c2}/{name}.html"


def check(lemma):
    nfc = unicodedata.normalize("NFC", lemma)
    url = BASE.format(c1=urllib.parse.quote(nfc[0]), c2=urllib.parse.quote(nfc[:2]),
                      name=urllib.parse.quote(nfc))
    row = {"lemma": nfc, "url_path": url.split("/dictionary/", 1)[1], "http_status": None,
           "entry_found": False, "bytes": 0, "error": None}
    try:
        req = urllib.request.Request(url, headers={"User-Agent": "mouseion-research-probe/2046"})
        with urllib.request.urlopen(req, timeout=60) as resp:
            body = resp.read()
            row["http_status"] = resp.status
    except urllib.error.HTTPError as e:
        row["http_status"] = e.code
        return row
    except Exception as e:  # recorded, not retried
        row["error"] = type(e).__name__
        return row
    row["bytes"] = len(body)
    text = unicodedata.normalize("NFC", body.decode("utf-8", errors="replace"))
    row["entry_found"] = row["http_status"] == 200 and nfc in text
    return row


def main():
    detail_dir = sys.argv[1]
    lemmas = json.load(open(f"{detail_dir}/selected_lemmas.json", encoding="utf-8"))
    with ThreadPoolExecutor(max_workers=6) as pool:
        rows = list(pool.map(check, lemmas))
    out = {r["lemma"]: r for r in rows}
    with open(f"{detail_dir}/dictionary.json", "w", encoding="utf-8") as f:
        json.dump(out, f, ensure_ascii=False, indent=1)
    print(json.dumps({"lemmas": len(out), "entry_found": sum(r["entry_found"] for r in rows),
                      "status": {str(k): sum(1 for r in rows if r["http_status"] == k)
                                 for k in sorted({r["http_status"] for r in rows}, key=str)},
                      "errors": sum(1 for r in rows if r["error"])}, indent=1))


if __name__ == "__main__":
    main()
