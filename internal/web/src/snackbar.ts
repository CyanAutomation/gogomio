export type SnackbarTone = "info" | "success" | "warning";

export interface SnackbarView {
    show(text: string, tone: SnackbarTone): void;
    hide(): void;
}

export interface SnackbarScheduler {
    set(callback: () => void, delayMs: number): unknown;
    clear(handle: unknown): void;
}

const browserScheduler: SnackbarScheduler = {
    set: (callback, delayMs) => window.setTimeout(callback, delayMs),
    clear: handle => window.clearTimeout(handle as number),
};

export class SnackbarController {
    private timer: unknown = null;
    private message: string | null = null;
    private tone: SnackbarTone | null = null;
    private paused = false;

    constructor(
        private readonly view: SnackbarView,
        private readonly scheduler: SnackbarScheduler = browserScheduler,
        private readonly durationMs = 4500,
    ) {}

    show(text: string, tone: SnackbarTone = "info"): void {
        if (this.message !== text || this.tone !== tone) {
            this.message = text;
            this.tone = tone;
            this.view.show(text, tone);
        }
        this.scheduleDismissal();
    }

    pause(): void {
        if (this.message === null || this.paused) return;
        this.paused = true;
        this.clearTimer();
    }

    resume(): void {
        if (this.message === null || !this.paused) return;
        this.paused = false;
        this.scheduleDismissal();
    }

    dismiss(): void {
        this.clearTimer();
        if (this.message !== null) this.view.hide();
        this.message = null;
        this.tone = null;
        this.paused = false;
    }

    private scheduleDismissal(): void {
        this.clearTimer();
        if (this.paused || this.message === null) return;
        this.timer = this.scheduler.set(() => this.dismiss(), this.durationMs);
    }

    private clearTimer(): void {
        if (this.timer === null) return;
        this.scheduler.clear(this.timer);
        this.timer = null;
    }
}
