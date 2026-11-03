#!/usr/bin/env python3

import argparse
import json
import os
import re
from collections import defaultdict
from datetime import datetime, timedelta, timezone
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]

SUCCESS_DIR = ROOT / "test-results"
ERROR_DIR = ROOT / "error"

RETENTION_DAYS = 21
RETENTION_COUNT = 20

# GitHub shows at most 10 annotations per level and step.
MAX_ANNOTATIONS = 10
MAX_ANNOTATION_CHARS = 1500

# Output limits for the Markdown summary and the step log.
MAX_SUMMARY_LINES = 60
MAX_SUMMARY_CHARS = 4000
MAX_CONSOLE_LINES = 40

# "file.go:12: message" or "file.go:12:3: message"
LOCATION_RE = re.compile(
    r"^\s*((?:[\w.\-]+[\\/])*[\w.\-]+\.go):(\d+)(?::(\d+))?:\s*(.*)$"
)

# Section markers written by the workflow around each formatter:
#   ### BEGIN prettier
#   ...output...
#   ### END prettier exit=0
SECTION_RE = re.compile(r"^### (BEGIN|END) (.+?)(?: exit=(-?\d+))?\s*$")

# prettier --write prints "path 12ms" (changed) or "path 3ms (unchanged)".
PRETTIER_FILE_RE = re.compile(
    r"^(?P<path>.+?)\s+\d+(?:\.\d+)?m?s(?:\s+\((?P<note>unchanged|cached)\))?$"
)
# prettier prints "[error] path: SyntaxError: message (line:col)".
PRETTIER_ERROR_RE = re.compile(
    r"^\[error\]\s+(?P<path>[^:]+?):\s+(?P<message>.*?)"
    r"(?:\s+\((?P<line>\d+):(?P<col>\d+)\))?$"
)
NOISE_RE = re.compile(r"^=== (RUN|PAUSE|CONT|NAME)\b")

STATUS_ICONS = {
    "pass": "✅",
    "fail": "❌",
    "skip": "➖",
    "incomplete": "⚠️",
}


# ---------------------------------------------------------------------------
# Reading input
# ---------------------------------------------------------------------------

def read_text(path):
    """Read a text file. A missing file is treated as empty."""

    if not path:
        return ""

    file_path = Path(path)

    if not file_path.exists():
        return ""

    return file_path.read_text(encoding="utf-8", errors="replace")


def parse_events(content):
    """Parse `go test -json` output (JSON Lines)."""

    events = []

    for line in content.splitlines():
        if not line.strip():
            continue

        try:
            events.append(json.loads(line))
        except json.JSONDecodeError:
            events.append({
                "action": "invalid_json",
                "output": line
            })

    return events


def read_module_path():
    """Return the module path written in go.mod (None if unavailable)."""

    try:
        lines = (ROOT / "go.mod").read_text(encoding="utf-8").splitlines()
    except OSError:
        return None

    for line in lines:
        line = line.strip()

        if line.startswith("module "):
            return line.split(None, 1)[1].strip().strip('"')

    return None


# ---------------------------------------------------------------------------
# Path helpers
# ---------------------------------------------------------------------------

def package_dir(package, module):
    """Map an import path to its directory inside the repository."""

    if not module or not package:
        return None

    if package == module:
        return ROOT

    if package.startswith(module + "/"):
        return ROOT / package[len(module) + 1:]

    return None


def short_package(package, module):
    """Shorten an import path: example.com/app/drive -> ./drive"""

    if module and package == module:
        return "."

    if module and package.startswith(module + "/"):
        return "./" + package[len(module) + 1:]

    return package


def resolve_location(raw_path, pkg_dir):
    """
    Return a repository-relative path for a path printed by Go.

    Test logs print paths relative to the package directory
    (files_test.go), build errors print paths relative to the module root
    (drive/files.go). Both are tried. Paths that do not exist are ignored.
    """

    raw = raw_path.replace("\\", "/")

    if raw.startswith("./"):
        raw = raw[2:]

    for base in (pkg_dir, ROOT):
        if base is None:
            continue

        candidate = (base / raw).resolve()

        if not candidate.is_file():
            continue

        try:
            return candidate.relative_to(ROOT).as_posix()
        except ValueError:
            continue

    return None


