"""Prove body-only group identity against the candidate producer run, then rebase offsets (scratch only).

Reads the existing out1 producer results read-only and writes NEW files under out2/.
out1 is never written. No Stanza/Producer call is made: a case is reusable only when
its exact group text (SHA256) sits at the positionally mapped body-only index.

Usage: python3 -I rebase_check.py <segmentation_old.json> <segmentation_body.json> <producer_results_out1.json> <out2_dir>
"""

import hashlib
import json
import os
import sys
from collections import Counter

old_seg_path, body_seg_path, producer_path, out_dir = sys.argv[1:5]
os.makedirs(out_dir, exist_ok=True)

old = json.load(open(old_seg_path, encoding="utf-8"))
body = json.load(open(body_seg_path, encoding="utf-8"))
producer = json.load(open(producer_path, encoding="utf-8"))

kept = {u["xhtml_path"]: u for u in body["spine"] if u.get("decision") == "kept"}
old_units = {u["xhtml_path"]: u for u in old["units"]}
report = {"checks": {}, "unit_mapping": {}, "records": []}

# 1) Old-to-body unit mapping: body groups must equal old groups minus the leading head groups.
shift_by_path = {}
for path, old_unit in old_units.items():
    if path not in kept:
        report["unit_mapping"][path] = {"status": "absent_in_body", "old_groups": len(old_unit["groups"])}
        continue
    new_groups = kept[path]["groups"]
    old_texts = [g["text_sha256"] for g in old_unit["groups"]]
    new_texts = [g["text_sha256"] for g in new_groups]
    shift = len(old_texts) - len(new_texts)
    shift_by_path[path] = shift
    report["unit_mapping"][path] = {"old_groups": len(old_texts), "body_groups": len(new_texts), "removed_leading": shift,
                                    "tail_hash_sequence_identical": old_texts[shift:] == new_texts,
                                    "removed_group_block_tags": [g["block_tag"] for g in old_unit["groups"][:shift]]}
report["checks"]["dropped_units"] = [u["xhtml_path"] for u in body["spine"] if u.get("decision") != "kept"]
report["checks"]["head_groups_removed_total"] = sum(shift_by_path.values())


def body_index(path, old_ordinal):
    """Positional mapping old ordinal -> body group; the caller verifies the text hash."""
    idx = old_ordinal - 1 - shift_by_path[path]
    groups = kept[path]["groups"]
    return groups[idx] if 0 <= idx < len(groups) else None


# 2) Per-record reproduction: each record must map positionally to a body group with the identical text hash.
all_match = True
records_by_label = {}
for rec in producer["records"]:
    entry = {"label": rec["label"], "unit_old": rec.get("unit"), "ordinal_old": rec.get("ordinal"),
             "unit_start_old": rec.get("unit_start"), "text_sha256": rec["sha256"]}
    records_by_label[rec["label"]] = entry
    report["records"].append(entry)
    if rec["label"].startswith("repo_prose"):
        # Repo Werther excerpt: not an EPUB member. Its "unit" label in out1 is the cover wrapper, which is wrong.
        entry.update({"status": "not_epub_member", "unit_new": None})
        continue
    path = rec["unit"]
    if rec.get("ordinal") is not None and not rec["label"].startswith("combined_"):
        g = body_index(path, rec["ordinal"])
        ok = g is not None and g["text_sha256"] == rec["sha256"]
        entry["same_hash_count_in_body_unit"] = sum(1 for x in kept[path]["groups"] if x["text_sha256"] == rec["sha256"])
        if ok:
            entry.update({"status": "match", "ordinal_new": g["ordinal"], "unit_start_new": g["unit_start"],
                          "unit_new": path, "lines": g["lines"], "block_tag": g["block_tag"], "block_role": g["block_role"]})
        else:
            entry["status"] = "NO_MATCH"
            all_match = False
    else:
        ords = rec["group_ordinals"]
        old_groups = [g for g in old_units[path]["groups"] if g["ordinal"] in ords]
        joined = "\n\n".join(g["text"] for g in old_groups)
        same_text = hashlib.sha256(joined.encode("utf-8")).hexdigest() == rec["sha256"]
        new_ords, all_ok = [], True
        for og in old_groups:
            g = body_index(path, og["ordinal"])
            good = g is not None and g["text_sha256"] == og["text_sha256"]
            all_ok = all_ok and good
            new_ords.append(g["ordinal"] if good else None)
        first = body_index(path, ords[0])
        entry.update({"status": "match" if same_text and all_ok else "NO_MATCH", "ordinals_new": new_ords,
                      "unit_start_new": first["unit_start"] if first else None, "unit_new": path,
                      "joined_text_sha256_equals_out1": same_text})
        all_match = all_match and same_text and all_ok

report["checks"]["all_records_match_body_groups"] = all_match

# 3) Write rebased copy (out2): analysis unchanged, unit offsets and ordinals rebased.
rebased = {"pipeline": producer["pipeline"], "provenance": {
    "source_out1": os.path.abspath(producer_path),
    "source_out1_sha256": hashlib.sha256(open(producer_path, "rb").read()).hexdigest(),
    "rebase": "unit_start, span_unit and ordinal rebased to the body-only segmentation; text, tokens and analysis unchanged; no Producer rerun",
}, "elapsed_seconds": producer.get("elapsed_seconds"), "records": []}
for rec in producer["records"]:
    e = records_by_label[rec["label"]]
    out = dict(rec)
    if e.get("status") == "match":
        new_start = e["unit_start_new"]
        out["unit_start_old"] = rec.get("unit_start")
        out["unit_start"] = new_start
        if "ordinal_new" in e:
            out["ordinal_old"] = rec.get("ordinal")
            out["ordinal"] = e["ordinal_new"]
        out["sentences"] = []
        for s in rec.get("sentences", []):
            s2 = dict(s)
            s2["span_unit_old"] = s["span_unit"]
            s2["span_unit"] = [new_start + s["span_local"][0], new_start + s["span_local"][1]]
            out["sentences"].append(s2)
    elif e.get("status") == "NO_MATCH" or e.get("ordinals_new") is not None:
        out["unit_start_old"] = rec.get("unit_start")
    rebased["records"].append(out)

json.dump(rebased, open(os.path.join(out_dir, "producer_results_rebased.json"), "w", encoding="utf-8"), ensure_ascii=False, indent=1)
json.dump(report, open(os.path.join(out_dir, "rebase_report.json"), "w", encoding="utf-8"), ensure_ascii=False, indent=1)
print(json.dumps({"all_records_match_body_groups": all_match,
                  "head_groups_removed_total": report["checks"]["head_groups_removed_total"],
                  "dropped_units": report["checks"]["dropped_units"]}, ensure_ascii=False, indent=1))
for path, m in report["unit_mapping"].items():
    print(path, m)
for e in report["records"]:
    print(e["label"], e.get("status"), e.get("ordinal_old"), e.get("ordinal_new", e.get("ordinals_new")),
          e.get("unit_start_old"), e.get("unit_start_new"), e.get("same_hash_count_in_body_unit"))
