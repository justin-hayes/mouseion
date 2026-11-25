"""Measure Citation alignment between canonical-latinLit source editions and
their English translations.

Usage: python3 -I align.py REPO_DATA_DIR OLA_MANIFEST_TSV OUT_DIR

Reads only the pinned canonical-latinLit checkout and the aggregate OLA
manifest of Research #2660 (urn, class, text licence). Writes per-Work
aggregates (counts, ids, digests) as JSON and TSV. No third-party text is
written.

Model
-----
A text chunk is a run of words with the Citation state in force: a mapping
unit -> value, set by `div[@type=textpart]` (unit = @subtype), old-style
`div1..div3` (unit = @type), composite `n="text=F:book=5:letter=7"`, and
`milestone` (unit = @unit, value = @n; set when met, never reset), and by
`l/@n` (unit "line").

The source defines the Citation units (the ordered subtypes of its textpart
divs, plus "line" for verse). The translation is projected onto the source
units it actually uses (a unit the translation never has, such as Livy's
"part", is dropped from both sides). A leaf is a Citation that owns text.
Leaves shallower than the translation's word-weighted modal depth that hold
under 3 % of its words are front matter (heads, arguments) and are dropped.

For every source leaf S:
  one    S is a leaf of the translation (same Citation)
  coarse a proper prefix of S is a translation leaf (one translation segment
         answers many source Citations)
  fine   S is not a translation leaf but translation leaves lie under S
  none   nothing equal, containing or under S

`one` only means the same Citation key. `plausible` is the share of `one`
leaves (source >= 5 words) whose English/Latin word ratio is in [0.8, 4.0]:
a translation that merely reuses the numbers but not the content scatters.
For verse the line key is only trusted when the translation's line count is
within 10 % of the source's (`line_key_valid`); otherwise the alignment is
re-measured one level up (`without_line`).
"""
import csv
import hashlib
import json
import re
import statistics
import sys
from collections import Counter, defaultdict
from pathlib import Path
from xml.etree import ElementTree as ET

TEI = "{http://www.tei-c.org/ns/1.0}"


def words(s):
    return len(re.findall(r"\w+", s or ""))


def expand(n):
    m = re.fullmatch(r"(\d+)\s*[-–]\s*(\d+)", n or "")
    if m and 0 < int(m.group(2)) - int(m.group(1)) < 200:
        return [str(i) for i in range(int(m.group(1)), int(m.group(2)) + 1)]
    return [n]


def events(e, state, info):
    """Yield (words, state-copy) in document order."""
    def emit(txt):
        if txt:
            w = words(txt)
            if w:
                return [(w, dict(state))]
        return []

    for c in e:
        tag = c.tag.replace(TEI, "") if isinstance(c.tag, str) else ""
        if tag == "note":
            yield from emit(c.tail)
            continue
        is_div = tag == "div" and c.get("type") == "textpart" and c.get("n") is not None
        is_old = re.fullmatch(r"div\d", tag) and c.get("n") is not None and c.get("type")
        composite = tag == "div" and c.get("n") and "=" in c.get("n")
        if is_div or is_old or composite:
            saved = dict(state)
            if composite and not is_div:
                for kv in c.get("n").split(":"):
                    if "=" in kv:
                        k, v = kv.split("=", 1)
                        state[k] = v
                        if k not in info["div_units"]:
                            info["div_units"].append(k)
            else:
                unit = (c.get("subtype") if is_div else c.get("type")) or "?"
                state[unit] = c.get("n")
                if unit not in info["div_units"]:
                    info["div_units"].append(unit)
            yield from emit(c.text)
            yield from events(c, state, info)
            state.clear(); state.update(saved)
            yield from emit(c.tail)
            continue
        if tag == "milestone":
            u, n = c.get("unit"), c.get("n")
            if u and n is not None:
                state[u] = n
                info["milestone_units"][u] += 1
            yield from emit(c.tail)
            continue
        if tag == "l":
            info["has_l"] = True
            info["l_count"] += 1
            saved = dict(state)
            if c.get("n"):
                state["line"] = c.get("n")
            yield from emit(c.text)
            yield from events(c, state, info)
            state.clear(); state.update(saved)
            yield from emit(c.tail)
            continue
        if tag in ("p", "ab"):
            info["has_p"] = True
        yield from emit(c.text)
        yield from events(c, state, info)
        yield from emit(c.tail)


