from __future__ import annotations

from pathlib import Path
from types import SimpleNamespace

from mouseion_nlp import separable_measurement
from mouseion_nlp.separable_measurement import EXIT_OK, EXIT_SKIPPED, run


def write_input(tmp_path: Path) -> Path:
    path = tmp_path / "input.txt"
    path.write_text("Ich stehe auf.\n", encoding="utf-8")
    return path


class UnavailableProducer:
    def __init__(self) -> None:
        pass

    def warmup(self, language: str) -> None:
        raise RuntimeError("model not downloaded")


class AvailableProducer:
    def __init__(self) -> None:
        pass

    def warmup(self, language: str) -> None:
        return None

    def analyze(self, text: str, language: str, document: object) -> SimpleNamespace:
        return SimpleNamespace(sentences=[])

    def model_version(self, language: str) -> str:
        return "test-model"


def test_skipped_run_exits_with_distinct_status_and_writes_nothing(
    tmp_path: Path, monkeypatch, capsys
) -> None:
    monkeypatch.setattr(separable_measurement, "Producer", UnavailableProducer)
    output = tmp_path / "report.md"

    status = run(write_input(tmp_path), output)

    assert status == EXIT_SKIPPED
    assert not output.exists()
    assert "SKIP" in capsys.readouterr().err


def test_completed_run_exits_zero_and_writes_report(tmp_path: Path, monkeypatch) -> None:
    monkeypatch.setattr(separable_measurement, "Producer", AvailableProducer)
    output = tmp_path / "report.md"

    status = run(write_input(tmp_path), output)

    assert status == EXIT_OK
    assert output.exists()


def test_skip_status_is_distinct_from_success_and_usage_error() -> None:
    assert EXIT_OK == 0
    assert EXIT_SKIPPED not in (EXIT_OK, 1, 2)
