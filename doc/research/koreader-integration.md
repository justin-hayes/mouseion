# What Mouseion could learn from KOReader

**Research date:** 2026-11-28

**Question.** Mouseion has no concept of reading position. The learner reads
EPUBs in KOReader on a Boox Palma 2 (Android, Onyx) and also reads physical
books. What could Mouseion learn from KOReader: reading progress (kosync),
dictionary lookups (Vocabulary Builder), highlights, and reading statistics?
For each, this file covers how the data leaves the device, what it means, how
it would identify a Book, and what Mouseion would have to expose and
authenticate. Two points are already decided. A KOReader lookup is a signal
only and never adds a word to Study by itself. Manual progress entry for
physical books is wanted. This is research only: no design decisions.

**Method.**

- **KOReader source.** Read at `koreader/koreader@b32d91a7`
  (master, 2026-11-26). The latest stable release is `v2026.07.1`
  (2026-08-01), so the file notes which features are in nightlies only.
- **Sync server source.** Read at `koreader/koreader-sync-server@46ef6b84`.
- **Android launcher manifest.** Read at
  `koreader/android-luajit-launcher@eaed834b`.
- **Tailscale.** Its documentation and issue tracker.
- **The partial-MD5 algorithm.** Reimplemented, then checked against both
  expected values in KOReader's own test (`spec/unit/util_spec.lua`:
  `leaves.epub` → `59d481d1…`, `tall.pdf` → `41cce710…`). Both matched.
- **The learner's files.** Copies of `vocabulary_builder.sqlite3` and
  `statistics.sqlite3` were opened read-only (`mode=ro&immutable=1`).
- **The local deployment.** The database (the `db` service in
  `compose.yaml`) was queried read-only. Partial MD5s of the stored EPUBs were
  computed inside Postgres; nothing was written.

The scripts and outputs were scratch work and are not kept.

## Answer

- **Reachability is not the obstacle.** KOReader on Android makes its HTTP
  requests from the app process (LuaSocket and Turbo), so with the Tailscale
  Android app active it reaches tailnet IPs and MagicDNS names like any other
  app. Its manifest sets `usesCleartextTraffic="true"`, and kosync, OPDS and
  WebDAV all accept a user-entered `http://` URL. The caveats are operational:
  the Tailscale tunnel being dropped on Android, and Android Private DNS
  conflicting with MagicDNS.
- **kosync is small enough for Mouseion to implement the server side.** It has
  four routes: register, auth, `PUT /syncs/progress` and
  `GET /syncs/progress/:document`. Every request authenticates with two
  headers, `x-auth-user` and `x-auth-key`, where the key is the unsalted MD5
  of the password. The server stores the latest push per (user, document) and
  timestamps it. The client resolves conflicts.
  - A document is identified by default by KOReader's **partial MD5**: the
    MD5 of 1 KiB samples at offsets 0, 1 KiB, 4 KiB … 1 GiB. The other option
    is the MD5 of the file name.
  - For an EPUB, `progress` is a crengine **xpointer** and `percentage` is
    page/pages in the current layout.
- **Hashes match only if the device holds the very bytes Mouseion can hash.**
  - **Where it already works.** Mouseion stores the **Catalog file** unchanged
    for the 62 of 68 EPUB imports made before ADR 0109. 5 of the 21
    statistics books matched a stored EPUB by partial MD5.
  - **Where it cannot.** Since ADR 0109, Mouseion stores the **Retained EPUB**
    and keeps only the Catalog file's SHA-256. A partial MD5 cannot be derived
    from that digest. All 6 post-0109 imports had resources stripped.
  - **What would make them match.** Either keep or compute the Catalog file's
    partial MD5 at acquisition, or serve the learner an EPUB whose bytes
    Mouseion knows, for example through an OPDS feed. KOReader's OPDS client
    writes the response body to disk unchanged.
- **Vocabulary Builder** is one SQLite file with two tables. A word is unique
  across all books, and only its *latest* lookup's book and context are kept.
  The file leaves the device through KOReader's cloud sync (WebDAV, Dropbox or
  FTP; for WebDAV: GET, then a PUT with `If-Match` and Basic auth) or by
  copying `/sdcard/koreader/settings/vocabulary_builder.sqlite3`.
- **Reading statistics** is one SQLite file. Its `book.md5` *is* the same
  partial MD5 that kosync sends. `page_stat_data` keeps each row's page number
  together with the book's page count at that time, so reflowed pages can be
  rescaled. It syncs the same way as Vocabulary Builder.
- **Highlights** live in per-book sidecar Lua files
  (`<book>.sdr/metadata.epub.lua`, `annotations`). They carry xpointers and,
  in nightlies since 2026-10-09, EPUB CFIs. They have no sync route to a
  custom server. The exporter writes JSON or Markdown files, or posts to
  Readwise, Joplin or Nextcloud Notes.
