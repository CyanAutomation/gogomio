import type { MetricTone, StatusTone } from "./dashboard-presentation";

export type BadgeTone = StatusTone | "info";
export type MetricRowIcon = "alert" | "camera" | "chart" | "clock" | "grid" | "image" | "link" | "quality";
export type MetricRowTone = "neutral" | "error";

export interface StatusBadgeView {
    root: Pick<HTMLElement, "className">;
    label: Pick<HTMLElement, "textContent">;
}

/** Updates a reusable status badge without rewriting an unchanged live-region label. */
export function updateStatusBadge(view: StatusBadgeView, label: string, tone: BadgeTone): void {
    view.root.className = `ui-badge ui-badge--${tone}`;
    if (view.label.textContent !== label) view.label.textContent = label;
}

/** Renders the shared status badge used in dynamically-built diagnostics cards. */
export function renderStatusBadge(label: string, tone: BadgeTone): string {
    return `<span class="ui-badge ui-badge--${tone}"><span class="ui-badge__dot" aria-hidden="true"></span><span class="ui-badge__label">${escapeHTML(label)}</span></span>`;
}

/** Renders a shared metric card with escaped content and a constrained semantic tone. */
export function renderMetricCard(label: string, value: unknown, tone: MetricTone = "neutral"): string {
    return `<div class="metric-card"><div class="metric-card__label">${escapeHTML(label)}</div><div class="metric-card__value metric-card__value--${tone}">${escapeHTML(value)}</div></div>`;
}

/** Renders a diagnostics row with escaped text and constrained icon and tone values. */
export function renderMetricRow(
    label: string,
    value: unknown,
    icon: MetricRowIcon,
    tone: MetricRowTone = "neutral",
): string {
    const iconMarkup = `<svg class="icon icon--small" aria-hidden="true" focusable="false"><use href="#icon-${icon}"></use></svg>`;
    const toneClass = tone === "error" ? " metric-card__value--error" : "";
    return `<div class="metric-row"><span class="metric-row__label">${iconMarkup}<span>${escapeHTML(label)}</span></span><span class="metric-row__value${toneClass}">${escapeHTML(value)}</span></div>`;
}

export function escapeHTML(value: unknown): string {
    const entities: Record<string, string> = {
        "&": "&amp;",
        "<": "&lt;",
        ">": "&gt;",
        '"': "&quot;",
        "'": "&#39;",
    };
    return String(value ?? "").replace(/[&<>"']/g, (character) => entities[character]!);
}
