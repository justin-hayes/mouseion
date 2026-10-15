"""Focused tests for the private packaging probe. Fixtures are invented bytes."""

import sys

sys.dont_write_bytecode = True

import io
import json
import subprocess
import tempfile
import unittest
import zipfile
from pathlib import Path
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parent))
import package_probe as probe
import reproduce_proof

HERE = Path(__file__).resolve().parent


def invented_members():
    rows, data = [], {}
    for container, member, tier, _role in probe.SELECTION:
        path = probe.local_path(container, member)
        blob = f"invented {path}\n".encode()
        data[path] = blob
        rows.append(
            {
                "container": container,
                "member": member,
                "tier": tier,
                "local_path": path,
                "bytes": len(blob),
                "sha256": probe.digest(blob),
            }
        )
    return rows, data


def rezip(blob, change):
    """Copy a package, letting change(name, data) return replacement data or None to drop."""
    out = io.BytesIO()
    with zipfile.ZipFile(io.BytesIO(blob)) as src, zipfile.ZipFile(out, "w") as dst:
        for info in src.infolist():
            data = change(info.filename, src.read(info.filename))
            if data is not None:
                dst.writestr(info, data)
    return out.getvalue()


class Selection(unittest.TestCase):
    def test_tiers_and_paths(self):
        paths = [probe.local_path(c, m) for c, m, _, _ in probe.SELECTION]
        self.assertEqual(len(paths), len(set(paths)))
        tiers = [t for _, _, t, _ in probe.SELECTION]
        self.assertEqual(tiers.count("core"), 9)
        self.assertTrue(set(tiers) <= {"core", "notice", "metadata", "schema", "optional"})

    def test_no_witness_interpretation_or_other_edition(self):
        for container, member, _, _ in probe.SELECTION:
            lowered = (container + member).lower()
            for banned in ("gorman", "morpheus", "udperseus", "perseus-ud", "repair", "eng2", "graphannis"):
                self.assertNotIn(banned, lowered)
        selected = {m for _, m, _, _ in probe.SELECTION}
        self.assertFalse(selected & set(probe.EXCLUDED))

    def test_committed_inventory_matches_selection(self):
        record = json.loads((HERE / "work-inventory.json").read_text(encoding="utf-8"))
        self.assertEqual(record["archive"]["sha256"], probe.ARCHIVE_SHA256)
        self.assertEqual(
            sorted(r["local_path"] for r in record["members"]),
            sorted(probe.local_path(c, m) for c, m, _, _ in probe.SELECTION),
        )


class Packages(unittest.TestCase):
    def setUp(self):
        self.rows, self.data = invented_members()

    def test_deterministic_and_fixed_metadata(self):
        _, first = probe.build_package("core", self.rows, self.data)
        _, second = probe.build_package("core", list(reversed(self.rows)), dict(self.data))
        self.assertEqual(first, second)
        with zipfile.ZipFile(io.BytesIO(first)) as zf:
            for info in zf.infolist():
                self.assertEqual(info.date_time, probe.FIXED_TIME)
                self.assertEqual(info.external_attr, 0o100644 << 16)
                self.assertEqual(info.compress_type, zipfile.ZIP_DEFLATED)

    def test_core_excludes_optional_and_complete_adds_it(self):
        core, _ = probe.build_package("core", self.rows, self.data)
        complete, _ = probe.build_package("complete", self.rows, self.data)
        self.assertNotIn("optional", {m["tier"] for m in core["members"]})
        self.assertEqual(len(complete["members"]), len(self.rows))
        paths = [m["local_path"] for m in core["members"]]
        self.assertEqual(paths, sorted(paths))

    def test_verify_round_trip(self):
        manifest, blob = probe.build_package("core", self.rows, self.data)
        checked, members = probe.verify_package(blob)
        self.assertEqual(checked, manifest)
        self.assertEqual(set(members), {m["local_path"] for m in manifest["members"]})

    def test_verify_rejects_changed_extra_missing_and_unsafe(self):
        manifest, blob = probe.build_package("core", self.rows, self.data)
        target = manifest["members"][0]["local_path"]
        cases = {
            "package_member_hash_mismatch": rezip(
                blob, lambda n, d: d.replace(b"invented", b"inventeD") if n == target else d
            ),
            "package_members_differ_from_manifest": rezip(blob, lambda n, d: None if n == target else d),
            "manifest_or_notice_missing": rezip(blob, lambda n, d: None if n == "NOTICE.txt" else d),
        }
        extra = io.BytesIO(blob)
        with zipfile.ZipFile(extra, "a") as zf:
            zf.writestr("extra.xml", b"x")
        cases["package_members_differ_from_manifest "] = extra.getvalue()
        unsafe = json.loads(json.dumps(manifest))
        unsafe["members"][0]["local_path"] = "../escape.xml"
        cases["unsafe_package_path"] = probe.zip_bytes(
            [("MANIFEST.json", probe.encoded(unsafe)), ("NOTICE.txt", b"notice")]
            + [(m["local_path"], self.data.get(m["local_path"], self.data[target])) for m in unsafe["members"]]
        )
        wrong = json.loads(json.dumps(manifest))
        wrong["source"]["archive_sha256"] = "0" * 64
        cases["wrong_package_format_or_source"] = rezip(
            blob, lambda n, d: probe.encoded(wrong) if n == "MANIFEST.json" else d
        )
        for reason, bad in cases.items():
            with self.subTest(reason=reason), self.assertRaisesRegex(ValueError, reason.strip()):
                probe.verify_package(bad)


