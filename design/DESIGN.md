# GoGoMio UI design system

The dashboard keeps the camera view first, makes camera health easy to scan, and uses MIO to add warmth without obscuring operational details. The CSS custom properties in `internal/web/index.html` are the source of truth for visual tokens. Reuse the component classes below before adding one-off styles.

## Visual tokens

### Color

| Role | Token | Value | Use |
| --- | --- | --- | --- |
| Primary text and action | `--color-primary` | `#2A5F8A` | Primary buttons, metric values, headings |
| Primary hover | `--color-primary-dark` | `#214A6B` | Hover and pressed states |
| Brand accent | `--color-brand-accent` | `#5FA8D3` | Decorative accents and borders |
| Light brand accent | `--color-primary-light` | `#A7D3E8` | Soft borders and fills |
| Main text | `--color-text` | `#1E293B` | Body copy and values |
| Supporting text | `--color-text-secondary` | `#52657A` | Labels and secondary copy |
| Success | `--color-success` / `--color-success-dark` | `#10B981` / `#065F46` | Indicators and success text |
| Warning | `--color-warning` / `--color-warning-dark` | `#F59E0B` / `#78350F` | In-progress indicators and warning text |
| Error | `--color-error` / `--color-error-dark` | `#EF4444` / `#7F1D1D` | Failed states and error text |

Use dark semantic colors for text and actions. Keep the light blue accent for fills, borders, and decoration. Text contrast should be at least 4.5:1 for normal text and 3:1 for large text. Pair every status color with a written label.

### Spacing, shape, and type

- Use the 4px spacing scale: `--spacing-xs` through `--spacing-3xl`.
- Use `--radius-sm`, `--radius-md`, and `--radius-lg` for controls, metric tiles, and panels.
- Use the declared font-size scale. Keep body text at 16px or larger where space permits.
- Use sentence case for headings, buttons, metric labels, and notices. Preserve technical abbreviations such as JPEG and fps.

## Reusable components

| Component | Classes | Guidance |
| --- | --- | --- |
| Panel and card | `.surface-panel`, `.card` | Use the shared white surface, radius, shadow, and accent edge for dashboard sections. |
| Section heading | `.section-title` | Pair one decorative SVG icon with a concise sentence-case heading. |
| Button | `.button`, `.button--primary`, `.button--secondary`, `.button--icon` | Use one primary action per region; keep keyboard focus visible. |
| Metric tile | `.metric-card`, `.metric-card__label`, `.metric-card__value` | Use for both overview and diagnostic summaries; keep labels and units consistent. |
| Metric row | `.metric-row`, `.metric-row__label`, `.metric-row__value` | Use for detailed diagnostic values. |
| Notice | `.notice`, `.notice--success`, `.notice--warning`, `.notice--error`, `.notice--info` | Share the same tone styling for transient messages and diagnostic status. |
| Stream status | `getStreamStatusPresentation()` and `.status-indicator--{tone}` | Use the lifecycle state and semantic tone table below. Neutral ready states are not errors. |
| Icon | SVG symbols in `.icon-library` | Reuse an existing symbol before adding a new one. Hide decorative icons from assistive technology. |

## Stream states

| State | Label | Tone |
| --- | --- | --- |
| Ready | Ready | Neutral |
| Connecting | Connecting… | Warning |
| Live | Live | Success |
| Reconnecting | Reconnecting… | Warning |
| Unavailable | Unavailable | Error |

Use the same state language in the viewer, status bar, controls, and diagnostics. Reserve red for a failed connection. Keep the primary action available in ready and unavailable states; allow stopping during connection, streaming, or reconnection.

## Voice and copy

MIO sounds upbeat, calm, and helpful. Put the useful information first, add a light touch of personality, and give one clear next step. Keep error messages kind and specific. Put detailed technical explanations in Diagnostics.

Examples:

- Ready: “MIO's ready when you are.”
- Connecting: “MIO is warming things up…”
- Reconnecting: “MIO is reconnecting (1 of 3)…”
- Unavailable: “MIO couldn't reconnect. Check the camera connection, then try again.”

Use consistent names and units in the overview and Diagnostics: Frame rate in fps, Resolution as `width × height`, JPEG quality as a percentage, and Active streams as `active / maximum`. Describe negative or missing frame-age values as “No frames yet.”
