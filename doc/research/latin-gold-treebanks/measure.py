"""Aggregate measurements for doc/research/latin-gold-treebanks.md.

Usage: python3 -I measure.py <data-dir>

<data-dir> holds the pinned checkouts (see the findings file). The script reads
them and prints counts only: no third-party text or annotation is printed.
Tokens are syntactic words. "content" tokens exclude punctuation (UD PUNCT, LDT
postag 'u', PROIEL part-of-speech 'S-'/punctuation tokens that carry no lemma).
"""
import collections
import glob
import gzip
import os
import re
import sys
import xml.etree.ElementTree as ET

D = sys.argv[1]


def conllu(path):
    """Yield (comments, rows) per sentence; rows are 10-column lists."""
    com, rows = [], []
    with open(path, encoding="utf-8") as f:
        for line in f:
            line = line.rstrip("\n")
            if not line:
                if rows or com:
                    yield com, rows
                com, rows = [], []
            elif line.startswith("#"):
                com.append(line)
            else:
                rows.append(line.split("\t"))
    if rows or com:
        yield com, rows


def words(rows):
    return [r for r in rows if re.fullmatch(r"\d+", r[0])]


def ud_stats(paths, keyfn):
    out = collections.defaultdict(lambda: collections.Counter())
    for p in paths:
        split = os.path.basename(p).rsplit("-", 1)[1].split(".")[0]
        for com, rows in conllu(p):
            k = keyfn(com, p)
            w = words(rows)
            c = out[k]
            c["sent"] += 1
            c["tok"] += len(w)
            c["content"] += sum(1 for r in w if r[3] != "PUNCT")
            c["lemma_missing"] += sum(1 for r in w if r[2] in ("_", ""))
            c["head_missing"] += sum(1 for r in w if r[6] in ("_", ""))
            c["tok_" + split] += len(w)
    return out


def show(title, d):
    print("\n##", title)
    for k in sorted(d):
        print(k, dict(d[k]))
    tot = collections.Counter()
    for v in d.values():
        tot.update(v)
    print("TOTAL", dict(tot))


# ---- UD Perseus ----
def perseus_key(com, p):
    for c in com:
        if c.startswith("# sent_id"):
            return c.split("=", 1)[1].strip().split("@")[0]
    return "?"


show("UD_Latin-Perseus r2.19", ud_stats(
    glob.glob(f"{D}/UD_Latin-Perseus/*.conllu"), perseus_key))


# ---- UD PROIEL ----
def proiel_key(com, p):
    for c in com:
        if c.startswith("# source"):
            s = c.split("=", 1)[1].strip()
            return s.split(",")[0]
    return "?"


show("UD_Latin-PROIEL r2.19", ud_stats(
    glob.glob(f"{D}/UD_Latin-PROIEL/*.conllu"), proiel_key))


# ---- UD CIRCSE ----
def circse_key(com, p):
    for c in com:
        if c.startswith("# sent_id"):
            s = c.split("=", 1)[1].strip()
            return re.sub(r"-\d+(-bis)?$", "", s) if "poetry" in s else \
                re.sub(r"-\d+(-\d+)?(-bis)?$", "", s)
    return "?"


show("UD_Latin-CIRCSE r2.19", ud_stats(
    glob.glob(f"{D}/UD_Latin-CIRCSE/*.conllu"), circse_key))

# ---- LDT (PerseusDL treebank_data v2.1/Latin) ----
print("\n## LDT v2.1 (treebank_data bf4334f)")
tot = collections.Counter()
for p in sorted(glob.glob(f"{D}/pdl/v2.1/Latin/texts/*.xml")):
    root = ET.parse(p).getroot()
    c = collections.Counter()
    subdocs = []
    cites = set()
    for s in root.iter("sentence"):
        c["sent"] += 1
        sd = s.get("subdoc")
        if sd:
            subdocs.append(sd)
        for w in s.iter("word"):
            c["tok"] += 1
            pos = w.get("postag") or ""
            if pos[:1] == "u":
                c["punct"] += 1
            if not (w.get("lemma") or "").strip() or w.get("lemma") == "_":
                c["lemma_missing"] += 1
            if w.get("artificial"):
                c["artificial"] += 1
            if w.get("head") in (None, ""):
                c["head_missing"] += 1
            if w.get("cite"):
                cites.add(w.get("cite").rsplit(":", 1)[0])
    c["content"] = c["tok"] - c["punct"]
    c["distinct_subdoc"] = len(set(subdocs))
    print(os.path.basename(p), root.get("format"), dict(c),
          "subdoc_first_last=", (subdocs[0] if subdocs else None,
                                 subdocs[-1] if subdocs else None),
          "cts_cite_prefixes=", sorted(cites)[:2])

