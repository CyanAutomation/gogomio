// resolutionToAspectRatio converts the API's [width, height] resolution into
// the CSS aspect-ratio syntax used by the stream container.
function resolutionToAspectRatio(resolution) {
    if (!Array.isArray(resolution) || resolution.length < 2) return null;

    const [width, height] = resolution;
    if (!Number.isFinite(width) || !Number.isFinite(height) || width <= 0 || height <= 0) {
        return null;
    }

    return `${width} / ${height}`;
}

if (typeof module !== "undefined" && module.exports) {
    module.exports = { resolutionToAspectRatio };
}
