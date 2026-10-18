import { animate, inView, scroll } from "motion";

const prefersReduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

/* Header: frosted state once the page leaves the top. */
const header = document.getElementById("site-header");
if (header) {
  const updateHeader = () => header.classList.toggle("is-scrolled", window.scrollY > 12);
  updateHeader();
  scroll(updateHeader);
}

/* Hero entrance: staggered spring for [data-enter] elements.
   CSS hides them only under html.js:not(.rm), so no-JS and reduced-motion
   users always see the final state. */
const enterEls = [...document.querySelectorAll("[data-enter]")];
if (!prefersReduced && enterEls.length) {
  animate(
    enterEls,
    { opacity: [0, 1], y: [16, 0] },
    {
      delay: (i) => 0.06 + i * 0.09,
      type: "spring",
      stiffness: 130,
      damping: 22,
    },
  );
} else {
  enterEls.forEach((el) => (el.style.opacity = "1"));
}

/* Section reveals: hide only what starts below the fold, then spring it in
   when it enters the viewport. Sections already visible load clean. */
const revealEls = [...document.querySelectorAll("[data-reveal]")];
if (!prefersReduced) {
  revealEls.forEach((el) => {
    if (el.getBoundingClientRect().top <= window.innerHeight * 0.88) return;
    el.style.opacity = "0";
    el.style.transform = "translateY(16px)";
    const stop = inView(
      el,
      () => {
        animate(el, { opacity: [0, 1], y: [16, 0] }, { type: "spring", stiffness: 110, damping: 24 });
        el.style.transform = "";
        stop();
      },
      { amount: 0.15 },
    );
  });
}

/* Live terminal: types the install-to-destroy story, then loops.
   Reduced motion prints the final transcript immediately. */
const term = document.querySelector("[data-terminal]");
if (term) {
  const body = term.querySelector("[data-terminal-body]");
  const lines = JSON.parse(term.dataset.lines || "[]");

  const renderStatic = () => {
    body.textContent = lines
      .map((line) => (line.type === "cmd" ? `$ ${line.text}` : line.text))
      .join("\n");
  };

  if (prefersReduced || !body || !lines.length) {
    renderStatic();
  } else {
    let firstLine = true;
    const newLineSpan = (cls) => {
      if (!firstLine) body.appendChild(document.createTextNode("\n"));
      firstLine = false;
      const span = document.createElement("span");
      if (cls) span.className = cls;
      body.appendChild(span);
      return span;
    };
    const typeCmd = async (text) => {
      const line = newLineSpan("cmd-line");
      const prompt = document.createElement("span");
      prompt.className = "prompt";
      prompt.textContent = "$ ";
      line.appendChild(prompt);
      const typed = document.createElement("span");
      line.appendChild(typed);
      for (let i = 1; i <= text.length; i += 1) {
        typed.textContent = text.slice(0, i);
        await sleep(13 + Math.random() * 26);
      }
      await sleep(200 + Math.random() * 200);
    };
    const showOut = async (line) => {
      newLineSpan(line.tone === "ok" ? "out-ok" : "out-dim").textContent = line.text;
      await sleep(340 + Math.random() * 280);
    };
    (async () => {
      // Let the hero entrance settle before the first keystroke.
      await sleep(650);
      for (;;) {
        body.textContent = "";
        firstLine = true;
        for (const line of lines) {
          if (line.type === "cmd") await typeCmd(line.text);
          else await showOut(line);
        }
        await sleep(4800);
      }
    })();
  }
}

/* Copy install command. */
document.querySelectorAll("[data-copy]").forEach((btn) => {
  const label = btn.querySelector(".copy-label");
  const original = label ? label.textContent : "";
  btn.addEventListener("click", async () => {
    const text = btn.dataset.copy;
    try {
      await navigator.clipboard.writeText(text);
    } catch {
      const scratch = document.createElement("textarea");
      scratch.value = text;
      scratch.setAttribute("readonly", "");
      scratch.style.position = "absolute";
      scratch.style.left = "-9999px";
      document.body.appendChild(scratch);
      scratch.select();
      document.execCommand("copy");
      scratch.remove();
    }
    btn.classList.add("is-copied");
    if (label) label.textContent = "copied";
    setTimeout(() => {
      btn.classList.remove("is-copied");
      if (label) label.textContent = original;
    }, 1600);
  });
});

/* Persona tabs: WAI tablist with a sliding ink bar and crossfading panels. */
const personasRoot = document.querySelector("[data-personas]");
if (personasRoot) {
  const tabs = [...personasRoot.querySelectorAll('[role="tab"]')];
  const panels = [...personasRoot.querySelectorAll('[role="tabpanel"]')];
  const ink = personasRoot.querySelector("[data-tab-ink]");
  let current = 0;

  const placeInk = (tab, animateIt) => {
    if (!ink) return;
    const x = tab.offsetLeft;
    const w = tab.offsetWidth;
    if (animateIt && !prefersReduced) {
      animate(ink, { x, width: w }, { type: "spring", stiffness: 340, damping: 32 });
    } else {
      ink.style.transform = `translateX(${x}px)`;
      ink.style.width = `${w}px`;
    }
  };

  const select = (index, animateIt = true) => {
    current = (index + tabs.length) % tabs.length;
    tabs.forEach((tab, i) => {
      tab.setAttribute("aria-selected", i === current ? "true" : "false");
      tab.tabIndex = i === current ? 0 : -1;
    });
    panels.forEach((panel, i) => {
      panel.hidden = i !== current;
    });
    placeInk(tabs[current], animateIt);
    const panel = panels[current];
    if (animateIt && !prefersReduced && panel) {
      animate(panel, { opacity: [0, 1], y: [10, 0] }, { type: "spring", stiffness: 180, damping: 26 });
    }
  };

  tabs.forEach((tab, i) => {
    tab.addEventListener("click", () => select(i));
    tab.addEventListener("keydown", (event) => {
      if (event.key === "ArrowRight") select(current + 1);
      else if (event.key === "ArrowLeft") select(current - 1);
      else if (event.key === "Home") select(0);
      else if (event.key === "End") select(tabs.length - 1);
      else return;
      event.preventDefault();
      tabs[current].focus();
    });
  });

  select(0, false);
  if (document.fonts && document.fonts.ready) {
    document.fonts.ready.then(() => placeInk(tabs[current], false));
  }
  if (typeof ResizeObserver !== "undefined") {
    new ResizeObserver(() => placeInk(tabs[current], false)).observe(personasRoot);
  }
}
