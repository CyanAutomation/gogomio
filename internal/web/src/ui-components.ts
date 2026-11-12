import type { MetricTone, StatusTone, StreamMascotPresentation } from "./dashboard-presentation";

export type BadgeTone = StatusTone | "info";
export type IconName = "alert" | "camera" | "chart" | "clock" | "close" | "grid" | "image" | "info" | "link" | "play" | "quality" | "refresh" | "settings" | "stop";
export type MetricRowIcon = Exclude<IconName, "close" | "play" | "settings" | "stop">;
export type MetricRowTone = "neutral" | "error";
export type NoticeTone = Exclude<BadgeTone, "neutral">;
export type MascotAvatarSize = "brand" | "status" | "footer" | "state";

export interface StatusBadgeView {
    root: Pick<HTMLElement, "className">;
    label: Pick<HTMLElement, "textContent">;
}

export interface MascotImageView {
    src: string;
    alt: string;
    className: string;
}

/** Renders an existing decorative SVG symbol from the page's shared icon library. */
export function renderIcon(name: IconName): string {
    return `<svg class="icon icon--small" aria-hidden="true" focusable="false"><use href="#icon-${name}"></use></svg>`;
}

/** Renders a consistent subsection heading for dashboard and diagnostic content. */
export function renderSectionHeading(title: string, icon: IconName): string {
    return `<div class="section-heading"><h3 class="section-title">${renderIcon(icon)}<span>${escapeHTML(title)}</span></h3></div>`;
}

/** Updates a reusable status badge without rewriting an unchanged live-region label. */
export function updateStatusBadge(view: StatusBadgeView, label: string, tone: BadgeTone): void {
    view.root.className = `ui-badge ui-badge--${tone}`;
    if (view.label.textContent !== label) view.label.textContent = label;
}

/** Updates a shared MIO avatar while keeping its size and motion as reusable variants. */
export function updateMascotImage(
    image: MascotImageView,
    presentation: StreamMascotPresentation,
    size: MascotAvatarSize,
    decorative = false,
): void {
    image.src = presentation.src;
    image.alt = decorative ? "" : presentation.alt;
    image.className = `mio-avatar mio-avatar--${size} ${presentation.animationClass}`;
}

/** Renders the shared status badge used in dynamically-built diagnostics cards. */
export function renderStatusBadge(label: string, tone: BadgeTone): string {
    return `<span class="ui-badge ui-badge--${tone}"><span class="ui-badge__dot" aria-hidden="true"></span><span class="ui-badge__label">${escapeHTML(label)}</span></span>`;
}

/** Renders a metric tile that pairs a readable label with the shared status badge. */
export function renderStatusMetricCard(label: string, status: string, tone: BadgeTone): string {
    return `<div class="metric-card metric-card--status"><div class="metric-card__label">${escapeHTML(label)}</div>${renderStatusBadge(status, tone)}</div>`;
}

/** Renders a tone-coded notice with escaped copy and a decorative shared icon. */
export function renderNotice(tone: NoticeTone, title: string, message: unknown, icon: IconName): string {
    return `<div class="notice notice--${tone} diagnostic-notice"><strong>${renderIcon(icon)} ${escapeHTML(title)}</strong><p>${escapeHTML(message)}</p></div>`;
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
    const toneClass = tone === "error" ? " metric-card__value--error" : "";
    return `<div class="metric-row"><span class="metric-row__label">${renderIcon(icon)}<span>${escapeHTML(label)}</span></span><span class="metric-row__value${toneClass}">${escapeHTML(value)}</span></div>`;
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
