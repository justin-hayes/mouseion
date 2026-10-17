"""THROWAWAY: pin/extract dictionary candidates, not sense gold or predictions.

python3 doc/research/prototypes/anabasis-word-sense/prepare_inputs.py SCRATCH
Requires hash-pinned oga.conllu, lexica.zip and raw.jsonl.gz in SCRATCH.
Gold alignment bytes are read directly from the already committed checkpoint.
"""

import gzip
import hashlib
import html
import json
import os
import re
import subprocess
import sys
import xml.etree.ElementTree as ET
import zipfile
from pathlib import Path

from betacode import beta_to_greek
from source_paths import candidate_paths

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[3]
SCRATCH = Path(sys.argv[1]).expanduser()
PINS = {
    "oga.conllu": "c51bba2df6e80e2eb8dd2543d00cf6d6e6b8c65cc9ed03c7d27176fed063eb61",
    "lexica.zip": "8d5bb101f912a9b0a8e09ecaa622b6a31d0dc895803a36e926ddcb63d0dfa910",
    "raw.jsonl.gz": "3dac8a09e57827bef493e2e6b552fe917bf3b1fc55dee05c4e4a3702aff5dbdb",
}


def digest(path):
    with path.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


for name, pin in PINS.items():
    assert digest(SCRATCH / name) == pin, f"source pin mismatch: {name}"

gold_bytes = subprocess.check_output(
    [
        "git",
        "show",
        "6455853e:doc/research/prototypes/anabasis-alignment/gold.json",
    ],
    cwd=ROOT,
)
gold = json.loads(gold_bytes)
(HERE / "alignment-gold.json").write_bytes(gold_bytes)

# Preserve each complete OGA identity and native morphology. No head/dependency
# inference: the production import explicitly records syntax as absent.
native = {}
sentence_id = None
for line in (SCRATCH / "oga.conllu").read_text().splitlines():
    if line.startswith("# sent_id = "):
        sentence_id = line.split(" = ", 1)[1]
    elif line and not line.startswith("#"):
        row = line.split("\t")
        native[sentence_id, row[0]] = row

targets = []
for sample in gold["samples"]:
    for link in sample["gold_word_links"]:
        token = next(
            t
            for t in sample["tokens"]
            if t["source_token_id"] == link["source_token_id"]
        )
        row = native[sample["source_sentence_id"], token["source_token_id"]]
        assert row[1:3] == [token["surface"], token["oga_lemma"]]
        numbered = re.search(r"\d+$", token["oga_lemma"])
        targets.append(
            {
                "id": f"{sample['source_sentence_id']}:{token['source_token_id']}",
                "sentence_id": sample["source_sentence_id"],
                "section": token["section"],
                "greek": sample["greek"],
                "english": [s["quote"] for s in sample["gold_spans"]],
                "aligned_english": [d["quote"] for d in link["destinations"]],
                **token,
                "headword": re.sub(r"\d+$", "", token["oga_lemma"]),
                "lexical_discriminator": "morpheus:" + numbered.group()
                if numbered
                else "",
                "native_postag": row[4],
            }
        )


# Identify relevant source rows using the production normalizer, not Python
# lower()/casefold(). The same helper normalizes every supported LSJ key too.
def normalize(words):
    env = {**os.environ, "GOTMPDIR": str(Path.home() / ".cache/gotmp")}
    return json.loads(
        subprocess.check_output(
            ["go", "run", str(HERE / "canonicalize.go")],
            input=json.dumps(sorted(set(words)), ensure_ascii=False).encode(),
            cwd=ROOT,
            env=env,
        )
    )


head_keys = normalize(t["headword"] for t in targets)
wanted = set(head_keys.values())
for target in targets:
    target["canonical_lemma"] = head_keys[target["headword"]]

# The grc ordinals are stable within this hash-pinned raw extract. Do not merge
# records, etymologies or same-text glosses. No examples/translated quotations.
kaikki_rows = []
grc_ordinal = 0
subset_hash = hashlib.sha256()
kaikki_words = set()
with (
    gzip.open(SCRATCH / "raw.jsonl.gz", "rb") as stream,
    (SCRATCH / "grc.jsonl").open("wb") as subset,
):
    for line in stream:
        if b'"lang_code": "grc"' not in line:
            continue
        data = json.loads(line)
        if data.get("lang_code") != "grc":
            continue
        subset_hash.update(line)
        subset.write(line)
        kaikki_words.add(data["word"])
kaikki_keys = normalize(kaikki_words)
with (SCRATCH / "grc.jsonl").open("rb") as stream:
    for line in stream:
        data = json.loads(line)
        grc_ordinal += 1
        if kaikki_keys[data["word"]] not in wanted:
            continue
        source_senses = []
        for ordinal, sense in enumerate(data.get("senses", []), 1):
            source_senses.append(
                {
                    "id": f"kaikki:grc-row-{grc_ordinal}:sense-{ordinal}",
                    "ordinal": ordinal,
                    **{
                        k: sense[k]
                        for k in ("glosses", "raw_glosses", "tags", "senseid", "topics")
                        if k in sense
                    },
                }
            )
        # A raw record often omits a separate row for a parent dictionary sense:
        # e.g. “give orders” survives only as the common breadcrumb of children.
        # Keep each raw row/depth distinct, including repeated glosses with
        # different construction notes. A supported parent never selects a child.
        kaikki_rows.append(
            {
                "id": f"kaikki:grc-row-{grc_ordinal}",
                "raw_line_sha256": hashlib.sha256(line).hexdigest(),
                "word": data["word"],
                "key": kaikki_keys[data["word"]],
                "source_url": "https://en.wiktionary.org/wiki/"
                + data["word"]
                + "#Ancient_Greek",
                **{
                    k: data[k]
                    for k in ("pos", "etymology_number", "etymology_text")
                    if k in data
                },
                "source_senses": source_senses,
                "senses": candidate_paths(source_senses),
            }
        )
