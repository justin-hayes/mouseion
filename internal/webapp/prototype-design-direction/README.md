# PROTOTYPE — design direction for #1480 (throwaway)

Question: **what should Mouseion's reading desk look like?** Three structurally
different directions over the same realistic content, switchable with
`?variant=A|B|C`, `?screen=books|reading|chooser|concordance|type`,
`?theme=light|dark` and `?lang=de|el`.

Run from the repo root:

    python3 -m http.server 8130 -d internal/webapp/prototype-design-direction

then open <http://127.0.0.1:8130/>. Use ← / → (or the bottom bar) to cycle
variants. Static HTML only: no Go, templ, Tailwind, or server state. Buttons
and forms do nothing. Book titles, authors, and sentences are invented fixture
content; "real" covers are CSS stand-ins for catalog images.

This directory must not be merged to `main`. Once a direction is chosen, the
decision is folded into #1480 and its children and the prototype moves to a
throwaway branch.

## Design plan

**Subject.** A self-hosted reading desk for one learner reading German,
Italian, and Modern Greek literature. Primary job: get back to the current Book
and the vocabulary around it; secondary jobs are choosing the next Book and
looking words up across Books.

**Colour (kept, deliberately).** The established cool paper avoids the
cream-and-terracotta default and reads like library card stock under a blue
editing pencil.

| Role | Light | Dark |
| --- | --- | --- |
| Ground (porcelain) | `#F3F6F7` | `#111A22` |
| Page (leaf) | `#FCFDFB` | `#192630` |
| Quiet (blue-grey) | `#E3EBF0` | `#233441` |
| Ink (carbon) | `#17232D` | `#EDF3F5` |
| Pencil (metadata) | `#586873` | `#A9B8C2` |
| Annotation blue | `#2457B2` | `#8FB4FF` |

Placeholder covers draw from five muted paper tints; they are not UI colours.

**Type.**

- *Literata* (OFL, Google Fonts/TypeTogether) for Book identity and passages.
  Designed for long-form screen reading (Google Play Books), with an optical
  size axis so 49px titles and 17px passages each get the right drawing, and
  native Greek.
- *Commissioner* (OFL, Kostas Bartsokas) for the interface. A Greek type
  designer's sans with first-class Greek, slightly warmer than the
  Source Sans/Inter defaults, and a variable weight axis.
- Scale: major third from 16px — 13 / 16 / 20 / 25 / 31 / 39 / 49. Page titles
  are 25px sans and orient; only the current Book's title reaches 39–49px.
  Passages are 17–19px Literata at 1.65 with a 34em measure.

**The one bold thing.** Book identity set large in Literata against quiet
Commissioner chrome. Everything else stays small and plain.

**Directions.**

- **A — Margin notes.** Top bar, no sidebar. Each page is a text column with
  a right-hand margin of pencil notes: evidence and status annotate the Book
  they belong to, the way an editor annotates a manuscript. My Books is a list;
  Concordance is a true KWIC grouped by Book.
- **B — Shelf.** Slim left rail. Covers lead: a cover grid in My Books, a
  two-page spread in Reading, and a horizontal To Read shelf. Missing covers
  become typographic covers. Concordance reads sentence-first.
- **C — Desk.** The current Book sits in a strip under the top bar on every
  page. My Books is grouped by disposition rather than filtered, with dense
  rows. Concordance is a table.

**Checked against the defaults.** A risked becoming a hairline-ruled
broadsheet, so it separates with whitespace and a tinted margin instead of
rules. B risked the SaaS card kit, so it uses no card boxes, shadows, or
uniform radii; covers sit directly on the ground. C risked a dashboard, so it
has no metric tiles and shows numbers only inside sentences.
Across all three: no all-caps eyebrows, no middle-dot metadata strings, no
entrance animation, and no ordinal numbering on unordered To Read candidates.
