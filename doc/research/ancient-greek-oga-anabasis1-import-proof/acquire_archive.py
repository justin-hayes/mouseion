#!/usr/bin/env python3
"""Explicit private official-archive acquisition; no retry, redirect or extraction."""

import hashlib
import json
import os
from pathlib import Path
import shutil
from urllib.request import HTTPRedirectHandler, build_opener


ROOT = Path.home() / ".cache/scratch/greek-oga-anabasis1-import-proof/archive"
NAME = "opera_graeca_adnotata_v0.2.0.zip"
URL = "https://zenodo.org/api/records/14206061/files/" + NAME + "/content"
SIZE = 8_629_032_478
MD5 = "d4117ae52f1cc6319c6ba20af0c1cf3b"


class NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):
        raise ValueError("redirect_requires_review; not followed")


def main():
    ROOT.mkdir(parents=True, exist_ok=True)
    final = ROOT / NAME
    partial = ROOT / (NAME + ".partial")
    if final.exists() or partial.exists():
        raise ValueError("existing_archive_or_partial; refusing overwrite or automatic retry")
    if shutil.disk_usage(ROOT).free < SIZE + 2_000_000_000:
        raise ValueError("insufficient_disk_margin")
    md5 = hashlib.md5()
    sha256 = hashlib.sha256()
    total = 0
    milestone = 256 * 1024 * 1024
    next_report = milestone
    with build_opener(NoRedirect).open(URL, timeout=90) as response:
        length = response.headers.get("Content-Length")
        if length is not None and int(length) != SIZE:
            raise ValueError("unexpected_content_length")
        with partial.open("xb") as destination:
            while data := response.read(1024 * 1024):
                total += len(data)
                if total > SIZE:
                    raise ValueError("archive_exceeds_expected_size")
                md5.update(data)
                sha256.update(data)
                destination.write(data)
                if total >= next_report:
                    print(f"Official archive: {total:,}/{SIZE:,} bytes", flush=True)
                    next_report += milestone
            destination.flush()
            os.fsync(destination.fileno())
    if total != SIZE or md5.hexdigest() != MD5:
        raise ValueError("archive_size_or_published_checksum_mismatch; partial retained")
    partial.rename(final)
    record = {"record_id": 14206061, "url": URL, "file": NAME, "bytes": total,
              "md5": md5.hexdigest(), "sha256": sha256.hexdigest(),
              "operation": "human_authorized_private_archive_inspection",
              "extracted": False}
    (ROOT / "acquisition.json").write_text(json.dumps(record, sort_keys=True, indent=2) + "\n")
    print(json.dumps(record), flush=True)


if __name__ == "__main__":
    main()
