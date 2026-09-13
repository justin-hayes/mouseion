"""One-shot provisioner for the Stanza model bundle on a persistent volume.

The NLP service owns the configured language set (ADR 0023) and advertises the
full runtime processor set; this entrypoint fills ``STANZA_RESOURCES_DIR`` with
``tokenize``, ``pos``, ``lemma``, ``depparse``, and ``ner`` for every configured
language so the serving container never downloads at startup.

Provisioning is idempotent and refresh-aware via a marker file written into the
resource directory:

- no marker (e.g. a volume from an image that only baked some processors) means
  the directory is wiped and every language is provisioned fresh;
- a changed Stanza version means the directory is wiped and re-provisioned;
- a grown language set only downloads the missing languages.

Any download failure propagates and aborts the process with a non-zero exit.
Run it as ``python -m mouseion_nlp.provision``.
"""

from __future__ import annotations

from dataclasses import dataclass
import json
import os
from pathlib import Path
import shutil
import sys
from typing import Callable, Iterable

import stanza
from stanza.resources.common import DEFAULT_MODEL_DIR

from .server import configured_languages


MARKER_FILENAME = ".mouseion-stanza-provision.json"
MARKER_VERSION = 1
PROCESSORS = "tokenize,pos,lemma,depparse,ner"


@dataclass(frozen=True)
class Marker:
    """The Stanza version and provisioned language set recorded on disk."""

    stanza_version: str
    languages: tuple[str, ...]


@dataclass(frozen=True)
class ProvisionPlan:
    """What a provision run must do, derived from the existing marker."""

    wipe: bool
    downloads: tuple[str, ...]
    provisioned: tuple[str, ...]


def marker_path(resources_dir: Path) -> Path:
    """Return the marker location inside a Stanza resource directory."""
    return resources_dir / MARKER_FILENAME


def read_marker(resources_dir: Path) -> Marker | None:
    """Read the marker, treating a missing or unreadable marker as absent."""
    path = marker_path(resources_dir)
    try:
        payload = json.loads(path.read_text(encoding="utf-8"))
        stanza_version = str(payload["stanza_version"])
        languages = tuple(sorted({str(code) for code in payload["languages"]}))
    except (OSError, ValueError, KeyError, TypeError):
        return None
    return Marker(stanza_version=stanza_version, languages=languages)


def write_marker(resources_dir: Path, marker: Marker) -> None:
    """Persist the marker describing what the directory now holds."""
    resources_dir.mkdir(parents=True, exist_ok=True)
    payload = {
        "marker_version": MARKER_VERSION,
        "stanza_version": marker.stanza_version,
        "languages": list(marker.languages),
    }
    marker_path(resources_dir).write_text(
        json.dumps(payload, indent=2, sort_keys=True) + "\n", encoding="utf-8"
    )


def plan_provisioning(
    marker: Marker | None, stanza_version: str, languages: Iterable[str]
) -> ProvisionPlan:
    """Decide whether to wipe and which languages to download.

    A missing marker or a changed Stanza version invalidates everything already
    on disk. Otherwise only languages absent from the marker are downloaded, and
    the marker retains previously provisioned languages so a shrunken configured
    set never forces a re-download.
    """
    configured = tuple(dict.fromkeys(languages))
    if marker is None or marker.stanza_version != stanza_version:
        return ProvisionPlan(wipe=True, downloads=configured, provisioned=configured)
    missing = tuple(language for language in configured if language not in marker.languages)
    provisioned = tuple(sorted(set(marker.languages) | set(configured)))
    return ProvisionPlan(wipe=False, downloads=missing, provisioned=provisioned)


def wipe_resources(resources_dir: Path) -> None:
    """Remove every entry in the resource directory, keeping the directory."""
    if not resources_dir.exists():
        return
    for entry in resources_dir.iterdir():
        if entry.is_dir() and not entry.is_symlink():
            shutil.rmtree(entry)
        else:
            entry.unlink()


def provision(
    resources_dir: Path,
    languages: Iterable[str],
    *,
    stanza_version: str,
    download: Callable[..., object] | None = None,
) -> ProvisionPlan:
    """Provision the resource directory for the configured languages.

    Returns the executed plan; raises whatever the download step raises so a
    failed provisioning run cannot write a marker and exits non-zero.
    """
    download = download or stanza.download
    resources_dir = Path(resources_dir)
    plan = plan_provisioning(read_marker(resources_dir), stanza_version, languages)
    resources_dir.mkdir(parents=True, exist_ok=True)
    if plan.wipe:
        wipe_resources(resources_dir)
    for language in plan.downloads:
        print(f"provisioning Stanza models for language '{language}'…", flush=True)
        download(language, model_dir=str(resources_dir), processors=PROCESSORS, verbose=False)
    write_marker(
        resources_dir, Marker(stanza_version=stanza_version, languages=plan.provisioned)
    )
    return plan


def main() -> int:
    """Provision the configured languages as a one-shot init command."""
    resources_dir = Path(os.getenv("STANZA_RESOURCES_DIR", DEFAULT_MODEL_DIR))
    languages = [descriptor.language for descriptor in configured_languages()]
    try:
        provision(resources_dir, languages, stanza_version=stanza.__version__)
    except Exception as error:  # noqa: BLE001
        print(f"stanza provisioning failed: {error}", file=sys.stderr, flush=True)
        return 1
    print(
        f"provisioned Stanza models for {', '.join(languages)} in {resources_dir}",
        flush=True,
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
