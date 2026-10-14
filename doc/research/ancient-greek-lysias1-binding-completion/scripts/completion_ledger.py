#!/usr/bin/env python3
"""Overlay the #2048 completion proposals on the reconciliation route table and check the result.

Usage:
  python3 completion_ledger.py [baseline-manifest-dir] [out-dir]

The baseline is the reconciliation manifest/ folder at 7ac01256 (route_nodes.tsv, sentences.tsv).
Defaults point at the sibling reconciliation folder and this folder's manifest/.

The overlay never changes text_status, text_rule, vocab_class, identity_status or readiness, and it
never removes a blocker. It only adds blocker labels and writes two proposal columns, so no node moves
toward a ready state the baseline withheld. Proposals are records for parent review, not adoption.
"""
import csv
import hashlib
import json
import sys
from collections import Counter, defaultdict
from pathlib import Path

HERE = Path(__file__).resolve().parent
DEFAULT_BASELINE = HERE.parents[1] / "ancient-greek-lysias1-reconciliation" / "manifest"
DEFAULT_OUT = HERE.parent / "manifest"

SENT11_NODES = ("s011w025", "s011w026", "s011w028", "s011w029")
# Canonical token keys for the two μήτε tokens, from nodes.tsv column b_canon_keys (baseline).
SENT11_CANON = {"s011w025": "cg2:w0264", "s011w026": "cg2:w0264",
                "s011w028": "cg2:w0266", "s011w029": "cg2:w0266"}
SENT11_BLOCKER = "BP-PRODUCER-IDENTITY"
SENT11_PROPOSAL = ("conditional_shared_span on {canon} (mu-eta-tau-epsilon, U+03BC U+03AE U+03C4 U+03B5); "
                   "component order -tau-epsilon before mu-eta from the historical producer rule; "
                   "not passed until producer identity is evidenced")
S125_NODE = "s125w015"
S125_BLOCKER = "BD-S125-ROUTE"
S125_PROPOSAL = ("decision_required: printed route for ou-tau-epsilon (native U+1F54 smooth; grc1/grc2 "
                 "U+1F55 rough); not decided in this research")
PROPOSAL_COLUMNS = ["completion_proposal", "completion_change"]
EXPECTED_ROUTE_COUNT = 6
EXPECTED_SENTENCE_COUNT = 134
EXPECTED_NODE_COUNT = 2834


def read_tsv(path):
    with path.open(encoding="utf-8", newline="") as f:
        return list(csv.DictReader(f, delimiter="\t"))


def write_tsv(path, fieldnames, rows):
    with path.open("w", encoding="utf-8", newline="") as f:
        writer = csv.DictWriter(f, fieldnames=fieldnames, delimiter="\t", lineterminator="\n")
        writer.writeheader()
        writer.writerows(rows)


def split_blockers(value):
    return [b for b in value.split(";") if b]


