"""Deterministic practical-acceptance samples over the retained OGA Book 1 repair outputs (#2030).

Read-only by default: verifies every pinned input, recomputes the review summary and
compares it with the committed `review-summary.json` and the private example inventory.
`--write` regenerates exactly those two files and nothing else. No network access, no
new interpretation: the repaired ledger, selection and card inputs are consumed as-is.
The spacing measure is a sensitivity diagnostic, not an adopted unit rule.
"""

from __future__ import annotations

import collections
import csv
import hashlib
import io
import itertools
import json
import subprocess
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
PROOF = HERE.parent / "ancient-greek-oga-anabasis1-import-proof"
PRIVATE = PROOF / "private"
SUMMARY = HERE / "review-summary.json"
EXAMPLES = PRIVATE / "acceptance-review" / "examples.json"
WRITABLE = frozenset({SUMMARY, EXAMPLES})

# Retained outputs pinned by the #2052 proof summaries (opus-repair-summary.json,
# opus-completion-v2-summary.json, opus-repair-summary.json inputs.original_pins).
PINS = {
    "opus-repair/out/node-ledger.json": "f5a61b438893eda30ffe94ae3c08f1f225251e339633569c02193834640f19fc",
    "opus-repair/out/card-inputs.json": "f0686b172e03c3f4ee2a895a01cd959f2fc1536dd615b82e2c629f97db522789",
    "opus-repair/out/selection-model.tsv": "855ea59e3b449cf3634416de99c42898b2e939890353baa747a9f92ed1f96076",
    "opus-completion/out-v2/sentences.json": "539190b646387602d312ae811685f8fbd165a58ec2dc6f0ab8a8d0db7941e46f",
    "import/node-to-tei.json": "1f9016a011f9f62ef87931a9e58d655c5152d0feae3e69b85f0eaa0885b7b631",
}
# GLAUx #2050 committed selection, read from the immutable proof commit.
GLAUX_COMMIT = "957c99f25de9ea16e139055e6a87bbcda1996cc9"
GLAUX_SELECTION = "doc/research/ancient-greek-glaux-anabasis1-import-proof/manifest/selection_model.tsv"
GLAUX_SELECTION_SHA256 = "cadf1e99bac36b9013beac6f68660497312caa2684f2177e063a8b023fdf32bf"
MAX_UNITS, MAX_CODE_POINTS = 50, 400
# The seven recorded wrong-homograph predictions (#2052 proof, v2 section B).
WRONG_HOMOGRAPH = ("t_1590", "t_3153", "t_3251", "t_3800", "t_5625", "t_5642", "t_5708")
REPAIRED = {
    "citation": ("t_437", "t_4624", "t_4638", "t_4930", "t_5121", "t_5145", "t_5775"),
    "pos": ("t_2739", "t_3351", "t_7640", "t_10265", "t_10412", "t_5989"),
}
CONTENT_SAMPLE_SIZE = 12


class ReviewError(Exception):
    pass


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def load_inputs(private: Path = PRIVATE) -> dict:
    for rel, expected in PINS.items():
        if sha256(private / rel) != expected:
            raise ReviewError(f"pinned input changed: {rel}")
    with (private / "opus-repair/out/selection-model.tsv").open(encoding="utf-8") as handle:
        selection = list(csv.DictReader(handle, delimiter="\t"))
    glaux = subprocess.run(
        ["git", "show", f"{GLAUX_COMMIT}:{GLAUX_SELECTION}"], cwd=HERE, capture_output=True, check=True
    ).stdout
    if hashlib.sha256(glaux).hexdigest() != GLAUX_SELECTION_SHA256:
        raise ReviewError("GLAUx selection model changed")
    return {
        "glaux_selection": list(csv.DictReader(io.StringIO(glaux.decode("utf-8")), delimiter="\t")),
        "ledger": json.loads((private / "opus-repair/out/node-ledger.json").read_text(encoding="utf-8")),
        "cards": json.loads((private / "opus-repair/out/card-inputs.json").read_text(encoding="utf-8")),
        "selection": selection,
        "sentences": json.loads((private / "opus-completion/out-v2/sentences.json").read_text(encoding="utf-8")),
        "tei": {
            row["id"]: row["tei_string_indices"]
            for row in json.loads((private / "import/node-to-tei.json").read_text(encoding="utf-8"))
        },
    }


# --- spacing ---------------------------------------------------------------------------------