class NestedArchive(unittest.TestCase):
    def build(self, inner_type=zipfile.ZIP_STORED):
        inner = io.BytesIO()
        with zipfile.ZipFile(inner, "w") as zf:
            zf.writestr("oga/work/a.xml", b"<a/>")
        outer = io.BytesIO()
        with zipfile.ZipFile(outer, "w") as zf:
            zf.writestr(probe.PREFIX + "README.md", b"readme", compress_type=zipfile.ZIP_DEFLATED)
            zf.writestr(probe.PREFIX + "workspace/oga.zip", inner.getvalue(), compress_type=inner_type)
        with tempfile.NamedTemporaryFile(suffix=".zip", delete=False) as handle:
            handle.write(outer.getvalue())
        self.addCleanup(Path(handle.name).unlink)
        return Path(handle.name)

    def test_reads_members_inside_stored_container_without_extracting_it(self):
        path = self.build()
        selection = (("outer", "README.md", "notice", "r"), ("workspace/oga.zip", "oga/work/a.xml", "core", "a"))
        with mock.patch.object(probe, "SELECTION", selection):
            got = {row[1]: data for row, _info, data in probe.selected_members(path)}
        self.assertEqual(got, {"README.md": b"readme", "oga/work/a.xml": b"<a/>"})

    def test_rejects_compressed_container_missing_member_and_unsafe_path(self):
        selection = (("workspace/oga.zip", "oga/work/a.xml", "core", "a"),)
        with mock.patch.object(probe, "SELECTION", selection), self.assertRaisesRegex(ValueError, "not_stored"):
            list(probe.selected_members(self.build(zipfile.ZIP_DEFLATED)))
        for member, reason in (("oga/work/b.xml", "missing_or_duplicate"), ("../a.xml", "unsafe_member_path")):
            selection = (("workspace/oga.zip", member, "core", "a"),)
            with mock.patch.object(probe, "SELECTION", selection), self.assertRaisesRegex(ValueError, reason):
                list(probe.selected_members(self.build()))

    def test_archive_identity_is_checked_before_reading(self):
        with self.assertRaisesRegex(ValueError, "archive_size_mismatch"):
            probe.verify_archive(self.build())


class Reproduction(unittest.TestCase):
    def test_committed_result_separates_package_from_witnesses(self):
        record = json.loads((HERE / "proof-reproduction.json").read_text(encoding="utf-8"))
        self.assertEqual(record["proof_commit"], reproduce_proof.PROOF_COMMIT)
        self.assertEqual(record["package"]["tier"], "core")
        self.assertTrue(all(p.startswith(reproduce_proof.GORMAN) for p in record["gorman_registry_entries_not_staged"]))
        self.assertEqual(record["runs"]["opus_repair"]["report"]["selected"], 438)
        self.assertEqual(record["runs"]["opus_repair"]["report"]["card_inputs"], 434)
        packages = json.loads((HERE / "package-manifests.json").read_text(encoding="utf-8"))
        self.assertEqual(record["package"]["sha256"], packages["packages"]["core"]["sha256"])

    def test_staging_refuses_a_non_empty_or_wrong_proof_tree(self):
        with tempfile.TemporaryDirectory() as tmp, self.assertRaises((ValueError, subprocess.CalledProcessError)):
            reproduce_proof.stage(Path(tmp) / "missing.zip", Path(tmp), Path(tmp))


if __name__ == "__main__":
    unittest.main()
