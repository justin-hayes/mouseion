"""Anabasis Book 1 Gorman-vs-canonical probe.

usage: python3 -I probe.py {reading|printed}

  reading  canonical variant R: <del> omitted, <choice> uses <corr>
  printed  canonical variant P: <del> kept as encoded, <choice> uses <sic>

Pipeline (all offline, no provider calls):
  1. Gorman printed nodes (artificial/elliptic nodes excluded from text and counts).
  2. Units: a leading-hyphen piece (clitic, e.g. "-δὲ") is merged into the preceding
     piece to reconstruct one canonical word; both nodes keep their own lemma/POS and
     share the unit span. Trailing-hyphen pieces (crasis-type "τ-") are NOT merged.
  3. Global difflib alignment of unit keys (NFC + Unicode lowercase + final sigma fold for
     words; NFC for punctuation) against canonical tokens. Equal blocks are exact;
     replace blocks are paired positionally and classified by reason.
  4. Candidate sentences from canonical terminal punctuation (. ; ! ?). Ano teleia is medial.
  5. Identities (diagnostic baseline): lemma key + UPOS proposal + homograph digit.
     Selection: >=3 printed nodes in this Book (3/15 rule collapses to this with one Book).
"""
import csv
import difflib
import json
import os
import re
import sys
import unicodedata as ud
from collections import Counter, defaultdict
import xml.etree.ElementTree as ET

sys.path.insert(0, os.path.dirname(__file__))
from common import RAW, OUT, tokenize, fold_key  # noqa: E402

VARIANT = sys.argv[1] if len(sys.argv) > 1 else "reading"
assert VARIANT in ("reading", "printed", "hybrid")
FILES = ["gorman_book1_1-5.xml", "gorman_book1_6-9.xml"]
APOS = {"'", "’", "ʼ", "᾽", "᾿", "̓"}
TERMINALS = {".", ";", "!", "?", ";"}
UPOS = {"n": "NOUN", "v": "VERB", "a": "ADJ", "d": "ADV", "l": "DET", "c": "CCONJ|SCONJ",
        "p": "PRON|DET", "r": "ADP", "i": "INTJ", "u": "PUNCT"}
AMBIG_POS = {"c", "p"}
MAX_WS, MAX_CP, MIN_CAND_WS = 50, 400, 0
# Sensitivity switches (diagnostic only; the strict run sets neither).
GLYPH_OK = os.environ.get("GLYPH_OK") == "1"      # treat U+2019 vs U+02BC elision glyph variants as aligned
IGNORE_AMBIG = os.environ.get("IGNORE_AMBIG") == "1"  # ignore CCONJ/SCONJ, PRON/DET, AUX mapping ambiguity
SUFFIX = ("_glyphok" if GLYPH_OK else "") + ("_ambigignored" if IGNORE_AMBIG else "")


def gorman_nodes():
    sents = []
    for f in FILES:
        for si, s in enumerate(ET.parse(os.path.join(RAW, f)).getroot().findall("sentence")):
            pieces = []
            for w in s.findall("word"):
                a = w.attrib
                pieces.append({"file": f, "sid": s.get("id"), "subdoc": s.get("subdoc"),
                               "wid": a.get("id"), "form": a.get("form"), "lemma": a.get("lemma"),
                               "postag": a.get("postag"), "relation": a.get("relation"),
                               "head": a.get("head"), "artificial": a.get("artificial")})
            sents.append({"file": f, "sid": s.get("id"), "subdoc": s.get("subdoc"),
                          "all_words": len(pieces),
                          "artificial": sum(1 for p in pieces if p["artificial"]),
                          "pieces": [p for p in pieces if not p["artificial"]]})
    return sents


