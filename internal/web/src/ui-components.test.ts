import assert from "node:assert/strict";
import test from "node:test";

import { renderMetricCard, renderStatusBadge, updateStatusBadge, type StatusBadgeView } from "./ui-components";

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

test("metric cards escape user-facing content and constrain tones", () => {
    assert.equal(
        renderMetricCard("System <health>", "Excellent & stable", "success"),
        '<div class="metric-card"><div class="metric-card__label">System &lt;health&gt;</div><div class="metric-card__value metric-card__value--success">Excellent &amp; stable</div></div>',
    );
});
