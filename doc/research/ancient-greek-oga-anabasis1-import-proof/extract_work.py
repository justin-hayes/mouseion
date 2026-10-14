#!/usr/bin/env python3
"""Verify the authorized archive and extract only the declared Anabasis inputs."""

import argparse
import hashlib
import io
import json
from pathlib import Path, PurePosixPath
import struct
import zipfile

HERE = Path(__file__).resolve().parent
ARCHIVE_NAME = "opera_graeca_adnotata_v0.2.0.zip"
ARCHIVE_SIZE = 8_629_032_478
ARCHIVE_SHA256 = "fffc246e9833f7ef265f0daffd1f4cc679489cb6de1fd9b652474a6313ecf6c3"
ARCHIVE_MD5 = "d4117ae52f1cc6319c6ba20af0c1cf3b"
PREFIX = "opera_graeca_adnotata_v0.2.0/"
UNIT = "tlg0032.tlg006.perseus-grc2"
OGA = f"oga/oga_v0.2.0_2/{UNIT}/"
LAULA = f"oga/laula/{UNIT}/"
ORIGINAL = "original_Greek_files/canonical-greekLit-0.0.11376425141/"
LAYERS = (
    "anno.xml", "anno_author.xml", "anno_title.xml", "anno_work_date.xml",
    "anno_urn_cts.xml", "anno_urn_cts_author.xml", "anno_urn_cts_edition.xml", "anno_urn_cts_work.xml",
    UNIT + ".xml", UNIT + ".tok01.xml", UNIT + ".tok01_cts01.xml",
    UNIT + ".tok01_ipa01.xml", UNIT + ".tok01_lemma01.xml",
    "postag." + UNIT + ".tok01_postag01.xml",
    "dep." + UNIT + ".tok01_dep01.xml", "dep." + UNIT + ".tok01_dep01_dep-fnc01.xml",
)
INNER_INPUTS = {
    "original_Greek_files.zip": (
        ORIGINAL + f"data/tlg0032/tlg006/{UNIT}.xml",
        ORIGINAL + f"text/{UNIT}.txt",
    ),
    "workspace/merge.zip": (f"merge/tlg0032/tlg006/{UNIT}.xml",),
    "workspace/oga.zip": tuple(OGA + name for name in LAYERS) + (
        LAULA + UNIT + ".xml", LAULA + UNIT + ".tok01.xml",
        LAULA + UNIT + ".tok01_sentence-seg01.xml",
    ),
    "workspace/conllu.zip": (f"conllu/{UNIT}.tok01_sentence-seg01_annotated_lemma.conllu",),
}
OUTER_INPUTS = ("README.md", "query/README.md", "query/morphology.md", "query/syntax.md")


def encoded(value):
    return (json.dumps(value, ensure_ascii=False, sort_keys=True, indent=2) + "\n").encode()


def require(condition, reason):
    if not condition:
        raise ValueError(reason)


class StoredMemberView(io.RawIOBase):
    """Seekable bounded local view: do not unpack a nested corpus ZIP."""

    def __init__(self, path, start, size):
        super().__init__()
        self.file = path.open("rb")
        self.start, self.size, self.position = start, size, 0

    def read(self, size=-1):
        if size < 0:
            size = self.size - self.position
        size = min(size, self.size - self.position)
        self.file.seek(self.start + self.position)
        data = self.file.read(size)
        self.position += len(data)
        return data

    def seek(self, offset, whence=0):
        require(whence in (0, 1, 2), "invalid_seek_mode")
        position = offset if whence == 0 else self.position + offset if whence == 1 else self.size + offset
        require(0 <= position <= self.size, "seek_outside_stored_member")
        self.position = position
        return position

    def tell(self):
        return self.position

    def seekable(self):
        return True

    def readable(self):
        return True

    def close(self):
        self.file.close()
        super().close()


def nested_view(path, info):
    require(info.compress_type == zipfile.ZIP_STORED and not info.flag_bits & 1,
            "unsupported_nested_compression_or_encryption")
    with path.open("rb") as source:
        source.seek(info.header_offset)
        header = struct.unpack("<4s5H3I2H", source.read(30))
        require(header[0] == b"PK\x03\x04", "invalid_local_zip_header")
        local_name = source.read(header[-2]).decode("utf-8")
        require(local_name == info.filename, "local_central_filename_mismatch")
        start = info.header_offset + 30 + header[-2] + header[-1]
        require(start + info.file_size <= path.stat().st_size, "nested_member_out_of_bounds")
    return StoredMemberView(path, start, info.file_size)


def verify_archive(path):
    require(path.stat().st_size == ARCHIVE_SIZE, "archive_size_mismatch")
    md5, sha256 = hashlib.md5(), hashlib.sha256()
    with path.open("rb") as source:
        while data := source.read(8 * 1024 * 1024):
            md5.update(data)
            sha256.update(data)
    require(md5.hexdigest() == ARCHIVE_MD5 and sha256.hexdigest() == ARCHIVE_SHA256,
            "archive_checksum_mismatch")


def retain(z, member, container, cache):
    path = PurePosixPath(member)
    require(not path.is_absolute() and ".." not in path.parts, "unsafe_member_path")
    require(sum(i.filename == member for i in z.infolist()) == 1, "missing_or_duplicate_member")
    info = z.getinfo(member)
    require(0 < info.file_size <= 32_000_000 and not info.flag_bits & 1,
            "selected_member_size_or_encryption")
    data = z.read(member)  # zipfile also verifies the member CRC.
    destination = cache / container.removesuffix(".zip") / member
    if destination.exists():
        require(destination.read_bytes() == data, "retained_original_differs")
    else:
        destination.parent.mkdir(parents=True, exist_ok=True)
        destination.write_bytes(data)
    return {"container": container, "member": member, "bytes": len(data),
            "compressed_bytes": info.compress_size, "crc32": f"{info.CRC:08x}",
            "sha256": hashlib.sha256(data).hexdigest(),
            "local_relative_path": destination.relative_to(cache).as_posix()}


def extract(path, cache):
    verify_archive(path)
    records = []
    with zipfile.ZipFile(path) as outer:
        for member in OUTER_INPUTS:
            records.append(retain(outer, PREFIX + member, "outer", cache))
        for container, members in INNER_INPUTS.items():
            info = outer.getinfo(PREFIX + container)
            require(sum(i.filename == info.filename for i in outer.infolist()) == 1,
                    "duplicate_nested_container")
            with nested_view(path, info) as view, zipfile.ZipFile(view) as inner:
                for member in members:
                    records.append(retain(inner, member, container, cache))
    return {"version": "oga-authorized-anabasis-extraction/1", "archive": {
        "record_id": 14206061, "file": ARCHIVE_NAME, "bytes": ARCHIVE_SIZE,
        "md5": ARCHIVE_MD5, "sha256": ARCHIVE_SHA256},
        "scope": "indivisible Anabasis work inputs only; analysis limited to Book 1",
        "members": records}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--archive", type=Path, default=HERE / "private/archive" / ARCHIVE_NAME)
    parser.add_argument("--cache", type=Path, default=HERE / "private/archive-members")
    parser.add_argument("--write", action="store_true")
    args = parser.parse_args()
    result = encoded(extract(args.archive, args.cache))
    manifest = HERE / "archive-members.json"
    if args.write:
        manifest.write_bytes(result)
    else:
        require(manifest.read_bytes() == result, "archive_member_manifest_differs")
    print("Archive verified; selected Anabasis originals retained; no corpus-wide extraction.")


if __name__ == "__main__":
    main()
