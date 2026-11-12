# Dashboard UI evidence

These contact sheets show the dashboard before and after the UI polish. Each sheet contains the Ready, Connecting, Live, Reconnecting, Unavailable, and Diagnostics states in that order.

The before captures use `origin/main` at `824e2b1`. The after captures use the implementation in this branch. Both sets were rendered in Chromium against the same deterministic mock camera feed at 360, 768, 1024, and 1280 px viewport widths, in light and dark themes. The Live tile uses the mock JPEG stream.

| Viewport | Before | After |
| --- | --- | --- |
| 360 px, light | ![Before at 360 px in light mode](before/360-light.webp) | ![After at 360 px in light mode](after/360-light.webp) |
| 360 px, dark | ![Before at 360 px in dark mode](before/360-dark.webp) | ![After at 360 px in dark mode](after/360-dark.webp) |
| 768 px, light | ![Before at 768 px in light mode](before/768-light.webp) | ![After at 768 px in light mode](after/768-light.webp) |
| 768 px, dark | ![Before at 768 px in dark mode](before/768-dark.webp) | ![After at 768 px in dark mode](after/768-dark.webp) |
| 1024 px, light | ![Before at 1024 px in light mode](before/1024-light.webp) | ![After at 1024 px in light mode](after/1024-light.webp) |
| 1024 px, dark | ![Before at 1024 px in dark mode](before/1024-dark.webp) | ![After at 1024 px in dark mode](after/1024-dark.webp) |
| 1280 px, light | ![Before at 1280 px in light mode](before/1280-light.webp) | ![After at 1280 px in light mode](after/1280-light.webp) |
| 1280 px, dark | ![Before at 1280 px in dark mode](before/1280-dark.webp) | ![After at 1280 px in dark mode](after/1280-dark.webp) |

Additional after captures show the new capacity and idle-sleep states:

- [At capacity, 1024 px light](after/1024-light-capacity.webp)
- [Idle sleep, 1024 px light](after/1024-light-sleeping.webp)
