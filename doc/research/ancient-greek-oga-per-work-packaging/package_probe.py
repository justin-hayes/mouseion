#!/usr/bin/env python3
"""Private per-work OGA packaging probe for Anabasis (#2053).

Reads only the retained, already verified official OGA v0.2.0 archive. It never
downloads, never range-fetches and never writes outside the explicit output
directory. Three subcommands:

    inventory  verify the archive; record Anabasis members, notices and
               central-directory-only corpus sizes in work-inventory.json
    assemble   build deterministic private core/complete packages and record
               their identities in package-manifests.json
    verify     check a private package against its embedded manifest

Every package member is a byte-identical original archive member. The package
adds only a manifest and a Mouseion notice; it subsets nothing inside a file.
"""

import sys

sys.dont_write_bytecode = True

import argparse
import hashlib
import io
import json
import statistics
import struct
import zipfile
import zlib
from pathlib import Path, PurePosixPath

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
CONLLU = f"conllu/{UNIT}.tok01_sentence-seg01_annotated_lemma.conllu"
FORMAT = "mouseion-oga-work-package/1"
PACKAGE_STEM = f"oga-v0.2.0_{UNIT}"
FIXED_TIME = (1980, 1, 1, 0, 0, 0)
MAX_MEMBER_BYTES = 32_000_000

# (container, member, tier, role). "core" members are the exact inputs the
# #2052 proof reads; "notice", "metadata" and "schema" travel with them in the
# core package; "optional" members are per-work layers or duplicates the proof
# does not read.
SELECTION = (
    (
        ("workspace/oga.zip", OGA + UNIT + ".xml", "core", "PAULA primary text: annotation-carried reading text"),
        ("workspace/oga.zip", OGA + UNIT + ".tok01.xml", "core", "PAULA token spans"),
        ("workspace/oga.zip", OGA + UNIT + ".tok01_cts01.xml", "core", "PAULA token CTS citations"),
        ("workspace/oga.zip", OGA + UNIT + ".tok01_lemma01.xml", "core", "PAULA automatic lemma predictions"),
        ("workspace/oga.zip", OGA + "postag." + UNIT + ".tok01_postag01.xml", "core", "PAULA automatic POS/morphology"),
        ("workspace/oga.zip", LAULA + UNIT + ".xml", "core", "LAULA merged TEI: section binding"),
        ("workspace/oga.zip", LAULA + UNIT + ".tok01.xml", "core", "LAULA token pointers: node join/diagnostic"),
        (
            "original_Greek_files.zip",
            ORIGINAL + f"data/tlg0032/tlg006/{UNIT}.xml",
            "core",
            "upstream canonical-greekLit TEI: Edition authority",
        ),
        ("workspace/conllu.zip", CONLLU, "core", "CoNLL-U sentence groups and syntax used by the εἰμί rule"),
        ("outer", "README.md", "notice", "OGA release README: CC BY-SA 4.0 declaration and citation"),
        ("outer", "query/README.md", "notice", "OGA layer documentation; pinned by the proof"),
        (
            "original_Greek_files.zip",
            "original_Greek_files/README.md",
            "notice",
            "OGA note on bundled original repositories",
        ),
        ("original_Greek_files.zip", ORIGINAL + "README.md", "notice", "canonical-greekLit README: license statement"),
        ("original_Greek_files.zip", ORIGINAL + "license.md", "notice", "canonical-greekLit CC BY-SA 4.0 legal code"),
        ("original_Greek_files.zip", ORIGINAL + "data/tlg0032/__cts__.xml", "metadata", "CTS textgroup metadata"),
        (
            "original_Greek_files.zip",
            ORIGINAL + "data/tlg0032/tlg006/__cts__.xml",
            "metadata",
            "CTS work/edition metadata",
        ),
    )
    + tuple(
        ("workspace/oga.zip", OGA + name, "metadata", "PAULA work metadata annotation")
        for name in (
            "anno.xml",
            "anno_author.xml",
            "anno_title.xml",
            "anno_work_date.xml",
            "anno_urn_cts.xml",
            "anno_urn_cts_author.xml",
            "anno_urn_cts_edition.xml",
            "anno_urn_cts_work.xml",
        )
    )
    + (
        ("workspace/oga.zip", OGA + UNIT + ".tok01_ipa01.xml", "optional", "PAULA IPA transcription layer"),
        ("workspace/oga.zip", OGA + "dep." + UNIT + ".tok01_dep01.xml", "optional", "PAULA dependency layer"),
        (
            "workspace/oga.zip",
            OGA + "dep." + UNIT + ".tok01_dep01_dep-fnc01.xml",
            "optional",
            "PAULA dependency functions",
        ),
        ("workspace/oga.zip", LAULA + UNIT + ".tok01_sentence-seg01.xml", "optional", "LAULA sentence segmentation"),
        (
            "workspace/merge.zip",
            f"merge/tlg0032/tlg006/{UNIT}.xml",
            "optional",
            "merged TEI; same bytes as LAULA merged TEI",
        ),
        ("workspace/ellipsis.zip", f"ellipsis/{UNIT}.tok01_sentence-seg01.csv", "optional", "ellipsis table"),
        ("original_Greek_files.zip", ORIGINAL + f"text/{UNIT}.txt", "optional", "upstream plain-text rendering"),
        ("outer", "query/morphology.md", "optional", "OGA query documentation"),
        ("outer", "query/syntax.md", "optional", "OGA query documentation"),
    )
    + tuple(
        (
            "workspace/oga.zip",
            "oga/paula_dtd/" + name,
            "schema",
            "PAULA DTD named by member DOCTYPEs (proof parser does not load it)",
        )
        for name in (
            "paula_feat.dtd",
            "paula_header.dtd",
            "paula_mark.dtd",
            "paula_multiFeat.dtd",
            "paula_rel.dtd",
            "paula_struct.dtd",
            "paula_text.dtd",
        )
    )
)
TIERS = {
    "core": ("core", "notice", "metadata", "schema"),
    "complete": ("core", "notice", "metadata", "schema", "optional"),
}
# Same-work members deliberately outside every package.
EXCLUDED = {
    ORIGINAL + "data/tlg0032/tlg006/tlg0032.tlg006.perseus-eng2.xml": "English translation: a different Edition",
    ORIGINAL + "text/tlg0032.tlg006.perseus-eng2.txt": "English translation: a different Edition",
}
CORE_LAYER_SUFFIXES = (".xml", ".tok01.xml", ".tok01_cts01.xml", ".tok01_lemma01.xml", ".tok01_postag01.xml")