def tei_joins(printed: list[dict], tei: dict[str, list[int]]) -> set[str]:
    """IDs of printed nodes whose next printed node is TEI-adjacent (no source space between).

    In PAULA every adjacent pair is separated by one space, so each join is one space the
    original TEI does not have: punctuation attached to its word, or a split word (οὐ δὲ).
    """
    joins = set()
    for a, b in itertools.pairwise(printed):
        if a["cts"] == b["cts"] and a["sentence"] == b["sentence"] and tei[a["id"]][-1] + 1 == tei[b["id"]][0]:
            joins.add(a["id"])
    return joins


def word_splits(printed: list[dict], joins: set[str]) -> list[tuple[str, str]]:
    """Joined pairs where neither side is punctuation: a word the PAULA text divides."""
    nxt = {a["id"]: b for a, b in itertools.pairwise(printed)}
    out = []
    for node in printed:
        if node["id"] in joins and node["native_pos"] != "u" and nxt[node["id"]]["native_pos"] != "u":
            out.append((node["id"], nxt[node["id"]]["id"]))
    return out


def sentence_measures(sentences: list[dict], printed: list[dict], joins: set[str]) -> list[dict]:
    """PAULA units/code points (as recorded) and the TEI-spacing equivalent."""
    nodes_in = collections.Counter(n["sentence"] for n in printed)
    joined_in = collections.Counter(n["sentence"] for n in printed if n["id"] in joins)
    out = []
    for s in sentences:
        if nodes_in[s["index"]] != s["units"]:
            raise ReviewError(f"sentence {s['index']}: PAULA units are not one per printed node")
        out.append(
            {
                "index": s["index"],
                "units": s["units"],
                "code_points": s["code_points"],
                "tei_units": s["units"] - joined_in[s["index"]],
                "tei_code_points": s["code_points"] - joined_in[s["index"]],
            }
        )
    return out


def within_caps(measure: dict, prefix: str = "") -> bool:
    return measure[prefix + "units"] <= MAX_UNITS and measure[prefix + "code_points"] <= MAX_CODE_POINTS


def earliest(occurrences: list[dict], measures: list[dict], prefix: str = "") -> str | None:
    """Earliest occurrence (reading order, then ID) whose sentence passes the caps."""
    for node in sorted(occurrences, key=lambda n: (n["reading_span"][0], n["id"])):
        if within_caps(measures[node["sentence"]], prefix) and not node["citation_flags"]:
            return node["id"]
    return None


# --- review ----------------------------------------------------------------------------------


def compare_selected(oga: list[tuple[str, str]], glaux_rows: list[dict]) -> dict:
    """Set overlap of selected (key, UPOS); OGA Morpheus numbers are dropped since GLAUx has none.

    A structural comparison of what each mapping would freeze, not an accuracy measure.
    """
    glaux = {(r["canonical_key"], r["upos"]) for r in glaux_rows if r["selected"] == "yes"}
    plain = {(k.split("#")[0], u) for k, u in oga}
    only_oga, only_glaux = (
        sorted(plain - glaux, key=lambda k: (k[1], k[0])),
        sorted(glaux - plain, key=lambda k: (k[1], k[0])),
    )
    return {
        "oga_selected": len(oga),
        "oga_selected_without_morpheus_numbers": len(plain),
        "glaux_selected": len(glaux),
        "shared": len(plain & glaux),
        "oga_only_by_upos": dict(sorted(collections.Counter(u for _, u in only_oga).items())),
        "glaux_only_by_upos": dict(sorted(collections.Counter(u for _, u in only_glaux).items())),
        "oga_only": [f"{k}|{u}" for k, u in only_oga],
        "glaux_only": [f"{k}|{u}" for k, u in only_glaux],
    }


def identity(node: dict) -> tuple[str, str]:
    return node["identity"], node["pos"]["upos"]


