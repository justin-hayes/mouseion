"""PROTOTYPE, throwaway (#2609): constructions and syntax in Sentence study.

Renders real OGA v0.2.0 sentences through the #2615 v2 detectors with the #2608
shipping gate into static HTML pages shaped like the shipped Sentence study page
(shell copied from cmd/fixtureserver output, compiled app.css). Three extent
markings are switchable with ?variant=A|B|C; ?focus=<construction id> shows the
focused-construction state Concordance's Study link returns to.

Usage (from ~/.cache/scratch/grc-syntax):
    python3 -I proto-2609/gen.py && cd proto-2609 && python3 -m http.server 18610
"""
import html
import json
import os
import re
import sys

ROOT = os.path.expanduser("~/.cache/scratch/grc-syntax")
sys.path.insert(0, ROOT)
sys.path.insert(0, os.path.join(ROOT, "v2"))
import detect2 as d  # noqa: E402

OUT = os.path.join(ROOT, "proto-2609")
E = html.escape

# ------------------------------------------------------------------ scenarios (real sentences)
SCENARIOS = [
    {"key": "nested", "file": "tlg0059.tlg004", "sid": "827", "target": 8,
     "book": "Phaedo", "author": "Plato", "chapter": "Phaedo 90", "ordinal": 827,
     "title": "Two constructions, one nested (and a discontinuous extent)"},
    {"key": "subtype", "file": "tlg0032", "sid": "1319", "target": 9,
     "book": "Anabasis, Book 3", "author": "Xenophon", "chapter": "3.3.16", "ordinal": 1319,
     "title": "Conditional containing a natural result clause"},
    {"key": "ellipsis", "file": "tlg0032", "sid": "843", "target": 8,
     "book": "Anabasis, Book 2", "author": "Xenophon", "chapter": "2.4.6", "ordinal": 843,
     "title": "Head through an elided word (ὅτι clause stays silent)"},
    {"key": "none", "file": "tlg0032", "sid": "8", "target": 3,
     "book": "Anabasis, Book 1", "author": "Xenophon", "chapter": "1.1.3", "ordinal": 8,
     "title": "No constructions (silent)"},
]

# ------------------------------------------------------------------ concept notes (DRAFT Mouseion wording)
NOTES = {
    "conditional": ("Conditional clause", "An “if” clause, introduced by εἰ or ἐάν, that states the condition the rest of the sentence depends on.", "2280–2368", 2280),
    "articular_infinitive": ("Articular infinitive", "An infinitive used as a noun. The neuter article before it (τό, τοῦ, τῷ) shows its case.", "2025–2034", 2025),
    "result": ("Result clause", "A clause introduced by ὥστε that says what follows from the main action.", "2249–2278", 2249),
    "purpose": ("Purpose clause", "A clause introduced by ἵνα, with a subjunctive or optative verb, that says what the main action is for.", "2193–2206", 2193),
    "genitive_absolute": ("Genitive absolute", "A participle and its own subject, both in the genitive, set beside the sentence to give its circumstances: when, since or although.", "2070–2075", 2070),
    "indirect_statement": ("Indirect statement", "Reports what someone says or thinks. After a verb of saying or thinking, the reported verb becomes an infinitive or follows ὅτι or ὡς.", "2016–2024", 2016),
}
SUBTYPE_NAMES = {("result", "natural"): "natural result", ("indirect_statement", "infinitive"): "infinitive",
                 ("articular_infinitive", "dat"): "dative", ("articular_infinitive", "gen"): "genitive"}
SMYTH = "https://www.perseus.tufts.edu/hopper/text?doc=Perseus:text:1999.04.0007:smythp={}"