NOTICE = """Mouseion private per-work package (research probe, #2053). Not a public release.

Contents: byte-identical members of Opera Graeca Adnotata v0.2.0
(Giuseppe G. A. Celano, 2024; doi:10.5281/zenodo.14206061; official archive
{archive} bytes, SHA-256 {sha}). The release README declares CC BY-SA 4.0
(https://creativecommons.org/licenses/by-sa/4.0/) and refers to the licences of
the original-text repositories. Requested citation: Celano, "Opera Graeca
Adnotata: Building a 34M+ Token Multilayer Corpus for Ancient Greek",
arXiv:2404.00739.

Original text: Xenophon, Anabasis, ed. E. C. Marchant (Oxford, 1904; repr.
1961), Perseus Digital Library canonical-greekLit release 0.0.11376425141
(commit 476fb6133498e5710de7eec0b492791da86be4b4), as bundled in the archive;
README declares CC BY-SA 4.0 unless otherwise indicated.

Modification notice: no member was edited, re-encoded or subset. Mouseion
selected these members from nested archive containers, renamed their paths to
<container>/<member>, and added MANIFEST.json and this NOTICE. Automatic lemma,
POS and syntax predictions are reproduced as released, without endorsement.
No Mouseion interpretation, repair, Morpheus or UD witness is included.
"""


def require(condition, reason):
    if not condition:
        raise ValueError(reason)


def encoded(value):
    return (json.dumps(value, ensure_ascii=False, sort_keys=True, indent=1) + "\n").encode()


def digest(data):
    return hashlib.sha256(data).hexdigest()


def local_path(container, member):
    if container == "outer":
        return "outer/" + PREFIX + member
    return container.removesuffix(".zip") + "/" + member


