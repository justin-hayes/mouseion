"""Sentence-completeness check for one authority route (run after probe.py).

usage: python3 -I completeness.py {reading|printed|hybrid} [flag-suffix]

Two segmentations are compared, and neither is validated by a human:
  * native: Gorman's <sentence> elements (treebank annotation, file/sid);
  * candidate: canonical text split after . ; ! ? (the 'punctuation candidate').
A native sentence counts as independently corroborated only when every one of its units is
'exact', all its mapped canonical spans fall in exactly one candidate, and its first and last
canonical tokens are that candidate's first and last tokens. Nothing here fills gaps or
accepts variant units. Writes WORK/out/completeness_<tag>.json (no source text).
"""
import csv
import json
import os
import re
import sys
from collections import Counter, defaultdict

sys.path.insert(0, os.path.dirname(__file__))
from common import OUT, tokenize, fold_key  # noqa: E402

VARIANT = sys.argv[1]
SUFFIX = sys.argv[2] if len(sys.argv) > 2 else ""
TAG = VARIANT + SUFFIX
TERM = {".", ";", "!", "?"}
MAX_WS, MAX_CP = 50, 400

canon = json.load(open(os.path.join(OUT, "canonical_book1.json"), encoding="utf-8"))
text = canon[VARIANT].rstrip()
toks = tokenize(text)

# Candidate sentences: identical rule to probe.py (ano teleia is medial, not terminal).
cands = []
st = 0
for i, (t, a, b) in enumerate(toks):
    if t in TERM or i == len(toks) - 1:
        span = text[toks[st][1]:b]
        cands.append({"cid": len(cands), "tok_start": st, "tok_end": i,
                      "cp_start": toks[st][1], "cp_end": b, "terminal": t if t in TERM else "",
                      "ws": len(span.split()), "cp": len(span)})
        st = i + 1

rows = list(csv.DictReader(open(os.path.join(OUT, f"units_{TAG}.tsv"), encoding="utf-8"), delimiter="\t"))
cand_rows = list(csv.DictReader(open(os.path.join(OUT, f"candidates_{TAG}.tsv"), encoding="utf-8"), delimiter="\t"))
full_exact = {int(r["cid"]): r["full_exact_coverage"] == "1" for r in cand_rows}

native = defaultdict(list)
for r in rows:
    native[(r["file"], r["sid"])].append(r)

stats = Counter()
corroborated_cands = set()
for key, us in native.items():
    stats["native_sentences"] += 1
    stats["native_units"] += len(us)
    stats["native_exact_units"] += sum(1 for u in us if u["status"] == "exact")
    if any(u["status"] != "exact" for u in us):
        stats["native_with_nonexact_unit"] += 1
        continue
    mapped = [u for u in us if u["cand_id"] != ""]
    if len(mapped) != len(us):
        stats["native_exact_but_unmapped_unit"] += 1
        continue
    cids = {int(u["cand_id"]) for u in mapped}
    if len(cids) != 1:
        stats["native_exact_spanning_candidates"] += 1
        continue
    cid = cids.pop()
    c = cands[cid]
    first = int(mapped[0]["canon_start_cp"])
    last_end = int(max(mapped, key=lambda u: int(u["canon_end_cp"]))["canon_end_cp"])
    if first != c["cp_start"] or last_end != c["cp_end"]:
        stats["native_exact_candidate_boundary_mismatch"] += 1
        # Diagnose: is the native sentence a fragment (missing start or end) of the candidate?
        head = "start-equal" if first == c["cp_start"] else "start-differs"
        tail = "end-equal" if last_end == c["cp_end"] else "end-differs"
        stats[f"boundary_mismatch:{head},{tail}"] += 1
        continue
    if not full_exact.get(cid):
        stats["native_exact_candidate_not_fully_exact"] += 1
        continue
    text_c = text[c["cp_start"]:c["cp_end"]]
    ws, cp = len(text_c.split()), len(text_c)
    stats["corroborated_sentences"] += 1
    corroborated_cands.add(cid)
    if ws <= MAX_WS and cp <= MAX_CP:
        stats["corroborated_within_length_bounds"] += 1

