"""Check repository skill metadata, links, catalog entries, and Go test selectors."""

from __future__ import annotations

import re
import shlex
import sys
from collections import Counter
from pathlib import Path
from urllib.parse import unquote, urlsplit


FRONT_MATTER_RE = re.compile(r"\A---\s*\n(.*?)\n---(?:\s*\n|\Z)", re.DOTALL)
MARKDOWN_LINK_RE = re.compile(r"\[[^\]]*\]\(([^)]+)\)")
SKILL_LINK_RE = re.compile(r"\]\(\./([a-z0-9-]+)/SKILL\.md\)")
SKILL_NAME_RE = re.compile(r"^[a-z0-9]+(?:-[a-z0-9]+)*$")
GO_TEST_FUNCTION_RE = re.compile(r"^func\s+(Test\w+|Benchmark\w+)\s*\(", re.MULTILINE)
GO_TEST_LINE_RE = re.compile(r"^\s*(?:\$\s*)?go\s+test(?:\s|$)")
GO_TEST_FLAGS_WITH_VALUES = {
    "-bench",
    "-benchtime",
    "-count",
    "-coverprofile",
    "-cpu",
    "-list",
    "-parallel",
    "-run",
    "-timeout",
}


def _front_matter(path: Path) -> dict[str, str] | None:
    match = FRONT_MATTER_RE.match(path.read_text(encoding="utf-8"))
    if not match:
        return None

    fields: dict[str, str] = {}
    for line in match.group(1).splitlines():
        key, separator, value = line.partition(":")
        if separator:
            fields[key.strip()] = value.strip().strip("\"'")
    return fields


def _check_markdown_links(path: Path, errors: list[str]) -> None:
    for destination in MARKDOWN_LINK_RE.findall(path.read_text(encoding="utf-8")):
        target = destination.strip().split(maxsplit=1)[0].strip("<>")
        parsed = urlsplit(target)
        if parsed.scheme or parsed.netloc or not parsed.path:
            continue

        linked_path = (path.parent / unquote(parsed.path)).resolve()
        if not linked_path.exists():
            errors.append(f"{path}: broken Markdown link: {target}")


def _parse_go_test_command(command: str) -> tuple[list[str], list[tuple[str, str]]]:
    tokens = shlex.split(command)
    packages: list[str] = []
    selectors: list[tuple[str, str]] = []
    index = 2  # Skip `go test`.

    while index < len(tokens):
        token = tokens[index]
        if token in GO_TEST_FLAGS_WITH_VALUES:
            if index + 1 < len(tokens):
                if token in {"-run", "-bench"}:
                    selectors.append((token, tokens[index + 1]))
                index += 2
                continue
        elif any(token.startswith(flag + "=") for flag in GO_TEST_FLAGS_WITH_VALUES):
            flag, value = token.split("=", 1)
            if flag in {"-run", "-bench"}:
                selectors.append((flag, value))
            index += 1
            continue

        if not token.startswith("-"):
            packages.append(token)
        index += 1

    return packages or ["."], selectors


def _package_test_names(root: Path, package: str) -> set[str]:
    if package == "./...":
        package_root = root
        test_files = package_root.rglob("*_test.go")
    elif package.endswith("/..."):
        package_root = root / package[2:-4]
        test_files = package_root.rglob("*_test.go")
    else:
        package_root = root / package.removeprefix("./") if package.startswith("./") else root / package
        test_files = package_root.glob("*_test.go")

    names: set[str] = set()
    for test_file in test_files:
        names.update(GO_TEST_FUNCTION_RE.findall(test_file.read_text(encoding="utf-8")))
    return names


def _check_go_test_commands(path: Path, root: Path, errors: list[str]) -> None:
    in_fence = False
    for line_number, line in enumerate(path.read_text(encoding="utf-8").splitlines(), start=1):
        if line.strip().startswith("```"):
            in_fence = not in_fence
            continue
        if not in_fence or not GO_TEST_LINE_RE.match(line):
            continue

        command = line.strip().removeprefix("$ ")
        try:
            packages, selectors = _parse_go_test_command(command)
        except ValueError as error:
            errors.append(f"{path}:{line_number}: cannot parse Go test command: {error}")
            continue

        for flag, selector in selectors:
            # `'^$'` is a common way to run only the other test category.
            if selector == "^$":
                continue
            try:
                matcher = re.compile(selector)
            except re.error as error:
                errors.append(f"{path}:{line_number}: invalid {flag} selector {selector!r}: {error}")
                continue

            matching_names = {
                name
                for package in packages
                for name in _package_test_names(root, package)
                if matcher.search(name)
            }
            if not matching_names:
                errors.append(
                    f"{path}:{line_number}: {flag} selector {selector!r} matches no test or benchmark"
                )


def validate_skills(root: Path) -> list[str]:
    root = Path(root).resolve()
    skills_root = root / ".github" / "skills"
    errors: list[str] = []
    if not skills_root.is_dir():
        return [f"missing skills directory: {skills_root}"]

    skill_dirs = sorted(path for path in skills_root.iterdir() if path.is_dir())
    skill_files = []
    for skill_dir in skill_dirs:
        skill_file = skill_dir / "SKILL.md"
        if not skill_file.is_file():
            errors.append(f"{skill_dir}: missing SKILL.md")
            continue
        skill_files.append(skill_file)

    names: Counter[str] = Counter()
    for skill_file in skill_files:
        fields = _front_matter(skill_file)
        if fields is None:
            errors.append(f"{skill_file}: missing YAML frontmatter")
            continue
        name = fields.get("name", "")
        if not name:
            errors.append(f"{skill_file}: frontmatter requires name")
        elif not SKILL_NAME_RE.fullmatch(name):
            errors.append(f"{skill_file}: invalid skill name {name!r}")
        elif name != skill_file.parent.name:
            errors.append(
                f"{skill_file}: name must match directory ({name!r} != {skill_file.parent.name!r})"
            )
        names[name] += 1
        if not fields.get("description"):
            errors.append(f"{skill_file}: frontmatter requires description")

        _check_markdown_links(skill_file, errors)
        _check_go_test_commands(skill_file, root, errors)

    for name, count in names.items():
        if count > 1:
            errors.append(f"duplicate skill name {name!r}")

    readme = skills_root / "README.md"
    if not readme.is_file():
        errors.append(f"missing skill catalog: {readme}")
    else:
        _check_markdown_links(readme, errors)
        catalog_counts = Counter(SKILL_LINK_RE.findall(readme.read_text(encoding="utf-8")))
        expected = {skill_file.parent.name for skill_file in skill_files}
        actual = set(catalog_counts)
        for missing in sorted(expected - actual):
            errors.append(f"{readme}: README catalog is missing {missing}")
        for extra in sorted(actual - expected):
            errors.append(f"{readme}: README catalog has unknown skill {extra}")
        for duplicate in sorted(name for name, count in catalog_counts.items() if count != 1):
            errors.append(f"{readme}: README catalog must list {duplicate} exactly once")

    return errors


def main() -> int:
    root = Path(__file__).resolve().parents[1]
    errors = validate_skills(root)
    if errors:
        print("Skill validation failed:", file=sys.stderr)
        for error in errors:
            print(f"- {error}", file=sys.stderr)
        return 1

    count = len(list((root / ".github" / "skills").glob("*/SKILL.md")))
    print(f"Validated {count} repository skills, Markdown links, and documented Go test selectors.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