# ------------------------------------------------------------------ plain-English AGDT relation names (DRAFT)
RELATIONS = {
    "PRED": "main verb", "SBJ": "subject", "OBJ": "object", "ATR": "describes a noun",
    "ADV": "adverbial (how, when, why…)", "PNOM": "predicate noun or adjective", "OCOMP": "object complement",
    "ATV": "predicative complement", "AtvV": "predicative complement", "APOS": "in apposition",
    "COORD": "joins coordinated words", "AuxC": "subordinating conjunction", "AuxP": "preposition",
    "AuxY": "sentence particle", "AuxZ": "emphasising or negating particle", "AuxV": "auxiliary",
    "ExD": "stands outside the sentence structure", "AuxX": "comma", "AuxK": "end punctuation", "AuxG": "punctuation",
}
PUNCT = {"AuxX", "AuxK", "AuxG"}

# ------------------------------------------------------------------ morphology labels (rough, shipped style)
NUM = {"s": "SG", "p": "PL", "d": "DU"}
GEN = {"m": "M", "f": "F", "n": "N"}
CASE = {"n": "NOM", "g": "GEN", "d": "DAT", "a": "ACC", "v": "VOC"}
TENSE = {"p": "PRS", "i": "IPF", "f": "FUT", "a": "AOR", "r": "PRF", "l": "PLPF"}
MOOD = {"i": "IND", "s": "SBJV", "o": "OPT", "m": "IMP", "n": "INF", "p": "PTCP"}
VOICE = {"a": "ACT", "m": "MID", "p": "PASS", "e": "MID/PASS"}
KEY = {"SG": "singular", "PL": "plural", "M": "masculine", "F": "feminine", "N": "neuter", "NOM": "nominative",
       "GEN": "genitive", "DAT": "dative", "ACC": "accusative", "VOC": "vocative", "PRS": "present",
       "IPF": "imperfect", "FUT": "future", "AOR": "aorist", "PRF": "perfect", "IND": "indicative",
       "SBJV": "subjunctive", "OPT": "optative", "IMP": "imperative", "INF": "infinitive", "PTCP": "participle",
       "ACT": "active", "MID": "middle", "PASS": "passive", "MID/PASS": "middle or passive",
       "1PL": "first person plural", "2SG": "second person singular", "3SG": "third person singular",
       "3PL": "third person plural", "1SG": "first person singular"}


def labels(n):
    p = n.pos
    if p[0] == "v":
        if p[4] == "p":
            parts = [NUM.get(p[2]), GEN.get(p[6]), CASE.get(p[7]), TENSE.get(p[3]), "PTCP", VOICE.get(p[5])]
        elif p[4] == "n":
            parts = [TENSE.get(p[3]), "INF", VOICE.get(p[5])]
        else:
            parts = [(p[1] + NUM.get(p[2], "")) if p[1] != "-" else None, TENSE.get(p[3]), MOOD.get(p[4]), VOICE.get(p[5])]
    elif p[0] in "nalp":
        parts = [NUM.get(p[2]), GEN.get(p[6]), CASE.get(p[7])]
    else:
        parts = []
    return [x for x in parts if x]


# ------------------------------------------------------------------ detection with the #2608 gate
LABELLED = set(SUBTYPE_NAMES)


def shipped(n):
    for name, fn in d.DETECTORS.items():
        r = fn(n)
        if not r:
            continue
        stratum, sub = r
        if name == "conditional" and stratum == "excluded":
            stratum = "hit"
        if stratum == "hit":
            yield name, sub if (name, sub) in LABELLED else None


def load(sc):
    path = next(os.path.join(ROOT, "data", f) for f in os.listdir(os.path.join(ROOT, "data")) if f.startswith(sc["file"]))
    s = next(s for s in d.read(path) if s["meta"].get("sent_id") == sc["sid"])
    real = [n for n in s["nodes"] if not n.art]
    occ = []
    for n in s["nodes"]:
        for name, sub in shipped(n):
            ext = {x.i for x in d.subtree(n) if not x.art}
            occ.append({"con": name, "sub": sub, "anchor": n.i, "extent": ext})
    pos = {n.i: k for k, n in enumerate(real)}
    occ.sort(key=lambda o: (min(pos[i] for i in o["extent"]), -len(o["extent"])))
    for k, o in enumerate(occ, 1):
        o["num"] = k
        o["id"] = f"c{k}"
        o["runs"] = runs(real, o["extent"])
        o["tier"] = 0
    for o in occ:  # nested ones go one tier deeper per enclosing occurrence
        o["tier"] = sum(1 for p in occ if p is not o and o["extent"] < p["extent"])
        o["parents"] = [p["num"] for p in occ if p is not o and o["extent"] < p["extent"]]
    return s, real, occ


