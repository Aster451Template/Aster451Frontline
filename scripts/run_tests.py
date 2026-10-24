from datetime import datetime, timezone
from pathlib import Path
import os
import subprocess
import sys


ROOT = Path(__file__).resolve().parents[1]
PACKAGES = [".", "./builder", "./parser", "./models", "./utils"]


def main() -> int:
    log_dir = ROOT / "log"
    log_dir.mkdir(exist_ok=True)
    timestamp = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    run_id = f"{timestamp}-{os.getenv('GITHUB_RUN_ID', 'local')}-{os.getenv('GITHUB_RUN_ATTEMPT', '1')}"
    results: list[tuple[str, str, str]] = []

    for package in PACKAGES:
        name = "root" if package == "." else package.removeprefix("./").replace("/", "-")
        log_path = log_dir / f"go-test-{run_id}-{name}.log"
        command = ["go", "test", package]
        result = subprocess.run(command, cwd=ROOT, capture_output=True, text=True)
        output = f"$ {' '.join(command)}\n{result.stdout}{result.stderr}"
        log_path.write_text(output, encoding="utf-8")
        print(output, end="" if output.endswith("\n") else "\n")
        results.append((package, "PASS" if result.returncode == 0 else "FAIL", log_path.relative_to(ROOT).as_posix()))

    readme = ROOT / "README.md"
    content = readme.read_text(encoding="utf-8")
    if "## CI test results" not in content:
        content = content.rstrip() + "\n\n## CI test results\n"
    report = [f"\n### {run_id} UTC\n", "| Package | Result | Log |", "| --- | --- | --- |"]
    report.extend(f"| `{package}` | **{status}** | [{Path(path).name}]({path}) |" for package, status, path in results)
    readme.write_text(content.rstrip() + "\n" + "\n".join(report) + "\n", encoding="utf-8")

    return 1 if any(status != "PASS" for _, status, _ in results) else 0


if __name__ == "__main__":
    sys.exit(main())