def build_units(sents):
    units = []
    for s in sents:
        cur = None
        for p in s["pieces"]:
            f = p["form"]
            if f.startswith("-") and cur is not None:
                cur["nodes"].append(p)
                cur["text"] += f[1:]
                cur["merged"] = True
                continue
            cur = {"sid": s["sid"], "file": s["file"], "subdoc": s["subdoc"],
                   "nodes": [p], "text": f, "merged": False,
                   "partial_trailing": f.endswith("-")}
            units.append(cur)
    for i, u in enumerate(units):
        u["uid"] = i
        is_p = all(n["postag"][0] == "u" for n in u["nodes"])
        u["is_punct"] = is_p
        u["key"] = ud.normalize("NFC", u["text"]) if is_p else fold_key(u["text"])
    return units


def strip_marks(s):
    return "".join(c for c in ud.normalize("NFD", s) if not ud.combining(c))


def classify_pair(gtext, ctok, g_punct):
    if g_punct or not any(ch.isalpha() for ch in ctok):
        if ud.normalize("NFC", gtext) == ud.normalize("NFC", ctok):
            return "exact"
        return "punctuation-mark-difference"
    if fold_key(gtext) == fold_key(ctok):
        return "exact"
    loose = lambda x: ud.normalize("NFC", "".join(c for c in ud.normalize("NFD", fold_key(x)) if c not in APOS))
    if loose(gtext) == loose(ctok):
        return "elision-or-apostrophe-glyph-variant"
    if strip_marks(fold_key(gtext)) == strip_marks(fold_key(ctok)):
        return "accent-breathing-only-difference"
    if gtext.endswith("-") or ctok.endswith("-"):
        return "partial-piece"
    return "different-token"


