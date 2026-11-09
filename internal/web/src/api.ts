export function requestCameraConfig(fetcher: typeof fetch = fetch): Promise<Response> {
    return fetcher("/v1/config/camera");
}

export function requestCameraReadiness(fetcher: typeof fetch = fetch): Promise<Response> {
    return fetcher("/v1/ready", { cache: "no-store" });
}

export function requestLiveMetrics(fetcher: typeof fetch = fetch): Promise<Response> {
    return fetcher("/v1/metrics/live", { cache: "no-store" });
}

export function requestDiagnostics(fetcher: typeof fetch = fetch): Promise<Response> {
    return fetcher("/v1/health/detailed", { cache: "no-store" });
}

export function requestStopStream(fetcher: typeof fetch = fetch): Promise<Response> {
    return fetcher("/v1/api/stream/stop", { method: "POST" });
}