class StoredView(io.RawIOBase):
    """Seekable window over one stored (uncompressed) nested ZIP in the archive."""

    def __init__(self, handle, start, size):
        super().__init__()
        self.handle, self.start, self.size, self.position = handle, start, size, 0

    def read(self, size=-1):
        size = self.size - self.position if size < 0 else min(size, self.size - self.position)
        self.handle.seek(self.start + self.position)
        data = self.handle.read(size)
        self.position += len(data)
        return data

    def seek(self, offset, whence=0):
        position = (offset, self.position + offset, self.size + offset)[whence]
        require(0 <= position <= self.size, "seek_outside_stored_member")
        self.position = position
        return position

    def tell(self):
        return self.position

    def seekable(self):
        return True

    def readable(self):
        return True


def nested(handle, outer, container):
    info = outer.getinfo(PREFIX + container)
    require(info.compress_type == zipfile.ZIP_STORED and not info.flag_bits & 1, "nested_container_not_stored")
    handle.seek(info.header_offset)
    header = struct.unpack("<4s5H3I2H", handle.read(30))
    require(header[0] == b"PK\x03\x04", "invalid_local_header")
    require(handle.read(header[-2]).decode() == info.filename, "local_central_name_mismatch")
    start = info.header_offset + 30 + header[-2] + header[-1]
    return zipfile.ZipFile(StoredView(handle, start, info.file_size))


def verify_archive(path):
    require(path.stat().st_size == ARCHIVE_SIZE, "archive_size_mismatch")
    md5, sha = hashlib.md5(), hashlib.sha256()
    with path.open("rb") as handle:
        while block := handle.read(16 * 1024 * 1024):
            md5.update(block)
            sha.update(block)
    require(md5.hexdigest() == ARCHIVE_MD5 and sha.hexdigest() == ARCHIVE_SHA256, "archive_checksum_mismatch")


def read_member(zf, name):
    path = PurePosixPath(name)
    require(not path.is_absolute() and ".." not in path.parts, "unsafe_member_path")
    require(sum(i.filename == name for i in zf.infolist()) == 1, "missing_or_duplicate_member: " + name)
    info = zf.getinfo(name)
    require(0 < info.file_size <= MAX_MEMBER_BYTES and not info.flag_bits & 1, "member_size_or_encryption")
    return info, zf.read(name)  # zipfile checks the stored CRC


def selected_members(archive):
    """Yield (selection row, ZipInfo, bytes) for every selected member."""
    with archive.open("rb") as handle, zipfile.ZipFile(handle) as outer:
        groups = {}
        for row in SELECTION:
            groups.setdefault(row[0], []).append(row)
        for container, rows in groups.items():
            zf = outer if container == "outer" else nested(handle, outer, container)
            for row in rows:
                name = PREFIX + row[1] if container == "outer" else row[1]
                info, data = read_member(zf, name)
                yield row, info, data


def corpus_sizes(archive):
    """Central-directory-only sizes; nothing is decompressed or interpreted."""
    with archive.open("rb") as handle, zipfile.ZipFile(handle) as outer:
        containers = {
            i.filename.removeprefix(PREFIX): i.file_size for i in outer.infolist() if i.filename.endswith(".zip")
        }
        works = {}
        for info in nested(handle, outer, "workspace/oga.zip").infolist():
            parts = info.filename.split("/")
            if len(parts) == 4 and parts[1].startswith("oga_v0.2.0_") and parts[3]:
                work = parts[2]
                if parts[3].startswith("anno") or any(
                    parts[3] == p + work + s for p in ("", "postag.") for s in CORE_LAYER_SUFFIXES
                ):
                    works.setdefault(work, [0, 0])
                    works[work][0] += info.compress_size
                    works[work][1] += info.file_size
            elif len(parts) == 4 and parts[1] == "laula" and parts[3] in (parts[2] + ".xml", parts[2] + ".tok01.xml"):
                works.setdefault(parts[2], [0, 0])
                works[parts[2]][0] += info.compress_size
                works[parts[2]][1] += info.file_size
        conllu = 0
        for info in nested(handle, outer, "workspace/conllu.zip").infolist():
            work = info.filename.removeprefix("conllu/").split(".tok01_")[0]
            if info.filename.endswith("_annotated_lemma.conllu") and work in works:
                conllu += 1
                works[work][0] += info.compress_size
                works[work][1] += info.file_size
    compressed = sorted(v[0] for v in works.values())
    expanded = sorted(v[1] for v in works.values())

    def profile(values):
        return {
            "total": sum(values),
            "median": int(statistics.median(values)),
            "p90": values[int(0.9 * (len(values) - 1))],
            "max": values[-1],
        }

    return {
        "interpretation": "archive-native deflate sizes from central directories; not packages, "
        "not validated, original TEI excluded; no member decompressed",
        "nested_container_bytes": dict(sorted(containers.items())),
        "works_with_paula_core_layers": len(works),
        "of_which_with_conllu": conllu,
        "core_layers_compressed_bytes": profile(compressed),
        "core_layers_uncompressed_bytes": profile(expanded),
        "anabasis_core_layers": dict(zip(("compressed_bytes", "uncompressed_bytes"), works[UNIT])),
    }