def runs(real, ext):
    """Maximal runs of extent members; a gap made only of punctuation is bridged."""
    out, cur, gap = [], [], []
    for n in real:
        if n.i in ext:
            if cur and gap:
                cur.extend(gap)
            gap = []
            cur.append(n.i)
        elif cur and n.rel in PUNCT:
            gap.append(n.i)
        elif cur:
            out.append(cur)
            cur, gap = [], []
    if cur:
        out.append(cur)
    return out


def joined(tokens):
    """Source-like spacing: no space before punctuation."""
    out = ""
    for k, n in enumerate(tokens):
        if k and n.rel not in PUNCT and n.form not in "·":
            out += " "
        out += n.form
    return out


# ------------------------------------------------------------------ rendering pieces
def name_of(o):
    base = NOTES[o["con"]][0]
    sub = SUBTYPE_NAMES.get((o["con"], o["sub"]))
    return base, sub


def parallels_url(o, sc):
    q = f"/vocabulary/concordance?match=construction&construction={o['con']}"
    if o["sub"]:
        q += f"&subtype={o['sub']}"
    return q + f"&origin={sc['file']}:{sc['sid']}:{o['anchor']}"


def anchor_form(real, o):
    return next(n.form for n in real if n.i == o["anchor"])


def target_relation(o, sc):
    if o["anchor"] == sc["target"]:
        return "The identified target signals this construction."
    if sc["target"] in o["extent"]:
        return "Contains the identified target."
    return None


def extent_text(real, o):
    pieces = [joined([n for n in real if n.i in set(r)]) for r in o["runs"]]
    return " … ".join(pieces)


def concept(o):
    name, definition, rng, first = NOTES[o["con"]]
    return (f'<p class="cx-definition">{E(definition)} '
            f'<a href="{SMYTH.format(first)}" rel="noopener noreferrer" target="_blank">Smyth 1920 §§{rng}'
            f'<span class="visually-hidden"> (opens Perseus in a new tab)</span></a></p>')


def entry(real, o, sc, variant):
    base, sub = name_of(o)
    label = f"{base}" + (f" <span class='cx-subtype'>({E(sub)})</span>" if sub else "")
    rel = target_relation(o, sc)
    inside = f" Inside construction {', '.join(map(str, o['parents']))}." if o["parents"] else ""
    marker = f'<span class="cx-num" aria-hidden="true">{o["num"]}</span>' if variant != "C" else ""
    plinks = f'<a class="cx-parallels" href="{E(parallels_url(o, sc))}">Parallels<span class="visually-hidden"> for {E(base)}{(", " + E(sub)) if sub else ""}</span></a>'
    words = ""
    if variant == "C":
        words = f'<p class="reading-text cx-extract" aria-hidden="true">{sentence_C(real, o, sc)}</p>'
    return (f'<li class="cx-entry" id="construction-{o["id"]}" data-cx="{o["id"]}">'
            f'<h4 class="cx-name">{marker}{label}<span class="cx-focus-note"> · from Concordance</span></h4>'
            f'{words}'
            f'<p class="cx-meta">Signalled by <strong lang="grc">{E(anchor_form(real, o))}</strong>.{(" " + rel) if rel else ""}{inside}</p>'
            f'<p class="cx-extent-words{" visually-hidden" if variant == "C" else ""}"><span class="cx-extent-label">Words: </span><span lang="grc">{E(extent_text(real, o))}</span></p>'
            f'{concept(o)}'
            f'<p class="cx-actions">{plinks}</p></li>')


def entries(real, occ, sc, variant):
    if not occ:
        return ""  # silent abstention: the region is absent
    lis = "".join(entry(real, o, sc, variant) for o in occ)
    return (f'<section class="cx-region" aria-labelledby="cx-heading-{variant}">'
            f'<h3 id="cx-heading-{variant}">Constructions</h3><ol class="cx-list">{lis}</ol></section>')


