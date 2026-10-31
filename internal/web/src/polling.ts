export type PollIntervalScheduler = (callback: () => void, intervalMs: number) => number;

/** Schedules a poll only while its owning view is visible and active. */
export function startDashboardPoller(
    isVisible: boolean,
    isActive: boolean,
    intervalMs: number,
    poll: () => void,
    schedule: PollIntervalScheduler = (callback, delay) => window.setInterval(callback, delay),
): number | null {
    if (!isVisible || !isActive) return null;
    return schedule(poll, intervalMs);
}