def load(path):
    raw = Path(path).read_text(encoding="utf-8")
    raw = re.sub(r"&(?!(?:amp|lt|gt|quot|apos|#\d+|#x[0-9a-fA-F]+);)[A-Za-z0-9.]+;", " ", raw)
    root = ET.fromstring(raw.encode("utf-8"))
    body = root.find(f".//{TEI}body")
    if body is None:
        body = root.find(".//body")
    info = {"has_l": False, "has_p": False, "l_count": 0, "milestone_units": Counter(), "div_units": []}
    chunks = list(events(body, {}, info)) if body is not None else []
    return chunks, info


def leaves(chunks, units):
    acc = defaultdict(float)
    for w, st in chunks:
        ref = []
        for u in units:
            if u in st:
                ref.append(st[u])
            else:
                break
        ms = expand(ref[-1]) if ref else [None]
        for m in ms:
            acc[tuple(ref[:-1] + [m]) if ref else ()] += w / len(ms)
    return {k: int(round(v)) for k, v in acc.items()}


def drop_front_matter(lv):
    """Drop leaves shallower than the word-weighted modal depth holding <3 % of words."""
    tot = sum(lv.values()) or 1
    by = Counter()
    for r, w in lv.items():
        by[len(r)] += w
    if not by:
        return lv, 0
    mode = by.most_common(1)[0][0]
    keep = {r: w for r, w in lv.items() if len(r) >= mode or w / tot >= 0.03}
    # shallower leaves under 3 % are removed; deeper ones always kept
    keep = {r: w for r, w in keep.items() if len(r) >= mode or w / tot >= 0.03}
    return keep, mode


def classify(src, tr):
    trset = set(tr)
    under = set()
    for r in tr:
        for i in range(1, len(r)):
            under.add(r[:i])
    cnt = Counter()
    seg = defaultdict(int)
    plaus_n = plaus_ok = 0
    for s, sw in src.items():
        if s in trset:
            cnt["one"] += 1
            if sw >= 5:
                plaus_n += 1
                if 0.8 <= tr[s] / sw <= 4.0:
                    plaus_ok += 1
            continue
        coarse = next((s[:i] for i in range(len(s) - 1, 0, -1) if s[:i] in trset), None)
        if coarse:
            cnt["coarse"] += 1
            seg[coarse] += 1
        elif s in under:
            cnt["fine"] += 1
        else:
            cnt["none"] += 1
    return cnt, seg, plaus_n, plaus_ok


def measure(src_chunks, tr_chunks, units, tr_words_total):
    src, _ = drop_front_matter(leaves(src_chunks, units))
    tr, tmode = drop_front_matter(leaves(tr_chunks, units))
    cnt, seg, pn, pok = classify(src, tr)
    n = sum(cnt.values())
    segw = [tr[k] for k in seg]
    return {
        "units": units,
        "source_leaves": len(src),
        "translation_leaves": len(tr),
        "translation_modal_depth": tmode,
        "one": cnt["one"], "coarse": cnt["coarse"], "fine": cnt["fine"], "none": cnt["none"],
        "share_one": round(cnt["one"] / n, 4) if n else None,
        "share_coarse": round(cnt["coarse"] / n, 4) if n else None,
        "share_fine": round(cnt["fine"] / n, 4) if n else None,
        "share_none": round(cnt["none"] / n, 4) if n else None,
        "plausible_share_of_one": round(pok / pn, 4) if pn else None,
        "median_words_per_translation_leaf": statistics.median(tr.values()) if tr else None,
        "median_words_per_coarse_segment": statistics.median(segw) if segw else None,
        "median_source_leaves_per_coarse_segment": statistics.median(seg.values()) if seg else None,
    }


def header(path):
    t = Path(path).read_text(encoding="utf-8")
    h = t[: t.find("</teiHeader>")] if "</teiHeader>" in t else t[:6000]
    ed = re.findall(r"<editor[^>]*>(.*?)</editor>", h, re.S)
    ed += re.findall(r"<resp>[Tt]ranslat\w*</resp>\s*<name>(.*?)</name>", h, re.S)
    lic = re.search(r"<licence[^>]*>(.*?)</licence>", h, re.S)
    imprint = re.search(r"<imprint>(.*?)</imprint>", h, re.S)
    date = pub = None
    if imprint:
        d = re.search(r"<date[^>]*>(.*?)</date>", imprint.group(1), re.S)
        p = re.search(r"<publisher[^>]*>(.*?)</publisher>", imprint.group(1), re.S)
        date = re.sub(r"\s+", " ", d.group(1)).strip() if d else None
        pub = re.sub(r"\s+", " ", p.group(1)).strip() if p else None
    return {
        "translators_or_editors": sorted({re.sub(r"\s+", " ", x).strip() for x in ed}),
        "print_publisher": pub,
        "print_date": date,
        "licence_element": re.sub(r"\s+", " ", lic.group(1)).strip()[:120] if lic else None,
    }