# Variant A: numbered brackets in the sentence text
def sentence_A(real, occ, sc):
    opens, closes = {}, {}
    for o in occ:
        for r in o["runs"]:
            opens.setdefault(r[0], []).append(o)
            closes.setdefault(r[-1], []).append(o)
    out = ""
    for k, n in enumerate(real):
        if k and n.rel not in PUNCT:
            out += " "
        for o in sorted(opens.get(n.i, []), key=lambda o: o["tier"]):
            out += f'<span class="cxA-open" data-cx="{o["id"]}" aria-hidden="true">[<sup>{o["num"]}</sup></span>'
        out += word_html(n, occ, sc)
        for o in sorted(closes.get(n.i, []), key=lambda o: -o["tier"]):
            out += f'<span class="cxA-close" data-cx="{o["id"]}" aria-hidden="true">]</span>'
    return out


def word_html(n, occ, sc):
    cls = []
    anchors = [o for o in occ if o["anchor"] == n.i]
    if anchors:
        cls.append("cx-anchor")
    attrs = f' data-anchor-of="{" ".join(o["id"] for o in anchors)}"' if anchors else ""
    if n.i == sc["target"]:
        return f'<strong class="sentence-study-target {" ".join(cls)}"{attrs}>{E(n.form)}</strong>'
    if cls:
        return f'<span class="{" ".join(cls)}"{attrs}>{E(n.form)}</span>'
    return E(n.form)


# Variant B: tiered rules beneath the interlinear morphology line
def line_B(real, occ, sc):
    depth = max((o["tier"] for o in occ), default=-1) + 1
    style = ["solid", "double", "dashed"]
    out = ""
    for k, n in enumerate(real):
        if n.rel in PUNCT:
            out += f'<span class="sentence-study-unit cxB-unit"><span class="sentence-study-word">{E(n.form)}</span><span class="sentence-study-labels">&nbsp;</span>'
        else:
            lab = ".".join(labels(n))
            w = f'<strong class="sentence-study-word">{E(n.form)}</strong>' if n.i == sc["target"] else f'<span class="sentence-study-word">{E(n.form)}</span>'
            anchors = [o for o in occ if o["anchor"] == n.i]
            if anchors:
                w = w.replace('sentence-study-word"', f'sentence-study-word cx-anchor" data-anchor-of="{" ".join(o["id"] for o in anchors)}"')
            out += f'<span class="sentence-study-unit cxB-unit">{w}<span class="sentence-study-labels">{E(lab) or "&nbsp;"}</span>'
        for t in range(depth):
            o = next((o for o in occ if o["tier"] == t and any(n.i in r for r in o["runs"])), None)
            if o is None:
                out += '<span class="cxB-rule cxB-empty" aria-hidden="true">&nbsp;</span>'
            else:
                first = any(r[0] == n.i for r in o["runs"])
                tag = f'{o["num"]} {E(NOTES[o["con"]][0])}' if first else "&nbsp;"
                out += f'<span class="cxB-rule cxB-{style[t % 3]}{" cxB-start" if first else ""}" data-cx="{o["id"]}" aria-hidden="true">{tag}</span>'
        out += "</span>"
    return f'<p class="sentence-study-line cxB-line">{out}</p>'


# Variant C: per-entry extract, extent set bold and the rest muted
def sentence_C(real, o, sc):
    out = ""
    ext = {i for r in o["runs"] for i in r}
    for k, n in enumerate(real):
        if k and n.rel not in PUNCT:
            out += " "
        f = E(n.form)
        if n.i == o["anchor"]:
            f = f'<span class="cx-anchor">{f}</span>'
        out += f'<b class="cxC-in">{f}</b>' if n.i in ext else f'<span class="cxC-out">{f}</span>'
    return out