def find_locations(text, pkg_dir):
    """Find 'file.go:line: message' entries in command output."""

    found = []

    for line in text.splitlines():
        match = LOCATION_RE.match(line)

        if not match:
            continue

        path = resolve_location(match.group(1), pkg_dir)

        if not path:
            continue

        found.append({
            "file": path,
            "line": int(match.group(2)),
            "col": int(match.group(3)) if match.group(3) else None,
            "message": match.group(4).strip(),
        })

    return found


# ---------------------------------------------------------------------------
# Analysis
# ---------------------------------------------------------------------------

def clean_output(lines):
    """Join output lines and remove '=== RUN' style noise."""

    text = "".join(lines)

    kept = [
        line.rstrip()
        for line in text.splitlines()
        if not NOISE_RE.match(line)
    ]

    while kept and not kept[0].strip():
        kept.pop(0)

    while kept and not kept[-1].strip():
        kept.pop()

    return "\n".join(kept)


def analyze_tests(events):
    """Turn `go test -json` events into a compact summary."""

    packages = {}
    tests = {}
    package_output = defaultdict(list)
    unparsed = []

    for event in events:
        if event.get("action") == "invalid_json":
            unparsed.append(event.get("output", ""))
            continue

        action = event.get("Action")
        package = (
            event.get("Package") or event.get("ImportPath") or ""
        ).split(" ")[0]
        test = event.get("Test")
        output = event.get("Output", "")

        if not package:
            continue

        info = packages.setdefault(
            package,
            {"status": None, "elapsed": 0.0, "build_failed": False}
        )

        # Build errors (Go 1.24+ reports them as JSON events).
        if action == "build-output":
            package_output[package].append(output)
            continue

        if action == "build-fail":
            info["build_failed"] = True
            continue

        if test:
            item = tests.setdefault(
                (package, test),
                {
                    "package": package,
                    "test": test,
                    "status": None,
                    "elapsed": 0.0,
                    "output": [],
                }
            )

            if action == "output":
                item["output"].append(output)
            elif action in ("pass", "fail", "skip"):
                item["status"] = action
                item["elapsed"] = event.get("Elapsed", 0.0)

            continue

        if action == "output":
            package_output[package].append(output)
        elif action in ("pass", "fail", "skip"):
            info["status"] = action
            info["elapsed"] = event.get("Elapsed", 0.0)

    def is_failed(item):
        if item["status"] == "fail":
            return True

        # Never finished (timeout, crash) inside a failed package.
        return (
            item["status"] is None
            and packages[item["package"]]["status"] == "fail"
        )

    failed_keys = {
        key for key, item in tests.items() if is_failed(item)
    }

    def has_failed_child(key):
        package, name = key
        prefix = name + "/"

        return any(
            other_package == package and other_name.startswith(prefix)
            for other_package, other_name in failed_keys
        )

    # Report only the innermost failing tests (not their parents).
    leaf_keys = sorted(
        key for key in failed_keys if not has_failed_child(key)
    )

    failed_tests = [
        {
            "package": tests[key]["package"],
            "test": tests[key]["test"],
            "elapsed_sec": round(tests[key]["elapsed"], 3),
            "output": clean_output(tests[key]["output"]),
        }
        for key in leaf_keys
    ]

    failed_in_package = defaultdict(int)
    for key in leaf_keys:
        failed_in_package[key[0]] += 1

    package_results = []
    failed_packages = []

    for package in sorted(packages):
        info = packages[package]
        status = info["status"] or (
            "fail" if info["build_failed"] else "incomplete"
        )

        passed = sum(
            1 for item in tests.values()
            if item["package"] == package and item["status"] == "pass"
        )
        skipped = sum(
            1 for item in tests.values()
            if item["package"] == package and item["status"] == "skip"
        )

        package_results.append({
            "package": package,
            "status": status,
            "elapsed_sec": round(info["elapsed"], 3),
            "tests": {
                "passed": passed,
                "failed": failed_in_package[package],
                "skipped": skipped,
            },
        })

        # Package failed without any failing test:
        # build error, panic in TestMain, timeout, ...
        if status in ("fail", "incomplete") and not failed_in_package[package]:
            failed_packages.append({
                "package": package,
                "output": clean_output(package_output[package]),
            })

    passed_total = sum(1 for t in tests.values() if t["status"] == "pass")
    skipped_total = sum(1 for t in tests.values() if t["status"] == "skip")

    return {
        "packages": {
            "total": len(package_results),
            "passed": sum(1 for p in package_results if p["status"] == "pass"),
            "failed": sum(
                1 for p in package_results
                if p["status"] in ("fail", "incomplete")
            ),
            "no_tests": sum(
                1 for p in package_results if p["status"] == "skip"
            ),
        },
        "tests": {
            "total": passed_total + skipped_total + len(failed_tests),
            "passed": passed_total,
            "failed": len(failed_tests),
            "skipped": skipped_total,
        },
        "duration_sec": round(
            sum(p["elapsed_sec"] for p in package_results), 3
        ),
        "package_results": package_results,
        "failed_tests": failed_tests,
        "failed_packages": failed_packages,
        "unparsed_output": "".join(unparsed).strip(),
    }