assert grc_ordinal == 68196
assert (
    subset_hash.hexdigest()
    == "65c795d9ca6e1df91dfbdbe288697d576afa7c289a361512431c8a0c24727a12"
)

# TEI P4 sense hierarchy is partly sibling level notation. Keep XML parent and
# ordered level path, never flatten every descendant <tr> into a new sense.
archive = zipfile.ZipFile(SCRATCH / "lexica.zip")
entries = []
unknown_keys = []
for name in sorted(
    n
    for n in archive.namelist()
    if re.search(r"/lsj/grc\.lsj\.perseus-eng\d+\.xml$", n)
):
    text = archive.read(name).decode()
    for match in re.finditer(
        r'<entryFree\b[^>]*type="main"[^>]*>.*?</entryFree>', text, re.DOTALL
    ):
        raw = match.group()
        key = re.search(r'\bkey="([^"]*)"', raw).group(1)
        word, number, unknown = beta_to_greek(key)
        if unknown:
            unknown_keys.append(key)
        else:
            entries.append((name, raw, key, word, number))
keys = normalize(e[3] for e in entries)
lsj_rows = []
for name, raw, key, word, number in entries:
    if keys[word] not in wanted:
        continue
    # Resolve named HTML entities locally; preserve unrecognised names literally
    # and disclose them. Never fetch a DTD or an external entity.
    unresolved = []

    def entity(match, unresolved=unresolved):
        name = match.group()
        if name in {"&amp;", "&lt;", "&gt;", "&quot;", "&apos;"}:
            return name
        decoded = html.unescape(name)
        if decoded == name:
            unresolved.append(name)
        return html.escape(decoded, quote=False)

    root = ET.fromstring(re.sub(r"&[A-Za-z][A-Za-z0-9]*;", entity, raw))
    parents = {child: parent for parent in root.iter() for child in parent}

    def content(element):
        return " ".join("".join(element.itertext()).split())

    def owned(element, tag, parents=parents):
        out = []
        for child in element.iter(tag):
            p = parents.get(child)
            barriers = {"sense"} if tag == "bibl" else {"sense", "cit", "bibl"}
            while p is not None and p is not element and p.tag not in barriers:
                p = parents.get(p)
            if p is element:
                out.append(child)
        return out

    senses, stack = [], []
    for ordinal, sense in enumerate(root.iter("sense"), 1):
        level = int(sense.get("level", "1"))
        while stack and stack[-1][0] >= level:
            stack.pop()
        native_id = sense.get("id")
        assert native_id
        path = [s[1] for s in stack] + [native_id]
        stack.append((level, native_id))
        parent = parents.get(sense)
        while parent is not None and parent.tag != "sense":
            parent = parents.get(parent)
        own_glosses = [content(t) for t in owned(sense, "tr")]

        # Keep own prose, excluding physically nested senses; it supplies usage
        # and construction context for human inspection, not a guessed label.
        def own_text(element):
            return (element.text or "") + "".join(
                ("" if c.tag == "sense" else own_text(c)) + (c.tail or "")
                for c in element
            )

        senses.append(
            {
                "id": "lsj:" + native_id,
                "native_id": native_id,
                "ordinal": ordinal,
                "n": sense.get("n"),
                "level": level,
                "level_path": path,
                "xml_parent": parent.get("id") if parent is not None else None,
                "glosses": own_glosses,
                "labels": [
                    content(t)
                    for tag in ("lbl", "gram", "pos", "tns")
                    for t in owned(sense, tag)
                ],
                "text": " ".join(own_text(sense).split()),
                "citations": [dict(t.attrib) for t in owned(sense, "bibl")],
            }
        )
    lsj_rows.append(
        {
            "id": "lsj:" + root.get("id"),
            "native_id": root.get("id"),
            "key_beta": key,
            "word": word,
            "key": keys[word],
            "native_number": number,
            "file": name.rsplit("/", 1)[1],
            "raw_entry_sha256": hashlib.sha256(raw.encode()).hexdigest(),
            "source_url": "https://github.com/PerseusDL/lexica/blob/56061ca127f4a2844980baffc5f2b6d1332897b3/CTS_XML_TEI/perseus/pdllex/grc/lsj/"
            + name.rsplit("/", 1)[1],
            "entry_pos": [content(t) for t in owned(root, "pos")],
            "entry_labels": [
                content(t) for tag in ("gram", "lbl", "tns") for t in owned(root, tag)
            ],
            "senses": senses,
            "unresolved_entities": sorted(set(unresolved)),
        }
    )

asset = {
    "schema": "anabasis-word-sense-inputs/1",
    "is_sense_gold": False,
    "sources": PINS,
    "alignment_commit": "6455853e",
    "alignment_sha256": hashlib.sha256(gold_bytes).hexdigest(),
    "research_commit": "c3fdfbd7eec9aa4cf7a73cdf06dec0e9046ca9e4",
    "normalization": "real Go ancient-greek/1, Unicode 17.0.0; Beta Code conversion as documented",
    "lsj_unsupported_keys_excluded": unknown_keys,
    "targets": targets,
    "kaikki": kaikki_rows,
    "lsj": lsj_rows,
}
(HERE / "inputs.json").write_text(
    json.dumps(asset, ensure_ascii=False, indent=2) + "\n"
)
print(
    json.dumps(
        {
            "targets": len(targets),
            "kaikki_entries": len(kaikki_rows),
            "lsj_entries": len(lsj_rows),
            "status": "sense labels still require live human review",
        }
    )
)
