---
type: lesson
title: Motion v12 vanilla DOM imports moved to main entry
description: In Motion v12 the vanilla DOM functions (animate, inView, scroll) export from the main "motion" package entry; the "motion/dom" subpath is not exported and fails Vite/Rolldown resolution.
tags: [motion, astro, vite, marketing-site]
status: stable
generated:
  by: cursor/mac
  at: 2026-08-04
---

# Motion v12 vanilla DOM imports moved to main entry

The public site (`website/`, formerly `websites/marketing`) uses Motion 12.x
for vanilla DOM animation. Framer-Motion-era docs and older examples import DOM helpers from
`motion/dom`. That subpath is **not exported** by Motion 12.x; the build fails
with `rolldown:vite-resolve` reporting `"./dom" is not exported by the motion
package`.

Working import (verified against `node_modules/motion/dist/es/index.mjs`):

```js
import { animate, inView, scroll } from "motion";
```

`animate` on DOM elements uses WAAPI springs; `inView` wraps
IntersectionObserver; `scroll` wraps scroll listeners. Check the installed
package's exports before trusting doc snippets — the package consolidated
subpaths when Framer Motion rebranded to Motion.