def morph_line(real, sc):
    out = ""
    for k, n in enumerate(real):
        if n.rel in PUNCT:
            out += f'<span class="sentence-study-source">{E(n.form)}</span>'
            continue
        if k:
            out += '<span class="sentence-study-source"> </span>'
        lab = ".".join(labels(n))
        w = f'<strong class="sentence-study-word">{E(n.form)}</strong>' if n.i == sc["target"] else f'<span class="sentence-study-word">{E(n.form)}</span>'
        out += f'<span class="sentence-study-unit">{w}' + (f' <span class="sentence-study-labels">{E(lab)}</span>' if lab else "") + "</span>"
    return f'<p class="sentence-study-line">{out}</p>'


def notation_key(real):
    used = sorted({p for n in real for lab in labels(n) for p in ([lab] if lab in KEY else lab.split("/"))})
    dl = "".join(f"<dt>{E(u)}</dt><dd>{E(KEY.get(u, u))}</dd>" for u in used if u in KEY)
    return (f'<details class="sentence-study-key"><summary>Notation key</summary><dl>{dl}</dl>'
            '<p>An unannotated word had no core features supplied for it here; that does not mean the word lacks those features.</p></details>')


def head_desc(n, by, real_pos):
    h = by.get(n.h)
    if h is None:
        return "— <span class='metadata'>(top of the sentence)</span>"
    if not h.art:
        return f'<span lang="grc">{E(h.form)}</span> <span class="metadata">(word {real_pos[h.i] + 1})</span>'
    # elided head: name it, then walk up to the first printed word
    chain = h
    while chain is not None and chain.art:
        chain = by.get(chain.h)
    up = (f', which is the {E(RELATIONS.get(h.rel, h.rel))} of <span lang="grc">{E(chain.form)}</span> '
          f'<span class="metadata">(word {real_pos[chain.i] + 1})</span>') if chain is not None else ", the top of the sentence"
    return f'<span class="cx-elided">an unprinted word</span>{up}'


def relations(s, real, sc):
    by = {n.i: n for n in s["nodes"]}
    real_pos = {n.i: k for k, n in enumerate(real)}
    rows, has_elided = "", False
    for n in real:
        if n.rel in PUNCT:
            continue
        h = by.get(n.h)
        has_elided |= h is not None and h.art
        word = f"<strong>{E(n.form)}</strong>" if n.i == sc["target"] else E(n.form)
        tgt = '<span class="visually-hidden"> (identified target)</span>' if n.i == sc["target"] else ""
        rows += (f'<tr><td class="numeric">{real_pos[n.i] + 1}</td><th scope="row" lang="grc">{word}{tgt}</th>'
                 f'<td>{E(RELATIONS.get(n.rel, n.rel))}</td><td>{head_desc(n, by, real_pos)}</td></tr>')
    note = ("<p class='sentence-study-note'>An <span class='cx-elided'>unprinted word</span> is one the sentence leaves out "
            "and the reader supplies, often a form of “to be”. The analysis records that it is there but not what it is, "
            "so words that depend on it are shown against it and its own place in the sentence.</p>") if has_elided else ""
    return (f'<details id="sentence-study-relations" class="sentence-study-key cx-relations relations"><summary>Word relations</summary>'
            f'<p class="sentence-study-note">How each word connects to the word it depends on, from the imported analysis. '
            f'Punctuation is left out.</p>'
            f'<table class="cx-rel-table"><thead><tr><th scope="col">#</th><th scope="col">Word</th><th scope="col">Relation</th>'
            f'<th scope="col">Depends on</th></tr></thead><tbody>{rows}</tbody></table>{note}</details>')


def text_alternative(real, occ, sc):
    lis = ""
    for k, n in enumerate(real):
        t = " — identified target" if n.i == sc["target"] else ""
        cx = [f"{NOTES[o['con']][0]} {o['num']}" + (" (signals it)" if o["anchor"] == n.i else "") for o in occ if any(n.i in r for r in o["runs"]) and n.rel not in PUNCT]
        cxs = f"; in {', '.join(cx)}" if cx else ""
        lis += f'<li><strong lang="grc">{E(n.form)}{t}</strong> <span>(token {k + 1})</span>: <code>{E(n.pos[0])}</code>, morphology <code>{E(".".join(labels(n)) or "—")}</code>{E(cxs)}.</li>'
    return (f'<details class="sentence-study-alternative"><summary>Complete text alternative</summary>'
            f'<ol class="sentence-study-tokens">{lis}</ol></details>')


