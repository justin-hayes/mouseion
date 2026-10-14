#!/usr/bin/env python3
"""Reproduce the stopped acquisition gate; never retrieve text or annotations."""

import argparse
import hashlib
import json
from pathlib import Path
import sys
import unicodedata
from urllib.request import HTTPRedirectHandler, Request, build_opener
import xml.etree.ElementTree as ET

HERE = Path(__file__).resolve().parent
PIN = "a69d917dbe117a307829977dd556e22f05ce0715"
TAG = "0.0.11376425141"
UPSTREAM = "476fb6133498e5710de7eec0b492791da86be4b4"
UNIT = "tlg0032.tlg006.perseus-grc2"
OGA_RAW = f"https://raw.githubusercontent.com/OperaGraecaAdnotata/OGA/{PIN}/"
UP_RAW = f"https://raw.githubusercontent.com/PerseusDL/canonical-greekLit/{TAG}/"
# Exact documentary requests only. URLs in responses are NEVER followed.
URLS = {
    "oga_README.md": OGA_RAW + "README.md",
    "oga_example_README.md": OGA_RAW + "annotation_example/README.md",
    "tree.json": f"https://api.github.com/repos/OperaGraecaAdnotata/OGA/git/trees/{PIN}?recursive=1",
    "urn_cts_plus_date_label.xml": OGA_RAW + "urn_cts/texts/urn_cts_plus_date_label.xml",
    "zenodo_record.json": "https://zenodo.org/api/records/14206061",
    "tagref.json": f"https://api.github.com/repos/PerseusDL/canonical-greekLit/git/refs/tags/{TAG}",
    "up_tag.json": "https://api.github.com/repos/PerseusDL/canonical-greekLit/git/tags/90ac67b935af3b46d6f910c1e173ffd99c657d78",
    "anab_listing.json": f"https://api.github.com/repos/PerseusDL/canonical-greekLit/contents/data/tlg0032/tlg006?ref={TAG}",
    "up_README.md": UP_RAW + "README.md",
    "up_license.md": UP_RAW + "license.md",
    "up_data_tlg0032_tlg006___cts__.xml": UP_RAW + "data/tlg0032/tlg006/__cts__.xml",
    "oga_index.html": "https://varro.informatik.uni-leipzig.de/oga/en/index.html",
    "oga_annis.html": "https://varro.informatik.uni-leipzig.de/oga/en/annis.html",
}
HEADERS = {name: {"Accept": "application/vnd.github+json"}
           for name in ("tree.json", "tagref.json", "anab_listing.json")}


def encoded(value):
    return (json.dumps(value, ensure_ascii=False, sort_keys=True, indent=2) + "\n").encode()


def digest(data):
    return hashlib.sha256(data).hexdigest()


def matches_input(data, item):
    if "json_sha256" in item:
        # GitHub content negotiation can reserialize the identical JSON object.
        # Compare ALL fields, not just the observations used by this assessment.
        return len(data) <= 2_000_000 and digest(encoded(json.loads(data))) == item["json_sha256"]
    return len(data) == item["bytes"] and digest(data) == item["sha256"]


def require(condition, reason):
    if not condition:
        raise ValueError(reason)


class NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):
        raise ValueError("redirect_not_authorized")


def load_inputs(cache, registry, fetch=False):
    require(set(registry) == set(URLS), "incomplete_metadata_registry")
    result = {}
    for name, item in sorted(registry.items()):
        require(item["url"] == URLS[name], "request_outside_documentary_allowlist")
        require(0 < item["bytes"] <= 2_000_000, "metadata_size_limit")
        path = cache / name
        if not path.exists() and fetch:
            request = Request(URLS[name], headers=HEADERS.get(name, {}))
            with build_opener(NoRedirect).open(request, timeout=60) as response:
                data = response.read(2_000_001)
            require(matches_input(data, item),
                    "documentary_snapshot_changed: " + name)
            cache.mkdir(parents=True, exist_ok=True)
            path.write_bytes(data)
        data = path.read_bytes()
        require(matches_input(data, item),
                "input_hash_or_size_mismatch: " + name)
        result[name] = data
    return result


