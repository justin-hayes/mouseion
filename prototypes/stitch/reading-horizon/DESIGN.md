---
version: alpha
name: Mouseion
description: A calm, serious, typographically led scholarly reading environment where books and learner intention outrank metrics.
colors:
  primary: "#373C44"
  secondary: "#646B79"
  tertiary: "#0172AD"
  tertiary-hover: "#015887"
  neutral: "#F7F8FA"
  surface: "#FFFFFF"
  surface-emphasis: "#EEF5F8"
  border: "#E7EAEF"
  on-tertiary: "#FFFFFF"
  success: "#1D6954"
  warning: "#7A4B00"
  danger: "#883835"
typography:
  page-title:
    fontFamily: "system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif"
    fontSize: 2rem
    fontWeight: 700
    lineHeight: 1.2
    letterSpacing: "-0.01em"
  section-title:
    fontFamily: "system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif"
    fontSize: 1.375rem
    fontWeight: 600
    lineHeight: 1.3
  bibliographic-title:
    fontFamily: "Georgia, 'Times New Roman', Times, serif"
    fontSize: 1.25rem
    fontWeight: 600
    lineHeight: 1.35
    letterSpacing: "-0.01em"
  reading-text:
    fontFamily: "Georgia, 'Times New Roman', Times, serif"
    fontSize: 1rem
    fontWeight: 400
    lineHeight: 1.7
  body:
    fontFamily: "system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif"
    fontSize: 1rem
    fontWeight: 400
    lineHeight: 1.55
  metadata:
    fontFamily: "system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif"
    fontSize: 0.875rem
    fontWeight: 400
    lineHeight: 1.45
  label:
    fontFamily: "system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif"
    fontSize: 0.875rem
    fontWeight: 600
    lineHeight: 1.3
  evidence-value:
    fontFamily: "system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif"
    fontSize: 1.375rem
    fontWeight: 600
    lineHeight: 1.2
rounded:
  sm: 4px
  md: 6px
  full: 9999px
spacing:
  xs: 4px
  sm: 8px
  md: 12px
  lg: 16px
  xl: 24px
  2xl: 32px
  3xl: 48px
  4xl: 64px
components:
  button-primary:
    backgroundColor: "{colors.tertiary}"
    textColor: "{colors.on-tertiary}"
    rounded: "{rounded.sm}"
    padding: "10px 16px"
  button-primary-hover:
    backgroundColor: "{colors.tertiary-hover}"
    textColor: "{colors.on-tertiary}"
  button-secondary:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.tertiary}"
    rounded: "{rounded.sm}"
    padding: "10px 16px"
  book-surface:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.primary}"
    rounded: "{rounded.md}"
    padding: "16px"
  evidence-note:
    backgroundColor: "{colors.neutral}"
    textColor: "{colors.primary}"
    rounded: "{rounded.sm}"
    padding: "12px 16px"
  metadata:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.secondary}"
    typography: "{typography.metadata}"
  status-information:
    backgroundColor: "{colors.surface-emphasis}"
    textColor: "{colors.tertiary}"
    rounded: "{rounded.full}"
    padding: "4px 12px"
  status-success:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.success}"
    rounded: "{rounded.full}"
    padding: "4px 12px"
  status-warning:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.warning}"
    rounded: "{rounded.full}"
    padding: "4px 12px"
  status-danger:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.danger}"
    rounded: "{rounded.full}"
    padding: "4px 12px"
  divider:
    backgroundColor: "{colors.border}"
    height: 1px
---

# Mouseion design context

Status: **Historical Stitch generation context.** Its visual character remains
consistent with Mouseion, but it is not a canonical design-system specification.
Use [`doc/design/design-system.md`](../../../doc/design/design-system.md),
[`components.md`](../../../doc/design/components.md), and
[`information-architecture.md`](../../../doc/design/information-architecture.md)
for current guidance.

## Character

Mouseion is a **digital scholarly reading desk**: calm, serious, approachable, book-centered, and information-rich without feeling busy. It should feel like a place for sustained literary intention, not administration. The page should first communicate works, authors, reading commitments, and meaningful choices; application chrome and quantitative analysis remain supporting structure.

For My Books, Reading Journey, and Primary Goal, express continuation through sequence, changing evidence, and learner choice—not adventurous copy or illustration. The internal experience principle is **the road continues beyond the book**: the learner chooses what to undertake now, sees how the books ahead change, and chooses again. Desire determines direction; analysis informs preparation.

## Typography and hierarchy

- Use the application sans serif for navigation, controls, labels, explanations, and evidence.
- Use the reading serif for book titles, quotations, and short literary text. Author, edition, language, and scope remain restrained sans-serif metadata.
- Let title and author establish bibliographic identity before readiness numbers or status.
- Keep page titles compact. Avoid oversized marketing typography.
- Use tabular numerals for coverage, lemma counts, dates, and before/after values.
- Prefer sentence case. Avoid all-caps eyebrow labels, except rare compact metadata where scanning clearly benefits.
- Preserve readable line lengths: about 38rem for forms, 42rem for prose, and up to 72rem for comparative data.

## Spacing and density