def release_catalogue(archive):
    """Release-level bibliographic lists usable for metadata-only discovery."""
    result = {}
    with zipfile.ZipFile(archive) as outer:
        for name in ("urn_cts/README.md", "urn_cts/texts/urn_cts.xml", "urn_cts/texts/urn_cts_plus_date_label.xml"):
            _, data = read_member(outer, PREFIX + name)
            record = {"bytes": len(data), "sha256": digest(data)}
            if name.endswith(".xml"):
                text = data.decode("utf-8")
                record["work_entries"] = text.count("<work>")
                record["lists_this_work"] = f"<urn_cts>{UNIT}</urn_cts>" in text
                record["names_partitions_members_or_hashes"] = any(
                    marker in text for marker in ("oga_v0.2.0_", ".conllu", "tok01", "sha", "<size")
                )
            result[name] = record
    return result


def inventory(archive):
    verify_archive(archive)
    rows = []
    for (container, member, tier, role), info, data in selected_members(archive):
        rows.append(
            {
                "container": container,
                "member": member,
                "tier": tier,
                "role": role,
                "local_path": local_path(container, member),
                "bytes": len(data),
                "archive_compressed_bytes": info.compress_size,
                "crc32": f"{info.CRC:08x}",
                "sha256": digest(data),
            }
        )
    return {
        "version": "oga-anabasis-work-inventory/1",
        "archive": {
            "record_id": 14206061,
            "doi": "10.5281/zenodo.14206061",
            "file": ARCHIVE_NAME,
            "bytes": ARCHIVE_SIZE,
            "md5": ARCHIVE_MD5,
            "sha256": ARCHIVE_SHA256,
        },
        "work": UNIT,
        "validated_scope": "Book 1 only (#2052); Books 2-7 are carried, not validated",
        "excluded_same_work_members": EXCLUDED,
        "members": rows,
        "corpus": corpus_sizes(archive),
        "release_catalogue": release_catalogue(archive),
    }


def zip_bytes(files):
    """Deterministic ZIP: fixed order, timestamp, mode and deflate level."""
    buffer = io.BytesIO()
    with zipfile.ZipFile(buffer, "w") as zf:
        for name, data in files:
            info = zipfile.ZipInfo(name, date_time=FIXED_TIME)
            info.compress_type = zipfile.ZIP_DEFLATED
            info.create_system = 3
            info.external_attr = 0o100644 << 16
            zf.writestr(info, data, compress_type=zipfile.ZIP_DEFLATED, compresslevel=9)
    return buffer.getvalue()


def package_files(tier, members):
    rows = sorted((r for r in members if r["tier"] in TIERS[tier]), key=lambda r: r["local_path"])
    manifest = {
        "format": FORMAT,
        "tier": tier,
        "work": UNIT,
        "cts_urn": "urn:cts:greekLit:tlg0032.tlg006.perseus-grc2",
        "source": {
            "release": "Opera Graeca Adnotata v0.2.0",
            "doi": "10.5281/zenodo.14206061",
            "archive_file": ARCHIVE_NAME,
            "archive_bytes": ARCHIVE_SIZE,
            "archive_sha256": ARCHIVE_SHA256,
            "archive_md5": ARCHIVE_MD5,
            "text_repository": "PerseusDL/canonical-greekLit",
            "text_release": "0.0.11376425141",
            "text_commit": "476fb6133498e5710de7eec0b492791da86be4b4",
        },
        "transformation": "byte-identical member selection; paths renamed to <container>/<member>",
        "validated_scope": "Book 1 (#2052 repaired research proof); whole-work files are indivisible",
        "members": [{k: r[k] for k in ("local_path", "container", "member", "tier", "bytes", "sha256")} for r in rows],
    }
    return manifest, rows


