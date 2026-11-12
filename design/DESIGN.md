# GoGoMio UI design system

The dashboard keeps the camera view first, makes camera health easy to scan, and treats MIO as a visible companion without obscuring operational details. The CSS custom properties in `internal/web/index.html` are the source of truth for visual tokens. Build new UI from the shared primitives and components below instead of adding one-off styles.

HTML renderers and stateful view updates live in `internal/web/src/ui-components.ts`. Reuse its typed, escaped renderers when composing dynamic dashboard or diagnostics content; keep application data and state decisions in `app.ts` and `dashboard-presentation.ts`.

## Visual tokens

### Color

| Role | Token | Value | Use |
| --- | --- | --- | --- |
| MIO blue | `--color-brand-accent` | `#5FA8D3` | Header, card accents, focus details, and brand surfaces |
| Ocean blue | `--color-brand-secondary` | `#3C7DC4` | Icons, links, and secondary emphasis |
| Deep blue text | `--color-primary` | `#2A5F8A` | Headings, metric values, and text on light blue |
| Primary action fill | `--color-action-background` | `#5FA8D3` | Primary action button |
| Primary action hover | `--color-action-hover-background` | `#A7D3E8` | Hover and pressed affordance |
| Action text | `--color-action-foreground` | `#0F1A2B` | Text on the bright action fill |
| Light brand accent | `--color-primary-light` | `#A7D3E8` | Soft fills, borders, and header highlights |
| Main text | `--color-text` | `#1E293B` | Body copy and values |
| Supporting text | `--color-text-secondary` | `#52657A` | Labels and secondary copy |
| Success | `--color-success` / `--color-success-dark` | `#10B981` / `#065F46` | Indicators and success text |
| Warning | `--color-warning` / `--color-warning-dark` | `#F59E0B` / `#78350F` | In-progress indicators and warning text |
| Error | `--color-error` / `--color-error-dark` | `#EF4444` / `#7F1D1D` | Failed states and error text |

Use the character sheet's blues as visible brand colors, then pair them with dark ink for readable text. The dark color scheme uses deep ocean surfaces and brighter MIO blues. Keep text contrast at 4.5:1 for normal text and 3:1 for large text. Pair every status color with a written label; do not use brand color as a substitute for success, warning, or error semantics.

### Spacing, shape, and type

- Use the 4px spacing scale: `--spacing-xs` through `--spacing-3xl`.
- Use `--radius-sm`, `--radius-md`, and `--radius-lg` for controls, metric tiles, and panels.
- Use the declared font-size scale. Keep body text at 16px or larger where space permits.
- Use sentence case for headings, buttons, metric labels, and notices. Preserve technical abbreviations such as JPEG and fps.

## Reusable components

| Component | Classes | Guidance |
| --- | --- | --- |
| Page frame | `.site-header`, `.page-header`, `.site-footer` | Use for the branded shell and page title; keep MIO visible in the header and footer. |
| Surface | `.surface-panel`, `.card`, `.section-heading` | Use the shared radius, border, shadow, and brand edge for sections. |
| MIO avatar | `.mio-avatar`, `.mio-avatar--{brand,status,footer,state}` | Reuse one image primitive with size variants; keep state avatars decorative when adjacent text names the state. |
| Status badge | `.ui-badge`, `.ui-badge--{neutral,success,warning,error,info}` | Use the same semantic badge in the stream summary and diagnostics; always include a written label. |
| Button | `.button`, `.button--primary`, `.button--secondary`, `.button--icon` | Use one primary action per region; keep keyboard focus visible. |
| Metric tile | `.metric-card`, `.metric-card__label`, `.metric-card__value` | Use for overview and diagnostic summaries; keep labels and units consistent. |
| Metric row | `.metric-row`, `.metric-row__label`, `.metric-row__value` | Use for detailed diagnostic values. |
| Notice | `.notice`, `.notice--success`, `.notice--warning`, `.notice--error`, `.notice--info` | Share tone styling for transient messages and diagnostic status. |
| Stream state | `getStreamStatusPresentation()` and `getStreamMascotPresentation()` | Keep status language, semantic tone, and MIO pose aligned across all five stream states. |
| Icon | SVG symbols in `.icon-library` | Reuse an existing symbol before adding a new one. Hide decorative icons from assistive technology. |

`ui-components.ts` exposes `renderIcon()`, `renderSectionHeading()`, `renderStatusBadge()`, `renderStatusMetricCard()`, `renderMetricCard()`, `renderMetricRow()`, and `renderNotice()`. These components compose the CSS primitives above, escape text content, and constrain icon and tone variants to their shared types. Add a renderer when repeated markup needs one consistent accessible treatment; keep one-off content in its owning page.

Use CSS custom properties for palette values, spacing, radius, shadow, and motion. Prefer a modifier on a shared component when a state or size needs variation. Keep decorative bubbles and mascot motion out of forced-colors mode and respect reduced-motion preferences.

## Stream states

| State | Label | Tone |
| --- | --- | --- |
| Ready | Ready | Neutral |
| Connecting | Connecting… | Warning |
| Live | Live | Success |
| Reconnecting | Reconnecting… | Warning |
| Unavailable | Unavailable | Error |

Use the same state language in the viewer, status bar, controls, and diagnostics. Reserve red for a failed connection. Keep the primary action available in ready and unavailable states; allow stopping during connection, streaming, or reconnection.

Keep the camera viewer dark enough for camera imagery. Use the bright palette around the viewer, in the idle screen, status companion, buttons, and metric surfaces. Show the camera detail tiles on first load so resolution, JPEG quality, and active streams are easy to scan; let users collapse the group when they want more space.

## Voice and copy

MIO sounds upbeat, calm, and helpful. Put the useful information first, add a light touch of personality, and give one clear next step. Keep error messages kind and specific. Put detailed technical explanations in Diagnostics.

Examples:

- Ready: “MIO's ready when you are.”
- Connecting: “MIO is warming things up…”
- Reconnecting: “MIO is reconnecting (1 of 3)…”
- Unavailable: “MIO couldn't reconnect. Check the camera connection, then try again.”

Use consistent names and units in the overview and Diagnostics: Frame rate in fps, Resolution as `width × height`, JPEG quality as a percentage, and Active streams as `active / maximum`. Describe negative or missing frame-age values as “No frames yet.”
