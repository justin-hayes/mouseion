"""Inspect the bounded Kaikki French sample and run it through Mouseion's unchanged
derivation functions (with "fr" admitted only in this scratch process).

Usage: python3 -I inspect_dict.py <repo_root> <dict_dir> <out_json>
"""

import importlib.util
import json
import sys
from pathlib import Path

repo, dict_dir, out_path = Path(sys.argv[1]), Path(sys.argv[2]), Path(sys.argv[3])
spec = importlib.util.spec_from_file_location("derive", repo / "nlp/scripts/derive_dictionary_index.py")
derive = importlib.util.module_from_spec(spec)
sys.modules["derive"] = derive
spec.loader.exec_module(derive)
derive.LANGUAGES = derive.LANGUAGES | {"fr"}  # scratch-only admission

manifest = json.loads((dict_dir / "manifest.json").read_text(encoding="utf-8"))
records = []
for item in manifest:
    if item["status"] != "fetched":
        continue
    for line in (dict_dir / item["file"]).read_text(encoding="utf-8").splitlines():
        if line.strip():
            records.append(json.loads(line))

report = {"absent_titles": [m["word"] for m in manifest if m["status"] != "fetched"], "words": {}}
entries: dict = {}
for rec in records:
    key = derive.indexed_key(rec)
    for upos, sense in derive.senses_for_item(rec):
        derive.record_sense(entries, (key[0], key[1], upos), sense, rec, upos)
    senses = rec.get("senses", [])
    reflexive = [
        {"gloss": (s.get("glosses") or [""])[0], "tags": s.get("tags", []), "raw_tags": s.get("raw_tags", [])}
        for s in senses
        if any(t in ("reflexive", "pronominal") for t in s.get("tags", []))
        or "reflexive" in " ".join(s.get("glosses", []))
        or "pronominal" in " ".join(s.get("raw_tags", []))
    ]
    report["words"].setdefault(rec["word"], []).append(
        {
            "pos": rec.get("pos"),
            "etymology_number": rec.get("etymology_number"),
            "sense_count": len(senses),
            "first_glosses": [(s.get("glosses") or [None])[0] for s in senses[:6]],
            "form_of": sorted({f.get("word") for s in senses for f in s.get("form_of", []) + s.get("alt_of", [])}),
            "word_tags": rec.get("tags", []),
            "gender_tags": sorted({t for s in senses for t in s.get("tags", []) if t in ("masculine", "feminine")} | {t for t in rec.get("tags", []) if t in ("masculine", "feminine")}),
            "gender_categories": [c for c in rec.get("categories", []) if isinstance(c, str) and ("masculine" in c or "feminine" in c)],
            "h_categories": [c for c in rec.get("categories", []) if isinstance(c, str) and ("aspirated" in c.lower() or "h " in c.lower()[:3] or "h-" in c.lower())],
            "plural_forms": [(f["form"], f.get("tags")) for f in rec.get("forms", []) if "plural" in f.get("tags", [])][:4],
            "form_count": len(rec.get("forms", [])),
            "reflexive_forms_sample": [f["form"] for f in rec.get("forms", []) if "reflexive" in f.get("tags", [])][:4],
            "reflexive_senses": reflexive,
            "sounds": [(s.get("ipa"), s.get("tags"), s.get("note")) for s in rec.get("sounds", []) if s.get("ipa")],
            "head_expansions": [t.get("expansion") for t in rec.get("head_templates", [])],
            "derived_or_related_pronominal": sorted({d.get("word") for d in rec.get("derived", []) + rec.get("related", []) if isinstance(d.get("word"), str) and d["word"].startswith(("se ", "s'", "s’"))}),
        }
    )

report["derived_rows"] = {
    f"{k[0]}|{k[1]}|{k[2]}": {
        "gloss_count": len(v["senses"]),
        "glosses": [s["Gloss"] for s in v["senses"]][:5],
        "gender": v["gender"],
        "article": v["article"],
        "plural": v["plural"],
        "ipa": v["ipa"],
        "principal_parts": v["principal_parts"],
    }
    for k, v in sorted(entries.items())
}
out_path.write_text(json.dumps(report, ensure_ascii=False, indent=1), encoding="utf-8")
