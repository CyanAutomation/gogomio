(function (root, factory) {
    if (typeof module === "object" && module.exports) {
        module.exports = factory();
    } else {
        root.GoGoMioDiagnosticsDialog = factory();
    }
})(typeof globalThis === "object" ? globalThis : this, function () {
    "use strict";

    const FOCUSABLE_SELECTOR = 'button, a[href], input, select, textarea, [tabindex]:not([tabindex="-1"])';

    function focusableElements(modal) {
        return Array.from(modal.querySelectorAll(FOCUSABLE_SELECTOR))
            .filter((element) => !element.disabled && !element.hidden && element.tabIndex >= 0);
    }

    function rememberReturnFocus(documentRef) {
        return documentRef.activeElement;
    }

    function restoreReturnFocus(element) {
        if (element && element.isConnected && typeof element.focus === "function") {
            element.focus();
        }
    }

    function handleKeydown(event, modal, closeModal, documentRef) {
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

        const first = focusable[0];
        const last = focusable[focusable.length - 1];
        const focusIsOutsideDialog = !modal.contains(documentRef.activeElement);
        if (focusIsOutsideDialog || (event.shiftKey ? documentRef.activeElement === first : documentRef.activeElement === last)) {
            event.preventDefault();
            (event.shiftKey ? last : first).focus();
        }
    }

    return { handleKeydown, rememberReturnFocus, restoreReturnFocus };
});