def main():
    canon = json.load(open(os.path.join(OUT, "canonical_book1.json"), encoding="utf-8"))
    if VARIANT == "reading":
        text, offs = canon["reading"].rstrip(), canon["offsets"]
    elif VARIANT == "printed":
        text, offs = canon["printed"].rstrip(), canon["offsets_printed"]
    else:
        text, offs = canon["hybrid"].rstrip(), canon["offsets_hybrid"]
    ctoks = tokenize(text)
    cen = [(t, a, b) for (t, a, b) in ctoks]

    def sec_of(pos):
        for o in offs:
            if o["start"] <= pos <= o["end"]:
                return o["ref"]
        return ""

    ckeys = []
    for t, a, b in cen:
        ckeys.append(ud.normalize("NFC", t) if not any(ch.isalpha() for ch in t) else fold_key(t))

    sents = gorman_nodes()
    units = build_units(sents)
    gkeys = [u["key"] for u in units]

    sm = difflib.SequenceMatcher(None, gkeys, ckeys, autojunk=False)
    ops = sm.get_opcodes()
    u_status = {}
    u_reason = {}
    u_cidx = {}
    c_matched = set()
    for tag, i1, i2, j1, j2 in ops:
        if tag == "equal":
            for k in range(i2 - i1):
                u = units[i1 + k]
                u_status[u["uid"]] = "exact"
                u_cidx[u["uid"]] = j1 + k
                c_matched.add(j1 + k)
        elif tag == "replace":
            for k in range(min(i2 - i1, j2 - j1)):
                u = units[i1 + k]
                ci = j1 + k
                r = classify_pair(u["text"], cen[ci][0], u["is_punct"])
                if r == "exact" or (GLYPH_OK and r == "elision-or-apostrophe-glyph-variant"):
                    u_status[u["uid"]] = "exact"
                else:
                    u_status[u["uid"]] = "variant"
                u_reason[u["uid"]] = r
                u_cidx[u["uid"]] = ci
                c_matched.add(ci)
            for k in range(j2 - j1, i2 - i1) if (i2 - i1) > (j2 - j1) else []:
                u = units[i1 + k]
                u_status[u["uid"]] = "unmatched"
                u_reason[u["uid"]] = "no-canonical-counterpart-in-block"
            for k in range(i2 - i1, j2 - j1) if (j2 - j1) > (i2 - i1) else []:
                pass
        elif tag == "delete":
            for k in range(i2 - i1):
                u = units[i1 + k]
                u_status[u["uid"]] = "unmatched"
                u_reason[u["uid"]] = "gorman-only"
        # insert: canonical-only tokens handled below

    # Canonical-only tokens
    c_only = [i for i in range(len(cen)) if i not in c_matched]

    # Candidate sentences over the whole reading string (terminal punctuation).
    cands = []
    start_tok = 0
    for i, (t, a, b) in enumerate(cen):
        if t in TERMINALS or i == len(cen) - 1:
            cands.append({"cid": len(cands), "tok_start": start_tok, "tok_end": i,
                          "cp_start": cen[start_tok][1], "cp_end": b,
                          "terminal": t if t in TERMINALS else "", "section": sec_of(a)})
            start_tok = i + 1
    # Coverage per candidate: canonical tokens matched by Gorman units
    cand_of_tok = {}
    for c in cands:
        covered = 0
        punct_only_tail = True
        tot = 0
        for i in range(c["tok_start"], c["tok_end"] + 1):
            cand_of_tok[i] = c["cid"]
            tot += 1
            if i in c_matched:
                covered += 1
        c["tok_n"] = tot
        c["covered_tok"] = covered
        c["text"] = text[c["cp_start"]:c["cp_end"]].strip()
        c["ws"] = len(c["text"].split())
        c["cp"] = len(c["text"])
        c["sections"] = sorted({sec_of(cen[i][1]) for i in range(c["tok_start"], c["tok_end"] + 1)})
        c["crosses_section"] = len(c["sections"]) > 1
        c["complete_by_punct"] = c["terminal"] != ""
    ##  Native sentences
    unit_by_sid = defaultdict(list)
    for u in units:
        unit_by_sid[u["sid"] + "|" + u["file"]].append(u)

    # Unit -> candidate
    unit_cand = {}
    for u in units:
        ci = u_cidx.get(u["uid"])
        if ci is not None:
            unit_cand[u["uid"]] = cand_of_tok[ci]  # candidate id

    # Candidate-level qualification
    cand_full = {}
    exact_tok = set()
    for u in units:
        if u_status.get(u["uid"]) == "exact" and u["uid"] in u_cidx:
            exact_tok.add(u_cidx[u["uid"]])
    for c in cands:
        # full coverage: every canonical token in the candidate is matched to an exact unit
        full = all(i in exact_tok for i in range(c["tok_start"], c["tok_end"] + 1))
        c["full_exact_coverage"] = full
        cand_full[c["cid"]] = full

    # Native sentence status
    native_rows = []
    for s in sents:
        us = [u for u in units if u["sid"] == s["sid"] and u["file"] == s["file"]]
        stats = Counter(u_status.get(u["uid"]) for u in us)
        mapped = [u_cidx[u["uid"]] for u in us if u["uid"] in u_cidx]
        first_sub = sec_of(cen[min(mapped)][1]) if mapped else ""
        native_rows.append({"file": s["file"], "sid": s["sid"], "subdoc": s["subdoc"],
                            "first_section": first_sub,
                            "subdoc_matches": (first_sub == s["subdoc"]),
                            "units": len(us), "exact": stats.get("exact", 0),
                            "variant": stats.get("variant", 0),
                            "unmatched": stats.get("unmatched", 0),
                            "artificial_nodes": s["artificial"],
                            "all_words": s["all_words"]})

    # Identities and selection (baseline: this Book only; 3/15 => count >= 3)
    nodes = []
    for u in units:
        for n in u["nodes"]:
            if n["postag"][0] == "u" or n["lemma"] == "[0]":
                continue
            m = re.match(r"^(.*?)(\d*)$", n["lemma"])
            base, disc = m.group(1), m.group(2)
            ident = f"{fold_key(base)}|{UPOS.get(n['postag'][0], '?')}|{disc or '-'}"
            nodes.append({"u": u, "n": n, "ident": ident, "base": base, "disc": disc or "",
                          "ambig_pos": (not IGNORE_AMBIG) and (n["postag"][0] in AMBIG_POS or base == "εἰμί")})
    cnt = Counter(x["ident"] for x in nodes)
    sel = {k for k, v in cnt.items() if v >= 3}

    selected_rows = []
    omissions = defaultdict(Counter)
    for k in sorted(sel, key=lambda k: (-cnt[k], k)):
        occ = [x for x in nodes if x["ident"] == k]
        first_ok = None
        reasons = Counter()
        for x in occ:
            u = x["u"]
            r = []
            c = None
            if u["is_punct"]:
                continue
            st = u_status.get(u["uid"])
            if st != "exact":
                r.append(f"alignment:{u_reason.get(u['uid'], st)}")
            ci = u_cidx.get(u["uid"])
            if ci is None:
                r.append("no-canonical-span")
            else:
                c = cands[cand_of_tok[ci]]
                if not cand_full.get(c["cid"]):
                    r.append("candidate-sentence-not-fully-aligned")
                if not c["complete_by_punct"]:
                    r.append("no-terminal-punctuation")
                if c["ws"] > MAX_WS:
                    r.append("length:whitespace-units>50")
                if c["cp"] > MAX_CP:
                    r.append("length:codepoints>400")
            if x["base"].strip() in ("", "[0]"):
                r.append("no-headword")
            if x["ambig_pos"]:
                r.append("pos-or-lemma-mapping-ambiguous")
            if not r:
                first_ok = (x, c)
                break
            for why in r:
                reasons[why] += 1
        omissions[k] = reasons
        selected_rows.append({"ident": k, "count": cnt[k],
                              "earliest_qualifying": (
                                  None if first_ok is None else {
                                      "sid": first_ok[0]["u"]["sid"], "subdoc": first_ok[0]["u"]["subdoc"],
                                      "form": first_ok[0]["n"]["form"],
                                      "cand_ws": first_ok[1]["ws"], "cand_cp": first_ok[1]["cp"],
                                      "cand_text": first_ok[1]["text"]}),
                              "reasons_before_first": dict(reasons)})

    # ---- Outputs
    os.makedirs(OUT, exist_ok=True)
    tag = VARIANT + SUFFIX
    with open(os.path.join(OUT, f"units_{tag}.tsv"), "w", encoding="utf-8", newline="") as fh:
        w = csv.writer(fh, delimiter="\t")
        w.writerow(["uid", "file", "sid", "subdoc", "text", "nodes", "merged", "partial_trailing",
                    "is_punct", "status", "reason", "canon_token", "canon_start_cp", "canon_end_cp",
                    "canon_section", "cand_id"])
        for u in units:
            ci = u_cidx.get(u["uid"])
            ct = cen[ci] if ci is not None else ("", "", "")
            w.writerow([u["uid"], u["file"], u["sid"], u["subdoc"], u["text"],
                        "+".join(f"{n['form']}/{n['lemma']}/{n['postag']}" for n in u["nodes"]),
                        int(u["merged"]), int(u["partial_trailing"]), int(u["is_punct"]),
                        u_status.get(u["uid"], "unmatched"), u_reason.get(u["uid"], ""),
                        ct[0], ct[1], ct[2], sec_of(ct[1]) if ci is not None else "",
                        unit_cand[u["uid"]] if u["uid"] in unit_cand else ""])
    with open(os.path.join(OUT, f"canon_only_{tag}.tsv"), "w", encoding="utf-8", newline="") as fh:
        w = csv.writer(fh, delimiter="\t")
        w.writerow(["c_idx", "token", "start_cp", "end_cp", "section"])
        for i in c_only:
            w.writerow([i, cen[i][0], cen[i][1], cen[i][2], sec_of(cen[i][1])])
    with open(os.path.join(OUT, f"candidates_{tag}.tsv"), "w", encoding="utf-8", newline="") as fh:
        w = csv.writer(fh, delimiter="\t")
        w.writerow(["cid", "sections", "crosses_section", "terminal", "tok_n", "covered_tok",
                    "full_exact_coverage", "ws_units", "codepoints", "text"])
        for c in cands:
            w.writerow([c["cid"], ";".join(c["sections"]), int(c["crosses_section"]), c["terminal"],
                        c["tok_n"], c["covered_tok"], int(cand_full[c["cid"]]), c["ws"], c["cp"], c["text"]])
    with open(os.path.join(OUT, f"native_sentences_{tag}.tsv"), "w", encoding="utf-8", newline="") as fh:
        w = csv.DictWriter(fh, fieldnames=list(native_rows[0].keys()), delimiter="\t")
        w.writeheader()
        w.writerows(native_rows)
    with open(os.path.join(OUT, f"identities_{tag}.tsv"), "w", encoding="utf-8", newline="") as fh:
        w = csv.writer(fh, delimiter="\t")
        w.writerow(["identity", "count", "selected", "earliest_qualifying", "omission_reasons"])
        for k, v in sorted(cnt.items(), key=lambda kv: (-kv[1], kv[0])):
            sr = next((r for r in selected_rows if r["ident"] == k), None)
            eq = sr["earliest_qualifying"] if sr else None
            w.writerow([k, v, int(k in sel),
                        json.dumps(eq, ensure_ascii=False) if eq else "",
                        json.dumps(dict(omissions[k]), ensure_ascii=False) if k in sel else ""])

    summary = {
        "variant": VARIANT,
        "flags": SUFFIX or "strict",
        "canonical_codepoints": len(text),
        "canonical_tokens": len(cen),
        "gorman_sentences": len(sents),
        "gorman_nodes_all_words": sum(s["all_words"] for s in sents),
        "gorman_artificial_nodes": sum(s["artificial"] for s in sents),
        "gorman_printed_units": len(units),
        "gorman_printed_nodes": sum(len(u["nodes"]) for u in units),
        "gorman_merged_clitic_units": sum(1 for u in units if u["merged"]),
        "gorman_trailing_hyphen_units": sum(1 for u in units if u["partial_trailing"]),
        "unit_status": dict(Counter(u_status.get(u["uid"], "unmatched") for u in units)),
        "unit_reasons": dict(Counter(u_reason.get(u["uid"], "") for u in units if u_status.get(u["uid"]) != "exact")),
        "canonical_only_tokens": len(c_only),
        "canonical_only_sample": [cen[i][0] for i in c_only[:20]],
        "candidate_sentences": len(cands),
        "candidate_full_exact": sum(1 for c in cands if cand_full[c["cid"]]),
        "candidate_ws_distribution_full": dict(Counter(
            ("<6" if c["ws"] < 6 else "6-50" if c["ws"] <= 50 else ">50") for c in cands)),
        "candidate_cp_over_400": sum(1 for c in cands if c["cp"] > 400),
        "candidate_ws_over_50": sum(1 for c in cands if c["ws"] > 50),
        "candidate_short_complete_lt6_full_exact": sum(1 for c in cands if c["ws"] < 6 and cand_full[c["cid"]]),
        "candidate_crossing_section": sum(1 for c in cands if c["crosses_section"]),
        "candidate_no_terminal": sum(1 for c in cands if not c["complete_by_punct"]),
        "native_sentences_subdoc_matches": sum(1 for r in native_rows if r["subdoc_matches"]),
        "native_sentences_exact_all": sum(1 for r in native_rows if r["unmatched"] == 0 and r["variant"] == 0),
        "native_sentences_with_variant": sum(1 for r in native_rows if r["variant"] > 0),
        "native_sentences_with_unmatched": sum(1 for r in native_rows if r["unmatched"] > 0),
        "identities_total": len(cnt),
        "identities_with_disc": sum(1 for k in cnt if not k.endswith("|-")),
        "identities_selected_ge3": len(sel),
        "occurrences_selected": sum(cnt[k] for k in sel),
        "identities_selected_with_qualifying": sum(1 for r in selected_rows if r["earliest_qualifying"]),
        "identities_selected_without_qualifying": [r["ident"] for r in selected_rows if not r["earliest_qualifying"]],
    }
    json.dump(summary, open(os.path.join(OUT, f"summary_{tag}.json"), "w", encoding="utf-8"),
              ensure_ascii=False, indent=1)
    print(json.dumps(summary, ensure_ascii=False, indent=1))


if __name__ == "__main__":
    main()
