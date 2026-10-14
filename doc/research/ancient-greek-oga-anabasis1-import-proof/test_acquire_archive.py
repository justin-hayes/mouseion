"""Small invented bytes only: acquisition boundary tests, not corpus evidence."""

import hashlib
import io
import json
from contextlib import redirect_stdout
from pathlib import Path
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch

import acquire_archive as acquire


class Response(io.BytesIO):
    def __init__(self, data, declared_length):
        super().__init__(data)
        self.headers = {"Content-Length": str(declared_length)}


class AcquisitionTests(unittest.TestCase):
    def setUp(self):
        scratch = Path.home() / ".cache/scratch/greek-oga-anabasis1-import-proof"
        scratch.mkdir(parents=True, exist_ok=True)
        self.temp = tempfile.TemporaryDirectory(dir=scratch)
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.data = b"invented archive fixture"
        for target, value in (("ROOT", self.root), ("SIZE", len(self.data)),
                              ("MD5", hashlib.md5(self.data).hexdigest())):
            patched = patch.object(acquire, target, value)
            patched.start()
            self.addCleanup(patched.stop)
        disk = patch.object(acquire.shutil, "disk_usage", return_value=SimpleNamespace(free=3_000_000_000))
        disk.start()
        self.addCleanup(disk.stop)

    def run_request(self, data=None, length=None):
        data = self.data if data is None else data
        response = Response(data, len(self.data) if length is None else length)
        with patch.object(acquire, "build_opener") as opener:
            opener.return_value.open.return_value = response
            acquire.main()
            opener.return_value.open.assert_called_once_with(acquire.URL, timeout=90)

    def test_single_official_request_verified_before_rename(self):
        with redirect_stdout(io.StringIO()):
            self.run_request()
        self.assertEqual((self.root / acquire.NAME).read_bytes(), self.data)
        self.assertFalse((self.root / (acquire.NAME + ".partial")).exists())
        record = json.loads((self.root / "acquisition.json").read_text())
        self.assertEqual(record["sha256"], hashlib.sha256(self.data).hexdigest())
        self.assertFalse(record["extracted"])

    def test_partial_prevents_unapproved_retry(self):
        (self.root / (acquire.NAME + ".partial")).write_bytes(b"partial")
        with patch.object(acquire, "build_opener") as opener:
            with self.assertRaisesRegex(ValueError, "refusing overwrite"):
                acquire.main()
            opener.assert_not_called()

    def test_existing_archive_is_not_overwritten(self):
        (self.root / acquire.NAME).write_bytes(b"retained")
        with patch.object(acquire, "build_opener") as opener:
            with self.assertRaisesRegex(ValueError, "refusing overwrite"):
                acquire.main()
            opener.assert_not_called()

    def test_wrong_checksum_preserves_partial_not_verified_input(self):
        with patch.object(acquire, "MD5", "wrong"):
            with self.assertRaisesRegex(ValueError, "checksum_mismatch"):
                self.run_request()
        self.assertFalse((self.root / acquire.NAME).exists())
        self.assertTrue((self.root / (acquire.NAME + ".partial")).exists())
        self.assertFalse((self.root / "acquisition.json").exists())

    def test_truncated_body_cannot_pass(self):
        with self.assertRaisesRegex(ValueError, "size_or_published_checksum_mismatch"):
            self.run_request(data=self.data[:-1])
        self.assertFalse((self.root / acquire.NAME).exists())

    def test_oversized_body_cannot_pass(self):
        with self.assertRaisesRegex(ValueError, "exceeds_expected_size"):
            self.run_request(data=self.data + b"x")
        self.assertFalse((self.root / acquire.NAME).exists())

    def test_wrong_declared_length_stops_before_body_file(self):
        with self.assertRaisesRegex(ValueError, "unexpected_content_length"):
            self.run_request(length=1)
        self.assertFalse((self.root / (acquire.NAME + ".partial")).exists())

    def test_disk_guard_prevents_request(self):
        with patch.object(acquire.shutil, "disk_usage", return_value=SimpleNamespace(free=1)):
            with patch.object(acquire, "build_opener") as opener:
                with self.assertRaisesRegex(ValueError, "insufficient_disk_margin"):
                    acquire.main()
                opener.assert_not_called()

    def test_access_error_is_not_retried(self):
        with patch.object(acquire, "build_opener") as opener:
            opener.return_value.open.side_effect = PermissionError("access denied fixture")
            with self.assertRaises(PermissionError):
                acquire.main()
            self.assertEqual(opener.return_value.open.call_count, 1)
        self.assertFalse((self.root / acquire.NAME).exists())

    def test_redirect_is_not_followed(self):
        with self.assertRaisesRegex(ValueError, "redirect_requires_review"):
            acquire.NoRedirect().redirect_request(None, None, 302, "redirect", {}, "https://example.org")


if __name__ == "__main__":
    unittest.main()
