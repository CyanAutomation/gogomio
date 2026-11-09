import assert from "node:assert/strict";
import { test } from "node:test";

import { closeNativeDialog, openNativeDialog } from "./diagnostics-dialog";

class FakeDialog {
    open = false;
    showCalls = 0;
    closeCalls = 0;

    showModal(): void {
        this.showCalls++;
        this.open = true;
    }

    close(): void {
        this.closeCalls++;
        this.open = false;
    }
}

test("opening a native dialog shows it modally and focuses the requested control", () => {
    const dialog = new FakeDialog();
    let focusCalls = 0;

    assert.equal(openNativeDialog(dialog, { focus: () => { focusCalls++; } }), true);

    assert.equal(dialog.open, true);
    assert.equal(dialog.showCalls, 1);
    assert.equal(focusCalls, 1);
});

test("opening an already-open native dialog does not reopen it", () => {
    const dialog = new FakeDialog();
    dialog.open = true;

    assert.equal(openNativeDialog(dialog), false);
    assert.equal(dialog.showCalls, 0);
});

test("closing a native dialog is idempotent", () => {
    const dialog = new FakeDialog();

    assert.equal(closeNativeDialog(dialog), false);
    assert.equal(dialog.closeCalls, 0);

    dialog.open = true;
    assert.equal(closeNativeDialog(dialog), true);
    assert.equal(dialog.open, false);
    assert.equal(dialog.closeCalls, 1);
});