def build_package(tier, members, data_by_path):
    manifest, rows = package_files(tier, members)
    notice = NOTICE.format(archive=f"{ARCHIVE_SIZE:,}", sha=ARCHIVE_SHA256).encode()
    files = [("MANIFEST.json", encoded(manifest)), ("NOTICE.txt", notice)]
    files += [(r["local_path"], data_by_path[r["local_path"]]) for r in rows]
    return manifest, zip_bytes(files)


def verify_package(blob):
    """Return {local_path: bytes} after checking the embedded manifest exactly."""
    with zipfile.ZipFile(io.BytesIO(blob)) as zf:
        names = [i.filename for i in zf.infolist()]
        require(len(names) == len(set(names)), "duplicate_package_member")
        require(names[:2] == ["MANIFEST.json", "NOTICE.txt"], "manifest_or_notice_missing")
        manifest = json.loads(zf.read("MANIFEST.json"))
        require(
            manifest.get("format") == FORMAT and manifest["source"]["archive_sha256"] == ARCHIVE_SHA256,
            "wrong_package_format_or_source",
        )
        listed = [m["local_path"] for m in manifest["members"]]
        require(names[2:] == listed, "package_members_differ_from_manifest")
        result = {}
        for record in manifest["members"]:
            path = PurePosixPath(record["local_path"])
            require(not path.is_absolute() and ".." not in path.parts, "unsafe_package_path")
            info = zf.getinfo(record["local_path"])
            require(info.file_size == record["bytes"] <= MAX_MEMBER_BYTES, "package_member_size_mismatch")
            data = zf.read(record["local_path"])
            require(digest(data) == record["sha256"], "package_member_hash_mismatch")
            result[record["local_path"]] = data
    return manifest, result


def assemble(archive, out, inventory_record):
    verify_archive(archive)
    data_by_path, members = {}, []
    expected = {r["local_path"]: r for r in inventory_record["members"]}
    for (container, member, tier, role), _info, data in selected_members(archive):
        path = local_path(container, member)
        require(
            expected[path]["sha256"] == digest(data) and expected[path]["bytes"] == len(data),
            "member_differs_from_committed_inventory: " + path,
        )
        data_by_path[path] = data
        members.append(expected[path])
    out.mkdir(parents=True, exist_ok=False)
    result = {
        "version": "oga-anabasis-work-packages/1",
        "python": sys.version.split()[0],
        "zlib": zlib.ZLIB_RUNTIME_VERSION,
        "packages": {},
    }
    for tier, included in TIERS.items():
        manifest, first = build_package(tier, members, data_by_path)
        _, second = build_package(tier, members, data_by_path)
        require(first == second, "package_not_deterministic")
        verify_package(first)
        name = f"{PACKAGE_STEM}_{tier}.zip"
        (out / name).write_bytes(first)
        result["packages"][tier] = {
            "file": name,
            "bytes": len(first),
            "sha256": digest(first),
            "manifest_sha256": digest(encoded(manifest)),
            "members": len(manifest["members"]),
            "member_bytes": sum(m["bytes"] for m in manifest["members"]),
            "by_tier": {t: sum(m["bytes"] for m in manifest["members"] if m["tier"] == t) for t in included},
        }
    return result


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    parser.add_argument("command", choices=("inventory", "assemble", "verify"))
    parser.add_argument("--archive", type=Path, required=False)
    parser.add_argument("--out", type=Path, help="new private output directory (assemble) or package (verify)")
    parser.add_argument("--write", action="store_true", help="rewrite the committed JSON instead of comparing")
    args = parser.parse_args(argv)
    if args.command == "verify":
        manifest, members = verify_package(args.out.read_bytes())
        print(json.dumps({"tier": manifest["tier"], "members": len(members)}))
        return 0
    if args.command == "inventory":
        result, target = encoded(inventory(args.archive)), HERE / "work-inventory.json"
    else:
        record = json.loads((HERE / "work-inventory.json").read_bytes())
        result, target = encoded(assemble(args.archive, args.out, record)), HERE / "package-manifests.json"
    if args.write:
        target.write_bytes(result)
    else:
        require(target.read_bytes() == result, target.name + "_differs")
    print(f"{args.command}: {target.name} {'written' if args.write else 'reproduced byte-for-byte'}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