def normalize_path(path):
    return path.strip().replace("\\", "/")


def parse_sections(text):
    """Split the formatting log into one section per tool."""

    sections = []
    current = None

    for line in text.splitlines():
        match = SECTION_RE.match(line)

        if match:
            kind, name, code = match.groups()

            if kind == "BEGIN":
                current = {"tool": name, "lines": [], "exit_code": None}
                sections.append(current)
            elif current is not None and current["tool"] == name:
                current["exit_code"] = int(code) if code is not None else None
                current = None

            continue

        if current is not None:
            current["lines"].append(line)

    return sections


def parse_tool_output(tool, lines):
    """Return (files reported as changed, located errors) for one tool."""

    changed = []
    errors = []

    for raw in lines:
        line = raw.strip()

        if not line:
            continue

        if tool == "prettier":
            match = PRETTIER_ERROR_RE.match(line)

            if match:
                path = resolve_location(match.group("path"), None)

                if path:
                    errors.append({
                        "file": path,
                        "line": (
                            int(match.group("line"))
                            if match.group("line") else None
                        ),
                        "col": (
                            int(match.group("col"))
                            if match.group("col") else None
                        ),
                        "message": match.group("message"),
                    })

                continue

            match = PRETTIER_FILE_RE.match(line)

            if match and not match.group("note"):
                changed.append(normalize_path(match.group("path")))

            continue

        # go fmt: prints the names of the files it rewrote.
        if line.endswith(".go") and " " not in line and ":" not in line:
            changed.append(normalize_path(line))
            continue

        for location in find_locations(line, None):
            errors.append(location)

    return changed, errors


def analyze_format(text, exit_code=0):
    """
    Summarize the formatting log.

    The workflow writes one section per tool and a final "changes" section
    containing `git diff --name-only`. That list is the most reliable
    "files changed" source, because tools may report files they rewrote
    without a real change (for example only line endings).
    """

    sections = parse_sections(text)

    if not sections:
        # Plain `go fmt` output without markers (older log format).
        sections = [{
            "tool": "go fmt",
            "lines": text.splitlines(),
            "exit_code": None,
        }]

    tools = []
    git_changes = None

    for section in sections:
        if section["tool"] == "changes":
            git_changes = [
                normalize_path(line)
                for line in section["lines"]
                if line.strip()
            ]
            continue

        changed, errors = parse_tool_output(
            section["tool"], section["lines"]
        )

        code = section["exit_code"]

        if code is None:
            code = exit_code

        output = "\n".join(section["lines"]).strip()

        tools.append({
            "tool": section["tool"],
            "exit_code": code,
            "status": "success" if code == 0 else "failure",
            "reported_files": changed,
            "errors": errors,
            # Output is kept only for failures (the raw text is in "details").
            "output": output if code != 0 else "",
        })

    if git_changes is not None:
        reformatted = git_changes
        source = "git diff"
    else:
        reformatted = []

        for tool in tools:
            for path in tool["reported_files"]:
                if path not in reformatted:
                    reformatted.append(path)

        source = "tool output"

    return {
        "tools": tools,
        "reformatted_files": reformatted,
        "files_source": source,
    }


