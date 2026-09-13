import json
from unittest.mock import patch

import pytest

from mouseion_nlp.provision import (
    MARKER_FILENAME,
    PROCESSORS,
    main,
    provision,
    read_marker,
    wipe_resources,
)

STANZA_VERSION = "1.14.0"


class RecordingDownload:
    """Stand-in for ``stanza.download`` that records each invocation."""

    def __init__(self) -> None:
        self.calls: list[tuple[str, dict]] = []

    def __call__(self, language: str, **kwargs) -> None:
        self.calls.append((language, kwargs))

    @property
    def languages(self) -> list[str]:
        return [language for language, _kwargs in self.calls]


def test_empty_directory_provisions_every_configured_language(tmp_path) -> None:
    download = RecordingDownload()

    provision(tmp_path, ["de", "it"], stanza_version=STANZA_VERSION, download=download)

    assert download.languages == ["de", "it"]
    for _language, kwargs in download.calls:
        assert kwargs == {
            "model_dir": str(tmp_path),
            "processors": PROCESSORS,
            "verbose": False,
        }
    marker = read_marker(tmp_path)
    assert (marker.stanza_version, marker.languages) == (STANZA_VERSION, ("de", "it"))

    assert json.loads((tmp_path / MARKER_FILENAME).read_text(encoding="utf-8")) == {
        "languages": ["de", "it"],
        "stanza_version": STANZA_VERSION,
    }


def test_second_run_against_the_same_directory_is_a_no_op(tmp_path) -> None:
    provision(
        tmp_path, ["de"], stanza_version=STANZA_VERSION, download=RecordingDownload()
    )
    rerun = RecordingDownload()

    provision(tmp_path, ["de"], stanza_version=STANZA_VERSION, download=rerun)

    assert rerun.calls == []
    assert read_marker(tmp_path).languages == ("de",)


def test_changed_stanza_version_wipes_and_provisions_from_scratch(tmp_path) -> None:
    provision(
        tmp_path, ["de"], stanza_version=STANZA_VERSION, download=RecordingDownload()
    )
    sentinel = tmp_path / "de" / "tokenize" / "stale.pt"
    sentinel.parent.mkdir(parents=True)
    sentinel.write_text("stale", encoding="utf-8")
    upgraded = RecordingDownload()

    provision(tmp_path, ["de"], stanza_version="1.15.0", download=upgraded)

    assert not sentinel.exists()
    assert upgraded.languages == ["de"]
    assert read_marker(tmp_path).stanza_version == "1.15.0"


def test_added_language_downloads_only_the_missing_language(tmp_path) -> None:
    provision(
        tmp_path, ["de"], stanza_version=STANZA_VERSION, download=RecordingDownload()
    )
    grown = RecordingDownload()

    provision(tmp_path, ["de", "it"], stanza_version=STANZA_VERSION, download=grown)

    assert grown.languages == ["it"]
    assert read_marker(tmp_path).languages == ("de", "it")


def test_missing_marker_wipes_a_partially_baked_volume(tmp_path) -> None:
    partial = tmp_path / "de" / "tokenize"
    partial.mkdir(parents=True)
    (partial / "baked.pt").write_text("baked", encoding="utf-8")
    download = RecordingDownload()

    provision(tmp_path, ["de", "it"], stanza_version=STANZA_VERSION, download=download)

    assert not (tmp_path / "de").exists()
    assert download.languages == ["de", "it"]
    assert read_marker(tmp_path).languages == ("de", "it")


def test_failed_download_aborts_without_writing_the_marker(tmp_path) -> None:
    def failing_download(language: str, **kwargs) -> None:  # noqa: ARG001
        raise RuntimeError("model source unreachable")

    with pytest.raises(RuntimeError, match="model source unreachable"):
        provision(tmp_path, ["de"], stanza_version=STANZA_VERSION, download=failing_download)

    assert read_marker(tmp_path) is None


def test_main_exits_nonzero_when_a_download_fails(tmp_path) -> None:
    with patch.dict(
        "os.environ",
        {"MOUSEION_NLP_WARM_LANGUAGES": "de", "STANZA_RESOURCES_DIR": str(tmp_path)},
        clear=True,
    ), patch("mouseion_nlp.provision.stanza.download", side_effect=RuntimeError("offline")):
        assert main() == 1


def test_main_provisions_the_configured_languages(tmp_path) -> None:
    with patch.dict(
        "os.environ",
        {"MOUSEION_NLP_WARM_LANGUAGES": "de,it", "STANZA_RESOURCES_DIR": str(tmp_path)},
        clear=True,
    ), patch("mouseion_nlp.provision.stanza.download") as download:
        assert main() == 0

    assert [call.args[0] for call in download.call_args_list] == ["de", "it"]
    assert read_marker(tmp_path).languages == ("de", "it")


def test_read_marker_treats_a_corrupt_marker_as_absent(tmp_path) -> None:
    (tmp_path / MARKER_FILENAME).write_text("not json", encoding="utf-8")

    assert read_marker(tmp_path) is None


def test_read_marker_ignores_a_wrong_marker_schema(tmp_path) -> None:
    (tmp_path / MARKER_FILENAME).write_text('{"languages": ["de"]}', encoding="utf-8")

    assert read_marker(tmp_path) is None


def test_wipe_resources_keeps_the_directory(tmp_path) -> None:
    (tmp_path / "de").mkdir()

    wipe_resources(tmp_path)

    assert tmp_path.is_dir()
    assert list(tmp_path.iterdir()) == []


def test_read_marker_ignores_a_non_string_language_set(tmp_path) -> None:
    (tmp_path / MARKER_FILENAME).write_text(
        json.dumps({"stanza_version": STANZA_VERSION, "languages": 5}), encoding="utf-8"
    )

    assert read_marker(tmp_path) is None
