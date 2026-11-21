import { resolutionToAspectRatio } from "./aspect-ratio";
import {
    isCapacitySignal,
    type MetricTone,
} from "./dashboard-presentation";
import {
    formatFrameRate,
    formatResolution,
    formatStreamConnections,
    getFrameRateTone,
} from "./metrics-presentation";

export interface CameraConfigResponse {
    resolution: [number, number];
    fps: number;
    target_fps?: number | null;
    jpeg_quality: number;
    mock_camera: boolean;
    camera_backend?: string;
    max_stream_connections: number;
}

export interface LiveMetricsResponse {
    fps_current: number;
    stream_connections: number;
}

export interface LiveMetricsPresentation {
    aspectRatio: string | null;
    feedModeHidden: boolean;
    fpsText: string;
    fpsTone: MetricTone;
    resolutionText: string;
    qualityText: string;
    connectionsText: string;
    capacitySignal: boolean;
}

export function getLiveMetricsPresentation(
    config: CameraConfigResponse,
    metrics: LiveMetricsResponse,
    isStreaming: boolean,
): LiveMetricsPresentation {
    const fps = Number(metrics.fps_current);
    const targetFps = config.target_fps ?? Number.NaN;

    return {
        aspectRatio: resolutionToAspectRatio(config.resolution),
        feedModeHidden: config.mock_camera !== true,
        fpsText: isStreaming ? formatFrameRate(fps) : "",
        fpsTone: getFrameRateTone(fps, targetFps, isStreaming),
        resolutionText: formatResolution(config.resolution),
        qualityText: (config.jpeg_quality ?? "--") + "%",
        connectionsText: formatStreamConnections(metrics.stream_connections, config.max_stream_connections),
        capacitySignal: isCapacitySignal(metrics.stream_connections, config.max_stream_connections, isStreaming),
    };
}