- **Measured fit on the learner's data is poor today, mostly for one reason.**
  The Books the learner read most in KOReader are **Previously read** imports
  with no stored EPUB and no Corpus. Details are under
  [Measured fit](#6-measured-fit-on-the-learners-data).

## 1. Reachability (Boox Palma 2, KOReader for Android)

- **KOReader's networking runs inside the app process.** kosync uses Spore
  and Turbo's `httpclient`
  ([`KOSyncClient.lua#L38-L61`](https://github.com/koreader/koreader/blob/b32d91a7c1c2395a15b6672e0ef63e719fcddf2a/plugins/kosync.koplugin/KOSyncClient.lua#L38-L61)).
  WebDAV and OPDS use LuaSocket's `socket.http`
  ([`webdav.lua#L8`](https://github.com/koreader/koreader/blob/b32d91a7c1c2395a15b6672e0ef63e719fcddf2a/plugins/cloudstorage.koplugin/providers/webdav.lua#L8),
  [`opdsbrowser.lua#L1513-L1526`](https://github.com/koreader/koreader/blob/b32d91a7c1c2395a15b6672e0ef63e719fcddf2a/plugins/opds.koplugin/opdsbrowser.lua#L1513-L1526)).
  Android sends every app's traffic and DNS through an active `VpnService`
  unless the app is excluded. Tailscale's app-based split tunnelling is
  opt-out per app
  ([Tailscale docs](https://tailscale.com/docs/features/client/android-app-split-tunneling)).
  So tailnet IPs and MagicDNS names (Mouseion's OPDS source is already
  `alexandria.taildc9a7d.ts.net`) resolve and connect from KOReader whenever
  Tailscale is up.
- **Plain HTTP is allowed.** The launcher manifest sets
  `android:usesCleartextTraffic="true"`
  ([`AndroidManifest.xml#L33`](https://github.com/koreader/android-luajit-launcher/blob/eaed834b6aaa98aa90eebd3253a057036ef31009/app/src/main/AndroidManifest.xml#L33)).
  The native LuaSocket stack is not subject to Android's Java cleartext policy
  anyway.
  - The kosync "Custom sync server" field defaults to `https://` but takes any
    URL and passes it to Spore as `base_url`
    ([`main.lua#L210-L222`](https://github.com/koreader/koreader/blob/b32d91a7c1c2395a15b6672e0ef63e719fcddf2a/plugins/kosync.koplugin/main.lua#L210-L222),
    [`#L463-L466`](https://github.com/koreader/koreader/blob/b32d91a7c1c2395a15b6672e0ef63e719fcddf2a/plugins/kosync.koplugin/main.lua#L463-L466)).
  - OPDS catalogs and WebDAV servers are also user-entered URLs with optional
    username and password.
- **Caveats.**
  - **Android Private DNS can break DNS while Tailscale is on.** This happens
    when Private DNS is set to a hostname
    ([tailscale#4252](https://github.com/tailscale/tailscale/issues/4252)).
    Leave it on Automatic or Off, or use the server's `100.x` address.
  - **Android can drop the tunnel.** Always-on VPN has been reported to drop
    after 30–120 min on Android 15
    ([tailscale#15736](https://github.com/tailscale/tailscale/issues/15736),
    open), and the tunnel may not come back after a reboot if it was stopped
    ([tailscale#2481](https://github.com/tailscale/tailscale/issues/2481)).
  - **Boox's own app freezing (unverified).** Onyx's battery and "freeze"
    settings may also kill the Tailscale service. No primary source
    documents them, so this is unverified: exempt Tailscale from battery
    optimisation and test.
  - **Pushes are lost only if the retry queue fails.** kosync pushes with very
    short timeouts (2 s/5 s), so a dropped tunnel makes a push fail. Since
    #15324 (2026-07-08, in v2026.07) a failed push is queued and retried on
    reconnect
    ([`main.lua#L942-L967`](https://github.com/koreader/koreader/blob/b32d91a7c1c2395a15b6672e0ef63e719fcddf2a/plugins/kosync.koplugin/main.lua#L942-L967),
    [`#L1012-L1048`](https://github.com/koreader/koreader/blob/b32d91a7c1c2395a15b6672e0ef63e719fcddf2a/plugins/kosync.koplugin/main.lua#L1012-L1048)). Since
    #16153 (2026-10-17, nightlies) the queue survives failed retries.

## 2. kosync

### Protocol

The API is defined in
[`api.json`](https://github.com/koreader/koreader/blob/b32d91a7c1c2395a15b6672e0ef63e719fcddf2a/plugins/kosync.koplugin/api.json) and served by
[`routes.lua#L10-L18`](https://github.com/koreader/koreader-sync-server/blob/46ef6b84a3930b3f0265efac74899f2256c158f1/config/routes.lua#L10-L18) and
[`syncs_controller.lua`](https://github.com/koreader/koreader-sync-server/blob/46ef6b84a3930b3f0265efac74899f2256c158f1/app/controllers/1/syncs_controller.lua). Every
request sends `Accept: application/vnd.koreader.v1+json`, and bodies are JSON.

| Route | Request | Success | Failure |
|---|---|---|---|
| `POST /users/create` | `{username, password}`; `password` is already `md5(password)` on the client | `201 {username}` | `402` (exists, or registration disabled) |
| `GET /users/auth` | headers `x-auth-user`, `x-auth-key` | `200 {authorized:"OK"}` | `401` |
| `PUT /syncs/progress` | `{document, progress, percentage, device, device_id, metadata?}` | `200 {document, timestamp}` | `401`, `403` (bad fields) |
| `GET /syncs/progress/:document` | — | `200 {document, percentage, progress, device, device_id, timestamp}`, or `{}` if none | `401` |

The reference server also has `DELETE /users/me` and `PUT /users/password`,
which the plugin does not call.

- **Auth.**
  - The plugin hashes the typed password with unsalted MD5 and stores the hex
    `userkey`
    ([`main.lua#L568`, `#L606`](https://github.com/koreader/koreader/blob/b32d91a7c1c2395a15b6672e0ef63e719fcddf2a/plugins/kosync.koplugin/main.lua#L560-L625)).
  - Every request sends `x-auth-user` and `x-auth-key`
    ([`KOSyncClient.lua#L33-L36`](https://github.com/koreader/koreader/blob/b32d91a7c1c2395a15b6672e0ef63e719fcddf2a/plugins/kosync.koplugin/KOSyncClient.lua#L33-L36)).
  - The reference server compares the key string for string against the one
    stored at registration
    ([`syncs_controller.lua#L108-L118`](https://github.com/koreader/koreader-sync-server/blob/46ef6b84a3930b3f0265efac74899f2256c158f1/app/controllers/1/syncs_controller.lua#L108-L118)).
    The key is a bearer secret, replayable, and equal for every device that
    uses the same password.
- **`document`.**
  - It is a hex digest that must match `^[A-Za-z0-9_]+$`.
  - By default (`checksum_method = BINARY`) it is the book's
    `partial_md5_checksum`. Under the "file name" setting it is the MD5 of the
    bare file name
    ([`main.lua#L666-L692`](https://github.com/koreader/koreader/blob/b32d91a7c1c2395a15b6672e0ef63e719fcddf2a/plugins/kosync.koplugin/main.lua#L666-L692)).
  - The partial MD5 is computed once, when a document is first opened, and
    cached in the book's sidecar
    ([`readerui.lua#L497-L501`](https://github.com/koreader/koreader/blob/b32d91a7c1c2395a15b6672e0ef63e719fcddf2a/frontend/apps/reader/readerui.lua#L497-L501)).
    Editing the file afterwards does not change it.
- **Partial MD5 algorithm**
  ([`util.lua#L1094-L1111`](https://github.com/koreader/koreader/blob/b32d91a7c1c2395a15b6672e0ef63e719fcddf2a/frontend/util.lua#L1094-L1111)).
  - KOReader MD5s 1024-byte samples at `1024 << 2i` for `i = -1 … 10`, and
    stops at the first offset at or past end of file.
  - LuaJIT's `bit.lshift(1024, -2)` wraps to 0, so the offsets are 0, 1 KiB,
    4 KiB, 16 KiB … 1 GiB.
  - The algorithm needs random access to the exact file bytes. It cannot be
    derived from any other digest.
- **`progress` and `percentage`.**
  - For reflowable documents, `progress` is the crengine xpointer of the top
    of the current view, e.g.
    `/body[1]/DocFragment[5]/body[1]/div[1]/p[19]/text()[1].28`.
  - For paged documents (PDF/DjVu) it is a page number
    ([`readerrolling.lua#L419-L421`](https://github.com/koreader/koreader/blob/b32d91a7c1c2395a15b6672e0ef63e719fcddf2a/frontend/apps/reader/modules/readerrolling.lua#L419-L421),
    [`readerpaging.lua#L175-L183`](https://github.com/koreader/koreader/blob/b32d91a7c1c2395a15b6672e0ef63e719fcddf2a/frontend/apps/reader/modules/readerpaging.lua#L175-L183)).
  - `percentage` is `current_page / number_of_pages` in page mode, or the
    xpointer's y-position over the document height in scroll mode, floored to
    4 decimals
    ([`readerrolling.lua#L510-L518`](https://github.com/koreader/koreader/blob/b32d91a7c1c2395a15b6672e0ef63e719fcddf2a/frontend/apps/reader/modules/readerrolling.lua#L510-L518),
    [`optmath.lua#L18-L20`](https://github.com/koreader/koreader/blob/b32d91a7c1c2395a15b6672e0ef63e719fcddf2a/frontend/optmath.lua#L18-L20)).
  - It therefore depends on font, margins and screen, and is not comparable
    across devices or layouts.
- **`metadata`** is optional and off by default. It is
  `{filename, title, authors}`, added in #15306 (2026-04-29, in v2026.07)
  ([`main.lua#L447-L452`](https://github.com/koreader/koreader/blob/b32d91a7c1c2395a15b6672e0ef63e719fcddf2a/plugins/kosync.koplugin/main.lua#L447-L452),
  [`#L694-L703`](https://github.com/koreader/koreader/blob/b32d91a7c1c2395a15b6672e0ef63e719fcddf2a/plugins/kosync.koplugin/main.lua#L694-L703)). The official
  server ignores it. A custom server could use it to link an unknown digest
  to a Book by title.
- **When pushes happen.**
  - On closing the document, and on suspend.
  - With auto-sync on, every *N* page turns after a 10-second idle debounce.
  - Manually.
  - Pulls happen on open (with auto-sync) and on resume
    ([`main.lua#L942-L1010`](https://github.com/koreader/koreader/blob/b32d91a7c1c2395a15b6672e0ef63e719fcddf2a/plugins/kosync.koplugin/main.lua#L942-L1010)).
- **Conflicts.**
  - The server is last-write-wins and stamps `timestamp = os.time()`
    ([`syncs_controller.lua#L244-L290`](https://github.com/koreader/koreader-sync-server/blob/46ef6b84a3930b3f0265efac74899f2256c158f1/app/controllers/1/syncs_controller.lua#L244-L290)).
  - The client decides what to do with a pulled position:
    - It ignores progress from its own `device`/`device_id`.
    - It ignores progress equal to its own.
    - Otherwise it compares the server `timestamp` with its last page turn,
      and applies the "sync forward" or "sync backward" policy (prompt,
      silent, or disabled)
      ([`main.lua#L851-L930`](https://github.com/koreader/koreader/blob/b32d91a7c1c2395a15b6672e0ef63e719fcddf2a/plugins/kosync.koplugin/main.lua#L851-L930)).

### Could Mouseion be the server?

Yes. The surface is four JSON routes plus header auth, and the server side
keeps one row per (learner, document). What Mouseion has to add is a mapping
from `document` to a Book. A pushed digest that maps to no Book can only be
stored or dropped, unless `metadata` is enabled and a title match is
attempted.

### Would hashes match the Books Mouseion holds?

- **What Mouseion stores** (`source_materials.content`, ADR 0109):
  - Before ADR 0109, acquisition stored the Catalog file unchanged.
  - Since ADR 0109 it stores the **Retained EPUB**: the Catalog file minus
    image, font, audio and video resources. It records only the Catalog
    file's SHA-256 (`processing_history.details.catalog_digest`), and the
    Catalog file is not kept.
  - In this deployment, 62 of 68 `epub.import` records predate ADR 0109
    (no `catalog_digest`). All 6 later ones removed items, so their stored
    bytes differ from the Catalog file.
- **Today, a match needs the device to hold the same Catalog file.** That
  happens when the learner loads the same file the OPDS server served to
  Mouseion before ADR 0109. Measured: 5 of the 21 statistics books (§6).
- **Serving the EPUB from Mouseion would make matches deterministic.**
  - KOReader's OPDS client streams the acquisition response straight to a
    file
    ([`opdsbrowser.lua#L1513-L1526`](https://github.com/koreader/koreader/blob/b32d91a7c1c2395a15b6672e0ef63e719fcddf2a/plugins/opds.koplugin/opdsbrowser.lua#L1513-L1526)),
    so the device's partial MD5 equals the partial MD5 of whatever bytes
    Mouseion serves.
  - Mouseion has no OPDS server today, only a client (`internal/opds`).
  - Two kinds of bytes could be served:
    - **The Retained EPUB.** Hashes are guaranteed to match, but the learner
      would read a copy without images, fonts or cover.
    - **A pass-through of the Catalog file.** This needs Mouseion to keep its
      partial MD5, or the file itself, at acquisition. ADR 0109 deliberately
      stores neither.
  - Either choice would amend ADR 0109.
- **One partial MD5 already appears on three different books.** In the
  learner's statistics, `3c17f9cf…` is the MD5 of *Tyll*, *Der Hellenismus*
  and *Erlöste und Verdammte*, each also present elsewhere under a different
  MD5.
  - The cause was not determined. A sidecar's cached checksum surviving the
    replacement of a file at the same path would produce this (see
    `readerui.lua` above).
  - The digest is not a reliable unique key on its own.

### Mapping an xpointer to a position in the analysed text

What exists on each side:

- **crengine.** `DocFragment[N]` is the N-th spine item, 1-based. The rest is
  an element path in crengine's own DOM, ending in `text()[k].offset`. That
  DOM drops whitespace-only nodes and may insert autoboxing elements, so the
  path is not guaranteed to equal a plain XHTML parse.
- **Mouseion.**
  - `source_material_units` records each unit's `spine_index`,
    `resolved_href` and `start_offset`/`end_offset` in the extracted text.
  - `corpus_sentences` records `unit_id` and character offsets.
  - Mouseion's extractor (`internal/epub/xhtml_v2.go`) streams XHTML tokens
    into normalised text and keeps no element paths.

What a mapping would take:

1. Resolve `DocFragment[N]` to the unit with that spine position. Check
   non-linear spine items and units that were not selected. Selection
   excludes front and back matter.
2. Re-walk that XHTML once, recording for each text node its element path in
   crengine's convention and the extracted-text offset it maps to. Then look
   up the xpointer's path and offset. Paths that miss need a fallback: the
   nearest preceding element, or a text-anchor search of the nearby text.
3. Find the sentence whose offsets contain the result.

Cheaper approximations are a whole-unit (chapter-level) position, or the
proportional position within the unit.

- **EPUB CFI.** Since `0deae500` (2026-10-09, **nightlies only**, not in
  v2026.07.1), KOReader also saves an EPUB CFI for `last_xpointer`
  (`last_epubcfi`) and for each highlight
  ([`readerrolling.lua#L294`](https://github.com/koreader/koreader/blob/b32d91a7c1c2395a15b6672e0ef63e719fcddf2a/frontend/apps/reader/modules/readerrolling.lua#L294)).
  A CFI is standard and defined on the publication's XHTML, not crengine's
  DOM, so it is easier to map. However, kosync sends the xpointer, not the
  CFI.
- **Matching identity.** The mapping is only meaningful against the same
  text. A Retained EPUB keeps every spine item, the navigation documents and
  the stylesheets, so spine positions and XHTML paths are unchanged from the
  Catalog file.

## 3. Vocabulary Builder

**Location on Android:** `/sdcard/koreader/settings/vocabulary_builder.sqlite3`.
The data directory is `android.getExternalStoragePath() .. "/koreader"`
([`datastorage.lua#L16-L22`](https://github.com/koreader/koreader/blob/b32d91a7c1c2395a15b6672e0ef63e719fcddf2a/datastorage.lua#L16-L22)). It is shared
storage: KOReader asks for `MANAGE_EXTERNAL_STORAGE`, and any file-sync app
with all-files access (e.g. Syncthing) or a USB/MTP connection can read it.

**Schema** (version `20240905`,
[`db.lua#L9-L33`](https://github.com/koreader/koreader/blob/b32d91a7c1c2395a15b6672e0ef63e719fcddf2a/plugins/vocabbuilder.koplugin/db.lua#L9-L33)):

```sql
CREATE TABLE vocabulary (
  word TEXT NOT NULL UNIQUE PRIMARY KEY, title_id INTEGER,
  create_time INTEGER NOT NULL, review_time INTEGER, due_time INTEGER NOT NULL,
  review_count INTEGER NOT NULL DEFAULT 0, prev_context TEXT, next_context TEXT,
  streak_count INTEGER NOT NULL DEFAULT 0, highlight TEXT);
CREATE TABLE title (id INTEGER NOT NULL UNIQUE PRIMARY KEY, name TEXT UNIQUE,
  filter INTEGER NOT NULL DEFAULT 1);
```

**Semantics.**

- **A lookup is recorded even with an external Android dictionary.** Every
  StarDict lookup fires `WordLookedUp(word, display_title)` *before* the
  branch that hands off to an external Android dictionary app
  ([`readerdictionary.lua#L1429-L1455`](https://github.com/koreader/koreader/blob/b32d91a7c1c2395a15b6672e0ef63e719fcddf2a/frontend/apps/reader/modules/readerdictionary.lua#L1429-L1455)).
- **What `word` is.** It is the lookup string as KOReader passes it to the
  dictionary. It is not lemmatised, and case is kept: 66 of the learner's 217
  words are capitalised, mostly German nouns.
- **What `highlight` is.** It is the cleaned selected text, stored only when
  it differs from `word` (74 of 217, e.g. `word = Eingehen`,
  `highlight = einzugehen`). So `highlight`, when present, is the surface form
  in the text.
- **Context.** `prev_context` and `next_context` are 15 *words* either side
  of the selection
  ([`vocabbuilder main.lua#L2104-L2124`](https://github.com/koreader/koreader/blob/b32d91a7c1c2395a15b6672e0ef63e719fcddf2a/plugins/vocabbuilder.koplugin/main.lua#L2104-L2124),
  [`readerhighlight.lua#L2046-L2053`](https://github.com/koreader/koreader/blob/b32d91a7c1c2395a15b6672e0ef63e719fcddf2a/frontend/apps/reader/modules/readerhighlight.lua#L2046-L2053)).
  Context is captured only when "with context" is on. All 217 of the
  learner's rows have it.
- **Uniqueness.** `word` is unique across all books. Looking it up again
  overwrites `title_id`, `create_time` and the context, keeping the old
  context only when the new one is null. It also decrements `review_count`
  and resets the streak
  ([`db.lua#L280-L300`](https://github.com/koreader/koreader/blob/b32d91a7c1c2395a15b6672e0ef63e719fcddf2a/plugins/vocabbuilder.koplugin/db.lua#L280-L300)).
  The file keeps one occurrence per word: the latest lookup, not a lookup log.
- **Title.** `title.name` is the book's `display_title` string. There is no
  file path, MD5 or author. The learner can also reassign or rename a title
  in the UI.
- **Review fields.** `review_count`, `streak_count`, `review_time` and
  `due_time` belong to Vocabulary Builder's own flashcard schedule
  (5 min → 30 min → 12 h → … → 60 days × 2ⁿ;
  [`db.lua#L218-L254`](https://github.com/koreader/koreader/blob/b32d91a7c1c2395a15b6672e0ef63e719fcddf2a/plugins/vocabbuilder.koplugin/db.lua#L218-L254)).
  None of the learner's 217 words has ever been reviewed (`review_count = 0`).

**How it leaves the device.**

- **Cloud sync.** Sync goes through the Cloud storage plugin to a chosen
  WebDAV, Dropbox or FTP folder
  ([`vocabbuilder main.lua#L2082-L2101`](https://github.com/koreader/koreader/blob/b32d91a7c1c2395a15b6672e0ef63e719fcddf2a/plugins/vocabbuilder.koplugin/main.lua#L2082-L2101)).
  The protocol is the former "SyncService"
  ([`cloudstorage main.lua#L180-L262`](https://github.com/koreader/koreader/blob/b32d91a7c1c2395a15b6672e0ef63e719fcddf2a/plugins/cloudstorage.koplugin/main.lua#L180-L262)):
  1. `GET <folder>/vocabulary_builder.sqlite3`. A 404 means first sync. The
     response's `ETag` is remembered.
  2. Merge locally in three ways: the local DB, the downloaded copy, and the
     `.sync` copy from the previous upload. Deletions on either side are
     propagated. On conflict, `due_time = max`, `review_count` is added or
     maxed, and context and highlight prefer the incoming non-null value
     ([`db.lua#L431-L530`](https://github.com/koreader/koreader/blob/b32d91a7c1c2395a15b6672e0ef63e719fcddf2a/plugins/vocabbuilder.koplugin/db.lua#L431-L530)).
     The merged file is then vacuumed.
  3. `PUT` the whole merged SQLite file with `If-Match: <etag>`. On a 412,
     repeat the cycle
     ([`webdav.lua#L142-L194`](https://github.com/koreader/koreader/blob/b32d91a7c1c2395a15b6672e0ef63e719fcddf2a/plugins/cloudstorage.koplugin/providers/webdav.lua#L142-L194)).
- **What it transfers.** The whole database file each time, in both
  directions, with HTTP Basic auth (LuaSocket `user`/`password`). Choosing the
  folder in the UI uses `PROPFIND`. `MKCOL` is offered for new folders.
- **What Mouseion would need to receive it this way.**
  - A WebDAV-compatible endpoint: `PROPFIND` for the folder picker, `GET`
    returning 404 or the file with a strong `ETag`, and `PUT` honouring
    `If-Match`.
  - Mouseion would then hold a full SQLite file per sync and would have to
    parse untrusted SQLite.
  - The device also *downloads* what Mouseion stores and merges it back.
    Mouseion must therefore return the last file it received, unchanged, or
    deliberately return a modified one. Returning nothing (404) every time
    makes each sync look like a first sync, which is harmless.
- **Other routes.** Syncthing, or manual copying from
  `/sdcard/koreader/settings/`. The file uses WAL, and KOReader checkpoints it
  before cloud sync (`walCheckpoint`). A file-level copy taken while KOReader
  is running may miss the `-wal` contents.

## 4. Reading statistics

**Location:** `/sdcard/koreader/settings/statistics.sqlite3`.

**Schema** (version `20221111`,
[`statistics main.lua#L404-L483`](https://github.com/koreader/koreader/blob/b32d91a7c1c2395a15b6672e0ef63e719fcddf2a/plugins/statistics.koplugin/main.lua#L404-L483)):

- `book(id, title, authors, notes, last_open, highlights, pages, series,
  language, md5, total_read_time, total_read_pages)`, unique on
  `(title, authors, md5)`.
- `page_stat_data(id_book, page, start_time, duration, total_pages)`, unique on
  `(id_book, page, start_time)`.
- A `page_stat` view rescales each row to the book's current `pages`.

**Semantics.**

- **`book.md5`.** It is the sidecar's `partial_md5_checksum`
  ([`main.lua#L2765`](https://github.com/koreader/koreader/blob/b32d91a7c1c2395a15b6672e0ef63e719fcddf2a/plugins/statistics.koplugin/main.lua#L2765)), so it
  is the same value kosync sends under the default checksum setting.
  `title`, `authors` and `language` come from the document metadata.
- **One row per page visit.**
  - `page` is the rendered page number and `total_pages` is the page count
    *at that time*. When reflow changes the count, older rows are scaled
    through `page_stat` rather than rewritten.
  - A visit counts only if it lasted between `min_sec` (5 s) and `max_sec`
    (120 s by default). Longer visits are capped
    ([`main.lua#L29-L30`](https://github.com/koreader/koreader/blob/b32d91a7c1c2395a15b6672e0ef63e719fcddf2a/plugins/statistics.koplugin/main.lua#L29-L30),
    [`#L2560-L2620`](https://github.com/koreader/koreader/blob/b32d91a7c1c2395a15b6672e0ef63e719fcddf2a/plugins/statistics.koplugin/main.lua#L2560-L2620)).
- **Reflow in the learner's data.** 7 of 19 books with page data have more
  than one `total_pages` value. For example, *Das archaische Griechenland*
  has 455, 288, 287, 383 and 399. Pages are layout units, not text positions,
  and only the proportion `page / total_pages` survives a layout change.

**How it leaves the device.** The same cloud-sync mechanism as Vocabulary
Builder, merging on `(title, authors, md5)` and on `(page, start_time)` rows
([`main.lua#L3126-L3260`](https://github.com/koreader/koreader/blob/b32d91a7c1c2395a15b6672e0ef63e719fcddf2a/plugins/statistics.koplugin/main.lua#L3126-L3260)).
It can also be copied off the device as a file.

## 5. Highlights

- **Where they are stored.**
  - Highlights are in the per-book sidecar under the `annotations` key.
  - By default the sidecar is `<book dir>/<book name>.sdr/metadata.epub.lua`,
    next to the book.
  - Two settings move it: "dir" moves it to `/sdcard/koreader/docsettings/…`,
    and "hash" to
    `/sdcard/koreader/hashdocsettings/<md5[0:2]>/<partial md5>.sdr/`
    ([`docsettings.lua#L118-L145`](https://github.com/koreader/koreader/blob/b32d91a7c1c2395a15b6672e0ef63e719fcddf2a/frontend/docsettings.lua#L118-L145)).
  - The file is a Lua table literal. A reader has to parse Lua data, not
    execute it.
- **What each annotation holds**
  ([`readerannotation.lua#L66-L84`](https://github.com/koreader/koreader/blob/b32d91a7c1c2395a15b6672e0ef63e719fcddf2a/frontend/apps/reader/modules/readerannotation.lua#L66-L84)):
  - `datetime`, `datetime_updated`
  - `text`, the highlighted text
  - `note`, `chapter`
  - `pageno`, `pageref`
  - `page`/`pos0`/`pos1`, which are xpointers
  - `drawer`, `color`
  - `epubcfi`, in nightlies since 2026-10-09
- **Export.** The Exporter plugin writes JSON, Markdown, HTML, text or
  My Clippings files to `/sdcard/koreader/clipboard`, or shares them through
  Android. It can also post to Readwise (hard-coded `readwise.io`), Joplin or
  Nextcloud Notes
  ([`exporter target/`](https://github.com/koreader/koreader/tree/b32d91a7c1c2395a15b6672e0ef63e719fcddf2a/plugins/exporter.koplugin/target)).
  - Each JSON entry includes `text`, `note`, `chapter`, `page`, `time`, and
    `pn_xp` (the xpointer)
    ([`clip.lua#L265-L273`](https://github.com/koreader/koreader/blob/b32d91a7c1c2395a15b6672e0ef63e719fcddf2a/plugins/exporter.koplugin/clip.lua#L265-L273)).
  - The optional `md5sum` is a *full-file* MD5, not the partial MD5
    ([`json.lua#L26-L37`](https://github.com/koreader/koreader/blob/b32d91a7c1c2395a15b6672e0ef63e719fcddf2a/plugins/exporter.koplugin/target/json.lua#L26-L37)).
  - There is no generic "post to a custom URL" target and no cloud sync of
    annotations.

## 6. Measured fit on the learner's data

**Deployment.** The local deployment has one owner with 97 Books: 27 `de`,
17 `it`, 1 `el`, and the rest `fr`, `grc` and `la`. There are 67 stored EPUBs.

**Statistics (21 `book` rows, 19 with page data, 357,278 s of reading in
total):**

| Match | Rows | Share of reading time |
|---|---|---|
| Title matches a Book (normalised: case, "(German Edition)", subtitle after `:`/`·`) | 13 (10 exact) | 68.0% |
| …and that Book has a stored EPUB and an analysis | 7 | 16.4% |
| Partial MD5 equals that of a stored EPUB | 5 (all pre-ADR-0109 imports) | 16.0% |
| Title matches a **Previously read** Book (no EPUB, no Corpus) | 5 | 51.6% |

- **The 5 MD5 matches** are *Wenn du erzählst, erblüht die Wüste*,
  *HERKUNFT*, *Nel mare ci sono i coccodrilli*, *Autour de la Lune* and
  *Der ferne Spiegel*. Each matched the Book that its title also matched.
- **The two analysed Books without an MD5 match:**
  - *Deutsche Hörer!*: the device file has a longer, different title, so it
    is a different file.
  - *Erlöste und Verdammte*: its MD5 is the shared `3c17f9cf…` value from §2.
- **Unmatched rows:**
  - Books the learner holds outside Mouseion (*Papyrus*, *Tyll*,
    *Die Gefährten*, *Fiabe italiane*).
  - The KOReader quickstart guide.
  - Easy Greek material.

**Vocabulary Builder (217 words, 5 titles):**

| Title | Words | Book in Mouseion | Corpus for that Book |
|---|---|---|---|
| Papyrus: Die Geschichte der Welt in Büchern | 102 | no | — |
| Das archaische Griechenland | 81 | yes, Previously read | none (no EPUB) |
| Das klassische Griechenland | 31 | yes, Previously read | none |
| Der Hellenismus · Der Hof und die Welt (C.H.Beck) | 2 | yes, normalised title; Previously read | none |
| Super Easy Greek for Beginners | 1 | no | — |

- **By title:** 3 of 5 titles (114 of 217 words, 52.5%) match a Book. None of
  those Books has an analysis, so **0 of 217 words** can be checked against
  their own Book's Corpus tokens or sentences.
- **Against the whole German Corpus** (the current analyses of every `de`
  Book), taking the surface form as `highlight` or else `word`:
  - The surface occurs as a token in **162/217 (74.7%)**.
  - A token with that surface has `canonical_lemma = word`
    (case-insensitive) in **122/217 (56.2%)**.
  - `word` is some token's canonical lemma in **160/217 (73.7%)**.
- **Context:** 0 of the 216 German contexts (the surface plus up to 20 chars
  of `next_context`) occurs in any German Corpus sentence. This is expected,
  because their Books are not in the Corpus.
- **What the gap reflects.** The difference between the surface and lemma
  rates is mostly `word` not being a lemma: KOReader stores the selection,
  e.g. `beigesetzt`, `handverlesen`. Mapping a lookup to a vocabulary identity
  therefore needs the analysed occurrence in context, not the `word` string.

**What the measurement says.** The learner's KOReader reading is
concentrated in nonfiction they recorded as Previously read without
acquiring it. Those Books have no source text, so lookups, statistics and
positions cannot be anchored to sentences or tokens unless the Book is
acquired and analysed. Where a Book *was* acquired before ADR 0109 and read
from the same file, MD5 linking worked every time it was tried.

## 7. Auth on the Mouseion side

Mouseion today has only web sessions: local accounts with argon2id and
HTTP-only session cookies (ADR 0009, `internal/auth`, `internal/webauth`).
None of these clients can use a cookie session, so each route needs a new
non-browser credential.

| Route | What the client sends | Notes |
|---|---|---|
| kosync | `x-auth-user` + `x-auth-key` = unsalted MD5 of a password, on every request; optional `POST /users/create` | The key is a replayable bearer secret. Mouseion should issue a device-specific kosync password rather than accept the account password, and store only a hash of the MD5 key. Registration should be disabled (`402`). |
| OPDS (serving) | HTTP Basic on every request (KOReader OPDS `username`/`password`) | Basic auth needs a per-device credential, not the web password. |
| WebDAV (Vocabulary Builder or statistics upload) | HTTP Basic on `PROPFIND`/`GET`/`PUT` | As for OPDS. Request bodies are whole SQLite files and need size limits and read-only, untrusted parsing. |
| File import (Syncthing, manual upload) | The existing web session, through an upload form | No new credential. |

All traffic stays inside the tailnet, so it is encrypted by WireGuard even
over plain HTTP (ADR 0009). Any new credential type or auth middleware
touches `internal/auth/` or `internal/webauth/`, which `.github/CODEOWNERS`
puts under human review. A new ADR (`doc/adr/`, `doc/product.md`) would need
the same review.

## Implications for design

Open questions for a later grilling:

1. **Reading position as a foundation.** What is a reading position in
   Mouseion: a Book plus a sentence? a unit plus a fraction? a percentage?
   - Manual progress for physical books can only give a page or a percentage.
   - kosync gives an xpointer and a layout-dependent percentage.
   - Should one concept cover both, and does it belong to the Book, the
     Current reading, or Reading history?
2. **Separate specs.** Candidates, in rough dependency order:
   - (a) a reading-position concept and manual progress entry;
   - (b) a kosync server;
   - (c) serving EPUBs over OPDS so hashes are known;
   - (d) Vocabulary Builder import;
   - (e) statistics import;
   - (f) highlight import.
   Each can ship alone except that (b) is far more useful with (c).
3. **Hash identity versus ADR 0109.**
   - Should acquisition also record the Catalog file's partial MD5? That is
     cheap, but only works when the device holds the identical file.
   - Or should Mouseion serve the Retained EPUB, so the learner reads
     Mouseion's copy without images, fonts or cover?
   - Or serve the Catalog file by pass-through, which means keeping it?
   - And how should Mouseion handle a digest shared by several books (§2)?
4. **Unknown documents.** What happens to a kosync push or a statistics row
   for a document Mouseion cannot identify: drop it, keep it, or match by
   title through `metadata`? Could it prompt the learner to add the Catalog
   entry?
5. **Lookups as a signal.** Lookups are a signal only. Where does the signal
   show: Book detail, the Study report, or new-word ordering? Should a lookup
   bind to a vocabulary identity only when the lookup's Book is analysed and
   the context is found in a sentence (otherwise unresolved)? How should
   re-lookups be handled, given Vocabulary Builder keeps only the latest
   occurrence?
6. **Previously read Books.** The learner's most-read KOReader Books have no
   text in Mouseion. Is KOReader data a reason to acquire them, and does that
   change the meaning of Previously read?
7. **Transport for SQLite files.**
   - A WebDAV endpoint is automatic but must implement the GET/merge/PUT
     protocol, ETags and the download-back step.
   - A web-session upload of files copied off the device (Syncthing) is simple
     but manual.
   - Which one, and how often must the data arrive to be useful?
8. **Credentials.** Should there be one device credential per protocol, or
   one per device shared across kosync, OPDS and WebDAV? Any choice means a
   human-reviewed auth change.
9. **Version floor.** Should Mouseion rely on nightly-only features such as
   EPUB CFI in sidecars and highlights, or the persistent retry queue, or
   target stable v2026.07.1?
