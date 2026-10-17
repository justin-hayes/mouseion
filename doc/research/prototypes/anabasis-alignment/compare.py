"""THROWAWAY standard-library comparison, not a production aligner.

Usage: python3 compare.py SCRATCH [--transductive]
Gold sections are excluded from statistical training. Labels are used only in
the scoring pass (plus separately labelled oracle-boundary diagnostics).
The optional transductive diagnostic trains on all 205 UNLABELLED sections,
including evaluation text, never gold links. It is not a held-out evaluation.
"""

import collections
import difflib
import hashlib
import json
import re
import sys
import unicodedata as U
from pathlib import Path

ROOT = Path(__file__).resolve().parent
SCRATCH = Path(sys.argv[1]).expanduser()
corpus = json.loads((SCRATCH / "corpus.json").read_text())
gold = json.loads((ROOT / "gold.json").read_text())
assert gold["is_gold"] and all(s["human_review"]["status"] == "confirmed" for s in gold["samples"])
sentences, sections = corpus["sentences"], corpus["sections"]
byid = {s["source_sentence_id"]: s for s in sentences}
transductive = "--transductive" in sys.argv[2:]
holdout = set() if transductive else {ref for s in gold["samples"] for ref in s["sections"]}
STOP = {
    "a", "an", "the", "and", "or", "but", "for", "of", "to", "in", "on", "at", "by", "as", "with", "from",
    "into", "through", "over", "under", "out", "up", "down", "off", "than", "that", "this", "these", "those",
    "he", "him", "his", "she", "her", "it", "its", "they", "them", "their", "we", "us", "our", "you", "your",
    "i", "my", "me", "who", "which", "whom", "what", "when", "where", "while", "so", "not", "no", "all", "any",
    "some", "both", "each", "every", "be", "is", "am", "are", "was", "were", "been", "being", "have", "has",
    "had", "do", "does", "did", "can", "could", "may", "might", "shall", "should", "will", "would", "very",
    "also", "only", "then", "there", "here", "now", "again", "most", "more", "much", "many", "such", "either", "neither",
}
POS = {"noun": "n", "verb": "v", "adj": "a", "adv": "d", "name": "n"}


def gkey(word):
    return U.normalize("NFC", word).lower().replace("ς", "σ")


def headword(word):
    return re.sub(r"\d+$", "", gkey(word))


def en_tokens(text, ref):
    return [{"section": ref, "index": i, "surface": m.group(), "span": [m.start(), m.end()]}
            for i, m in enumerate(re.finditer(r"\w+(?:['’\-]\w+)*|[^\w\s]", text))]


english = {ref: en_tokens(text, ref) for ref, text in sections.items()}


def isword(token):
    return any(ch.isalpha() or ch.isdigit() for ch in token["surface"])


def forms(word):
    """Small generic suffix heuristic, not a trained lemmatiser or synonym list."""
    word = word.lower()
    result = {word}
    for suffix in ("s", "es", "ies", "ed", "ing", "ly"):
        if word.endswith(suffix) and len(word) > len(suffix) + 2:
            base = word[:-len(suffix)]
            result.add(base)
            if suffix == "ies":
                result.add(base + "y")
            if suffix in {"ed", "ing"}:
                result.add(base + "e")
                if len(base) > 2 and base[-1] == base[-2]:
                    result.add(base[:-1])
    return result


dictionary = json.loads((ROOT / "dictionary-subset.json").read_text())
glosses = collections.defaultdict(set)
for entry in dictionary["entries"]:
    pos = POS.get(entry["pos"])
    if not pos:
        continue
    for sense in entry["senses"]:
        if sense.get("form_of"):
            continue
        for gloss in sense.get("glosses", []):
            for word in re.findall(r"[A-Za-z]+(?:['’\-][A-Za-z]+)*", gloss):
                if word.lower() not in STOP:
                    glosses[headword(entry["word"]), pos].update(forms(word))


def dict_match(greek, eng):
    return eng["surface"].lower() not in STOP and bool(
        glosses.get((headword(greek["oga_lemma"]), greek["native_pos"]), set()) & forms(eng["surface"])
    )


def spans(tokens):
    groups = collections.defaultdict(list)
    for token in tokens:
        groups[token["section"]].append(token)
    return [{"section": ref, "span": [ts[0]["span"][0], ts[-1]["span"][1]],
             "quote": sections[ref][ts[0]["span"][0]:ts[-1]["span"][1]]}
            for ref, ts in groups.items()]


def tokens_in(extent):
    return [t for span in extent for t in english[span["section"]]
            if t["span"][0] >= span["span"][0] and t["span"][1] <= span["span"][1]]


def span_key(extent):
    return sorted((x["section"], *x["span"]) for x in extent)


def loc(token):
    return token["section"], token["index"]