def build(data: dict) -> tuple[dict, dict]:
    ledger, sentences, tei = data["ledger"], data["sentences"], data["tei"]
    by_id = {n["id"]: n for n in ledger}
    printed = [n for n in ledger if not n["empty"]]
    eligible = [n for n in printed if n["status"] == "eligible"]
    occurrences: dict[tuple[str, str], list[dict]] = collections.defaultdict(list)
    for node in eligible:
        occurrences[identity(node)].append(node)
    selected = [(r["canonical_key"], r["upos"]) for r in data["selection"] if r["selected"] == "yes"]
    selected_set = set(selected)
    cards = {(c["key"], c["upos"]): c for c in data["cards"]}
    if set(cards) != selected_set or len(selected) != 438:
        raise ReviewError("card inputs do not cover exactly the 438 selected identities")
    for key in selected:
        if len(occurrences[key]) != int(
            next(r["occurrences"] for r in data["selection"] if (r["canonical_key"], r["upos"]) == key)
        ):
            raise ReviewError(f"occurrence count disagrees for {key}")

    joins = tei_joins(printed, tei)
    splits = word_splits(printed, joins)
    measures = sentence_measures(sentences, printed, joins)
    split_sentences = {by_id[a]["sentence"] for a, _ in splits}
    editorial = {s["index"]: s["editorial"] for s in sentences if s["editorial"]}

    # Reproduce the recorded choices before using the same function for sensitivity.
    for key in selected:
        if earliest(occurrences[key], measures) != cards[key]["chosen"]:
            raise ReviewError(f"earliest-occurrence replay disagrees for {key}")
    tei_choice = {key: earliest(occurrences[key], measures, "tei_") for key in selected}
    chosen = {key: cards[key]["chosen"] for key in selected}
    first = {key: min(occurrences[key], key=lambda n: (n["reading_span"][0], n["id"]))["id"] for key in selected}

    def card_row(key: tuple[str, str]) -> dict:
        node_id = chosen[key]
        row = {"key": key[0], "upos": key[1], "occurrences": len(occurrences[key]), "chosen": node_id}
        if node_id:
            node = by_id[node_id]
            m = measures[node["sentence"]]
            row.update(
                {
                    "cts": node["cts"],
                    "sentence": node["sentence"],
                    "units": m["units"],
                    "tei_units": m["tei_units"],
                    "word_split_in_sentence": node["sentence"] in split_sentences,
                    "editorial_in_sentence": editorial.get(node["sentence"], []),
                }
            )
        return row

    # 1. Ordinary content vocabulary: plain lowercase-attested NOUN/VERB/ADJ, no discriminator.
    def capitalized_only(key):
        return all(n["surface"][:1].isupper() for n in occurrences[key])

    plain_content = sorted(
        k
        for k in selected
        if k[1] in {"NOUN", "VERB", "ADJ"}
        and "#" not in k[0]
        and not capitalized_only(k)
        and not any(n["id"] in REPAIRED["citation"] for n in occurrences[k])
    )
    step = len(plain_content) / CONTENT_SAMPLE_SIZE
    content_sample = [card_row(plain_content[int(i * step)]) for i in range(CONTENT_SAMPLE_SIZE)]

    # 2. Sentence lengths among chosen inputs.
    chosen_sentences = sorted({by_id[c]["sentence"] for c in chosen.values() if c})
    by_units = sorted(chosen_sentences, key=lambda i: (measures[i]["units"], i))
    lengths = {"shortest": by_units[0], "median": by_units[len(by_units) // 2], "longest": by_units[-1]}

    # 3. Selected ADV: other UPOS for the same canonical key elsewhere in the Book.
    upos_by_key: dict[str, collections.Counter] = collections.defaultdict(collections.Counter)
    for node in printed:
        if node["lemma"]["key"]:
            upos_by_key[node["lemma"]["key"]][node["pos"]["upos"]] += 1
    adv = []
    for key in sorted((k for k in selected if k[1] == "ADV"), key=lambda k: (-len(occurrences[k]), k)):
        base = key[0].split("#")[0]
        others = {u: c for u, c in sorted(upos_by_key[base].items()) if u != "ADV"}
        adv.append(
            {
                "key": key[0],
                "occurrences": len(occurrences[key]),
                "also_function_upos_in_book": {
                    u: c for u, c in others.items() if u in {"CCONJ", "SCONJ", "PART", "DET", "PRON"}
                },
                "also_adj_in_book": "ADJ" in others,
            }
        )
    function_adv = [a for a in adv if a["also_function_upos_in_book"]]
    selected_occurrences = sum(len(occurrences[k]) for k in selected)

    # 4. Names: selected NOUN identities never attested in lowercase in Book 1.
    names = sorted(
        (k for k in selected if k[1] == "NOUN" and capitalized_only(k)), key=lambda k: (-len(occurrences[k]), k)
    )
    capitalized_adj = sorted(k for k in selected if k[1] == "ADJ" and capitalized_only(k))

    # 5. Wrong-homograph predictions reaching the selected snapshot or a card.
    wrong = []
    for node_id in WRONG_HOMOGRAPH:
        key = identity(by_id[node_id])
        wrong.append(
            {
                "id": node_id,
                "cts": by_id[node_id]["cts"],
                "key": key[0],
                "upos": key[1],
                "selected": key in selected_set,
                "is_chosen_card_target": chosen.get(key) == node_id,
                "identity_all_occurrences_wrong": all(n["id"] in WRONG_HOMOGRAPH for n in occurrences[key]),
            }
        )

    # 6. Plain/numbered splits of one headword.
    by_headword: dict[str, list[tuple[str, str]]] = collections.defaultdict(list)
    for key, nodes in occurrences.items():
        by_headword[nodes[0]["lemma"]["headword"]].append(key)
    splits_by_headword = []
    for headword, keys in sorted(by_headword.items()):
        if len({k[0] for k in keys}) > 1:
            parts = [
                {
                    "key": k[0],
                    "upos": k[1],
                    "occurrences": len(occurrences[k]),
                    "selected": k in selected_set,
                    "card": bool(chosen.get(k)),
                }
                for k in sorted(keys)
            ]
            splits_by_headword.append(
                {"headword": headword, "identities": parts, "selected_identities": sum(p["selected"] for p in parts)}
            )

    # 7. Omissions and later-occurrence choices under PAULA versus TEI spacing.
    omissions = [
        {
            **card_row(k),
            "tei_spacing_choice": tei_choice[k],
            "occurrence_tei_units": [measures[n["sentence"]]["tei_units"] for n in occurrences[k]],
        }
        for k in selected
        if chosen[k] is None
    ]
    later = [k for k in selected if chosen[k] and chosen[k] != first[k]]
    later_tei_first = [k for k in later if tei_choice[k] == first[k]]

    # 8. Word-internal splits in chosen sentences.
    split_pairs = collections.Counter((by_id[a]["surface"], by_id[b]["surface"]) for a, b in splits)
    chosen_with_split = sorted({by_id[c]["sentence"] for c in chosen.values() if c} & split_sentences)
    split_targets = sorted(k for k in selected if chosen[k] and any(chosen[k] in pair for pair in splits))

    # 9-11. Repair-affected items.
    def node_row(node_id):
        node = by_id[node_id]
        return {
            "id": node_id,
            "cts": node["cts"],
            "surface_label": node["surface"],
            "identity": node["identity"],
            "upos": node["pos"]["upos"],
            "status": node["status"],
            "sentence": node["sentence"],
        }

    eimi_verb = ("εἰμί", "VERB")
    comparison = compare_selected(selected, data["glaux_selection"])
    summary = {
        "version": "oga-anabasis1-acceptance-review/1",
        "inputs": {rel: digest for rel, digest in PINS.items()},
        "scope": "OGA v0.2.0 Anabasis Book 1, repaired research layer oga-anabasis1-opus-repair/1",
        "totals": {
            "selected": len(selected),
            "with_input": sum(1 for c in chosen.values() if c),
            "selected_occurrences": selected_occurrences,
            "sentences": len(sentences),
        },
        "content_sample": {
            "rule": f"plain lowercase-attested NOUN/VERB/ADJ selected identities without discriminator "
            f"or citation repair, sorted by (key, UPOS); every {step:.2f}th of {len(plain_content)}",
            "rows": content_sample,
        },
        "sentence_lengths": {
            name: {
                "sentence": i,
                **{k: v for k, v in measures[i].items() if k != "index"},
                "word_split": i in split_sentences,
                "editorial": editorial.get(i, []),
            }
            for name, i in lengths.items()
        },
        "adv": {
            "selected": len(adv),
            "occurrences": sum(a["occurrences"] for a in adv),
            "with_function_upos_for_same_key": len(function_adv),
            "with_function_upos_occurrences": sum(a["occurrences"] for a in function_adv),
            "share_of_selected_occurrences": round(
                sum(a["occurrences"] for a in function_adv) / selected_occurrences, 4
            ),
            "also_adj_in_book": [a["key"] for a in adv if a["also_adj_in_book"]],
            "rows": adv,
        },
        "names": {
            "selected_noun_never_lowercase": len(names),
            "occurrences": sum(len(occurrences[k]) for k in names),
            "with_input": sum(1 for k in names if chosen[k]),
            "rows": [
                {"key": k[0], "occurrences": len(occurrences[k]), "first": first[k], "chosen": chosen[k]} for k in names
            ],
            "selected_adj_never_lowercase": [k[0] for k in capitalized_adj],
        },
        "glaux_selected_comparison": comparison,
        "wrong_homographs": wrong,
        "headword_splits": {
            "headwords": len(splits_by_headword),
            "with_two_selected_identities": [s["headword"] for s in splits_by_headword if s["selected_identities"] > 1],
            "rows": splits_by_headword,
        },
        "spacing": {
            "joins": len(joins),
            "word_splits": len(splits),
            "sentences_with_word_split": len(split_sentences),
            "split_pairs": [{"pair": list(p), "count": c} for p, c in sorted(split_pairs.items())],
            "chosen_sentences": len(chosen_sentences),
            "chosen_sentences_with_word_split": len(chosen_with_split),
            "inputs_in_sentences_with_word_split": sum(
                1 for c in chosen.values() if c and by_id[c]["sentence"] in split_sentences
            ),
            "selected_occurrences_that_are_split_parts": dict(
                sorted(
                    collections.Counter(
                        f"{by_id[i]['identity']}|{by_id[i]['pos']['upos']}"
                        for pair in splits
                        for i in pair
                        if by_id[i]["status"] == "eligible" and identity(by_id[i]) in selected_set
                    ).items()
                )
            ),
            "selected_targets_that_are_split_parts": [k[0] + "|" + k[1] for k in split_targets],
            "over_unit_cap": {
                "paula": sum(1 for m in measures if m["units"] > MAX_UNITS),
                "tei_spacing": sum(1 for m in measures if m["tei_units"] > MAX_UNITS),
            },
            "within_both_caps": {
                "paula": sum(1 for m in measures if within_caps(m)),
                "tei_spacing": sum(1 for m in measures if within_caps(m, "tei_")),
            },
            "omissions": omissions,
            "later_occurrence": len(later),
            "later_occurrence_that_tei_spacing_would_make_first": len(later_tei_first),
            "tei_spacing_inputs": sum(1 for c in tei_choice.values() if c),
        },
        "repairs": {
            "enelmi_input": card_row(("ἔνειμι", "VERB")),
            "eimi_verb_input": card_row(eimi_verb),
            "eimi_changed_nodes": [node_row(i) for i in REPAIRED["pos"]],
            "formed_citation": {
                **node_row("t_5145"),
                "selected": identity(by_id["t_5145"]) in selected_set,
                "occurrences": len(occurrences[identity(by_id["t_5145"])]),
            },
            "citation_repairs_selected": sorted(
                {identity(by_id[i])[0] for i in REPAIRED["citation"] if identity(by_id[i]) in selected_set}
            ),
        },
        "editorial": {
            "sentences_with_editorial": len(editorial),
            "chosen_sentences_with_editorial": sorted(i for i in chosen_sentences if i in editorial),
            "elements": {str(i): v for i, v in sorted(editorial.items())},
        },
    }

    # Private inventory: the full sentences behind every public row.
    wanted = (
        set(chosen_with_split)
        | set(lengths.values())
        | set(editorial)
        | {r["sentence"] for r in content_sample if r.get("sentence") is not None}
    )
    wanted |= {by_id[w["id"]]["sentence"] for w in wrong}
    wanted |= {by_id[i]["sentence"] for i in REPAIRED["pos"] + REPAIRED["citation"]}
    wanted |= {by_id[n["id"]]["sentence"] for o in omissions for n in occurrences[(o["key"], o["upos"])]}
    for key in names + [(a["key"], "ADV") for a in adv] + [eimi_verb, ("ἔνειμι", "VERB")]:
        if chosen[key]:
            wanted.add(by_id[chosen[key]]["sentence"])
    examples = {
        "version": summary["version"],
        "notice": "Private. OGA v0.2.0 (CC BY-SA 4.0) Book 1 reading text; not for redistribution.",
        "sentences": {str(i): sentences[i]["text"] for i in sorted(wanted)},
        "card_targets": {f"{k[0]}|{k[1]}": chosen[k] for k in sorted(selected)},
        "word_split_pairs": [[a, b] for a, b in splits],
    }
    return summary, examples


def dump(value) -> str:
    return json.dumps(value, ensure_ascii=False, indent=1, sort_keys=True) + "\n"


def write(path: Path, text: str) -> None:
    if path not in WRITABLE:
        raise ReviewError(f"refusing to write {path}")
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(text, encoding="utf-8")


def main(argv: list[str]) -> int:
    summary, examples = build(load_inputs())
    outputs = {SUMMARY: dump(summary), EXAMPLES: dump(examples)}
    if "--write" in argv:
        for path, text in outputs.items():
            write(path, text)
        print("wrote", *(str(p) for p in outputs))
        return 0
    problems = [str(p) for p, text in outputs.items() if not p.exists() or p.read_text(encoding="utf-8") != text]
    print("ok" if not problems else "mismatch: " + ", ".join(problems))
    print("examples.json sha256", hashlib.sha256(outputs[EXAMPLES].encode()).hexdigest())
    return 1 if problems else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
