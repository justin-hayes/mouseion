"""Invented metadata failure cases; none is evidence of real Greek coverage."""

import copy
import json
from pathlib import Path
import tempfile
import unittest

import gate


def fixture():
    files = {name: b"documentary fixture" for name in gate.URLS}
    files.update({
        "tree.json": gate.encoded({"sha": gate.PIN, "truncated": False, "tree": [
            {"path": "annotation_example/ggm0001.ggm001.1st1K-grc1/anno.xml", "type": "blob"}]}),
        "urn_cts_plus_date_label.xml": (
            f"<works><work><urn_cts>{gate.UNIT}</urn_cts><author>Xenophon</author>"
            "<title_labels>Anabasis</title_labels></work></works>").encode(),
        "oga_README.md": (
            "Creative Commons Attribution-ShareAlike 4.0 International License "
            "https://doi.org/10.5281/zenodo.14206061").encode(),
        "zenodo_record.json": gate.encoded({"id": 14206061, "doi": "fixture", "metadata": {
            "version": "0.2.0", "license": {"id": "other-at"}}, "files": [{
                "key": "opera_graeca_adnotata_v0.2.0.zip", "id": "fixture", "size": 900,
                "checksum": "md5:fixture"}]}),
        "tagref.json": gate.encoded({"object": {"sha": "tag-fixture"}}),
        "up_tag.json": gate.encoded({"sha": "tag-fixture", "tag": gate.TAG, "object": {
            "sha": gate.UPSTREAM, "type": "commit",
            "url": f"https://api.github.com/repos/PerseusDL/canonical-greekLit/git/commits/{gate.UPSTREAM}"}}),
        "anab_listing.json": gate.encoded([{"name": gate.UNIT + ".xml", "type": "file",
                                           "path": "fixture", "size": 12, "sha": "fixture"}]),
        "up_data_tlg0032_tlg006___cts__.xml": (
            f'<work><edition urn="urn:cts:greekLit:{gate.UNIT}"/></work>').encode(),
    })
    return files


