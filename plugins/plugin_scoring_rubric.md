Recommended scoring model

Use a 100-point weighted score with one pre-check.

Pre-check: should this even be a plugin?

Before scoring, ask these four questions:

* Is it optional?
* Can it be turned off without harming the base app?
* Can a hobbyist understand what it does?
* Can it live mostly inside its own folder and manifest?

If the answer is mostly no, it probably belongs in core, not as a plugin.

⸻

Motion In Ocean plugin rubric

Score each category from 0 to 5.

* 0 = very poor fit
* 1 = weak
* 3 = acceptable
* 5 = excellent

Then multiply by the weighting.

1. Practical user value — weight 25

Does it solve a real problem for your hobbyist / maker / workshop-monitoring audience?

What to look for:

* useful in real setups
* improves day-one or week-one experience
* not just “technically interesting”

Scoring guide:

* 0 = almost no clear use
* 1 = novelty only
* 3 = useful for a subset of users
* 5 = clearly useful for many users

2. Local-first fit — weight 20

Does it respect the local-network, Pi-in-a-container nature of the product?

What to look for:

* works well on LAN
* does not depend heavily on cloud services
* internet dependency, if any, is limited and obvious
* still useful even when offline

Scoring guide:

* 0 = mostly cloud-dependent
* 1 = poor offline behaviour
* 3 = partly local, partly external
* 5 = strongly local-first

3. Plugin separability — weight 15

How cleanly can this exist as a plugin instead of core?

What to look for:

* isolated folder
* clear manifest metadata
* limited coupling to stream lifecycle, device control, and camera core
* can add a panel without reshaping the whole UI

Scoring guide:

* 0 = deeply entangled with core
* 1 = awkward to isolate
* 3 = separable with some shared hooks
* 5 = clearly modular

4. Reliability on Pi 4 + single container — weight 15

Will it work dependably on the minimum supported device?

What to look for:

* reasonable CPU and memory use
* no brittle system assumptions
* no dependence on sidecars or extra services
* realistic within one container

Scoring guide:

* 0 = unrealistic or unstable
* 1 = very fragile
* 3 = workable with care
* 5 = robust and realistic

5. First-run clarity — weight 10

Will a user understand it and get value from it without much setup pain?

What to look for:

* clear name and purpose
* obvious enable/disable behaviour
* simple config
* understandable UI panel if present

Scoring guide:

* 0 = confusing
* 1 = heavy setup / poor clarity
* 3 = understandable with docs
* 5 = obvious and friendly

6. Maintenance burden — weight 10

How likely is it to stay healthy over time?

What to look for:

* few moving parts
* limited third-party dependency risk
* low chance of frequent breakage from Pi / camera stack changes
* manageable testing burden

Scoring guide:

* 0 = high support cost
* 1 = likely to break often
* 3 = moderate upkeep
* 5 = low-maintenance

7. Safety / trustworthiness — weight 5

Does it behave in a way users will see as safe and predictable?

What to look for:

* clear network behaviour
* no hidden uploads
* no unnecessary permissions
* no privacy surprises

Scoring guide:

* 0 = high trust risk
* 1 = concerning behaviour
* 3 = acceptable with clear warnings
* 5 = low-risk and transparent

⸻

Why these weights fit your product

Your priorities were:

* easy first-run experience
* practical usefulness
* local-first reliability

So the rubric deliberately gives the most weight to:

* Practical user value
* Local-first fit
* Plugin separability
* Reliability on Pi 4

That means a plugin can’t score highly just because it is technically neat.

⸻

Decision bands

After weighting, score out of 100.

* 85–100 = excellent first-party plugin candidate
* 70–84 = good candidate
* 55–69 = borderline, only worth it if strategically useful
* 40–54 = weak candidate
* Below 40 = probably not worth building as a first-party plugin

⸻

Add one more flag: internet dependency

Since you said internet is allowed but must be clear, I’d add a simple label outside the score:

* Local only
* Local-first with optional internet
* Internet-assisted
* Internet-dependent

A plugin can still score well if it uses internet, but it should lose points in Local-first fit if internet is central to its value.

⸻

Suggested evaluation template

# Plugin Review
## Plugin
Name:
Type:
Summary:
## Classification
- Optional? Yes/No
- Good plugin shape? Yes/No
- Adds UI panel? Yes/No
- Internet dependency: Local only / Local-first with optional internet / Internet-assisted / Internet-dependent
## Scores
| Category | Score (0-5) | Weight | Weighted |
|---|---:|---:|---:|
| Practical user value |  | 25 |  |
| Local-first fit |  | 20 |  |
| Plugin separability |  | 15 |  |
| Reliability on Pi 4 + single container |  | 15 |  |
| First-run clarity |  | 10 |  |
| Maintenance burden |  | 10 |  |
| Safety / trustworthiness |  | 5 |  |
## Total
Total score: /100
## Decision
- Build now
- Build later
- Keep in backlog
- Reject
- Move to core instead
## Notes
Strengths:
Risks:
Why this should be a plugin instead of core:

⸻

Example scoring

Here are a few example plugin types for your app.

1. Motion-triggered snapshot saver

Saves snapshots locally when motion is detected.

* Practical user value: 5
* Local-first fit: 5
* Plugin separability: 4
* Reliability on Pi 4: 4
* First-run clarity: 5
* Maintenance burden: 4
* Safety / trustworthiness: 5

This would score very highly. It fits your audience well and feels like a classic first-party plugin.

2. Workshop timelapse exporter

Captures frames and creates a timelapse for 3D prints or bench work.

* Practical user value: 5
* Local-first fit: 5
* Plugin separability: 4
* Reliability on Pi 4: 3
* First-run clarity: 4
* Maintenance burden: 3
* Safety / trustworthiness: 5

Still strong. Slightly lower on reliability and maintenance because timelapse generation can be a bit resource-heavy.

3. Telegram alert plugin

Sends a message or image when motion is detected.

* Practical user value: 4
* Local-first fit: 2
* Plugin separability: 4
* Reliability on Pi 4: 4
* First-run clarity: 3
* Maintenance burden: 3
* Safety / trustworthiness: 2

Potentially useful, but not quite as aligned with your local-first values. Still a candidate, but not as strong as local storage or local automation plugins.

4. AI object recognition plugin

Runs heavier classification or detection locally.

* Practical user value: 3
* Local-first fit: 4
* Plugin separability: 3
* Reliability on Pi 4: 1
* First-run clarity: 2
* Maintenance burden: 1
* Safety / trustworthiness: 4

Interesting, but probably weak as an early first-party plugin for this product unless you keep the scope very small.

⸻