def concordance_preview(real, occ, sc, variant):
    if not occ:
        return ""
    o = occ[-1] if len(occ) > 1 else occ[0]
    if variant == "A":
        body = sentence_A(real, [o], sc)
    elif variant == "B":
        one = dict(o, tier=0)
        body = line_B(real, [one], sc)
        return (f'<aside class="proto-frame"><p class="proto-tag">PROTOTYPE: the same marking reused in a Concordance '
                f'construction parallel (expanded row, one construction)</p>{body}</aside>')
    else:
        body = sentence_C(real, o, sc)
    return (f'<aside class="proto-frame"><p class="proto-tag">PROTOTYPE: the same marking reused in a Concordance '
            f'construction parallel (expanded row, one construction)</p><p class="reading-text">{body}</p></aside>')


def page(sc, shell_head, shell_tail):
    s, real, occ = load(sc)
    tgt = next(n.form for n in real if n.i == sc["target"])
    variants = {}
    common_tail = notation_key(real) + relations(s, real, sc) + text_alternative(real, occ, sc)
    # A: brackets in the sentence, list under it, then morphology line
    variants["A"] = (f'<p class="reading-text sentence-study-text cxA-text" lang="grc">{sentence_A(real, occ, sc)}</p>'
                     f'{entries(real, occ, sc, "A")}{morph_line(real, sc)}{common_tail}{concordance_preview(real, occ, sc, "A")}')
    # B: plain sentence, interlinear line carries tiered rules, list after
    variants["B"] = (f'<p class="reading-text sentence-study-text" lang="grc">{joined(real)}</p>'
                     f'{line_B(real, occ, sc)}{entries(real, occ, sc, "B")}{common_tail}{concordance_preview(real, occ, sc, "B")}')
    # C: plain sentence, each construction re-sets the sentence with its extent bold
    variants["C"] = (f'<p class="reading-text sentence-study-text" lang="grc">{joined(real)}</p>'
                     f'{entries(real, occ, sc, "C")}{morph_line(real, sc)}{common_tail}{concordance_preview(real, occ, sc, "C")}')
    names = {"A": "Brackets in the sentence", "B": "Rules under the interlinear line", "C": "One extract per construction"}
    body = (f'<header class="page-header"><div class="page-header__content"><h1>Study this sentence</h1>'
            f'<p>The complete source sentence with its constructions, a source-aligned morphology line and the complete analyzer evidence.</p></div></header>'
            f'<p><a href="#">Return to Concordance results</a></p>'
            f'<section class="reading-width" aria-labelledby="sentence-study-heading">'
            f'<h2 id="sentence-study-heading">{E(sc["book"])}</h2>'
            f'<p>{E(sc["author"])} · {E(sc["chapter"])} · sentence {sc["ordinal"]}</p>'
            f'<p>Identified target: <strong lang="grc">{E(tgt)}</strong></p>'
            + "".join(f'<div class="proto-variant" data-variant="{k}" hidden>{v}</div>' for k, v in variants.items())
            + "</section>")
    meta = {"scenarios": [{"key": x["key"], "title": x["title"]} for x in SCENARIOS], "current": sc["key"],
            "variants": names, "constructions": [{"id": o["id"], "name": NOTES[o["con"]][0]} for o in occ]}
    return shell_head + body + PROTO_CSS + switcher(meta) + shell_tail