# Coarse parallel training units: whole sections, excluding every gold section.
# No claimed pre-existing sentence alignment; no books 2–7 or pretrained weights.
training = []
for ref in sections:
    if ref in holdout:
        continue
    gs = [gkey(t["oga_lemma"]) for s in sentences for t in s["tokens"]
          if t["section"] == ref and t["native_pos"] != "u"]
    es = [t["surface"].lower() for t in english[ref] if isword(t)]
    training.append((gs, es))


def ibm1(pairs, iterations=20):
    """IBM Model 1 EM over section bags, with a NULL source token."""
    counts = [(collections.Counter(["<NULL>", *g]), collections.Counter(e)) for g, e in pairs]
    possible = collections.defaultdict(set)
    vocabulary = set()
    for gs, es in counts:
        vocabulary.update(es)
        for g in gs:
            possible[g].update(es)
    table = {g: dict.fromkeys(sorted(es), 1 / len(vocabulary)) for g, es in sorted(possible.items())}
    for _ in range(iterations):
        expected = collections.defaultdict(collections.Counter)
        for gs, es in counts:
            for e, ec in es.items():
                denominator = sum(gc * table[g].get(e, 0) for g, gc in gs.items())
                if not denominator:
                    continue
                for g, gc in gs.items():
                    expected[g][e] += ec * gc * table[g].get(e, 0) / denominator
        for g, row in expected.items():
            total = sum(row.values())
            table[g] = {e: value / total for e, value in row.items()}
    return table


forward = ibm1(training)
reverse = ibm1([(e, g) for g, e in training])
train_g = collections.Counter(g for gs, _ in training for g in gs)
train_e = collections.Counter(e for _, es in training for e in es)
ratio = sum(len(t["surface"]) for r in sections if r not in holdout for t in english[r]) / sum(
    len(t["surface"]) for s in sentences for t in s["tokens"]
    if t["section"] not in holdout and t["native_pos"] != "u"
)

# Join only section components linked by an actual OGA crossing sentence.
components = []
for sentence in sentences:
    connected = [c for c in components if c & set(sentence["sections"])]
    merged = set(sentence["sections"])
    for component in connected:
        merged |= component
        components.remove(component)
    components.append(merged)
ref_order = {ref: i for i, ref in enumerate(sections)}

TRANSLIT = dict(zip("αβγδεζηθικλμνξοπρστυφχψω", [
    "a", "b", "g", "d", "e", "z", "e", "th", "i", "k", "l", "m", "n", "x", "o", "p", "r", "s", "t", "y", "ph", "ch", "ps", "o",
]))


def name_anchor(greek, eng):
    if greek.get("import_upos") != "PROPN" or not eng["surface"][:1].isupper():
        return False
    plain = "".join(ch for ch in U.normalize("NFD", greek["surface"]).lower()
                    if U.category(ch) != "Mn")
    latin = "".join(TRANSLIT.get(ch, ch) for ch in plain.replace("ς", "σ"))
    return difflib.SequenceMatcher(None, latin, eng["surface"].lower()).ratio() >= 0.72


def allocate(group, kind):
    refs = sorted(group, key=ref_order.get)
    gs = [s for s in sentences if set(s["sections"]) <= group]
    es = [token for ref in refs for token in english[ref]]
    # Every word boundary is available; punctuation reduces boundary cost.
    # Many-to-one English sentences can split; Greek may span English sentences.
    lengths = [0]
    for token in es:
        lengths.append(lengths[-1] + len(token["surface"]))
    previous = {0: (0.0, [])}
    for i, sentence in enumerate(gs):
        expected = ratio * sum(len(t["surface"]) for t in sentence["tokens"] if t["native_pos"] != "u")
        rewards = [0.0]
        for e in es:
            reward = 0.0
            for g in sentence["tokens"]:
                if kind == "names" and name_anchor(g, e):
                    reward += 6.0
                if kind == "dictionary" and dict_match(g, e):
                    reward += 2.0
            rewards.append(rewards[-1] + reward)
        current = {}
        remaining = len(gs) - i - 1
        for end in range(i + 1, len(es) - remaining + 1):
            if i == len(gs) - 1 and end != len(es):
                continue
            best = None
            for start, (cost, history) in previous.items():
                if start >= end:
                    continue
                length = lengths[end] - lengths[start]
                length_cost = (length - expected) ** 2 / (length + expected + 1)
                last = es[end - 1]["surface"]
                boundary_cost = 0.0 if last in ".!?;:" or end == len(es) else 1.0 if last in ",—" else 4.0
                value = cost + length_cost + boundary_cost - (rewards[end] - rewards[start])
                candidate = (value, [*history, (start, end)])
                if best is None or candidate[0] < best[0]:
                    best = candidate
            if best is not None:
                current[end] = best
        previous = current
    _, path = previous[len(es)]
    return {s["source_sentence_id"]: spans(es[a:b]) for s, (a, b) in zip(gs, path)}


