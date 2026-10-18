// resolutionToAspectRatio converts the API's [width, height] resolution into
// the CSS aspect-ratio syntax used by the stream container.
export function resolutionToAspectRatio(resolution: unknown): string | null {
    if (!Array.isArray(resolution) || resolution.length < 2) return null;

    const [width, height] = resolution;
    if (typeof width !== "number" || typeof height !== "number"
        || !Number.isFinite(width) || !Number.isFinite(height)
        || width <= 0 || height <= 0) {
        return null;
    }

    return `${width} / ${height}`;
}