class GateTests(unittest.TestCase):
    def setUp(self):
        self.files = fixture()

    def change_json(self, name, change):
        value = json.loads(self.files[name])
        change(value)
        self.files[name] = gate.encoded(value)

    def test_bibliography_does_not_establish_coverage(self):
        evidence = gate.inspect(self.files)
        ledger = gate.verdict(evidence)
        self.assertIsNone(ledger["book1_counts_selection_inputs_and_omissions"])
        self.assertFalse(ledger["vocabulary_ready_publication"])
        self.assertEqual(ledger["real_go_selection_and_known_comparison"], "not_run")

    def test_metadata_shorthand_does_not_negate_explicit_declaration(self):
        result = gate.verdict(gate.inspect(self.files))
        self.assertIn("explicit_BY-SA_declarations_retained", result["rights"])
        self.assertFalse(result["source_or_mvp_accepted"])

    def test_truncated_tree_fails(self):
        self.change_json("tree.json", lambda d: d.update(truncated=True))
        with self.assertRaisesRegex(ValueError, "incomplete_or_wrong_tree"):
            gate.inspect(self.files)

    def test_wrong_tree_revision_fails(self):
        self.change_json("tree.json", lambda d: d.update(sha="wrong"))
        with self.assertRaisesRegex(ValueError, "incomplete_or_wrong_tree"):
            gate.inspect(self.files)

    def test_new_target_path_requires_review_not_automatic_acquisition(self):
        self.change_json("tree.json", lambda d: d["tree"].append({
            "path": gate.UNIT + ".xml", "type": "blob"}))
        with self.assertRaisesRegex(ValueError, "new_candidate_requires_gate_review"):
            gate.inspect(self.files)

    def test_missing_and_duplicate_work_references_fail(self):
        for xml in (b"<works/>", self.files["urn_cts_plus_date_label.xml"].replace(
                b"</works>", self.files["urn_cts_plus_date_label.xml"].split(b"<works>")[1])):
            with self.subTest(xml=xml):
                self.files["urn_cts_plus_date_label.xml"] = xml
                with self.assertRaisesRegex(ValueError, "missing_or_ambiguous_work_record"):
                    gate.inspect(self.files)

    def test_wrong_work_title_fails(self):
        self.files["urn_cts_plus_date_label.xml"] = self.files["urn_cts_plus_date_label.xml"].replace(
            b"Anabasis", b"Wrong work")
        with self.assertRaisesRegex(ValueError, "wrong_work_reference"):
            gate.inspect(self.files)

    def test_missing_payload_listing_fails(self):
        self.files["anab_listing.json"] = b"[]"
        with self.assertRaisesRegex(ValueError, "missing_text_listing"):
            gate.inspect(self.files)

    def test_individual_release_file_requires_fresh_review(self):
        self.change_json("zenodo_record.json", lambda d: d["files"][0].update(key="anabasis.zip"))
        with self.assertRaisesRegex(ValueError, "release_inventory_changed_requires_review"):
            gate.inspect(self.files)

    def test_tag_object_is_not_a_commit(self):
        self.change_json("up_tag.json", lambda d: d["object"].update(type="tag"))
        with self.assertRaisesRegex(ValueError, "upstream_tag_reference_mismatch"):
            gate.inspect(self.files)

    def test_wrong_edition_reference_fails(self):
        self.files["up_data_tlg0032_tlg006___cts__.xml"] = b"<work/>"
        with self.assertRaisesRegex(ValueError, "missing_edition_reference"):
            gate.inspect(self.files)

    def test_missing_explicit_declaration_fails(self):
        self.files["oga_README.md"] = b"Public availability alone"
        with self.assertRaisesRegex(ValueError, "missing_declaration_or_release_link"):
            gate.inspect(self.files)

    def test_archive_or_payload_url_in_registry_is_rejected(self):
        registry = {name: {"url": url, "bytes": 1, "sha256": gate.digest(b"x")}
                    for name, url in gate.URLS.items()}
        registry["anab_listing.json"]["url"] = "https://zenodo.org/records/14206061/files/archive.zip"
        with self.assertRaisesRegex(ValueError, "request_outside_documentary_allowlist"):
            gate.load_inputs(Path("unused"), registry, fetch=True)

    def test_missing_registry_entry_fails(self):
        with self.assertRaisesRegex(ValueError, "incomplete_metadata_registry"):
            gate.load_inputs(Path("unused"), {})

    def test_corrupt_retained_metadata_fails(self):
        scratch = Path.home() / ".cache/scratch/greek-oga-anabasis1-import-proof"
        scratch.mkdir(parents=True, exist_ok=True)
        with tempfile.TemporaryDirectory(dir=scratch) as directory:
            cache = Path(directory)
            registry = {}
            for name, url in gate.URLS.items():
                (cache / name).write_bytes(b"x")
                registry[name] = {"url": url, "bytes": 1, "sha256": gate.digest(b"x")}
            (cache / "tree.json").write_bytes(b"y")
            with self.assertRaisesRegex(ValueError, "input_hash_or_size_mismatch"):
                gate.load_inputs(cache, copy.deepcopy(registry))

    def test_redirect_is_not_followed(self):
        with self.assertRaisesRegex(ValueError, "redirect_not_authorized"):
            gate.NoRedirect().redirect_request(None, None, 302, "redirect", {}, "https://example.org")

    def test_json_serialization_variation_is_not_inventory_change(self):
        item = {"json_sha256": gate.digest(gate.encoded({"a": 1, "b": 2}))}
        self.assertTrue(gate.matches_input(b'{"b":2,"a":1}', item))
        self.assertFalse(gate.matches_input(b'{"b":3,"a":1}', item))


if __name__ == "__main__":
    unittest.main()
