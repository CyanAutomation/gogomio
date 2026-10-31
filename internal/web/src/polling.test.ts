import assert from "node:assert/strict";
import test from "node:test";
import { startDashboardPoller } from "./polling";

test("dashboard poller schedules work only for visible, active views", () => {
    for (const [isVisible, isActive] of [
        [true, true],
        [false, true],
        [true, false],
        [false, false],
    ] as const) {
        let scheduleCalls = 0;
        let scheduledCallback: (() => void) | undefined;
        let scheduledInterval = 0;
        let pollCalls = 0;

        const handle = startDashboardPoller(
            isVisible,
            isActive,
            2000,
            () => { pollCalls++; },
            (callback, interval) => {
                scheduleCalls++;
                scheduledCallback = callback;
                scheduledInterval = interval;
                return 17;
            },
        );

        if (isVisible && isActive) {
            assert.equal(handle, 17);
            assert.equal(scheduleCalls, 1);
            assert.equal(scheduledInterval, 2000);
            assert.ok(scheduledCallback);
            scheduledCallback();
            assert.equal(pollCalls, 1);
        } else {
            assert.equal(handle, null);
            assert.equal(scheduleCalls, 0);
            assert.equal(pollCalls, 0);
        }
    }
});
