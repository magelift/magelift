# Design System Master File

> **LOGIC:** Page overrides in `pages/` win over this file.
> Human-locked 2026-08-03 after ui-ux-pro-max + design-taste-frontend + frontend-design review.
> Deliberately **not** the skill default (dark + acid green + mono display).

**Project:** MageLift  
**Category:** B2B Magento cloud CLI (open-source)  
**Audience:** Agency tech leads / SME Magento owners  
**Dials:** Variance 6 · Motion 5 · Density 3  

**Design read:** Daylight industrial dock: cool concrete, ink navy, single teal accent. Trust-first Lexend + Source Sans 3. Terminal outcome is the signature visual.

---

## Color Palette

| Role | Hex | CSS Variable |
|------|-----|--------------|
| Background | `#EEF2F6` | `--bg0` |
| Surface | `#E2E8F0` | `--bg1` |
| Ink | `#0B1220` | `--ink` |
| Muted | `#475569` | `--muted` |
| Accent (CTA) | `#0F766E` | `--accent` |
| On accent | `#F8FAFC` | `--accent-ink` |
| Rule | `rgba(11,18,32,0.12)` | `--rule` |
| Panel | `#FFFFFF` | `--panel` |

**Locks:** One accent only. No purple. No cream/terracotta. No dark-mode default.

## Typography

- **Display / brand:** Lexend 600-700  
- **Body:** Source Sans 3 400-600  
- **Mono / terminal:** IBM Plex Mono 400-500  

**Signature:** Full-bleed cool dock atmosphere photo + left brand/headline + right live CLI outcome panel (destroy → spend stopped). Brand mark: isometric ink **M** with teal lift accent (`/media/logo.png`); flat SVG favicon for tabs.

## Motion

1. Hero copy + terminal rise on load (400-700ms)  
2. Section reveal on scroll (opacity + 12px y)  
3. Button `:active` scale 0.98  

Honor `prefers-reduced-motion`.

## Anti-patterns (do not reintroduce)

- Dark mesh + neon green  
- Fraunces / Instrument Serif display  
- Card grids in the hero  
- Eyebrow labels on every section  
- Three equal feature cards with icons  
- Fake div dashboards  