# ---------------------------------------------------------------------------
# GitHub annotations
# ---------------------------------------------------------------------------

def _escape_data(value):
    return (
        value.replace("%", "%25")
        .replace("\r", "%0D")
        .replace("\n", "%0A")
    )


def _escape_property(value):
    return _escape_data(value).replace(":", "%3A").replace(",", "%2C")


class Annotator:
    """Emit workflow commands while respecting GitHub's per-step limit."""

    def __init__(self, limit=MAX_ANNOTATIONS):
        self.limit = limit
        self.counts = defaultdict(int)
        self.omitted = 0

    def emit(
        self,
        level,
        message,
        title=None,
        file=None,
        line=None,
        col=None
    ):
        if self.counts[level] >= self.limit:
            self.omitted += 1
            return

        self.counts[level] += 1

        properties = []

        if file:
            properties.append(f"file={_escape_property(file)}")
        if line:
            properties.append(f"line={line}")
        if col:
            properties.append(f"col={col}")
        if title:
            properties.append(f"title={_escape_property(title)}")

        prefix = f"::{level} {','.join(properties)}" if properties else f"::{level}"

        print(f"{prefix}::{_escape_data(message[:MAX_ANNOTATION_CHARS])}")

    def finish(self):
        if self.omitted:
            print(
                f"Note: {self.omitted} more annotation(s) were omitted "
                "because of GitHub's limit. See the job summary for all "
                "failures."
            )


# ---------------------------------------------------------------------------
# Text helpers for reports
# ---------------------------------------------------------------------------

def truncate(text, max_lines, max_chars=None):
    lines = text.splitlines()
    omitted = 0

    if len(lines) > max_lines:
        omitted = len(lines) - max_lines
        lines = lines[:max_lines]

    result = "\n".join(lines)

    if max_chars is not None and len(result) > max_chars:
        return result[:max_chars] + "\n... (truncated)"

    if omitted:
        result += f"\n... ({omitted} more line(s) not shown)"

    return result


def code_block(text):
    text = text.replace("````", "'''")
    return f"````text\n{text}\n````"


def indent(text, prefix="    "):
    return "\n".join(prefix + line for line in text.splitlines())


def write_step_summary(markdown):
    path = os.environ.get("GITHUB_STEP_SUMMARY")

    if not path:
        return

    with open(path, "a", encoding="utf-8") as file:
        file.write(markdown + "\n")


def file_link(result, location):
    """Markdown link to a source line at the tested commit."""

    repository = result["github"]["repository"]
    sha = result["commit"]["sha"]
    file = location["file"]
    line = location["line"]

    url = f"https://github.com/{repository}/blob/{sha}/{file}#L{line}"

    return f"[`{file}:{line}`]({url})"


def header_markdown(result):
    title = {
        "formatting": "Formatting (`prettier`, `go fmt`)",
        "testing": "Testing (`go test`)",
    }[result["type"]]

    icon = "✅" if result["status"] == "success" else "❌"
    sha = result["commit"]["sha"]
    github = result["github"]

    return (
        f"## {icon} {title}\n\n"
        f"- Commit: [`{sha[:7]}`]({result['commit']['url']})\n"
        f"- Run: [#{github['run_id']} (attempt {github['run_attempt']})]"
        f"({github['run_url']})\n"
        f"- Exit code: `{result['exit_code']}`\n"
    )


# ---------------------------------------------------------------------------
# Reports: formatting
# ---------------------------------------------------------------------------

