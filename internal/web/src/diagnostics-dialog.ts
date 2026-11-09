export type NativeDialog = Pick<HTMLDialogElement, "open" | "showModal" | "close">;
export type InitialDialogFocus = Pick<HTMLElement, "focus">;

/** Opens a native modal and moves focus to the action most useful for its content. */
export function openNativeDialog(dialog: NativeDialog, initialFocus?: InitialDialogFocus): boolean {
    if (dialog.open) return false;
    dialog.showModal();
    initialFocus?.focus();
    return true;
}

/** Closes an open native modal; browser behavior restores focus to its invoker. */
export function closeNativeDialog(dialog: NativeDialog, returnValue?: string): boolean {
    if (!dialog.open) return false;
    dialog.close(returnValue);
    return true;
}
