export function shouldRunDashboardPoller(isVisible: boolean, isActive: boolean): boolean {
    return isVisible && isActive;
}
