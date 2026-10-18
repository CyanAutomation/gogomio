export function requestCameraConfig(fetcher: typeof fetch = fetch): Promise<Response> {
    return fetcher("/api/config");
}

export function requestDiagnostics(fetcher: typeof fetch = fetch): Promise<Response> {
    return fetcher("/api/diagnostics");
}

export function requestStopStream(fetcher: typeof fetch = fetch): Promise<Response> {
    return fetcher("/api/stream/stop", { method: "POST" });
}
