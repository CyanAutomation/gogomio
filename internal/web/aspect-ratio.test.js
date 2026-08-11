const test = require("node:test");
const assert = require("node:assert/strict");

const { resolutionToAspectRatio } = require("./aspect-ratio.js");

test("uses configured resolution dimensions for the stream aspect ratio", () => {
    assert.equal(resolutionToAspectRatio([800, 600]), "800 / 600");
    assert.equal(resolutionToAspectRatio([800, 320]), "800 / 320");
});

test("rejects resolutions that CSS cannot use", () => {
    assert.equal(resolutionToAspectRatio(undefined), null);
    assert.equal(resolutionToAspectRatio([640, 0]), null);
});
