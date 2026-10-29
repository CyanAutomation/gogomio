import assert from "node:assert/strict";
import test from "node:test";
import { shouldRunDashboardPoller } from "./polling";

test("dashboard polling runs only for visible, active views", () => {
    assert.equal(shouldRunDashboardPoller(true, true), true);
    assert.equal(shouldRunDashboardPoller(false, true), false);
    assert.equal(shouldRunDashboardPoller(true, false), false);
    assert.equal(shouldRunDashboardPoller(false, false), false);
});