PROTO_CSS = """<style>
/* PROTOTYPE ONLY (#2609) */
.visually-hidden{position:absolute;width:1px;height:1px;overflow:hidden;clip:rect(0 0 0 0);white-space:nowrap}
.cx-region{margin-block:var(--mouseion-space-5)}
.cx-region h3{font-size:1.25rem;margin-block-end:var(--mouseion-space-2)}
.cx-list{list-style:none;padding:0;margin:0;display:grid;gap:var(--mouseion-space-4)}
.cx-entry{border-inline-start:3px solid var(--mouseion-color-border);padding-inline-start:var(--mouseion-space-3)}
.cx-name{font-size:1rem;font-weight:600;margin:0}
.cx-subtype{font-weight:450;color:var(--mouseion-color-text-muted)}
.cx-num{display:inline-block;min-width:1.4em;font-variant-numeric:tabular-nums;color:var(--mouseion-color-text-muted)}
.cx-meta,.cx-extent-words,.cx-definition,.cx-actions{margin:var(--mouseion-space-1) 0;font-size:var(--mouseion-font-size-metadata,0.8125rem)}
.cx-extent-words span[lang]{font-family:var(--mouseion-font-reading);font-size:1rem}
.cx-extent-label{color:var(--mouseion-color-text-muted)}
.cx-actions a{display:inline-flex;align-items:center;min-height:44px}
.cx-anchor{text-decoration:underline;text-decoration-thickness:2px;text-underline-offset:0.22em}
.cx-focus-note{display:none;font-weight:450;color:var(--mouseion-color-text-muted)}
.cxA-text{line-height:2}
.cxA-open,.cxA-close{font-family:var(--mouseion-font-application);font-weight:600;color:var(--mouseion-color-text-muted);padding-inline:0.05em}
.cxA-open sup{font-size:0.65em;vertical-align:super;margin-inline-start:0.05em}
.cxB-line{line-height:1.35}
.cxB-unit{padding-inline:0.2em !important}
.cxB-rule{display:block;font-size:0.72rem;line-height:1.1;height:1.25em;margin-block-start:0.2em;text-align:start;white-space:nowrap;color:var(--mouseion-color-text-muted);border-bottom:2px solid var(--mouseion-color-text-muted);margin-inline:-0.2em;padding-inline-start:0.2em}
.cxB-empty{border-bottom-color:transparent}
.cxB-double{border-bottom:4px double var(--mouseion-color-text-muted)}
.cxB-dashed{border-bottom:2px dashed var(--mouseion-color-text-muted)}
.cxB-start{border-inline-start:2px solid var(--mouseion-color-text-muted)}
.cxB-start{overflow:visible}
.cxC-out{color:var(--mouseion-color-text-muted);font-weight:400}
.cxC-in{font-weight:600}
.cx-extract{margin:var(--mouseion-space-2) 0}
.cx-rel-table{border-collapse:collapse;margin-block:var(--mouseion-space-2);font-size:0.9rem}
.cx-rel-table th,.cx-rel-table td{text-align:start;padding:0.3rem 0.6rem;border-bottom:1px solid var(--mouseion-color-border);vertical-align:top}
.cx-rel-table th[lang]{font-family:var(--mouseion-font-reading);font-weight:450}
.cx-elided{font-style:italic}
.cx-elided::before{content:"‹"}.cx-elided::after{content:"›"}
/* focused-construction state */
.has-focus [data-cx]:not(.is-focused){opacity:0.35}
.has-focus .cx-entry:not(.is-focused){opacity:1}
.has-focus .cx-entry.is-focused{border-inline-start-color:var(--mouseion-color-accent);border-inline-start-width:5px}
.has-focus .cx-entry.is-focused .cx-focus-note{display:inline}
.has-focus .cx-anchor:not(.is-focused-anchor){text-decoration:none}
.proto-frame{margin-block:var(--mouseion-space-6);padding:var(--mouseion-space-3);border:2px dashed var(--mouseion-color-border)}
.proto-tag{font:600 0.75rem var(--mouseion-font-application);letter-spacing:0.05em;color:var(--mouseion-color-text-muted)}
#proto-bar{position:fixed;left:50%;bottom:1rem;transform:translateX(-50%);z-index:99;display:flex;gap:0.5rem;align-items:center;
 background:#111;color:#fff;padding:0.4rem 0.6rem;border-radius:999px;box-shadow:0 4px 18px rgba(0,0,0,.35);font:13px system-ui,sans-serif}
#proto-bar button,#proto-bar select{background:#333;color:#fff;border:1px solid #666;border-radius:999px;padding:0.25rem 0.6rem;font:inherit;min-height:0}
#proto-bar .lbl{min-width:15rem;text-align:center}
</style>"""


