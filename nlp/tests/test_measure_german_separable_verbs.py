from __future__ import annotations

import importlib.util
import sys
from pathlib import Path


def load_script():
    path = Path(__file__).parents[1] / "scripts" / "measure_german_separable_verbs.py"
    spec = importlib.util.spec_from_file_location("measure_german_separable_verbs", path)
    assert spec
    assert spec.loader
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class UnavailableProducer:
    def warmup(self, language: str) -> None:
        raise RuntimeError(f"{language} model not downloaded")


def test_missing_model_exits_skipped_without_writing_report(tmp_path: Path, monkeypatch, capsys):
    module = load_script()
    monkeypatch.setattr(module, "Producer", UnavailableProducer)
    input_path = tmp_path / "input.txt"
    input_path.write_text("Er fängt morgen mit der Arbeit an.", encoding="utf-8")
    output = tmp_path / "report.md"
    monkeypatch.setattr(
        sys,
        "argv",
        ["measure", "--input", str(input_path), "--output", str(output)],
    )

    assert module.main() == module.EXIT_SKIPPED
    assert module.EXIT_SKIPPED != module.EXIT_COMPLETED
    assert not output.exists()
    assert "SKIP: German Stanza model unavailable" in capsys.readouterr().err