def sha256(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def overlay(rows):
    """Return (new_rows, changes). Only blockers and the two proposal columns may differ from the baseline."""
    new_rows, changes = [], []
    for row in rows:
        new = dict(row)
        new["completion_proposal"] = ""
        new["completion_change"] = "unchanged"
        node = row["node_key"]
        added = None
        if node in SENT11_NODES:
            added = SENT11_BLOCKER
            new["completion_proposal"] = SENT11_PROPOSAL.format(canon=SENT11_CANON[node])
        elif node == S125_NODE:
            added = S125_BLOCKER
            new["completion_proposal"] = S125_PROPOSAL
        if added:
            blockers = split_blockers(row["blockers"])
            if added not in blockers:
                blockers.append(added)
            new["blockers"] = ";".join(blockers)
            new["completion_change"] = "blocker_added:" + added
            changes.append({"route": row["route"], "node_key": node, "sentence_id": int(node[1:4]),
                            "added_blocker": added, "readiness": row["readiness"],
                            "completion_proposal": new["completion_proposal"]})
        new_rows.append(new)
    return new_rows, changes


def producer_ops(path, baseline_t1a2):
    """Map each non-equal producer op (c9b60a3, grc1 2014) to its baseline T1A2 status, for review."""
    out = []
    for op in read_tsv(path):
        if op["tag"] == "equal":
            continue
        for node in filter(None, op["native_nodes"].split(",")):
            base = baseline_t1a2.get(node, {})
            out.append({"tag": op["tag"], "node_key": node, "native_forms": op["native_forms"],
                        "sim_forms": op["sim_forms"], "baseline_text_status": base.get("text_status", ""),
                        "baseline_readiness": base.get("readiness", "")})
    return out


def run_checks(base_rows, new_rows, sentence_rows, changes, ops):
    checks = {}
    routes = Counter(r["route"] for r in base_rows)
    checks["route_count"] = len(routes)
    checks["route_count_expected"] = EXPECTED_ROUTE_COUNT
    checks["nodes_per_route"] = dict(sorted(routes.items()))
    checks["node_count_per_route_expected"] = EXPECTED_NODE_COUNT
    unique = len({(r["route"], r["node_key"]) for r in base_rows}) == len(base_rows)
    checks["node_keys_unique_per_route"] = unique
    checks["sentence_count"] = len(sentence_rows)
    checks["sentence_node_total"] = sum(int(r["nodes"]) for r in sentence_rows)

    base_index = {(r["route"], r["node_key"]): r for r in base_rows}
    new_index = {(r["route"], r["node_key"]): r for r in new_rows}
    checks["same_row_set"] = base_index.keys() == new_index.keys()

    locked = ("route", "node_key", "text_status", "text_rule", "vocab_class", "identity_status",
              "discriminator_status", "discriminator_origin", "readiness")
    checks["locked_fields_unchanged"] = all(
        base_index[k][f] == new_index[k][f] for k in base_index for f in locked)
    checks["blockers_only_grow"] = all(
        set(split_blockers(base_index[k]["blockers"])) <= set(split_blockers(new_index[k]["blockers"]))
        for k in base_index)

    changed_keys = {(c["route"], c["node_key"]) for c in changes}
    checks["changed_rows"] = len(changed_keys)
    checks["changed_rows_gained_their_blocker"] = all(
        c["added_blocker"] in split_blockers(new_index[(c["route"], c["node_key"])]["blockers"])
        for c in changes)
    checks["changed_rows_all_still_blocked"] = all(
        "blocked" in base_index[k]["readiness"] for k in changed_keys)
    checks["ready_claims_added"] = sum(
        1 for k in base_index if new_index[k]["readiness"].endswith("_ready")
        and base_index[k]["readiness"] != new_index[k]["readiness"])
    checks["sent11_rows"] = sum(1 for k in base_index if k[1] in SENT11_NODES)
    checks["sent11_rows_expected"] = 4 * EXPECTED_ROUTE_COUNT
    checks["s125_rows"] = sum(1 for k in base_index if k[1] == S125_NODE)
    checks["s125_rows_expected"] = EXPECTED_ROUTE_COUNT
    checks["producer_ops_checked"] = len(ops)

    checks["readiness_counts_before"] = dict(Counter(r["readiness"] for r in base_rows))
    checks["readiness_counts_after"] = dict(Counter(r["readiness"] for r in new_rows))

    boolean = {k: v for k, v in checks.items() if isinstance(v, bool)}
    counted = {"route_count": (checks["route_count"] == EXPECTED_ROUTE_COUNT),
               "sentence_count": (checks["sentence_count"] == EXPECTED_SENTENCE_COUNT),
               "sentence_node_total": (checks["sentence_node_total"] == EXPECTED_NODE_COUNT),
               "nodes_per_route": all(v == EXPECTED_NODE_COUNT for v in routes.values()),
               "sent11_rows": checks["sent11_rows"] == checks["sent11_rows_expected"],
               "s125_rows": checks["s125_rows"] == checks["s125_rows_expected"]}
    checks["all_checks_pass"] = (all(boolean.values()) and all(counted.values())
                                 and checks["ready_claims_added"] == 0)
    checks["check_results"] = {**boolean, **counted}
    return checks


def sentence_overlay(sentence_rows, changes):
    by_sentence = defaultdict(lambda: {"blockers": set(), "rows": 0})
    for c in changes:
        entry = by_sentence[c["sentence_id"]]
        entry["blockers"].add(c["added_blocker"])
        entry["rows"] += 1
    out = []
    for row in sentence_rows:
        sid = int(row["sentence_id"])
        entry = by_sentence.get(sid)
        out.append({"sentence_id": row["sentence_id"], "nodes": row["nodes"],
                    "completion_changed_rows": str(entry["rows"]) if entry else "0",
                    "completion_added_blockers": ";".join(sorted(entry["blockers"])) if entry else ""})
    return out


def main(argv):
    baseline = Path(argv[1]) if len(argv) > 1 else DEFAULT_BASELINE
    out_dir = Path(argv[2]) if len(argv) > 2 else DEFAULT_OUT
    out_dir.mkdir(parents=True, exist_ok=True)

    base_path = baseline / "route_nodes.tsv"
    base_rows = read_tsv(base_path)
    fieldnames = list(base_rows[0].keys())
    sentence_rows = read_tsv(baseline / "sentences.tsv")

    new_rows, changes = overlay(base_rows)
    sentence_out = sentence_overlay(sentence_rows, changes)

    baseline_t1a2 = {r["node_key"]: r for r in new_rows if r["route"] == "T1A2"}
    ops_path = out_dir / "producer_alignment_grc1-aa0dd9e6_c9b60a3.tsv"
    ops = producer_ops(ops_path, baseline_t1a2) if ops_path.exists() else []

    checks = run_checks(base_rows, new_rows, sentence_rows, changes, ops)
    checks["baseline_sha256"] = {"route_nodes.tsv": sha256(base_path),
                                 "sentences.tsv": sha256(baseline / "sentences.tsv")}

    write_tsv(out_dir / "completion_route_nodes.tsv", fieldnames + PROPOSAL_COLUMNS, new_rows)
    write_tsv(out_dir / "completion_changes.tsv",
              ["route", "node_key", "sentence_id", "added_blocker", "readiness", "completion_proposal"],
              changes)
    write_tsv(out_dir / "completion_sentences.tsv",
              ["sentence_id", "nodes", "completion_changed_rows", "completion_added_blockers"], sentence_out)
    write_tsv(out_dir / "completion_producer_ops.tsv",
              ["tag", "node_key", "native_forms", "sim_forms", "baseline_text_status", "baseline_readiness"],
              ops)
    (out_dir / "completion_checks.json").write_text(
        json.dumps(checks, indent=2, ensure_ascii=False, sort_keys=True) + "\n", encoding="utf-8")
    print(json.dumps({"all_checks_pass": checks["all_checks_pass"], "changed_rows": checks["changed_rows"],
                      "ready_claims_added": checks["ready_claims_added"]}, indent=2))
    return 0 if checks["all_checks_pass"] else 1


if __name__ == "__main__":
    sys.exit(main(sys.argv))