def report_formatting(result, annotator):
    summary = result["summary"]
    tools = summary["tools"]
    files = summary["reformatted_files"]
    failed_tools = [tool for tool in tools if tool["exit_code"] != 0]

    # ---- Step log --------------------------------------------------------

    for tool in tools:
        icon = "✅" if tool["exit_code"] == 0 else "❌"
        print(f"{icon} {tool['tool']}: exit code {tool['exit_code']}")

    if files:
        print(f"✏️ {len(files)} file(s) were changed by formatting:")
        for path in files:
            print(f"  - {path}")
    else:
        print("✅ No files needed formatting.")

    for tool in failed_tools:
        print(f"\n--- ❌ {tool['tool']} failed (exit code {tool['exit_code']})")
        print(indent(truncate(tool["output"], MAX_CONSOLE_LINES)))

    # ---- Annotations -----------------------------------------------------

    for tool in failed_tools:
        title = f"{tool['tool']} error"

        if tool["errors"]:
            for error in tool["errors"]:
                annotator.emit(
                    "error",
                    error["message"],
                    title=title,
                    file=error["file"],
                    line=error["line"],
                    col=error["col"],
                )
        else:
            annotator.emit(
                "error",
                f"{tool['tool']} failed with exit code "
                f"{tool['exit_code']}:\n" + truncate(tool["output"], 10),
                title=title,
            )

    for path in files:
        annotator.emit(
            "notice",
            "Changed by automatic formatting. The change is committed "
            "by the bot.",
            title="Auto-formatted",
            file=path,
        )

    # ---- Job summary -----------------------------------------------------

    markdown = header_markdown(result)

    markdown += "\n| Tool | Result | Exit code |\n|---|:---:|---:|\n"

    for tool in tools:
        icon = "✅" if tool["exit_code"] == 0 else "❌"
        markdown += f"| `{tool['tool']}` | {icon} | {tool['exit_code']} |\n"

    if files:
        markdown += f"\n### Files changed by formatting ({len(files)})\n\n"
        markdown += "\n".join(f"- `{path}`" for path in files)
        markdown += (
            "\n\n> These changes are committed automatically by the bot.\n"
        )
    else:
        markdown += "\nNo files needed formatting.\n"

    for tool in failed_tools:
        markdown += f"\n### ❌ {tool['tool']} output\n\n" + code_block(
            truncate(tool["output"], MAX_SUMMARY_LINES, MAX_SUMMARY_CHARS)
        ) + "\n"

    write_step_summary(markdown)


# ---------------------------------------------------------------------------
# Reports: testing
# ---------------------------------------------------------------------------

