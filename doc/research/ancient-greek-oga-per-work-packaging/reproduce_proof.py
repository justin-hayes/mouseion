#!/usr/bin/env python3
"""Rerun the retained #2052 Book 1 proof from a privately assembled package (#2053).

The proof code is the unchanged research code at PROOF_COMMIT, checked out as a
disposable detached worktree. Its private/ tree starts empty and is populated:

* OGA/original source members only from the verified package;
* the native-node/reading/coordinate projections by calling that commit's own
  import_gate.assess() on the package bytes;
* proof-only witnesses (Morpheus, UD Perseus, producer and UD documentation),
  hash-checked copies from the retained proof, kept apart from the package;
* the retained expected outputs, used only as comparison targets.

Gorman files are deliberately never copied or read. The child run therefore
replaces opus_completion.verify_inputs with an otherwise identical check that
skips the four Gorman registry entries; nothing else in the proof is altered.
Retained originals and outputs are opened read-only.
"""

import sys

sys.dont_write_bytecode = True

import argparse
import contextlib
import hashlib
import io
import json
import shutil
import subprocess
from pathlib import Path

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
import package_probe as probe

PROOF_COMMIT = "e610a5aa6c77c25864114ef9604ac9d50515039b"
PROOF_DIR = Path("doc/research/ancient-greek-oga-anabasis1-import-proof")
GORMAN = "private/opus-completion/gorman/"
WITNESS_DIRS = (
    "private/opus-completion/morpheus-stemsrc",
    "private/opus-completion/morpheus-docs",
    "private/opus-completion/udperseus",
    "private/opus-completion/ud-docs-grc",
    "private/opus-completion/ud-docs-u",
    "private/mapping-completion/producer-docs",
    "private/producer-docs",
)
WITNESS_FILES = ("private/mapping-completion/morpheus-index.tsv", "private/mapping-completion/morpheus-manifest.json")
EXPECTED_DIRS = ("private/opus-repair/out", "private/mapping-completion-v2")
PROJECTIONS = ("native-nodes.json", "reading.txt", "reading-to-paula.json", "node-to-tei.json")


def require(condition, reason):
    if not condition:
        raise ValueError(reason)


