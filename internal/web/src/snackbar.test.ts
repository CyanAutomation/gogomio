import assert from "node:assert/strict";
import test from "node:test";
import { SnackbarController, type SnackbarScheduler, type SnackbarTone, type SnackbarView } from "./snackbar";

class FakeScheduler implements SnackbarScheduler {
    private nextHandle = 0;
    readonly pending = new Map<number, { callback: () => void; delayMs: number }>();

    set(callback: () => void, delayMs: number): number {
        const handle = ++this.nextHandle;
        this.pending.set(handle, { callback, delayMs });
        return handle;
    }

    clear(handle: unknown): void {
        this.pending.delete(handle as number);
    }

    fireNext(): void {
        const [handle, timer] = this.pending.entries().next().value as [number, { callback: () => void; delayMs: number }];
        this.pending.delete(handle);
        timer.callback();
    }
}

class FakeView implements SnackbarView {
    readonly shown: Array<{ text: string; tone: SnackbarTone }> = [];
    hidden = 0;

    show(text: string, tone: SnackbarTone): void {
        this.shown.push({ text, tone });
    }

    hide(): void {
        this.hidden++;
    }
}

test("snackbar automatically dismisses after its short duration", () => {
    const scheduler = new FakeScheduler();
    const view = new FakeView();
    const snackbar = new SnackbarController(view, scheduler, 4500);

    snackbar.show("Stream stopped.", "success");
    assert.equal(view.shown[0]?.text, "Stream stopped.");
    assert.equal([...scheduler.pending.values()][0]?.delayMs, 4500);

    scheduler.fireNext();
    assert.equal(view.hidden, 1);
    assert.equal(scheduler.pending.size, 0);
});

test("a newer message replaces the old one without inheriting its dismissal timer", () => {
    const scheduler = new FakeScheduler();
    const view = new FakeView();
    const snackbar = new SnackbarController(view, scheduler, 4500);

    snackbar.show("First message");
    snackbar.show("Second message", "warning");
    assert.deepEqual(view.shown, [
        { text: "First message", tone: "info" },
        { text: "Second message", tone: "warning" },
    ]);
    assert.equal(scheduler.pending.size, 1);

    scheduler.fireNext();
    assert.equal(view.hidden, 1);
});

test("repeated identical messages coalesce while extending visibility", () => {
    const scheduler = new FakeScheduler();
    const view = new FakeView();
    const snackbar = new SnackbarController(view, scheduler, 4500);

    snackbar.show("Saved.", "success");
    snackbar.show("Saved.", "success");
    assert.equal(view.shown.length, 1);
    assert.equal(scheduler.pending.size, 1);
});

test("hover or keyboard focus pauses dismissal until the snackbar is resumed", () => {
    const scheduler = new FakeScheduler();
    const view = new FakeView();
    const snackbar = new SnackbarController(view, scheduler, 4500);

    snackbar.show("Saved.", "success");
    snackbar.pause();
    assert.equal(scheduler.pending.size, 0);

    snackbar.resume();
    assert.equal(scheduler.pending.size, 1);
    scheduler.fireNext();
    assert.equal(view.hidden, 1);
});

test("snackbar can be dismissed immediately", () => {
    const scheduler = new FakeScheduler();
    const view = new FakeView();
    const snackbar = new SnackbarController(view, scheduler);

    snackbar.show("Saved.", "success");
    snackbar.dismiss();
    assert.equal(view.hidden, 1);
    assert.equal(scheduler.pending.size, 0);
});
