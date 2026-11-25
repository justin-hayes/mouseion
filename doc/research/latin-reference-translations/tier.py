"""Rank the OLA BY-SA Works by translation readiness.

Usage: python3 -I tier.py results/alignment.json OUT_DIR

Reads alignment.json (from align.py) and writes summary.json and
works.tsv (ids, tiers and counts only).

Per translation, the usable measure is `with_line` for prose and for verse
whose line count matches the source (`line_key_valid`), else `without_line`
(the Citation scheme one level up). Tiers, applied to the best translation:

  A  >= 90 % of source Citations share a key with a translation segment
     ("one"), prose or verse with a valid line key. (Plausibility among keyed
     leaves >= 90 %.)
  B  >= 90 % covered, but at a coarser level than the source Citation (or
     verse without a valid line key, measured one level up), and the median
     translation segment <= 400 words (chapter, scene, short poem).
  C  >= 90 % covered with a median segment > 400 words (a book or long
     poem), or 50-90 % covered, or keyed leaves 70-90 % plausible.
  D  < 50 % covered, or < 70 % plausible among keyed leaves (numbering does
     not follow the source).
  -  no translation file in canonical-latinLit.
"""
import json
import sys
from collections import Counter, defaultdict
from pathlib import Path


def pd_by_print_date(date):
    if not date:
        return "unknown"
    import re
    m = re.search(r"(\d{4})", date)
    if not m:
        return "unknown"
    y = int(m.group(1))
    return "pre-1931" if y <= 1930 else "1931-or-later"


def usable(t):
    if "with_line" not in t:
        return None
    if "line_key_valid" in t and not t["line_key_valid"]:
        m = dict(t["without_line"])
        m["level"] = "without_line"
    else:
        m = dict(t["with_line"])
        m["level"] = "with_line"
    return m


def tier_of(m, verse_container):
    n = m["one"] + m["coarse"] + m["fine"] + m["none"]
    if not n:
        return "D"
    cov = (m["one"] + m["coarse"] + m["fine"]) / n
    pl = m["plausible_share_of_one"]
    keyed = m["one"] / n >= 0.5
    if cov < 0.5 or (keyed and pl is not None and pl < 0.7):
        return "D"
    if cov < 0.9 or (keyed and pl is not None and pl < 0.9):
        return "C"
    gran = m["median_words_per_translation_leaf"] or 0
    if m["one"] / n >= 0.9 and not verse_container:
        return "A"
    return "B" if gran <= 400 else "C"


def main(path, out):
    d = json.load(open(path))
    rows = []
    for k, v in sorted(d.items()):
        best = None
        for t in v["translations"]:
            m = usable(t)
            if not m:
                continue
            tr = tier_of(m, m["level"] == "without_line")
            key = ("ABCD".index(tr), pd_by_print_date(t["print_date"]) != "pre-1931", -(m["one"] + m["coarse"] + m["fine"]))
            if best is None or key < best[0]:
                best = (key, t, m, tr)
        row = {"work": k, "class": v["class"], "author": v["author"], "title": v["title"],
               "verse": v.get("source_verse"), "source_words": v.get("source_words", v["ola_words"]),
               "in_perseus_repo": v["in_perseus_repo"], "translation_files": len(v["translations"])}
        if best:
            _, t, m, tr = best
            row.update({"tier": tr, "file": t["file"], "sha256": t["sha256"],
                        "translators_or_editors": t["translators_or_editors"],
                        "print_date": t["print_date"], "print_date_class": pd_by_print_date(t["print_date"]),
                        "licence_element": bool(t["licence_element"]),
                        "level": m["level"], "units": m["units"], "one": m["one"], "coarse": m["coarse"],
                        "fine": m["fine"], "none": m["none"], "plausible": m["plausible_share_of_one"],
                        "median_words_per_coarse_segment": m["median_words_per_coarse_segment"],
                        "median_words_per_translation_leaf": m["median_words_per_translation_leaf"],
                        "word_ratio": t["word_ratio_to_source"]})
        else:
            row["tier"] = "-"
        rows.append(row)
    out = Path(out)
    (out / "works.json").write_text(json.dumps(rows, indent=1, ensure_ascii=False))
    import csv
    with open(out / "works.tsv", "w", newline="") as f:
        w = csv.writer(f, delimiter="\t")
        cols = ["work", "class", "author", "title", "verse", "source_words", "tier", "file", "print_date", "print_date_class", "licence_element", "level", "units", "one", "coarse", "fine", "none", "plausible", "median_words_per_coarse_segment", "median_words_per_translation_leaf", "word_ratio"]
        w.writerow(cols)
        for r in rows:
            w.writerow([("/".join(r[c]) if c == "units" and c in r else r.get(c, "")) for c in cols])
    # aggregates
    summ = {"works": len(rows), "by_class": dict(Counter(r["class"] for r in rows))}
    for label, sel in (("all", rows), ("core", [r for r in rows if r["class"] == "core"])):
        a = {}
        a["works"] = len(sel)
        a["words"] = sum(r["source_words"] for r in sel)
        for kind, f in (("prose", lambda r: not r["verse"]), ("verse", lambda r: r["verse"])):
            s = [r for r in sel if f(r)]
            c = Counter(r["tier"] for r in s)
            wsum = defaultdict(int)
            for r in s:
                wsum[r["tier"]] += r["source_words"]
            a[kind] = {"works": len(s), "by_tier_works": dict(sorted(c.items())), "by_tier_words": dict(sorted(wsum.items()))}
        summ[label] = a
    # by author
    byauth = defaultdict(lambda: Counter())
    for r in rows:
        byauth[r["author"]][r["tier"]] += 1
    summ["by_author"] = {k: dict(v) for k, v in sorted(byauth.items(), key=lambda kv: str(kv[0]))}
    (out / "summary.json").write_text(json.dumps(summ, indent=1, ensure_ascii=False))
    print(json.dumps({k: summ[k] for k in ("works", "by_class", "core")}, indent=1))


if __name__ == "__main__":
    main(*sys.argv[1:3])