def sha(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def copy_tree(source, target):
    files = sorted(p for p in source.rglob("*") if p.is_file())
    for path in files:
        destination = target / path.relative_to(source)
        destination.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(path, destination)
    return len(files)


def stage(package, proof, retained):
    """Populate an empty proof private/ tree; return the package-side comparison."""
    head = subprocess.run(
        ["git", "-C", str(proof), "rev-parse", "HEAD"], capture_output=True, text=True, check=True
    ).stdout.strip()
    require(head == PROOF_COMMIT, "proof_worktree_not_at_reviewed_commit")
    require(not (proof / "private").exists(), "proof_private_tree_not_empty")
    manifest, members = probe.verify_package(package.read_bytes())
    for path, data in members.items():
        destination = proof / "private/archive-members" / path
        destination.parent.mkdir(parents=True, exist_ok=True)
        destination.write_bytes(data)

    sys.path.insert(0, str(proof))
    import import_gate  # the unchanged #2052 binding code

    source = {
        (m["container"], (probe.PREFIX if m["container"] == "outer" else "") + m["member"]): members[m["local_path"]]
        for m in manifest["members"]
    }
    public, private = import_gate.assess(source)
    ledger_identical = (proof / "import-ledger.json").read_bytes() == import_gate.encoded(public)
    (proof / "private/import").mkdir(parents=True)
    projections = {}
    for name in PROJECTIONS:
        (proof / "private/import" / name).write_bytes(private[name])
        projections[name] = {
            "sha256": hashlib.sha256(private[name]).hexdigest(),
            "retained_identical": sha(retained / "private/import" / name) == hashlib.sha256(private[name]).hexdigest(),
        }

    registry = json.loads((proof / "opus-completion-inputs.json").read_text(encoding="utf-8"))
    pins = {e["path"]: e["sha256"] for e in registry["inputs"]}
    producers = json.loads((proof / "producer-docs.json").read_text(encoding="utf-8"))
    copied = 0
    for relative in WITNESS_DIRS:
        copied += copy_tree(retained / relative, proof / relative)
    for relative in WITNESS_FILES:
        shutil.copyfile(retained / relative, proof / relative)
        copied += 1
    for path, expected in pins.items():
        if not path.startswith(GORMAN):
            require(sha(proof / path) == expected, "witness_pin_mismatch: " + path)
    for name, record in producers.items():
        found = [
            proof / d / name
            for d in ("private/producer-docs", "private/mapping-completion/producer-docs")
            if (proof / d / name).is_file()
        ]
        require(found and sha(found[0]) == record["sha256"], "producer_document_mismatch: " + name)
    for relative in EXPECTED_DIRS:
        copy_tree(retained / relative, proof / relative)
    require(not (proof / GORMAN).exists(), "gorman_must_not_be_staged")
    return {
        "package": {"file": package.name, "sha256": sha(package), "tier": manifest["tier"], "members": len(members)},
        "import_gate_assess": {"import_ledger_byte_identical": ledger_identical, "private_projections": projections},
        "staged_witness_files": copied,
        "gorman_registry_entries_not_staged": sorted(p for p in pins if p.startswith(GORMAN)),
    }


def child(proof, scratch):
    """Run the unchanged v2 and repair reproductions inside the staged proof."""
    sys.path.insert(0, str(proof))
    import mapping_completion_v2 as v2
    import opus_completion as oc

    def verify_inputs_without_gorman():
        for path, expected in oc.PINS.items():
            oc.check_pin(path, expected)
        registry = json.loads(oc.REGISTRY.read_text(encoding="utf-8"))
        for entry in registry["inputs"]:
            if not entry["path"].startswith(GORMAN):
                oc.check_pin(oc.HERE / entry["path"], entry["sha256"])
        manifest = json.loads(v2.MANIFEST.read_text(encoding="utf-8"))
        for path in sorted(oc.STEMSRC.iterdir()):
            if manifest["stemsrc_files"].get(path.name) != oc.sha256_file(path):
                raise oc.InputError(f"stem file {path.name} differs from the Morpheus manifest")
        producers = v2.verify_producer_docs()
        return {
            "original_pins": {p.name: d for p, d in sorted(oc.PINS.items(), key=lambda kv: kv[0].name)},
            "new_inputs": len(registry["inputs"]),
            "producer_documents": sorted(producers),
            "morpheus_pin": v2.MORPHEUS_PIN,
        }

    oc.verify_inputs = verify_inputs_without_gorman
    oc.SCRATCH = scratch
    import opus_repair

    results = {}
    for name, call in (
        ("mapping_completion_v2", lambda: v2.run(write=False)),
        ("opus_repair", lambda: opus_repair.run(write=False, use_go=True)),
    ):
        out = io.StringIO()
        with contextlib.redirect_stdout(out):
            code = call()
        results[name] = {"exit": code, "report": json.loads(out.getvalue().strip().splitlines()[-1])}
    print(json.dumps(results, ensure_ascii=False, sort_keys=True))
    return 0


def unchanged_cli(package, proof, scratch):
    """Run the historical import_gate.py command unchanged on complete-tier members.

    Its producer-document check reads only private/producer-docs, while the
    retained proof keeps three of those pinned documents under
    private/mapping-completion/producer-docs; they are copied (layout only).
    """
    _, members = probe.verify_package(package.read_bytes())
    cache = scratch / "complete-members"
    require(not cache.exists(), "complete_member_cache_exists")
    for path, data in members.items():
        destination = cache / path
        destination.parent.mkdir(parents=True, exist_ok=True)
        destination.write_bytes(data)
    moved = []
    for path in sorted((proof / "private/mapping-completion/producer-docs").iterdir()):
        target = proof / "private/producer-docs" / path.name
        if not target.exists():
            shutil.copyfile(path, target)
            moved.append(path.name)
    done = subprocess.run(
        [sys.executable, "-B", "import_gate.py", "--cache", str(cache)],
        cwd=proof,
        capture_output=True,
        text=True,
        check=False,
    )
    log = done.stdout + done.stderr
    errors = sorted(line.split(" ", 2)[1] for line in log.splitlines() if line.startswith(("ERROR: ", "FAIL: ")))
    missing = sorted(
        {
            line.split("input missing: ")[1].removeprefix(str(proof) + "/")
            for line in log.splitlines()
            if "InputError: input missing: " in line
        }
    )
    ran = [line for line in log.splitlines() if line.startswith("Ran ")]
    return {
        "package_tier": "complete",
        "producer_documents_copied_for_layout": moved,
        "exit": done.returncode,
        "ledger_and_projection_checks_passed": "import_diagnostics_not_byte_identical" not in log
        and "retained_projection_differs" not in log
        and bool(ran),
        "tests": ran[0].split(" in ")[0] if ran else None,
        "test_errors": errors,
        "missing_inputs_reported": missing,
    }


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    parser.add_argument("--package", type=Path, help="core-tier package")
    parser.add_argument("--complete-package", type=Path, help="also run the unchanged import_gate.py command")
    parser.add_argument("--proof-worktree", type=Path, required=True)
    parser.add_argument("--retained", type=Path, help="retained #2052 proof directory (read-only)")
    parser.add_argument("--scratch", type=Path, required=True)
    parser.add_argument("--child", action="store_true", help=argparse.SUPPRESS)
    parser.add_argument("--write", action="store_true", help="rewrite proof-reproduction.json")
    args = parser.parse_args(argv)
    proof = args.proof_worktree.resolve() / PROOF_DIR
    if args.child:
        return child(proof, args.scratch)
    result = {"version": "oga-anabasis1-package-reproduction/1", "proof_commit": PROOF_COMMIT}
    result.update(stage(args.package.resolve(), proof, args.retained.resolve()))
    done = subprocess.run(
        [
            sys.executable,
            "-I",
            str(Path(__file__).resolve()),
            "--child",
            "--proof-worktree",
            str(args.proof_worktree.resolve()),
            "--scratch",
            str(args.scratch),
        ],
        capture_output=True,
        text=True,
        check=False,
        cwd=proof,
    )
    require(done.returncode == 0, "child_failed: " + done.stderr[-2000:])
    result["runs"] = json.loads(done.stdout.strip().splitlines()[-1])
    if args.complete_package:
        result["unchanged_import_gate_cli"] = unchanged_cli(args.complete_package.resolve(), proof, args.scratch)
    text = probe.encoded(result)
    target = HERE / "proof-reproduction.json"
    if args.write:
        target.write_bytes(text)
    else:
        require(target.read_bytes() == text, "proof_reproduction_differs")
    passed = (
        result["import_gate_assess"]["import_ledger_byte_identical"]
        and all(p["retained_identical"] for p in result["import_gate_assess"]["private_projections"].values())
        and all(r["exit"] == 0 and not r["report"]["reproduction_problems"] for r in result["runs"].values())
    )
    print(json.dumps({"passed": passed, "runs": result["runs"]}, ensure_ascii=False, sort_keys=True))
    return 0 if passed else 1


if __name__ == "__main__":
    sys.exit(main())
