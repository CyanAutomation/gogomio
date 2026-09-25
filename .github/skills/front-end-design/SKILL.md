---
name: front-end-design
description: Use when asked to create or substantially redesign a GoGoMio web page or app surface, especially when visual hierarchy, art direction, or interaction design matters.
---

# Front-end Design

Use this skill for visual design work. For a small functional change, preserve the existing layout and design system unless the user asks for a redesign.

## Start with the brief and existing UI

- Inspect the relevant page, its styles, assets, and responsive behavior before proposing a new pattern. [`internal/web/index.html`](../../../internal/web/index.html) contains the current GoGoMio UI and design tokens.
- Follow the user's requested style, content, and interaction. Treat the guidance below as defaults that yield to the brief and existing product conventions.
- Establish a clear hierarchy: the page's primary purpose, the most important information, and the next useful action should be easy to scan.

## Visual direction

- Use spacing, alignment, type scale, color, and imagery to create hierarchy before adding decorative chrome.
- Use the existing design tokens and components where they fit. Cards, columns, borders, and full-bleed sections are layout choices; choose them for the content and task rather than banning or requiring them.
- For marketing pages, make the product promise and action clear. Use a dominant image or visual plane when it supports the story; do not force a hero image, section sequence, or full-bleed layout onto every page.
- For dashboards and operational surfaces, prioritize status, labels, freshness, and actions. Avoid marketing copy when users need to operate or diagnose the service.
- Add images only when they clarify context or create useful atmosphere. Use project assets when appropriate; do not add stock or generated imagery just to fill space.

## Interaction and accessibility

- Add motion only when it communicates state, guides attention, or improves feedback. Do not require a fixed number of animations or add a new motion library for decorative effects.
- Respect `prefers-reduced-motion`; keep transitions brief and preserve the same information without animation.
- Maintain semantic controls, keyboard operation, visible focus, readable contrast, and usable touch targets.
- Check responsive behavior at narrow and wide viewports. Keep long labels, status messages, and controls usable at small widths.

## Review before finishing

- Confirm the main task and next action are clear without relying on decorative treatment.
- Check that existing product patterns remain consistent unless the brief calls for a change.
- Check keyboard focus, contrast, reduced-motion behavior, and narrow-screen layout when the change affects them.
- Describe any notable visual changes and the viewports or interactions inspected.
