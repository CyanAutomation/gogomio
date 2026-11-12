import assert from "node:assert/strict";
import test from "node:test";

import { getStreamMascotPresentation } from "./dashboard-presentation";
import {
    renderIcon,
    renderMetricCard,
    renderMetricRow,
    renderNotice,
    renderSectionHeading,
    renderStatusBadge,
    renderStatusMetricCard,
    updateMascotImage,
    updateStatusBadge,
    type StatusBadgeView,
} from "./ui-components";

test("status badge rendering escapes its label and marks its decorative dot", () => {
    assert.equal(
        renderStatusBadge("Camera <ready>", "success"),
        '<span class="ui-badge ui-badge--success"><span class="ui-badge__dot" aria-hidden="true"></span><span class="ui-badge__label">Camera &lt;ready&gt;</span></span>',
    );
});

test("status badge updates keep a stable accessible label and use a semantic tone", () => {
    const view: StatusBadgeView = {
        root: { className: "ui-badge ui-badge--neutral" },
        label: { textContent: "Checking camera…" },
    };

    updateStatusBadge(view, "Camera ready", "success");

    assert.equal(view.root.className, "ui-badge ui-badge--success");
    assert.equal(view.label.textContent, "Camera ready");
});

test("unchanged status badges do not rewrite their live-region label", () => {
    let writes = 0;
    let currentLabel = "Camera ready";
    const view: StatusBadgeView = {
        root: { className: "ui-badge ui-badge--success" },
        label: Object.defineProperty({}, "textContent", {
            get: () => currentLabel,
            set: (value: string | null) => { writes++; currentLabel = value ?? ""; },
        }) as Pick<HTMLElement, "textContent">,
    };

    updateStatusBadge(view, "Camera ready", "success");

    assert.equal(writes, 0);
});

test("MIO avatar component updates its pose and uses a reusable size variant", () => {
    const image = { src: "", alt: "old description", className: "old-avatar" };

    updateMascotImage(image, getStreamMascotPresentation("live"), "status", true);

    assert.equal(image.src, getStreamMascotPresentation("live").src);
    assert.equal(image.alt, "");
    assert.equal(image.className, "mio-avatar mio-avatar--status mio-float");
});

test("metric cards escape user-facing content and constrain tones", () => {
    assert.equal(
        renderMetricCard("System <health>", "Excellent & stable", "success"),
        '<div class="metric-card"><div class="metric-card__label">System &lt;health&gt;</div><div class="metric-card__value metric-card__value--success">Excellent &amp; stable</div></div>',
    );
});

test("metric rows escape labels and values and use only a known tone class", () => {
    assert.equal(
        renderMetricRow(`Recent <img src=x onerror="alert(1)">`, "3 & counting", "alert", "error"),
        '<div class="metric-row"><span class="metric-row__label"><svg class="icon icon--small" aria-hidden="true" focusable="false"><use href="#icon-alert"></use></svg><span>Recent &lt;img src=x onerror=&quot;alert(1)&quot;&gt;</span></span><span class="metric-row__value metric-card__value--error">3 &amp; counting</span></div>',
    );
});

test("icons are decorative, non-focusable, and restricted to the shared symbol library", () => {
    assert.equal(
        renderIcon("camera"),
        '<svg class="icon icon--small" aria-hidden="true" focusable="false"><use href="#icon-camera"></use></svg>',
    );
});

test("section headings compose a shared icon with escaped sentence-case content", () => {
    assert.equal(
        renderSectionHeading("Current <health>", "chart"),
        '<div class="section-heading"><h3 class="section-title"><svg class="icon icon--small" aria-hidden="true" focusable="false"><use href="#icon-chart"></use></svg><span>Current &lt;health&gt;</span></h3></div>',
    );
});

test("status metric cards compose the shared tile and badge primitives", () => {
    assert.equal(
        renderStatusMetricCard("System <health>", "Ready & steady", "success"),
        '<div class="metric-card metric-card--status"><div class="metric-card__label">System &lt;health&gt;</div><span class="ui-badge ui-badge--success"><span class="ui-badge__dot" aria-hidden="true"></span><span class="ui-badge__label">Ready &amp; steady</span></span></div>',
    );
});

test("notices compose accessible decorative icons and escaped user-facing copy", () => {
    assert.equal(
        renderNotice("error", "System <status>", "Camera <offline> & waiting", "alert"),
        '<div class="notice notice--error diagnostic-notice"><strong><svg class="icon icon--small" aria-hidden="true" focusable="false"><use href="#icon-alert"></use></svg> System &lt;status&gt;</strong><p>Camera &lt;offline&gt; &amp; waiting</p></div>',
    );
});
