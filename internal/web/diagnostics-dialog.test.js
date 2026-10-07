const test = require("node:test");
const assert = require("node:assert/strict");

const {
    handleKeydown,
    rememberReturnFocus,
    restoreReturnFocus,
} = require("./diagnostics-dialog.js");

class FocusableElement {
    constructor({ disabled = false, hidden = false, tabIndex = 0 } = {}) {
        this.disabled = disabled;
        this.hidden = hidden;
        this.tabIndex = tabIndex;
        this.isConnected = true;
        this.focusCalls = 0;
    }

    focus() {
        this.focusCalls += 1;
    }
}

function makeDialog(elements, activeElement) {
    return {
        hidden: false,
        elements,
        contains(element) {
            return this.elements.includes(element);
        },
        querySelectorAll() {
            return this.elements;
        },
        document: { activeElement },
    };
}

function keyEvent(key, { shiftKey = false } = {}) {
    return {
        key,
        shiftKey,
        defaultPrevented: false,
        preventDefault() {
            this.defaultPrevented = true;
        },
    };
}

test("Tab from the last dialog control wraps to the first", () => {
    const first = new FocusableElement();
    const last = new FocusableElement();
    const modal = makeDialog([first, last], last);
    const event = keyEvent("Tab");

    handleKeydown(event, modal, () => {}, modal.document);

    assert.equal(event.defaultPrevented, true);
    assert.equal(first.focusCalls, 1);
    assert.equal(last.focusCalls, 0);
});

test("Shift+Tab from the first dialog control wraps to the last", () => {
    const first = new FocusableElement();
    const last = new FocusableElement();
    const modal = makeDialog([first, last], first);
    const event = keyEvent("Tab", { shiftKey: true });

    handleKeydown(event, modal, () => {}, modal.document);

    assert.equal(event.defaultPrevented, true);
    assert.equal(last.focusCalls, 1);
});

test("Tab returns focus to the dialog when focus is outside it", () => {
    const first = new FocusableElement();
    const outside = new FocusableElement();
    const modal = makeDialog([first], outside);
    const event = keyEvent("Tab", { shiftKey: true });

    handleKeydown(event, modal, () => {}, modal.document);

    assert.equal(event.defaultPrevented, true);
    assert.equal(first.focusCalls, 1);
});

test("Tab is blocked when the dialog has no focusable controls", () => {
    const disabled = new FocusableElement({ disabled: true });
    const hidden = new FocusableElement({ hidden: true });
    const excluded = new FocusableElement({ tabIndex: -1 });
    const modal = makeDialog([disabled, hidden, excluded], disabled);
    const event = keyEvent("Tab");

    handleKeydown(event, modal, () => {}, modal.document);

    assert.equal(event.defaultPrevented, true);
    assert.equal(disabled.focusCalls + hidden.focusCalls + excluded.focusCalls, 0);
});

test("Escape closes an open dialog and prevents browser default behavior", () => {
    const modal = makeDialog([], null);
    const event = keyEvent("Escape");
    let closeCalls = 0;

    handleKeydown(event, modal, () => { closeCalls += 1; }, modal.document);

    assert.equal(event.defaultPrevented, true);
    assert.equal(closeCalls, 1);
});

test("hidden dialog ignores keyboard events", () => {
    const modal = makeDialog([], null);
    modal.hidden = true;
    const event = keyEvent("Escape");
    let closeCalls = 0;

    handleKeydown(event, modal, () => { closeCalls += 1; }, modal.document);

    assert.equal(event.defaultPrevented, false);
    assert.equal(closeCalls, 0);
});

test("dialog restores focus to the connected element that opened it", () => {
    const opener = new FocusableElement();
    const documentRef = { activeElement: opener };

    const returnFocus = rememberReturnFocus(documentRef);
    restoreReturnFocus(returnFocus);

    assert.equal(opener.focusCalls, 1);
});

test("dialog does not restore focus to an element removed from the document", () => {
    const opener = new FocusableElement();
    const returnFocus = rememberReturnFocus({ activeElement: opener });
    opener.isConnected = false;

    restoreReturnFocus(returnFocus);

    assert.equal(opener.focusCalls, 0);
});