# Candidate-side accounting, to show the gap between candidate and corroborated coverage.
stats["candidates_total"] = len(cands)
stats["candidates_full_exact_by_probe"] = sum(1 for v in full_exact.values() if v)
stats["candidates_full_exact_and_corroborated"] = sum(1 for cid in corroborated_cands if full_exact.get(cid))
stats["candidates_full_exact_not_corroborated"] = (
    stats["candidates_full_exact_by_probe"] - stats["candidates_full_exact_and_corroborated"])

# Identity selection under the corroboration rule. Same identity key, threshold (>=3 nodes) and
# qualifying conditions as probe.py, plus: the occurrence's candidate must be corroborated by an
# exact native sentence. probe.py's 'with qualifying' figure does not require corroboration.
UPOS = {"n": "NOUN", "v": "VERB", "a": "ADJ", "d": "ADV", "l": "DET", "c": "CCONJ|SCONJ",
        "p": "PRON|DET", "r": "ADP", "i": "INTJ", "u": "PUNCT"}
IGNORE_AMBIG = os.environ.get("IGNORE_AMBIG") == "1"
occ = defaultdict(list)
for r in rows:
    if r["is_punct"] == "1":
        continue
    for node in r["nodes"].split("+"):
        form, lemma, postag = node.rsplit("/", 2)
        if postag[0] == "u" or lemma == "[0]":
            continue
        m = re.match(r"^(.*?)(\d*)$", lemma)
        base, disc = m.group(1), m.group(2)
        ident = f"{fold_key(base)}|{UPOS.get(postag[0], '?')}|{disc or '-'}"
        ambig = (not IGNORE_AMBIG) and (postag[0] in ("c", "p") or base == "εἰμί")
        cid = r["cand_id"]
        c = cands[int(cid)] if cid != "" else None
        ok = (r["status"] == "exact" and c is not None and full_exact.get(c["cid"], False)
              and c["terminal"] != "" and c["ws"] <= MAX_WS and c["cp"] <= MAX_CP
              and base.strip() not in ("", "[0]") and not ambig)
        occ[ident].append((ok, ok and c["cid"] in corroborated_cands))
selected = {k: v for k, v in occ.items() if len(v) >= 3}
stats["identities_total"] = len(occ)
stats["identities_selected_ge3"] = len(selected)
stats["identities_selected_with_qualifying_candidate_diagnostic"] = sum(1 for v in selected.values() if any(q for q, _ in v))
stats["identities_selected_with_qualifying_corroborated"] = sum(1 for v in selected.values() if any(c for _, c in v))

# Denominators (counts only).
stats["gorman_units"] = len(rows)
stats["gorman_units_punctuation"] = sum(1 for r in rows if r["is_punct"] == "1")
stats["gorman_units_word"] = sum(1 for r in rows if r["is_punct"] == "0")
stats["gorman_units_word_exact"] = sum(1 for r in rows if r["is_punct"] == "0" and r["status"] == "exact")
stats["gorman_units_status"] = dict(Counter(r["status"] for r in rows))
stats["gorman_merged_units"] = sum(1 for r in rows if r["merged"] == "1")
stats["gorman_nodes_in_units"] = sum(len(r["nodes"].split("+")) for r in rows)
stats["canonical_tokens_all"] = len(toks)
stats["canonical_tokens_word"] = sum(1 for t, _, _ in toks if any(ch.isalpha() for ch in t))
stats["canonical_tokens_punct"] = stats["canonical_tokens_all"] - stats["canonical_tokens_word"]
stats["canonical_whitespace_units"] = len(text.split())
stats["canonical_codepoints"] = len(text)

with open(os.path.join(OUT, f"completeness_{TAG}.json"), "w", encoding="utf-8") as fh:
    json.dump({"route": VARIANT, "flags": SUFFIX or "strict", **stats}, fh, ensure_ascii=False, indent=1)
print(json.dumps({"route": VARIANT, "flags": SUFFIX or "strict", **stats}, ensure_ascii=False, indent=1))
