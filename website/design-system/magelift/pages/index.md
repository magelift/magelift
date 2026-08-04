# Page override: marketing index

Overrides MASTER for https://magelift.dev/

## Layout
- Full-bleed dock atmosphere as responsive `<picture>`: AVIF → WebP → JPEG,
  1600w + 2560w under `/media/hero-atmosphere-*.{avif,webp,jpg}`.
  Readability comes from a `--bg0` gradient scrim, not from image darkening.
- Split hero (≥901px): brand + value + CTAs left (1.05fr), live CLI terminal
  right (0.95fr). Stacks to one column on mobile.
- No cards in hero; feature/proof rows use hairline dividers (`--rule`).
- Max 1 eyebrow total on page (the `.brand-mark` lockup in the hero).
- Section order: proof strip → personas (tabs) → how-it-works (steps + YAML
  figure) → certified targets → trust grid → FAQ (`<details>`) → final CTA.

## CTA
- Single primary intent label: **Install the CLI**
- Secondary: Get started
- Repeated verbatim in the final CTA section.
- Header GitHub entry: octocat mark + text (icon-only on mobile, always visible).
- Terminal chrome: macOS traffic lights (`#ff5f57` / `#febc2e` / `#28c840`), not gray dots.

## Motions
- Hero entrance: staggered springs over `[data-enter]` (60ms + 90ms/step,
  stiffness 130 / damping 22).
- Section reveal: `[data-reveal]` starts hidden only when below the fold,
  springs in (16px y) via `inView` at 15% visibility.
- Terminal: live typing loop (cmd keystrokes → dim/ok output → destroy
  outcome), static transcript under reduced motion / no-JS (`<pre class="sr-only">`
  always present for assistive tech).
- Persona tabs: WAI tablist, sliding ink bar spring, panel crossfade.
- Hero grid overlay drifts on scroll via `animation-timeline: scroll(root)`
  behind `@supports`, `0–100dvh` range.
- FAQ answers animate open via `::details-content` + `interpolate-size`.
- Button `:active` scale 0.98.
- All motion gated on `prefers-reduced-motion`; hidden-tab WAAPI freezes
  resume cleanly on visibility.

## Assets
- Fonts self-hosted variable woff2 under `/fonts/` (Lexend, Source Sans 3,
  IBM Plex Mono 400/500), preloaded in `Base.astro`, `font-display: swap`.
- The 2.3MB `hero-atmosphere.png` source is retired; keep only the generated
  AVIF/WebP/JPEG renditions in `public/media/`.