Use a 4px-based rhythm. Interfaces may be information-dense, but every cluster needs a clear reading order. Use 12–16px within a book item, 24–32px between related groups, and 48–64px for major section changes. Prefer alignment, whitespace, and rules over extra containers. On wide screens, use horizontal space for comparison; do not stretch prose.

## Books and bibliographic identity

- A book is the visual center of each item: serif title, author, then edition/source/scope details only when relevant.
- Cover art is optional supporting identity, never the only identifier. If shown, keep covers modest, consistently proportioned, and subordinate to title and author.
- Keep learner-owned intention visible near identity with direct language such as **Interested**, **In my Journey**, or **Primary Goal**. Intention is categorical, not scored.
- Do not style low preparation as a stronger recommendation than high desire.
- Unassessed or unsupported desired works remain full members of My Books and may remain in the Journey; never make them look disabled or absent merely because Mouseion lacks numbers.

## Surfaces, borders, and grouping

- Prefer a strong document flow, semantic lists, thin dividers, and lightly tinted evidence regions.
- Use cards only when a book or the Primary Goal is a coherent resource with its own identity, state, and action. Keep corners modest, borders quiet, and shadows absent.
- Avoid nested cards, card grids of interchangeable metrics, floating glass panels, and rounded containers around every paragraph.
- A restrained visual field may use a quiet baseline, aligned bands, or depth layers to make preparation distance and current-versus-projected movement perceptible. It must remain legible as a list and must not resemble a fantasy map, project timeline, or game board.

## Actions

- Use one high-emphasis action for the current decision; keep peer and secondary actions outlined or textual.
- Actions name learner outcomes: **Add to Reading Journey**, **Choose as Primary Goal**, **See what preparation might help**, **Review evidence**.
- Avoid vague actions such as *Continue*, *Process*, *Unlock*, or *Optimize*.
- Keep actions beside the work they affect. Consequential actions explain what changes before submission.
- Interactive targets are at least 44×44px, with visible hover, focus, active, disabled, busy, success, and error states.

## Status, evidence, interpretation, and decisions

Keep four semantic layers visually distinct:

1. **Learner intention and decision** — strongest: interest, Journey membership, the Primary Goal, and the next explicit choice.
2. **Interpretation** — concise plain language such as “Within your selected 97% planning threshold” or “Preparation required.”
3. **Evidence** — exact current scoped token coverage, additional lemma identities to the selected threshold, structural signals, and projected deltas.
4. **Trust and provenance** — quality warnings, stale evidence, scope/edition, analysis identity, and methodology disclosures.

Status is always written in text; color is supplemental. Use accent for interaction or active work, green for confirmed successful/current states, amber for reviewable uncertainty, and red only for failure or destructive consequence. Do not color a book itself as good/bad or easy/hard.

Never combine lexical preparation, structural complexity, evidence quality, and desire into one score. Label every number as current, projected, scoped, token-weighted, or conditional as applicable. Keep **After Primary Goal vocabulary work** visibly separate from current state; a projected value must never masquerade as known or completed. Reading completion alone does not justify a vocabulary gain.

## Accessibility and responsive behavior

- Preserve a semantic heading structure and a list or table source presentation; any spatial field is supplementary.
- Keyboard users can reach every work, control, disclosure, and evidence link in a predictable order. Focus is unmistakable.
- Do not rely on color, position, hover, animation, or spatial memory to convey readiness or change.
- Pair movement with textual before/after values and state labels. Respect `prefers-reduced-motion`.
- Maintain WCAG AA contrast, 200% zoom resilience, visible labels, and meaningful screen-reader names.
- At narrow widths, preserve this order: book identity → desire/commitment → current interpretation → exact evidence → caution → action → provenance. Stack controls without reordering them; avoid page-level horizontal scrolling.
- Provide coherent loading, empty, unsupported, not-assessed, stale, failed, and unavailable-projection states.

## Progressive enhancement

The initial rendered experience must be complete and navigable. Filters, threshold selection, evidence disclosure, and current/projected views need stable textual states and ordinary destinations. Motion, spatial repositioning, and live recalculation may enhance comprehension, but the same works, values, warnings, and actions remain available without animation or advanced interaction.

## Emotional qualities

Aim for attention, trust, possibility, and quiet resolve. Completion may reveal a changed field of literature with deliberate pacing, but the completed reading remains the accomplishment. Use restraint rather than celebration mechanics.

## Avoid

- Generic SaaS dashboards, KPI tiles, or analytic control rooms.
- Fantasy maps, illustrated roads, landscapes, constellations, planets, territories, treasure, Tolkien references, or “unlocking.”
- XP, levels, streaks, achievements, badges-as-rewards, confetti, leaderboards, Journey completion percentages, or progress rings without a literal progress contract.
- Composite difficulty/readiness scores, traffic-light judgments, unexplained rankings, or “recommended for you” prescriptions.
- Decorative gradients, heavy shadows, excessive pills, giant rounded cards, oversized headings, and icon-first bibliographic identity.
- Dense tables as the whole experience. Exact data may use table semantics, but the primary composition should preserve books, desire, and changing possibility.