def report_testing(result, annotator, module, stderr_text):
    summary = result["summary"]
    exit_code = result["exit_code"]

    packages = summary["packages"]
    tests = summary["tests"]
    failed_tests = summary["failed_tests"]
    failed_packages = summary["failed_packages"]

    # ---- Step log --------------------------------------------------------

    if exit_code == 0:
        print(
            f"✅ All tests passed: {tests['passed']} passed, "
            f"{tests['skipped']} skipped in {packages['total']} package(s) "
            f"({summary['duration_sec']}s)"
        )
    else:
        print(
            f"❌ Tests failed: {tests['failed']} failed test(s), "
            f"{packages['failed']} failed package(s) "
            f"(exit code {exit_code})"
        )

    print("::group::Package results")
    for item in summary["package_results"]:
        counts = item["tests"]
        print(
            f"{STATUS_ICONS.get(item['status'], '?')} "
            f"{short_package(item['package'], module):<30} "
            f"pass={counts['passed']} fail={counts['failed']} "
            f"skip={counts['skipped']} ({item['elapsed_sec']}s)"
        )
    print("::endgroup::")

    for failure in failed_tests:
        title = f"{short_package(failure['package'], module)} › {failure['test']}"
        print(f"\n--- ❌ FAIL {title} ({failure['elapsed_sec']}s)")
        print(indent(truncate(failure["output"], MAX_CONSOLE_LINES)))

    for failure in failed_packages:
        title = short_package(failure["package"], module)
        print(f"\n--- ❌ PACKAGE FAILED {title} (build error, panic or timeout)")
        print(indent(truncate(failure["output"], MAX_CONSOLE_LINES)))

    if stderr_text.strip():
        print("::group::go test stderr")
        print(stderr_text.rstrip())
        print("::endgroup::")

    # ---- Annotations -----------------------------------------------------

    for failure in failed_tests:
        pkg_dir = package_dir(failure["package"], module)
        locations = find_locations(failure["output"], pkg_dir)
        location = locations[0] if locations else None

        message_lines = [
            line for line in failure["output"].splitlines()
            if not line.lstrip().startswith("--- FAIL")
        ]

        annotator.emit(
            "error",
            "\n".join(message_lines).strip() or "Test failed",
            title=(
                f"{short_package(failure['package'], module)}: "
                f"{failure['test']} failed"
            ),
            file=location["file"] if location else None,
            line=location["line"] if location else None,
            col=location["col"] if location else None,
        )

    for failure in failed_packages:
        pkg_dir = package_dir(failure["package"], module)
        locations = find_locations(failure["output"], pkg_dir)
        title = f"{short_package(failure['package'], module)}: build or setup error"

        if locations:
            for location in locations:
                annotator.emit(
                    "error",
                    location["message"],
                    title=title,
                    file=location["file"],
                    line=location["line"],
                    col=location["col"],
                )
        else:
            annotator.emit(
                "error",
                truncate(failure["output"], 10) or "Package failed",
                title=title,
            )

    if exit_code != 0 and not failed_tests and not failed_packages:
        annotator.emit(
            "error",
            f"go test exited with code {exit_code}, but no failing test "
            "was found in its output. Check the step log and stderr.",
            title="go test",
        )

    # ---- Job summary -----------------------------------------------------

    markdown = header_markdown(result)

    markdown += (
        f"\n**Packages:** {packages['passed']} passed · "
        f"{packages['failed']} failed · {packages['no_tests']} without tests"
        f"  \n**Tests:** {tests['passed']} passed · {tests['failed']} failed "
        f"· {tests['skipped']} skipped  \n"
        f"**Time:** {summary['duration_sec']}s\n"
    )

    ordered = sorted(
        summary["package_results"],
        key=lambda item: (item["status"] == "pass", item["package"])
    )

    markdown += (
        "\n| Package | Result | Passed | Failed | Skipped | Time |\n"
        "|---|:---:|---:|---:|---:|---:|\n"
    )

    for item in ordered:
        counts = item["tests"]
        markdown += (
            f"| `{short_package(item['package'], module)}` "
            f"| {STATUS_ICONS.get(item['status'], '?')} "
            f"| {counts['passed']} | {counts['failed']} "
            f"| {counts['skipped']} | {item['elapsed_sec']}s |\n"
        )

    if failed_tests or failed_packages or exit_code != 0:
        markdown += "\n### Failures\n"

    for failure in failed_tests:
        pkg_dir = package_dir(failure["package"], module)
        locations = find_locations(failure["output"], pkg_dir)
        where = f" — {file_link(result, locations[0])}" if locations else ""

        markdown += (
            f"\n#### ❌ `{short_package(failure['package'], module)}` › "
            f"`{failure['test']}`{where}\n\n"
            + code_block(
                truncate(
                    failure["output"],
                    MAX_SUMMARY_LINES,
                    MAX_SUMMARY_CHARS
                )
            )
            + "\n"
        )

    for failure in failed_packages:
        pkg_dir = package_dir(failure["package"], module)
        locations = find_locations(failure["output"], pkg_dir)
        where = f" — {file_link(result, locations[0])}" if locations else ""

        markdown += (
            f"\n#### ❌ `{short_package(failure['package'], module)}` "
            f"(build error, panic or timeout){where}\n\n"
            + code_block(
                truncate(
                    failure["output"],
                    MAX_SUMMARY_LINES,
                    MAX_SUMMARY_CHARS
                )
            )
            + "\n"
        )

    if exit_code != 0 and stderr_text.strip():
        markdown += "\n#### stderr\n\n" + code_block(
            truncate(stderr_text, MAX_SUMMARY_LINES, MAX_SUMMARY_CHARS)
        ) + "\n"

    if exit_code != 0 and summary["unparsed_output"]:
        markdown += "\n#### Unparsed output\n\n" + code_block(
            truncate(
                summary["unparsed_output"],
                MAX_SUMMARY_LINES,
                MAX_SUMMARY_CHARS
            )
        ) + "\n"

    write_step_summary(markdown)


# ---------------------------------------------------------------------------
# Saving
# ---------------------------------------------------------------------------