def inspect(files):
    """Derive bounded inventory observations, not coverage or a legal verdict."""
    tree = json.loads(files["tree.json"])
    require(tree["sha"] == PIN and tree["truncated"] is False, "incomplete_or_wrong_tree")
    paths = [item["path"] for item in tree["tree"] if item["type"] == "blob"]
    require(not any(UNIT in path or "tlg0032" in path for path in paths),
            "new_candidate_requires_gate_review")
    packages = sorted({p.split("/")[1] for p in paths
                       if p.startswith("annotation_example/") and p.count("/") >= 2})
    require(packages == ["ggm0001.ggm001.1st1K-grc1"], "example_inventory_changed")
    bibliog = ET.fromstring(files["urn_cts_plus_date_label.xml"])
    matches = [w for w in bibliog.findall("work") if w.findtext("urn_cts") == UNIT]
    require(len(matches) == 1, "missing_or_ambiguous_work_record")
    work = {child.tag: child.text for child in matches[0]}
    require(work["author"] == "Xenophon" and work["title_labels"] == "Anabasis",
            "wrong_work_reference")
    record = json.loads(files["zenodo_record.json"])
    require(record["id"] == 14206061 and record["metadata"]["version"] == "0.2.0",
            "wrong_release")
    inventory = [{k: f[k] for k in ("key", "id", "size", "checksum")}
                 for f in record["files"]]
    require(len(inventory) == 1 and inventory[0]["key"] == "opera_graeca_adnotata_v0.2.0.zip",
            "release_inventory_changed_requires_review")
    ref = json.loads(files["tagref.json"])
    tag = json.loads(files["up_tag.json"])
    require(ref["object"]["sha"] == tag["sha"] and tag["tag"] == TAG
            and tag["object"] == {"sha": UPSTREAM, "type": "commit",
                                "url": f"https://api.github.com/repos/PerseusDL/canonical-greekLit/git/commits/{UPSTREAM}"},
            "upstream_tag_reference_mismatch")
    listed = json.loads(files["anab_listing.json"])
    text = [f for f in listed if f["name"] == UNIT + ".xml"]
    require(len(text) == 1 and text[0]["type"] == "file", "missing_text_listing")
    cts = ET.fromstring(files["up_data_tlg0032_tlg006___cts__.xml"])
    require(any(e.get("urn") == "urn:cts:greekLit:" + UNIT for e in cts),
            "missing_edition_reference")
    readme = files["oga_README.md"].decode()
    require("Creative Commons Attribution-ShareAlike 4.0 International License" in readme
            and "https://doi.org/10.5281/zenodo.14206061" in readme,
            "missing_declaration_or_release_link")
    return {
        "oga_documentation_pin": PIN,
        "repository_inventory_complete": True,
        "anabasis_payload_paths_in_repository": [],
        "example_packages": packages,
        "bibliographic_lead_not_coverage": work,
        "release": {"id": record["id"], "version": record["metadata"]["version"],
                    "doi": record["doi"], "files": inventory,
                    "metadata_license_id": record["metadata"]["license"]["id"]},
        "upstream_text_lead_not_acquired": {
            "release_tag": TAG, "tag_object": tag["sha"], "commit": UPSTREAM,
            "path": text[0]["path"], "bytes": text[0]["size"], "git_blob": text[0]["sha"],
            "sha256": None, "edition_binding": "unverified"},
        "documentary_input_hashes": {
            name: {"semantics": "canonical_JSON_all_fields" if name.endswith(".json") else "original_bytes",
                   "sha256": digest(encoded(json.loads(data))) if name.endswith(".json") else digest(data)}
            for name, data in sorted(files.items())},
    }


def verdict(evidence):
    return {
        "version": "oga-anabasis1-acquisition-gate/1",
        "unicode_version": unicodedata.unidata_version,
        "result": "stopped_at_gate_1_no_demonstrated_in_scope_package",
        "acquisition": "blocked_under_issue_scope; archive_not_requested",
        "rights": "explicit_BY-SA_declarations_retained; exact_unacquired_components_unassessed",
        "complete_unit_and_edition": "unverified_bibliographic_lead_only",
        "binding_and_reading_text": "not_performed",
        "required_lexical_identities": "not_performed",
        "complete_sentences": "not_performed",
        "optional_capabilities": "not_assessed",
        "practical_quality": "not_assessed",
        "real_go_selection_and_known_comparison": "not_run",
        "deterministic_card_inputs": "not_measured",
        "semantic_and_artifact_checks": "not_performed",
        "book1_counts_selection_inputs_and_omissions": None,
        "vocabulary_ready_publication": False,
        "source_or_mvp_accepted": False,
        "corpus_archive": evidence["release"]["files"][0],
        "see_operation_specific_rights": "NOTICE.md",
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--cache", type=Path, default=HERE / "private/inputs")
    parser.add_argument("--fetch", action="store_true", help="fetch only missing allowlisted documentary snapshots")
    parser.add_argument("--write", action="store_true", help="regenerate documentary evidence and verdict, not import outputs")
    args = parser.parse_args()
    require(sys.version_info[:2] == (3, 13) and unicodedata.unidata_version == "15.1.0",
            "requires_Python_3.13_Unicode_15.1.0")
    files = load_inputs(args.cache, json.loads((HERE / "inputs.json").read_text()), args.fetch)
    evidence = inspect(files)
    for name, value in (("evidence.json", evidence), ("ledger.json", verdict(evidence))):
        data = encoded(value)
        if args.write:
            (HERE / name).write_bytes(data)
        else:
            require((HERE / name).read_bytes() == data, "committed_diagnostic_differs: " + name)
    import unittest
    suite = unittest.defaultTestLoader.discover(str(HERE), pattern="test_gate.py")
    if not unittest.TextTestRunner(verbosity=1).run(suite).wasSuccessful():
        return 1
    print("Gate 1 stop reproduced; no full-unit acquisition/import/selection performed.")
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except (ValueError, OSError, KeyError, ET.ParseError) as error:
        print(f"gate verification failed: {error}", file=sys.stderr)
        sys.exit(2)
