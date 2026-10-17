"""Run the real production Producer over bounded candidate-#2147 samples (scratch only).

Uses mouseion_nlp.producer.Producer with the default pipeline factory, so the
pinned German config in model_config.py and STANZA_RESOURCES_DIR are exercised
unchanged. Python -I ignores PYTHONPATH, so the repo paths are inserted here.

Usage (venv python, from anywhere):
  OMP_NUM_THREADS=2 STANZA_RESOURCES_DIR=<stanza dir> \
  python3 -I run_producer.py <segmentation.json> <out-dir> <repo>
"""

import json
import logging
import os
import sys
import time
from collections import Counter

PROBE = os.path.dirname(os.path.abspath(__file__))
seg_path, out_dir, repo = sys.argv[1], sys.argv[2], sys.argv[3]
sys.path[0:0] = [os.path.join(repo, "nlp", "src"), os.path.join(repo, "gen", "python")]

import torch  # noqa: E402

torch.set_num_threads(int(os.environ.get("OMP_NUM_THREADS", "2")))

from mouseion_nlp.producer import Producer, SourceDocument, _clean_surface  # noqa: E402
import mouseion_nlp.producer as producer_module  # noqa: E402

os.makedirs(out_dir, exist_ok=True)

warnings_seen: list[str] = []


class _Capture(logging.Handler):
    def emit(self, record):
        warnings_seen.append(record.getMessage())


logging.getLogger("mouseion_nlp.producer").addHandler(_Capture())
logging.getLogger("mouseion_nlp.producer").setLevel(logging.WARNING)

data = json.load(open(seg_path, encoding="utf-8"))
units = data["units"]


def find_group(unit_index, ordinal):
    for group in units[unit_index]["groups"]:
        if group["ordinal"] == ordinal:
            return group
    raise KeyError((unit_index, ordinal))


# Selections: (label, [(unit_index, ordinal), ...]). Unit index 1 = h-0, 2 = h-1,
# 3 = h-2, 4 = h-3, 5 = h-4, 6 = h-5 (index 0 is the cover wrapper).
SINGLE = [
    ("director_first_speech", 1, 27),
    ("director_entrance_line", 1, 25),
    ("director_label", 1, 26),
    ("was_faellt_surrounding_group", 1, 0),  # resolved below by text search
    ("thule_stanza_1", 4, 57),
    ("thule_stanza_2", 4, 58),
    ("thule_stanza_3", 4, 59),
    ("thule_stanza_4", 4, 60),
    ("thule_stanza_5", 4, 61),
    ("thule_stanza_6", 4, 62),
    ("geist_du_hast_mich", 1, 116),
    ("ach_gretchen_single", 5, 142),
    ("o_weh_valentin_i10", 5, 219),
    ("valentin_faellt_nested_speaker_sd", 5, 218),
    ("mephisto_zu_faust_nested_sd", 5, 0),  # resolved below by text search
    ("trueber_tag_prose_wrapped", 6, 0),  # resolved below by text search
    ("elision_wir_gehn_auf_s", 1, 196),
    ("elision_mir_ist_s", 2, 225),
    ("separable_faengt_an_zu_schwanken", 3, 353),
    ("separable_macht_sich_los", 3, 445),
    ("ins_correction_hatt_ein", 6, 21),
]


def find_by_text(unit_index, needle):
    hits = [g for g in units[unit_index]["groups"] if needle in g["text"]]
    if len(hits) != 1:
        raise SystemExit(f"text search {needle!r} in unit {unit_index}: {len(hits)} hits")
    return hits[0]


resolved = []
for label, ui, ordinal in SINGLE:
    if label == "was_faellt_surrounding_group":
        group = find_by_text(1, "Was fällt euch an?")
    elif label == "mephisto_zu_faust_nested_sd":
        hits = [g for g in units[5]["groups"] if "zu Faust." in g["text"] and any(s["role"] == "stage_direction" for s in g["spans"])]
        # Two identical repeats exist (ordinals 204 and 216); the first is used.
        group = hits[0]
    elif label == "trueber_tag_prose_wrapped":
        group = find_by_text(6, "Im Elend! Verzweifelnd!")
    else:
        group = find_group(ui, ordinal)
    resolved.append((label, ui, group))

# Combined handful: groups joined by a block boundary to test that no sentence
# crosses a line group. Joined exactly as the unit text joins them.
COMBINED = [
    ("combined_director_entrance_label_speech", 1, [25, 26, 27]),
    ("combined_valentin_faellt_and_o_weh", 5, [218, 219]),
    ("combined_thule_two_stanzas", 4, [57, 58]),
]

producer = Producer()
started = time.time()
records = []


