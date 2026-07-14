# Design standard for apps built in this project

Every app built here must look and feel like a polished, native macOS app —
something you'd believe came from the Mac App Store. Follow the macOS Human
Interface Guidelines. These rules are mandatory unless the user overrides them.

## Layout & structure
- Use a clean window structure: `NavigationSplitView` for browse/detail apps,
  a single well-proportioned window for tools. Set a sensible default size and
  a `minSize`; content must not break when resized.
- Spacing on an 8pt grid: paddings of 8/12/16/24, consistent everywhere.
  Align controls; never let elements float at odd offsets.
- Prefer a native `.toolbar` with SF Symbol buttons over rows of labeled
  buttons inside the content.

## Color & dark mode
- Only semantic system colors: `.primary`, `.secondary`, `Color.accentColor`,
  `.background`, and materials (`.regularMaterial`, `.thinMaterial`).
  Never hardcode white/black or hex colors for surfaces.
- Both appearances must look intentional — check every view in light AND dark.

## Typography & icons
- System text styles only (`.largeTitle`, `.title2`, `.headline`, `.body`,
  `.caption`); create hierarchy with weight and `.secondary`, not font sizes.
- SF Symbols for all iconography (`Image(systemName:)`) — never emoji or
  ASCII glyphs as icons. Pick symbols that match Apple's usage.

## States — no dead screens
- Empty state: `ContentUnavailableView` (or equivalent) with an SF Symbol,
  one-line explanation, and a call-to-action button.
- Loading: `ProgressView` with a short label; never a frozen blank view.
- Errors: human-readable, inline, with a recovery action. No raw error dumps.

## Interaction & feedback
- Keyboard: Cmd+F focuses search (`.searchable` when there's a list),
  Return confirms, Esc cancels, arrow keys navigate lists.
- Destructive actions get a `confirmationDialog`; prefer undo over "are you
  sure" when feasible.
- Subtle motion: `withAnimation(.snappy)` for list/selection changes; no
  gratuitous effects. Hover effects on clickable rows/cards.

## Accessibility & finish
- Every icon-only button gets an accessibility label.
- Ship an app icon: generate one on-device with Apple Intelligence —
  `sh Tools/generate-icon.sh "flat minimal app icon of <subject> on a rounded
  square <color> background, no text" icon.png` — then read the PNG to check
  it suits the app. Fall back to drawing a simple AppKit icon if generation
  is unavailable.
- Before declaring the work done: build it, then walk each screen and fix
  anything misaligned, cramped, default-looking, or unlabeled.

## Visual review loop
After any build that changes UI, run `sh Tools/visual-review.sh <ExecutableName>`
to capture the real window to `review.png`, read that image, and critique it
against this document. Fix what fails and re-review. A build that has never
been looked at does not ship.
