import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";
import test from "node:test";

import { getStreamMascotPresentation, type StreamState } from "./dashboard-presentation";

const html = readFileSync(new URL("../index.html", import.meta.url), "utf8");
const app = readFileSync(new URL("./app.ts", import.meta.url), "utf8");

function themeBlock(pattern: RegExp): string {
    const match = html.match(pattern);
    if (!match?.[1]) throw new Error("expected a theme token block in the dashboard stylesheet");
    return match[1];
}

function token(block: string, name: string): string {
    const match = block.match(new RegExp(`--${name}:\\s*(#[\\da-fA-F]{6});`));
    if (!match?.[1]) throw new Error(`expected --${name} to be a six digit hex color`);
    return match[1].toLowerCase();
}

function luminance(hex: string): number {
    const channels = hex.slice(1).match(/.{2}/g)!.map(channel => parseInt(channel, 16) / 255);
    const linear = channels.map(value => value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4);
    return 0.2126 * linear[0]! + 0.7152 * linear[1]! + 0.0722 * linear[2]!;
}

function contrastRatio(foreground: string, background: string): number {
    const values = [luminance(foreground), luminance(background)].sort((a, b) => b - a);
    return (values[0]! + 0.05) / (values[1]! + 0.05);
}

const lightTheme = themeBlock(/:root\s*\{([^}]+)\}/);
const darkTheme = themeBlock(/@media\s*\(prefers-color-scheme:\s*dark\)\s*\{\s*:root\s*\{([^}]+)\}/);

test("dashboard tokens restore MIO blues and soft ocean surfaces", () => {
    assert.equal(token(lightTheme, "color-brand-accent"), "#5fa8d3");
    assert.equal(token(lightTheme, "color-brand-secondary"), "#3c7dc4");
    assert.equal(token(lightTheme, "color-brand-ink"), "#0f1a2b");
    assert.equal(token(lightTheme, "color-primary-light"), "#a7d3e8");
    assert.equal(token(lightTheme, "color-bg"), "#f0f7fc");
    assert.equal(token(lightTheme, "color-surface"), "#ffffff");
    assert.equal(token(darkTheme, "color-bg"), "#142b3b");
    assert.equal(token(darkTheme, "color-surface"), "#1d3748");
});

test("primary action text stays readable in light and dark mode", () => {
    const buttonRule = html.match(/\.button--primary\s*\{([^}]+)\}/);
    if (!buttonRule?.[1]) throw new Error("expected a primary button rule");
    assert.match(buttonRule[1], /background:\s*var\(--color-action-background\)/);
    assert.match(buttonRule[1], /color:\s*var\(--color-action-foreground\)/);
    const hoverRule = html.match(/\.button--primary:hover:not\(:disabled\)\s*\{([^}]+)\}/);
    if (!hoverRule?.[1]) throw new Error("expected a primary button hover rule");
    assert.match(hoverRule[1], /background:\s*var\(--color-action-hover-background\)/);

    for (const theme of [lightTheme, darkTheme]) {
        for (const background of ["color-action-background", "color-action-hover-background"]) {
            assert.ok(
                contrastRatio(token(theme, "color-action-foreground"), token(theme, background)) >= 4.5,
                `primary action foreground must have WCAG AA contrast against --${background}`,
            );
        }
        for (const background of ["color-brand-accent", "color-primary-light"]) {
            assert.ok(
                contrastRatio(token(theme, "color-brand-ink"), token(theme, background)) >= 4.5,
                `brand text must have WCAG AA contrast against --${background}`,
            );
        }
    }
});

test("MIO stays visible in the header and beside the live stream status", () => {
    assert.match(html, /<img[^>]*class="mio-avatar mio-avatar--brand mio-float"[^>]*src="[^"]*mio_pose_idle\.png/);
    assert.match(html, /<img[^>]*id="stream-status-mascot"[^>]*class="mio-avatar mio-avatar--status mio-blink"[^>]*aria-hidden="true"/);
    assert.match(html, /MIO's ready when you are\./);
    assert.match(app, /MIO is warming things up/);
});

test("brand header keeps its intrinsic height inside the flex page shell", () => {
    const siteHeader = html.match(/\.site-header\s*\{([^}]+)\}/);
    assert.ok(siteHeader?.[1], "expected a branded site header rule");
    assert.match(siteHeader[1], /flex-shrink:\s*0/);
});

test("section titles stay together beside their status badges", () => {
    const sectionTitle = html.match(/\.section-title\s*\{([^}]+)\}/);
    assert.ok(sectionTitle?.[1], "expected a shared section title rule");
    assert.match(sectionTitle[1], /white-space:\s*nowrap/);
});

test("every state mascot pose is backed by a bundled MIO asset", () => {
    const states: StreamState[] = ["ready", "connecting", "live", "reconnecting", "unavailable"];
    for (const state of states) {
        const pose = getStreamMascotPresentation(state);
        const filename = pose.src.split("?")[0]!.replace("/static/mio/", "");
        assert.ok(existsSync(new URL(`../mio/${filename}`, import.meta.url)), `missing MIO pose ${filename}`);
    }
});
