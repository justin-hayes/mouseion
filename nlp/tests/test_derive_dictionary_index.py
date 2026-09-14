from __future__ import annotations

import importlib.util
import gzip
import json
import sqlite3
import stat
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
    assert connection.execute("SELECT value FROM metadata WHERE key = 'license'").fetchone() == ("Wiktionary-derived data: CC BY-SA 3.0 / GFDL",)
    assert connection.execute("SELECT value FROM metadata WHERE key = 'attribution'").fetchone() == ("Wiktionary contributors; CC BY-SA 3.0 / GFDL",)
    row = connection.execute("SELECT language, lemma, upos, senses_json, gender, article, plural FROM entries WHERE lemma = 'haus'").fetchone()
    assert row[:3] == ("de", "haus", "NOUN")
    assert json.loads(row[3])[0]["Gloss"] == "house"
    assert row[4:] == ("Neut", "das", "Häuser")
    assert connection.execute("SELECT count(*) FROM entries").fetchone() == (7,)
    assert connection.execute("SELECT count(*) FROM entries WHERE lemma = 'unbekannt'").fetchone() == (0,)
    assert set(connection.execute("SELECT DISTINCT language FROM entries").fetchall()) == {("de",), ("it",)}
    for (senses_json,) in connection.execute("SELECT senses_json FROM entries"):
        senses = json.loads(senses_json)
        assert senses
        assert all(sense["Gloss"].strip() for sense in senses)
    assert connection.execute("SELECT upos, plural, senses_json FROM entries WHERE lemma IN ('aufstehen', 'gut') ORDER BY lemma").fetchall() == [
        ("VERB", "", '[{"Gloss":"to get up","Examples":[],"Topics":[],"Tags":[],"Phrase":"","Gender":"","Article":"","Plural":"","IPA":""}]'),
        ("ADJ", "", '[{"Gloss":"good","Examples":[],"Topics":[],"Tags":[],"Phrase":"","Gender":"","Article":"","Plural":"","IPA":""}]'),
    ]
    italian = connection.execute("SELECT lemma, gender, article, plural FROM entries WHERE language = 'it' ORDER BY lemma").fetchall()
    assert italian == [
        ("albero", "Masc", "l'", "alberi"),
        ("casa", "Fem", "la", "case"),
        ("libro", "Masc", "il", "libri"),
        ("zaino", "Masc", "lo", "zaini"),
    ]
    connection.close()


def test_gzipped_dump_is_a_supported_input(tmp_path: Path):
    module = load_script()
    source = Path(__file__).parents[1] / "testdata" / "dictionary_fixture.jsonl"
    compressed = tmp_path / "kaikki.json.gz"
    with gzip.open(compressed, "wt", encoding="utf-8") as output:
        output.write(source.read_text(encoding="utf-8"))

    index = tmp_path / "dictionary.sqlite"
    module.derive(compressed, index, "dump-2026-09-14")

    connection = sqlite3.connect(index)
    assert connection.execute("SELECT count(*) FROM entries").fetchone() == (7,)
    connection.close()


def test_derived_index_is_readable_by_the_nonroot_container(tmp_path: Path):
    module = load_script()
    output = tmp_path / "dictionary.sqlite"
    module.derive(Path(__file__).parents[1] / "testdata" / "dictionary_fixture.jsonl", output, "dump-2026-09-14")

    assert stat.S_IMODE(output.stat().st_mode) == 0o644


def test_german_fixture_keys_match_runtime_lookup_keys(tmp_path: Path):
    module = load_script()
    fixture = Path(__file__).parents[1] / "testdata" / "german_normalization_parity.jsonl"
    output = tmp_path / "dictionary.sqlite"
    module.derive(fixture, output, "dump-2026-09-14")

    expected = {}
    for line in fixture.read_text(encoding="utf-8").splitlines():
        item = json.loads(line)
        expected[item["word"]] = item["runtime_lookup_key"]
        assert module.normalize("de", item["word"]) == item["runtime_lookup_key"]

    connection = sqlite3.connect(output)
    actual = dict(connection.execute("SELECT lemma, senses_json FROM entries").fetchall())
    connection.close()
    assert set(actual) == set(expected.values())
