from __future__ import annotations

import importlib.util
from pathlib import Path
from types import SimpleNamespace


def load_script():
    path = Path(__file__).parents[1] / "scripts" / "measure_german_separable_verbs.py"
    spec = importlib.util.spec_from_file_location("measure_german_separable_verbs", path)
    assert spec
    assert spec.loader
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


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
    module = load_script()
    monkeypatch.setattr(module, "Producer", UnavailableProducer)
    output = tmp_path / "report.md"

    status = module.main(["--input", str(write_input(tmp_path)), "--output", str(output)])

    assert status == module.EXIT_SKIPPED
    assert status != 0
    assert not output.exists()
    assert "SKIP" in capsys.readouterr().err


def test_completed_run_exits_zero_and_writes_report(tmp_path: Path, monkeypatch) -> None:
    module = load_script()
    monkeypatch.setattr(module, "Producer", AvailableProducer)
    output = tmp_path / "report.md"

    status = module.main(["--input", str(write_input(tmp_path)), "--output", str(output)])

    assert status == 0
    assert output.exists()


def test_skip_status_is_not_success_status() -> None:
    module = load_script()

    assert module.EXIT_OK == 0
    assert module.EXIT_SKIPPED not in (module.EXIT_OK, 1, 2)