predicted_spans = {name: {} for name in ("section_context", "punctuation_order", "length", "length_names", "dictionary")}
for sentence in sentences:
    sid = sentence["source_sentence_id"]
    predicted_spans["section_context"][sid] = spans([t for ref in sentence["sections"] for t in english[ref]])
for ref in sections:
    units, current = [], []
    for token in english[ref]:
        current.append(token)
        if token["surface"] in ".!?;:":
            units.append(current)
            current = []
    if current:
        units.append(current)
    owned = [s for s in sentences if s["sections"][0] == ref]
    for i, sentence in enumerate(owned):
        predicted_spans["punctuation_order"][sentence["source_sentence_id"]] = spans(units[i]) if i < len(units) else []
for group in sorted(components, key=lambda c: min(ref_order[r] for r in c)):
    for method, kind in (("length", "length"), ("length_names", "names"), ("dictionary", "dictionary")):
        predicted_spans[method].update(allocate(group, kind))
predicted_spans["ibm1_forward"] = predicted_spans["length"]
predicted_spans["ibm1_mutual"] = predicted_spans["length"]
predicted_spans["agreement"] = {
    sid: extent if span_key(extent) == span_key(predicted_spans["length_names"][sid]) else []
    for sid, extent in predicted_spans["dictionary"].items()
}


def ibm_links(sentence, target, extent, mutual=False):
    g = gkey(target["oga_lemma"])
    options = [t for t in tokens_in(extent) if isword(t)]
    if g not in forward or not options:
        return set()
    best = max(options, key=lambda t: forward[g].get(t["surface"].lower(), 0))
    if forward[g].get(best["surface"].lower(), 0) == 0:
        return set()
    if mutual:
        e = best["surface"].lower()
        candidates = ["<NULL>", *(gkey(t["oga_lemma"]) for t in sentence["tokens"] if t["native_pos"] != "u")]
        back = max(candidates, key=lambda x: reverse.get(e, {}).get(x, 0))
        if back != g:
            return set()
    return {loc(best)}


existing = json.loads((ROOT / "existing-ugarit.json").read_text())
native_english = collections.defaultdict(set)
for token in existing["english"]:
    if token["group"] != "notAligned":
        native_english[token["group"]].update(tuple(x) for x in token["bound_occurrences"])
existing_links = collections.defaultdict(set)
for token in existing["greek"]:
    if token["group"] != "notAligned":
        for sid, tid in token["bound_occurrences"]:
            existing_links[sid, tid].update(native_english[token["group"]])
predicted_spans["existing_ugarit"] = {}
for sentence in sentences:
    sid = sentence["source_sentence_id"]
    linked = set().union(*(existing_links[sid, t["source_token_id"]] for t in sentence["tokens"]))
    extent = []
    for ref in sections:
        indices = [index for r, index in linked if r == ref]
        if indices:
            start, end = min(indices), max(indices) + 1
            if end < len(english[ref]) and english[ref][end]["surface"] in ".!?;:":
                end += 1
            extent.extend(spans(english[ref][start:end]))
    predicted_spans["existing_ugarit"][sid] = extent


def links(method, sentence, target, extent):
    if method == "existing_ugarit":
        return existing_links[sentence["source_sentence_id"], target["source_token_id"]]
    if method.startswith("ibm1_"):
        return ibm_links(sentence, target, extent, mutual=method == "ibm1_mutual")
    if method in {"dictionary", "agreement"}:
        matches = {loc(t) for t in tokens_in(extent) if dict_match(target, t)}
        if method == "agreement":
            matches &= ibm_links(sentence, target, extent, mutual=True)
        return matches
    return set()  # No invented bilingual word links from monolingual splitting.


def score(method, all_spans, word_method=None):
    exact = covered = tp = fp = fn = target_covered = target_exact = 0
    cases = []
    for sample in gold["samples"]:
        sid = sample["source_sentence_id"]
        extent = all_spans.get(sid, [])
        sentence_exact = span_key(extent) == span_key(sample["gold_spans"])
        exact += sentence_exact
        covered += bool(extent)
        case = {"source_sentence_id": sid, "sentence_exact": sentence_exact,
                "gold_spans": sample["gold_spans"], "predicted_spans": extent, "targets": []}
        for target in sample["gold_word_links"]:
            actual = {(d["section"], index) for d in target["destinations"] for index in d["english_token_indices"]}
            token = next(t for t in byid[sid]["tokens"] if t["source_token_id"] == target["source_token_id"])
            proposed = links(word_method or method, byid[sid], token, extent)
            hit, wrong, missed = len(actual & proposed), len(proposed - actual), len(actual - proposed)
            tp += hit
            fp += wrong
            fn += missed
            target_covered += bool(proposed)
            target_exact += actual == proposed
            case["targets"].append({
                "source_token_id": target["source_token_id"], "surface": target["surface"], "oga_lemma": target["oga_lemma"],
                "gold": sorted(actual), "predicted": sorted(proposed),
                "gold_quotes": [d["quote"] for d in target["destinations"]],
                "predicted_words": [english[r][i]["surface"] for r, i in sorted(proposed)],
                "tp": hit, "fp": wrong, "fn": missed,
            })
        cases.append(case)
    return {"sentence_exact": exact, "sentences": 8, "sentence_accuracy": exact / 8,
            "sentence_coverage": covered, "tp": tp, "fp": fp, "fn": fn,
            "precision": tp / (tp + fp) if tp + fp else None,
            "recall": tp / (tp + fn), "target_coverage": target_covered,
            "target_exact": target_exact, "target_occurrences": 24, "cases": cases}