# ---- PROIEL XML (Syntacticus tag 20230428) ----
print("\n## PROIEL XML (syntacticus-treebank-data 20230428), Latin")
for f in ["caes-gal", "cic-att", "cic-off", "pal-agr", "per-aeth", "latin-nt"]:
    p = f"{D}/syn/proiel/{f}.xml"
    root = ET.parse(p).getroot()
    st = collections.Counter()
    tok = collections.Counter()
    books = collections.defaultdict(collections.Counter)
    for s in root.iter("sentence"):
        status = s.get("status")
        st[status] += 1
        for t in s.iter("token"):
            if t.get("empty-token-sort"):
                tok["empty_node"] += 1
                continue
            tok["tok_" + status] += 1
            if status in ("reviewed", "annotated"):
                cp = (t.get("citation-part") or "").split(".")[0]
                books[cp]["tok"] += 1
                if t.get("part-of-speech") and not t.get("part-of-speech").startswith("S"):
                    tok["content_" + status] += 1
                if not t.get("lemma"):
                    tok["lemma_missing_annotated"] += 1
    print(f, "sentences", dict(st), "tokens", dict(tok))
    print("   annotated tokens by first citation level:",
          {k: v["tok"] for k, v in sorted(books.items(), key=lambda kv: (len(kv[0]), kv[0]))})

# ---- LASLA conllup ----
print("\n## LASLA conllup (CIRCSE/LASLA c2e2bb65)")
works = collections.defaultdict(collections.Counter)
for p in sorted(glob.glob(f"{D}/lasla/conllup/*.conllup")):
    b = os.path.basename(p)[:-8]
    a, w, code = b.split("_", 2)
    c = works[(a, w)]
    c["files"] += 1
    for com, rows in conllu(p):
        ws = words(rows)
        c["sent"] += 1
        c["tok"] += len(ws)
        c["content"] += sum(1 for r in ws if r[3] != "PUNCT")
        c["lemma_missing"] += sum(1 for r in ws if r[2] in ("_", ""))
        for r in ws:
            if r[3] == "PUNCT":
                c["punct"] += 1
for k in sorted(works):
    print(k, dict(works[k]))
alltok = sum(v["tok"] for v in works.values())
print("TOTAL works", len(works), "tokens", alltok)
by_author = collections.Counter()
for (a, w), v in works.items():
    by_author[a] += v["tok"]
print("by author", dict(sorted(by_author.items())))

# ---- EvaLatin ----
print("\n## EvaLatin")
for label, pat in [("2022 training-new", "ev22/EvaLatin2022-training-new/*.conllu"),
                   ("2022 test (LASLA-derived gold published separately)", "ev22/EvaLatin2022-test/*.conllu"),
                   ("2022 gold-test (zip gold-test-EvaLatin2022.zip)", "ev22g/*.conllu"),
                   ("2020 gold + training (UDPipe output, manually revised)", "ev20/*/*.conllu"),
                   ("2024 gold parsing", "ev24/EvaLatin_2024_Syntactic_Parsing_test_data_gold/*.conllu")]:
    print("--", label)
    for p in sorted(glob.glob(f"{D}/{pat}")):
        c = collections.Counter()
        for com, rows in conllu(p):
            ws = words(rows)
            c["sent"] += 1
            c["tok"] += len(ws)
            c["content"] += sum(1 for r in ws if r[3] != "PUNCT")
            c["head_set"] += sum(1 for r in ws if r[6] not in ("_", ""))
            c["lemma_missing"] += sum(1 for r in ws if r[2] in ("_", ""))
        print(os.path.basename(p), dict(c))

# ---- LASLA Reading-unit summary (Works as Mouseion would count them) ----
VERSE_AUTHORS = {"Catullus", "Horatius", "Juvenalis", "Lucretius", "Ovidius",
                 "Persius", "Plautus", "Propertius", "Tibullus", "Vergilius"}
SENECA_VERSE = {"Agamemnon", "HerculesFurens", "HerculesOetaeus", "Medea",
                "Oedipus", "Phaedra", "Phoenissae", "Thyestes", "Troades"}
ovid_prose = set()  # none: every Ovid work in the release is verse
print("\n## LASLA summary")
frag = [k for k in works if "Fragmenta" in k[1] or "Fragmenta" in k[1].replace("Fragment", "Fragmenta")]
small = [k for k in works if works[k]["tok"] < 1000]
whole = [k for k in works if k not in small and "Fragmenta" not in k[1]]
verse = [k for k in whole if k[0] in VERSE_AUTHORS or (k[0] == "Seneca" and k[1] in SENECA_VERSE)]
prose = [k for k in whole if k not in verse]
tk = lambda ks: sum(works[k]["tok"] for k in ks)
print("works", len(works), "fragment-or-under-1000-token works", len(small),
      "remaining", len(whole), "verse", len(verse), tk(verse), "prose", len(prose), tk(prose))
print("verse authors:", sorted({k[0] for k in verse}))
print("prose works >=5000 tokens", sum(1 for k in prose if works[k]["tok"] >= 5000))