def cts_names(repo, auth, wk):
    a = (repo / auth / "__cts__.xml")
    w = (repo / auth / wk / "__cts__.xml")
    g = re.search(r"<ti:groupname[^>]*>(.*?)</ti:groupname>", a.read_text(encoding="utf-8"), re.S) if a.exists() else None
    t = None
    if w.exists():
        txt = w.read_text(encoding="utf-8")
        m = re.search(r'<ti:title xml:lang="lat">(.*?)</ti:title>', txt, re.S) or re.search(r"<ti:title[^>]*>(.*?)</ti:title>", txt, re.S)
        t = m.group(1).strip() if m else None
    return (g.group(1).strip() if g else None), t


def main(repo, manifest, out):
    repo, out = Path(repo), Path(out)
    out.mkdir(parents=True, exist_ok=True)
    rows = list(csv.DictReader(open(manifest), delimiter="\t"))
    pool = {}
    for r in rows:
        lic = r["text_licence_instrument"]
        if r["class"] in ("core", "borderline") and "BY-SA" in lic and "NC" not in lic:
            pool[r["urn"].rsplit(".", 1)[0]] = r
    result = {}
    for work, r in sorted(pool.items()):
        auth, wk = work.split(".")
        d = repo / auth / wk
        grp, title = cts_names(repo, auth, wk)
        rec = {"class": r["class"], "author": grp, "title": title, "ola_edition": r["urn"].rsplit(".", 1)[1],
               "ola_words": int(r["words"]), "source_repo": r["text_source_repo"], "translations": []}
        result[work] = rec
        rec["in_perseus_repo"] = d.is_dir()
        if not d.is_dir():
            continue
        lat = d / f"{work}.{rec['ola_edition']}.xml"
        if not lat.exists():
            lats = sorted(d.glob(f"{work}.*-lat*.xml"))
            lat = lats[0] if lats else None
        if lat is None:
            continue
        sch, sinfo = load(lat)
        src_units = list(sinfo["div_units"])
        verse = bool(sinfo["has_l"]) and sinfo["l_count"] > 0 and not sinfo["has_p"]
        if verse and "line" not in src_units:
            src_units.append("line")
        if not verse:
            src_units = [u for u in src_units if u != "line"]  # quoted verse inside prose
        rec.update({"source_file": lat.name, "source_sha256": hashlib.sha256(lat.read_bytes()).hexdigest(),
                    "source_words": sum(w for w, _ in sch), "source_units": src_units,
                    "source_lines": sinfo["l_count"],
                    "source_verse": verse})
        for eng in sorted(d.glob(f"{work}.*-eng*.xml")):
            tch, tinfo = load(eng)
            tw = sum(w for w, _ in tch)
            tunits_avail = set(tinfo["div_units"]) | set(tinfo["milestone_units"]) | ({"line"} if tinfo["has_l"] and verse else set())
            present = [i for i, u in enumerate(src_units) if u in tunits_avail]
            # drop source units the translation lacks above its deepest shared
            # unit (Livy "part"); keep trailing source-only units (Caesar "section")
            units = [u for i, u in enumerate(src_units) if not present or i > present[-1] or u in tunits_avail] if present else []
            entry = {"file": eng.name, "sha256": hashlib.sha256(eng.read_bytes()).hexdigest(), **header(eng),
                     "words": tw, "word_ratio_to_source": round(tw / max(1, rec["source_words"]), 2),
                     "translation_lines": tinfo["l_count"],
                     "translation_div_units": tinfo["div_units"],
                     "translation_milestone_units": dict(tinfo["milestone_units"]),
                     "shared_units": units}
            if units:
                entry["with_line"] = measure(sch, tch, units, tw)
                if "line" in units:
                    ratio = tinfo["l_count"] / max(1, sinfo["l_count"])
                    entry["line_count_ratio"] = round(ratio, 3)
                    entry["line_key_valid"] = abs(ratio - 1) <= 0.10
                    entry["without_line"] = measure(sch, tch, [u for u in units if u != "line"], tw)
            result[work]["translations"].append(entry)
    (out / "alignment.json").write_text(json.dumps(result, indent=1, sort_keys=True, ensure_ascii=False))
    print(len(result), "works;", sum(1 for v in result.values() if v["translations"]), "with a translation file")


if __name__ == "__main__":
    main(*sys.argv[1:4])
