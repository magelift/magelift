# Design System Master File

The landing page is for Magento developers and agencies. The site uses a light
background, navy text, and one teal accent. See `src/styles/global.css` for the
color values and component styles.

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

**Signature:** Full-bleed cool dock atmosphere photo + left brand/headline + right live CLI outcome panel (destroy → teardown reported). Brand mark: isometric ink **M** with teal lift accent (`/media/logo.png`); flat SVG favicon for tabs.

## Motion

1. Hero copy + terminal rise on load: staggered springs (`[data-enter]`)  
2. Section reveal on scroll: spring, opacity + 16px y, `inView` at 15%  
3. Terminal live-typing loop (static transcript when motion is reduced)  
4. Button `:active` scale 0.98  

Honor `prefers-reduced-motion`. Springs via Motion (vanilla `motion` package), scroll effects via `animation-timeline` behind `@supports`.

Fonts are self-hosted variable woff2 in `public/fonts/`: no Google Fonts `<link>`.

## Anti-patterns (do not reintroduce)

- Dark mesh + neon green  
- Fraunces / Instrument Serif display  
- Card grids in the hero  
- Eyebrow labels on every section  
- Three equal feature cards with icons  
- Fake div dashboards  
