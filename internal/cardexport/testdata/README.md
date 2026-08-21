# Manual Anki import check

1. Create a Cloze note type named `Mouseion Cloze` with fields in this order: `Key`, `Text`, `Back Extra`.
2. Use `{{cloze:Text}}` on the front and `{{cloze:Text}}<hr id=answer>{{Back Extra}}` on the back.
3. Import `anki_export.golden.tsv` as UTF-8, tab-separated text. Map columns to `Key`, `Text`, `Back Extra`, and `Tags`, and enable HTML.
4. Confirm the preview shows one cloze deletion, the UTF-8 text and embedded tab/newline are intact, and the tags are `mouseion de Der_Zauberberg`.
5. Import it again with duplicate checking based on the first field; confirm Anki updates/skips the existing note instead of creating a duplicate.
