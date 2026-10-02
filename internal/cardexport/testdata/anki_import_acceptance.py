"""Real Anki-backend import assertions for the optional Go acceptance test."""

import sys
from pathlib import Path

from anki.collection import Collection, ImportAnkiPackageRequest


work = Path(sys.argv[1])
expected_guid, first_identity, changed_identity, first_custom_deck, renamed_deck = sys.argv[2:]
assert first_identity != changed_identity
first_custom_deck = first_custom_deck.replace("::", "\x1f")
renamed_deck = renamed_deck.replace("::", "\x1f")

collection = Collection(str(work / "collection.anki2"))


def import_package(filename: str) -> None:
    collection.import_anki_package(
        ImportAnkiPackageRequest(package_path=str(work / filename))
    )


def snapshot():
    notes = collection.db.all("select id,guid,flds from notes")
    cards = collection.db.all("select nid,did from cards")
    decks = dict(collection.db.all("select name,id from decks"))
    assert len(notes) == 1, f"expected one shared note, got {len(notes)}"
    assert len(cards) == 1, f"expected one card, got {len(cards)}"
    return notes[0], cards[0], decks


import_package("book.apkg")
book_note, book_card, decks = snapshot()
assert book_note[1] == expected_guid
assert "Mouseion\x1fde\x1fBook A" in decks
book_fields = book_note[2]
book_deck_id = decks["Mouseion\x1fde\x1fBook A"]
assert book_card[1] == book_deck_id

# Anki's real importer reports the shared GUID as a duplicate. With its default
# import behavior it preserves the existing note fields and card placement.
import_package("custom.apkg")
note_after_custom, card_after_custom, decks = snapshot()
assert note_after_custom[1] == expected_guid
assert note_after_custom[2] == book_fields
assert card_after_custom[1] == book_deck_id
assert first_custom_deck in decks

# Preparing again with changed source evidence and a renamed Custom deck still
# imports one note. Anki retains the original fields/card location and creates
# the renamed package deck; the earlier imported deck remains in the collection.
import_package("renamed-prepare-again.apkg")
note_after_rename, card_after_rename, decks = snapshot()
assert note_after_rename[1] == expected_guid
assert note_after_rename[2] == book_fields
assert card_after_rename[1] == book_deck_id
assert first_custom_deck in decks
assert renamed_deck in decks

print(
    "Observed with Anki backend: Book then Custom produced one note; default "
    "imports retained the original note fields and Book deck placement. "
    "Prepare again with changed source sentence and rename retained that note, "
    "left both imported Custom deck names, and did not update/move the card."
)
collection.close()