def sentence_records(text, sentences, unit_base, group_label):
    """Compare each sentence and token against the authoritative slice."""
    out = []
    covered = [False] * len(text)
    for s_index, sentence in enumerate(sentences):
        start = sentence.location.start_offset
        end = sentence.location.end_offset
        slice_text = text[start:end]
        for i in range(start, end):
            covered[i] = True
        token_checks = []
        span_counts = Counter((t.location.start_offset, t.location.end_offset) for t in sentence.tokens)
        for t_index, token in enumerate(sentence.tokens):
            ts, te = token.location.start_offset, token.location.end_offset
            token_slice = text[ts:te]
            if token.surface == token_slice:
                category = "exact"
            elif token.surface == _clean_surface(token_slice):
                category = "clean_surface_edge"
            elif span_counts[(ts, te)] > 1:
                category = "mwt_repeated_span"
            else:
                category = "UNEXPLAINED"
            token_checks.append({
                "i": t_index, "surface": token.surface, "slice": token_slice,
                "span": [ts, te], "category": category, "raw_lemma": token.raw_lemma,
                "canonical_lemma": token.canonical_lemma, "pos": token.pos,
                "dep": token.dependency, "head": token.head,
                "morph": dict(token.morphology),
            })
        roots = sum(1 for t in sentence.tokens if t.dependency == "root")
        out.append({
            "index": s_index,
            "text": sentence.text,
            "span_local": [start, end],
            "span_unit": [unit_base + start, unit_base + end],
            "text_equals_slice": sentence.text == slice_text,
            "lf_in_text": sentence.text.count("\n"),
            "token_count": len(sentence.tokens),
            "roots": roots,
            "tokens": token_checks,
        })
    uncovered = [i for i, c in enumerate(covered) if not c and not text[i].isspace()]
    return out, uncovered


def run(label, text, unit_index, unit_base, groups_for_doc, ordinal_label):
    entry = {"label": label, "unit": units[unit_index]["xhtml_path"], "group_ordinals": ordinal_label,
             "unit_start": unit_base, "text": text,
             "source_lf": text.count("\n"), "sha256": __import__("hashlib").sha256(text.encode()).hexdigest()}
    warnings_seen.clear()
    try:
        corpus = producer.analyze(text, "de", SourceDocument(id=units[unit_index]["xhtml_path"], title="pg21000", chapter="", section=label), run_id=f"probe-{label}")
        sents, uncovered = sentence_records(text, corpus.sentences, unit_base, label)
        entry["sentences"] = sents
        entry["uncovered_nonspace_codepoints"] = uncovered
        entry["lf_outside_sentences"] = sum(1 for c in range(len(text)) if text[c] == "\n" and all(not (s["span_local"][0] <= c < s["span_local"][1]) for s in sents))
        entry["sentence_count"] = len(sents)
        entry["exception"] = None
    except Exception as exc:  # record, never hide
        entry["exception"] = f"{type(exc).__name__}: {exc}"
        entry["sentences"] = []
    entry["producer_warnings"] = list(warnings_seen)
    return entry


for label, ui, group in resolved:
    records.append(run(label, group["text"], ui, group["unit_start"], [group["ordinal"]], [group["ordinal"]]))
    records[-1]["ordinal"] = group["ordinal"]
    records[-1]["group_sha256"] = group["text_sha256"]
    records[-1]["block_role"] = group["block_role"]
    records[-1]["spans"] = group["spans"]

for label, ui, ordinals in COMBINED:
    groups = [find_group(ui, o) for o in ordinals]
    joined = "\n\n".join(g["text"] for g in groups)
    rec = run(label, joined, ui, groups[0]["unit_start"], ordinals, ordinals)
    # Boundary check: each sentence must sit inside one group's joined range.
    bounds = []
    cursor = 0
    for g in groups:
        bounds.append((cursor, cursor + len(g["text"]), g["ordinal"]))
        cursor += len(g["text"]) + 2
    crossing = []
    for s in rec.get("sentences", []):
        a, b = s["span_local"]
        owners = {o for lo, hi, o in bounds if a < hi and b > lo}
        if len(owners) != 1:
            crossing.append({"text": s["text"][:80], "owner_groups": sorted(owners)})
    rec["cross_group_sentences"] = crossing
    rec["group_boundaries_local"] = bounds
    records.append(rec)

# Pinned German prose comparison excerpt in the repo (existing test data).
prose_path = os.path.join(repo, "nlp", "testdata", "german_prose_werther_excerpt.txt")
prose_text = open(prose_path, encoding="utf-8").read()
records.append(run("repo_prose_werther_excerpt", prose_text, 0, 0, [], []))
records[-1]["ordinal"] = None
records[-1]["block_role"] = None
records[-1]["spans"] = []
records[-1]["group_sha256"] = __import__("hashlib").sha256(prose_text.encode()).hexdigest()

# Pipeline configuration actually built by the production factory.
pipeline = producer_module._stanza_pipeline("de")
config_record = {
    "processors_config_string": pipeline.config.get("processors") if hasattr(pipeline, "config") else None,
    "processor_names_built": list(pipeline.processors.keys()),
    "packages_used": {name: str(getattr(proc, "config", {}).get("model_path", "")) for name, proc in pipeline.processors.items()},
    "stanza_version": producer_module.stanza.__version__,
    "torch_version": torch.__version__,
    "torch_threads": torch.get_num_threads(),
    "resources_dir": os.environ.get("STANZA_RESOURCES_DIR"),
    "model_version_recorded": producer.model_version("de"),
}

with open(os.path.join(out_dir, "producer_results.json"), "w", encoding="utf-8") as sink:
    json.dump({"pipeline": config_record, "records": records, "elapsed_seconds": round(time.time() - started, 1)},
              sink, ensure_ascii=False, indent=1)
print(json.dumps({"pipeline": config_record, "elapsed_seconds": round(time.time() - started, 1)}, ensure_ascii=False, indent=2))
for r in records:
    print(r["label"], "sentences", r.get("sentence_count"), "exception", r.get("exception"), "warnings", len(r.get("producer_warnings", [])))
