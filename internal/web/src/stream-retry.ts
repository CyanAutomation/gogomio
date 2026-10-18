export function nextStreamRetryCount(current: number, maxRetries: number): number | null {
    return current < maxRetries ? current + 1 : null;
}

export function shouldRunStreamRetry(isStreaming: boolean, scheduledRetryCount: number, currentRetryCount: number): boolean {
    return !isStreaming && scheduledRetryCount === currentRetryCount;
}