def switcher(meta):
    return """<div id="proto-bar" role="region" aria-label="Prototype switcher (not part of the design)">
<select id="pb-s" aria-label="Scenario"></select>
<button id="pb-prev" aria-label="Previous variant">←</button><span class="lbl" id="pb-l"></span><button id="pb-next" aria-label="Next variant">→</button>
<select id="pb-f" aria-label="Focused construction"><option value="">no focus</option></select></div>
<script>
const M=""" + json.dumps(meta, ensure_ascii=False) + """;
const P=new URLSearchParams(location.search);const keys=Object.keys(M.variants);let v=P.get('variant')||'A';
function go(p){const u=new URLSearchParams(location.search);for(const[k,x]of Object.entries(p)){if(x)u.set(k,x);else u.delete(k)}location.search=u}
document.querySelectorAll('.proto-variant').forEach(e=>e.hidden=e.dataset.variant!==v);
document.getElementById('pb-l').textContent=v+' ('+M.variants[v]+')';
const s=document.getElementById('pb-s');M.scenarios.forEach(x=>s.add(new Option(x.title,x.key,false,x.key===M.current)));
s.onchange=()=>{location.href=s.value+'.html?variant='+v};
const f=document.getElementById('pb-f');M.constructions.forEach(c=>f.add(new Option('focus: '+c.id.slice(1)+' '+c.name,c.id,false,c.id===P.get('focus'))));
f.onchange=()=>go({focus:f.value});
const step=d=>go({variant:keys[(keys.indexOf(v)+d+keys.length)%keys.length]});
document.getElementById('pb-prev').onclick=()=>step(-1);document.getElementById('pb-next').onclick=()=>step(1);
addEventListener('keydown',e=>{if(e.target.closest('input,textarea,select,[contenteditable]'))return;if(e.key==='ArrowLeft')step(-1);if(e.key==='ArrowRight')step(1)});
(P.get('open')||'').split(',').filter(Boolean).forEach(id=>document.querySelectorAll('.proto-variant:not([hidden]) details.'+id).forEach(d=>d.open=true));
const fc=P.get('focus');if(fc){const root=document.querySelector('.proto-variant[data-variant="'+v+'"]');root.classList.add('has-focus');
root.querySelectorAll('[data-cx="'+fc+'"]').forEach(e=>e.classList.add('is-focused'));
root.querySelectorAll('[data-anchor-of~="'+fc+'"]').forEach(e=>e.classList.add('is-focused-anchor'));
const en=root.querySelector('.cx-entry[data-cx="'+fc+'"]');if(en)en.scrollIntoView({block:'center'})}
</script>"""


def main():
    raw = open(os.path.join(OUT, "study.html"), encoding="utf-8").read()
    head = raw[:raw.index('<main id="main-content" class="page-container" tabindex="-1">') + len('<main id="main-content" class="page-container" tabindex="-1">')]
    head = re.sub(r'value="[^"]+\.de"', 'value="prototype"', head).replace("<title>", "<title>PROTOTYPE · ")
    head = head.replace('<option value="de" selected>Deutsch</option>', '<option value="grc" selected>Ἑλληνική (Ancient)</option>')
    tail = "</main></body></html>"
    for sc in SCENARIOS:
        with open(os.path.join(OUT, sc["key"] + ".html"), "w", encoding="utf-8") as fh:
            fh.write(page(sc, head, tail))
    with open(os.path.join(OUT, "index.html"), "w", encoding="utf-8") as fh:
        fh.write('<meta http-equiv="refresh" content="0; url=nested.html?variant=A">')
    print("wrote", ", ".join(sc["key"] + ".html" for sc in SCENARIOS))


if __name__ == "__main__":
    main()
