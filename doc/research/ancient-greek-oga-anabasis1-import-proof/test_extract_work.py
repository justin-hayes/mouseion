"""Small invented ZIPs only: never download or unpack the corpus in tests."""

import hashlib
import io
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import zipfile

import extract_work as extract


class ExtractionTests(unittest.TestCase):
    def setUp(self):
        scratch = Path.home() / ".cache/scratch/greek-oga-anabasis1-import-proof"
        scratch.mkdir(parents=True, exist_ok=True)
        temp = tempfile.TemporaryDirectory(dir=scratch)
        self.addCleanup(temp.cleanup)
        self.root = Path(temp.name)
        self.path = self.root / "invented.zip"

    def archive(self, compressed=False):
        inner = io.BytesIO()
        with zipfile.ZipFile(inner, "w", compression=zipfile.ZIP_DEFLATED) as z:
            z.writestr("unit.xml", b"<unit>invented</unit>")
            z.writestr("unrelated.xml", b"must not be retained")
        with zipfile.ZipFile(self.path, "w", compression=zipfile.ZIP_DEFLATED if compressed else zipfile.ZIP_STORED) as z:
            z.writestr("inner.zip", inner.getvalue())

    def test_bounded_view_reads_only_requested_member(self):
        self.archive()
        with zipfile.ZipFile(self.path) as outer:
            with extract.nested_view(self.path, outer.getinfo("inner.zip")) as view:
                with zipfile.ZipFile(view) as inner:
                    record = extract.retain(inner, "unit.xml", "inner.zip", self.root / "retained")
                    self.assertEqual(record["sha256"], hashlib.sha256(b"<unit>invented</unit>").hexdigest())
        self.assertFalse((self.root / "retained/inner/unrelated.xml").exists())

    def test_view_cannot_seek_outside_nested_container(self):
        self.archive()
        with extract.StoredMemberView(self.path, 0, 10) as view:
            for position in (-1, 11):
                with self.assertRaisesRegex(ValueError, "outside_stored"):
                    view.seek(position)
            self.assertEqual(len(view.read(9999)), 10)
            self.assertEqual(view.read(), b"")

    def test_compressed_nested_corpus_is_not_unpacked(self):
        self.archive(compressed=True)
        with zipfile.ZipFile(self.path) as outer:
            with self.assertRaisesRegex(ValueError, "unsupported_nested_compression"):
                extract.nested_view(self.path, outer.getinfo("inner.zip"))

    def test_wrong_archive_checksum_fails(self):
        self.archive()
        with patch.object(extract, "ARCHIVE_SIZE", self.path.stat().st_size):
            with self.assertRaisesRegex(ValueError, "archive_checksum_mismatch"):
                extract.verify_archive(self.path)

    def test_wrong_archive_size_fails(self):
        self.archive()
        with self.assertRaisesRegex(ValueError, "archive_size_mismatch"):
            extract.verify_archive(self.path)

    def test_member_path_traversal_rejected(self):
        with zipfile.ZipFile(self.path, "w") as z:
            z.writestr("../outside.xml", b"invented")
        with zipfile.ZipFile(self.path) as z:
            with self.assertRaisesRegex(ValueError, "unsafe_member"):
                extract.retain(z, "../outside.xml", "inner.zip", self.root / "retained")

    def test_changed_retained_original_is_not_overwritten(self):
        with zipfile.ZipFile(self.path, "w") as z:
            z.writestr("unit.xml", b"invented")
        with zipfile.ZipFile(self.path) as z:
            record = extract.retain(z, "unit.xml", "inner.zip", self.root / "retained")
            output = self.root / "retained" / record["local_relative_path"]
            output.write_bytes(b"user changes")
            with self.assertRaisesRegex(ValueError, "retained_original_differs"):
                extract.retain(z, "unit.xml", "inner.zip", self.root / "retained")
            self.assertEqual(output.read_bytes(), b"user changes")

    def test_missing_target_member_fails(self):
        self.archive()
        with zipfile.ZipFile(self.path) as z:
            with self.assertRaisesRegex(ValueError, "missing_or_duplicate_member"):
                extract.retain(z, "missing.xml", "outer", self.root / "retained")


if __name__ == "__main__":
    unittest.main()