# Only now consult span/link labels. Gold sections, not their labels, were used
# for the training split above. This is not blinded development: proposals and
# reviewed examples were visible while writing the prototype. No tuning loop.
results = {method: score(method, extent) for method, extent in predicted_spans.items()}
oracle = {s["source_sentence_id"]: s["gold_spans"] for s in gold["samples"]}
oracles = {method: score("oracle_" + method, oracle, word_method=method)
           for method in ("dictionary", "ibm1_forward", "ibm1_mutual")}
target_support = [{"source_sentence_id": s["source_sentence_id"], "surface": t["surface"],
                   "oga_lemma": t["oga_lemma"], "training_occurrences": train_g[gkey(t["oga_lemma"])],
                   "dictionary_gloss_word_count": len(glosses.get((headword(t["oga_lemma"]), next(
                       token["native_pos"] for token in byid[s["source_sentence_id"]]["tokens"]
                       if token["source_token_id"] == t["source_token_id"])), set()))}
                  for s in gold["samples"] for t in s["gold_word_links"]]
asset = {
    "schema": "anabasis-alignment-comparison/1", "gold_commit": "6455853e",
    "gold_sha256": hashlib.sha256((ROOT / "gold.json").read_bytes()).hexdigest(),
    "prototype_sha256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
    "input_pins": corpus["pins"],
    "dictionary_subset_sha256": hashlib.sha256((ROOT / "dictionary-subset.json").read_bytes()).hexdigest(),
    "existing_subset_sha256": hashlib.sha256((ROOT / "existing-ugarit.json").read_bytes()).hexdigest(),
    "training": {"regime": "all unlabelled Book 1 sections, including evaluation text; not held out" if transductive else "gold sections excluded",
                 "section_pairs": len(training), "excluded_sections": sorted(holdout, key=ref_order.get),
                 "greek_tokens": sum(train_g.values()), "english_tokens": sum(train_e.values()),
                 "greek_types": len(train_g), "english_types": len(train_e), "iterations_each_direction": 20,
                 "english_characters_per_greek_printed_letter": ratio,
                 "unseen_target_occurrences": sum(t["training_occurrences"] == 0 for t in target_support),
                 "target_support": target_support},
    "fixed_parameters": {"nonpunctuation_boundary_penalty": 4, "comma_dash_boundary_penalty": 1,
                         "name_similarity_threshold": 0.72, "name_anchor_reward": 6, "dictionary_anchor_reward": 2,
                         "tuning": "No parameter search or post-result tuning; developer saw reviewed examples. These are cheap baselines, not fast_align/eflomal packages or their published performance."},
    "metrics": "Exact source-relative span sets (punctuation included); micro token links for 24 selected Greek occurrences only. Abstentions retain gold links in recall. No predictions means undefined precision, not 0 or 100%. Target-exact counts require all and only the approved English tokens.",
    "methods": results, "oracle_boundary_diagnostics": oracles,
    "limitations": "8 purposive, proposal-biased live-confirmed examples, no independent annotator or untouched test set. Development was not blinded. Coarse section pairs, not clean sentence pairs. Statistical model uses no external training inputs. Lexicon matches union all POS-compatible gloss words, not contextual senses. Ugarit contributes only one gold sentence; partial coverage must not be generalised. No uncertainty threshold is calibrated. All-section training is a separately labelled post-initial-run data-availability diagnostic; no scoring/algorithm parameters were tuned.",
}
output = "results-transductive.json" if transductive else "results.json"
(ROOT / output).write_text(json.dumps(asset, ensure_ascii=False, indent=2) + "\n")
print(json.dumps({"training": {k: v for k, v in asset["training"].items() if k != "target_support"},
                  "methods": {m: {k: v for k, v in r.items() if k != "cases"} for m, r in results.items()},
                  "oracle_boundary_diagnostics": {m: {k: v for k, v in r.items() if k != "cases"} for m, r in oracles.items()}}, indent=2))
