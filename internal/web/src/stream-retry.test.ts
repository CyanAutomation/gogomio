import assert from "node:assert/strict";
import test from "node:test";
import { nextStreamRetryCount, shouldRunStreamRetry } from "./stream-retry";

test("stream retry count includes the final configured retry", () => {
    assert.equal(nextStreamRetryCount(0, 3), 1);
    assert.equal(nextStreamRetryCount(1, 3), 2);
    assert.equal(nextStreamRetryCount(2, 3), 3);
    assert.equal(nextStreamRetryCount(3, 3), null);
});

test("the final scheduled retry is allowed to run", () => {
    assert.equal(shouldRunStreamRetry(false, 3, 3), true);
});

test("stale scheduled retries do not restart a stopped or newer stream", () => {
    assert.equal(shouldRunStreamRetry(false, 1, 0), false);
    assert.equal(shouldRunStreamRetry(false, 1, 2), false);
    assert.equal(shouldRunStreamRetry(true, 1, 1), false);
});
