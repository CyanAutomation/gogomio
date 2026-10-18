const FOCUSABLE_SELECTOR = 'button, a[href], input, select, textarea, [tabindex]:not([tabindex="-1"])';

export type ReturnFocusTarget = Element & { focus(): void };

function focusableElements(modal: HTMLElement): HTMLElement[] {
    return Array.from(modal.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR))
        .filter((element) => {
            const control = element as HTMLElement & { disabled?: boolean };
            return !control.disabled && !element.hidden && element.tabIndex >= 0;
        });
}

export function rememberReturnFocus(documentRef: Document): ReturnFocusTarget | null {
    const activeElement = documentRef.activeElement;
    if (activeElement && "focus" in activeElement && typeof activeElement.focus === "function") {
        return activeElement as ReturnFocusTarget;
    }
    return null;
}

export function restoreReturnFocus(element: ReturnFocusTarget | null): void {
    if (element && element.isConnected) {
        element.focus();
    }
}

export function handleKeydown(
    event: KeyboardEvent,
    modal: HTMLElement,
    closeModal: () => void,
    documentRef: Document,
): void {
    if (modal.hidden) return;

    if (event.key === "Escape") {
        event.preventDefault();
        closeModal();
        return;
    }

    if (event.key !== "Tab") return;

    const focusable = focusableElements(modal);
    if (focusable.length === 0) {
        event.preventDefault();
        return;
    }

    const first = focusable[0]!;
    const last = focusable[focusable.length - 1]!;
    const focusIsOutsideDialog = !modal.contains(documentRef.activeElement);
    if (focusIsOutsideDialog || (event.shiftKey ? documentRef.activeElement === first : documentRef.activeElement === last)) {
        event.preventDefault();
        (event.shiftKey ? last : first).focus();
    }
}
