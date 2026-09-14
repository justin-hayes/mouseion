from __future__ import annotations

import importlib.util
import json
import sqlite3
from pathlib import Path


def load_script():
    path = Path(__file__).parents[1] / "scripts" / "derive_dictionary_index.py"
    spec = importlib.util.spec_from_file_location("derive_dictionary_index", path)
    assert spec and spec.loader
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def test_fixture_derives_filtered_entries_and_metadata(tmp_path: Path):
    module = load_script()
    output = tmp_path / "dictionary.sqlite"
    module.derive(Path(__file__).parents[1] / "testdata" / "dictionary_fixture.jsonl", output, "dump-2026-09-14")

    connection = sqlite3.connect(output)
    assert connection.execute("SELECT value FROM metadata WHERE key = 'provider_version'").fetchone() == ("dump-2026-09-14",)
    row = connection.execute("SELECT language, lemma, upos, senses_json, gender, article, plural FROM entries WHERE lemma = 'haus'").fetchone()
    assert row[:3] == ("de", "haus", "NOUN")
    assert json.loads(row[3])[0]["Gloss"] == "house"
    assert row[4:] == ("Neut", "das", "Häuser")
    assert connection.execute("SELECT count(*) FROM entries").fetchone() == (3,)
    assert connection.execute("SELECT senses_json FROM entries WHERE lemma = 'aufstehen'").fetchone() == ('[{"Gloss":"to get up","Examples":[],"Topics":[],"Tags":[],"Phrase":"","Gender":"","Article":"","Plural":"","IPA":""}]',)
    connection.close()