def save_log(
    source,
    kind,
    exit_code,
    run_id,
    run_attempt,
    commit_sha,
    repository,
    stderr_source=None
):
    now = datetime.now(timezone.utc)
    timestamp = now.strftime("%Y%m%dT%H%M%SZ")

    target_dir = SUCCESS_DIR if exit_code == 0 else ERROR_DIR
    target_dir.mkdir(parents=True, exist_ok=True)

    filename = f"{timestamp}_{run_id}_{run_attempt}_{kind}.json"

    commit_url = f"https://github.com/{repository}/commit/{commit_sha}"

    run_url = (
        f"https://github.com/{repository}/actions/runs/"
        f"{run_id}/attempts/{run_attempt}"
    )

    content = read_text(source)
    stderr_text = read_text(stderr_source)

    if kind == "testing":
        details = parse_events(content)
        summary = analyze_tests(details)
    else:
        details = content
        summary = analyze_format(content, exit_code)

    # "summary" comes before "details" so the important part is at the
    # top of the file.
    result = {
        "timestamp": timestamp,
        "type": kind,
        "status": "success" if exit_code == 0 else "failure",
        "exit_code": exit_code,
        "commit": {
            "sha": commit_sha,
            "url": commit_url
        },
        "github": {
            "repository": repository,
            "run_id": run_id,
            "run_attempt": run_attempt,
            "run_url": run_url
        },
        "summary": summary,
        "details": details
    }

    if stderr_text.strip():
        result["stderr"] = stderr_text

    output_path = target_dir / filename

    with output_path.open("w", encoding="utf-8") as file:
        json.dump(
            result,
            file,
            ensure_ascii=False,
            indent=2
        )

    print(f"Saved: {output_path.relative_to(ROOT)}")

    annotator = Annotator()

    if kind == "testing":
        report_testing(result, annotator, read_module_path(), stderr_text)
    else:
        report_formatting(result, annotator)

    annotator.finish()


def cleanup_logs():
    logs = []

    for directory in (SUCCESS_DIR, ERROR_DIR):
        if not directory.exists():
            continue

        for path in directory.glob("*.json"):
            match = re.fullmatch(
                r"(\d{8}T\d{6}Z)_\d+_\d+_"
                r"(formatting|testing)\.json",
                path.name
            )

            if not match:
                continue

            timestamp = datetime.strptime(
                match.group(1),
                "%Y%m%dT%H%M%SZ"
            ).replace(tzinfo=timezone.utc)

            logs.append((timestamp, path))

    logs.sort(
        key=lambda item: (item[0], item[1].name),
        reverse=True
    )

    cutoff = datetime.now(timezone.utc) - timedelta(
        days=RETENTION_DAYS
    )

    deleted = 0

    for index, (timestamp, path) in enumerate(logs):

        # Keep logs from the last 21 days
        # OR the most recent 20 logs.

        if timestamp >= cutoff or index < RETENTION_COUNT:
            continue

        path.unlink()
        deleted += 1

        print(f"Deleted: {path.relative_to(ROOT)}")

    print(
        f"Cleanup: {len(logs) - deleted} log(s) kept, {deleted} deleted "
        f"(retention: {RETENTION_DAYS} days or latest {RETENTION_COUNT})"
    )


def main():
    parser = argparse.ArgumentParser(
        description="Manage Go formatting and testing logs."
    )

    parser.add_argument("--source", required=True)

    parser.add_argument(
        "--stderr",
        default=None,
        help="Optional file containing the command's stderr."
    )

    parser.add_argument(
        "--kind",
        required=True,
        choices=["formatting", "testing"]
    )

    parser.add_argument("--exit-code", required=True, type=int)
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--run-attempt", required=True)
    parser.add_argument("--commit-sha", required=True)
    parser.add_argument("--repository", required=True)

    args = parser.parse_args()

    save_log(
        source=args.source,
        kind=args.kind,
        exit_code=args.exit_code,
        run_id=args.run_id,
        run_attempt=args.run_attempt,
        commit_sha=args.commit_sha,
        repository=args.repository,
        stderr_source=args.stderr
    )

    cleanup_logs()


if __name__ == "__main__":
    SUCCESS_DIR.mkdir(parents=True, exist_ok=True)
    ERROR_DIR.mkdir(parents=True, exist_ok=True)

    main()