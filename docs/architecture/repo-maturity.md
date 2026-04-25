🧭 Maturity Scoring Rubric (v2 — Deterministic)

Overview

This rubric measures how close a repository is to being a reliable, runnable, and maintainable product.

It is:

* deterministic
* repeatable
* automation-friendly

It does not measure:

* popularity
* code cleverness
* project size

⸻

Core Principle

Your maturity score answers:

“How close is this repo to being a reliable, runnable, maintainable product?”

⸻

Scoring System Structure

Final Score = Base Score (0–100) + Modifiers (±10 max) – Penalties (0–20 max)

* Base Score → universal, signal-based (0–100)
* Modifiers → repo-type adjustments (±10 max)
* Penalties → deterministic deductions (capped at -20)

⸻

🧱 Base Rubric (Signal-Based)

Each category:

* contains 5 binary signals (0 or 1)
* total signals → category score (0–5)
* weighted into final base score

Category Score Formula

category_score = number_of_signals_met (0–5)
category_contribution = (category_score / 5) × weight

⸻

1. Repository Completeness (Weight: 10)

Signal	Detection Rule
README exists	README* file in repo root
License exists	LICENSE* file present
Description set	GitHub repo description is non-empty
Topics present	≥1 GitHub topic/tag
Version signal exists	≥1 of: Git tag OR GitHub release OR version field in manifest

⸻

2. Setup & Reproducibility (Weight: 15)

Signal	Detection Rule
Setup instructions present	README contains “install”, “setup”, or “getting started” section
Config template exists	.env.example, config.example.*, or similar present
Dependency install documented	Explicit install command present (e.g. npm install, pip install, etc.)
Run/start command documented	Explicit run command present
One-command bootstrap exists	Script, Makefile, Docker Compose, or package script enabling startup

⸻

3. Runtime Operability (Weight: 15)

Signal	Detection Rule
Project starts successfully	Defined entrypoint exists (CLI, server, or main script)
Logs or output visible	Console output, logging framework, or stdout activity
Failure handling exists	Non-zero exit codes OR try/catch OR error handling patterns
Runtime status exposed	Health endpoint OR CLI help (--help) OR status output
Safe/demo/mock mode exists	Explicit mock mode, sample data mode, or demo configuration

⸻

4. Testing & Verification (Weight: 15)

Signal	Detection Rule
Tests directory/files exist	/tests, __tests__, or test naming patterns
Tests runnable locally	Test command present (e.g. npm test, pytest)
Tests executed in CI	CI workflow includes test step
Multiple test types exist	≥2 of: unit, integration, e2e, smoke
Build/test passes	Latest CI test run = success

⸻

5. CI/CD & Delivery (Weight: 10)

Signal	Detection Rule
CI workflow exists	.github/workflows/* present
Build step exists	CI includes build/install step
Test step exists	CI includes test execution
Artifact or package produced	CI produces build artifact OR package
Release mechanism exists	GitHub release OR publish workflow

⸻

6. Codebase Maintainability (Weight: 10)

Signal	Detection Rule
Standard directory structure	Uses src, app, lib, or equivalent
Config separated from code	Config files not embedded in main logic
Linting config exists	.eslintrc, .flake8, .prettierrc, etc.
Type checking present (if applicable)	TypeScript, mypy, or equivalent
No oversized files	No source files >1000 lines

⸻

7. Security & Dependency Hygiene (Weight: 10)

Signal	Detection Rule
Dependency manifest exists	package.json, requirements.txt, etc.
Lockfile exists	package-lock.json, poetry.lock, etc.
Dependency automation configured	Dependabot or equivalent config
Versions pinned	Dependencies not all latest / wildcards
CI permissions restricted	GitHub Actions permissions explicitly defined

⸻

8. Documentation Depth (Weight: 10)

Signal	Detection Rule
Usage examples present	README contains example usage
Config documented	Config variables explained
Architecture documented	Architecture section or diagram present
Troubleshooting guide present	Section for errors/debugging
Development/deployment guide	Instructions for dev or deploy environments

⸻

9. Project Governance Signals (Weight: 5)

Signal	Detection Rule
Issue template exists	.github/ISSUE_TEMPLATE
PR template exists	.github/PULL_REQUEST_TEMPLATE
Labels configured	Repository has ≥3 labels
Ownership defined	CODEOWNERS or equivalent
Activity signal present	Commit OR issue activity within last 6 months OR marked “stable/complete”

⸻

⚙️ Modifiers (Max ±10)

Modifiers are additive micro-signals, each worth +1, capped per category.

⸻

App / Product (Max +4)

Signal	Points
UI or demo interface exists	+1
Persistent storage strategy exists	+1
Config system exists	+1
Mock/demo mode exists	+1

⸻

Library / Tooling (Max +4)

Signal	Points
Versioned API	+1
Usage examples provided	+1
Published package or distribution	+1
CLI or documented interface	+1

⸻

Hardware-Integrated (Max +3)

Signal	Points
Hardware assumptions documented	+1
Device mapping documented	+1
Fallback/mock mode exists	+1

⸻

Experimental / Prototype

Signal	Points
Marked experimental AND lacks setup	-3
Demo mode exists	+2

⸻

🚨 Penalties (Max -20)

Penalties are applied after base + modifiers.

⸻

Critical (-10 each)

Condition	Detection Rule
Cannot run from instructions	No valid run command OR bootstrap fails
Secrets detected	API keys, tokens, or credentials in repo

⸻

Medium (-5 each)

Condition	Detection Rule
Default branch CI fails	Latest CI run = failed
No install/run path	No install OR run command documented

⸻

Minor (-2 to -3)

Condition	Detection Rule
Broken dependencies	Install step fails
No license (if reusable)	No LICENSE file present
Stale repo	No activity >12 months AND not marked stable
Generated artifacts committed	Large build outputs committed outside allowed dirs

⸻

📊 Output Interpretation

Score	Classification	Meaning
0–24	Idea / Abandoned	Concept or inactive
25–44	Prototype	Early stage
45–64	Working Project	Functional but gaps exist
65–79	Maintainable Product	Reliable and usable
80–100	Mature Product	Production-ready

⸻

🧠 Output Model (Recommended)

When scoring a repo, output:

{
  "repo": "example-repo",
  "score": 72,
  "base_score": 68,
  "modifiers": 4,
  "penalties": 0,
  "category_scores": {
    "setup_reproducibility": 4,
    "testing": 3
  },
  "weakest_categories": [
    "CI/CD & Delivery",
    "Testing & Verification"
  ],
  "penalties_triggered": [],
  "next_best_actions": [
    {
      "action": "Add CI test workflow",
      "estimated_score_gain": 6
    },
    {
      "action": "Add troubleshooting documentation",
      "estimated_score_gain": 2
    }
  ]
}
