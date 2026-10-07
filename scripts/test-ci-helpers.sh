#!/usr/bin/env bash
set -euo pipefail

python3 scripts/check_skills.py
bash scripts/test-docker-tags.sh
bash scripts/test-docker-go-tests.sh
python3 -B -m unittest discover -s scripts -p 'test_*.py'
node --test internal/web/aspect-ratio.test.js internal/web/diagnostics-dialog.test.js
