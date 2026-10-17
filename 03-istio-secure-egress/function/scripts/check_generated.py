"""Regenerate and check required outputs against HEAD, including staged drift."""

from pathlib import Path
import subprocess
import sys


def main():
    root = Path(__file__).resolve().parents[2]
    required = {Path("xrd.yaml"), Path("function/api/v1alpha1/zz_generated.deepcopy.go")}
    required.update(path.relative_to(root) for path in (root / "function/api").rglob("zz_generated.deepcopy.go"))
    missing = {path for path in required if not (root / path).is_file()}
    dirty = subprocess.run(["git", "diff", "--quiet", "HEAD", "--", *map(str, sorted(required))], cwd=root).returncode != 0
    subprocess.run(["make", "generate"], cwd=root, check=True)
    required.update(path.relative_to(root) for path in (root / "function/api").rglob("zz_generated.deepcopy.go"))
    failed = False
    for path in sorted(required):
        tracked = subprocess.run(["git", "ls-files", "--error-unmatch", "--", str(path)], cwd=root, capture_output=True).returncode == 0
        if path in missing or not (root / path).is_file() or not tracked:
            print(f"Required generated artifact missing or untracked: {path}", file=sys.stderr)
            failed = True
    result = subprocess.run(["git", "diff", "--exit-code", "HEAD", "--", *map(str, sorted(required))], cwd=root)
    if dirty or failed or result.returncode:
        sys.exit("Generated artifacts differ from HEAD; regenerate and commit the required outputs.")


if __name__ == "__main__":
    try:
        main()
    except subprocess.CalledProcessError as err:
        sys.exit(f"Generation failed: {err}")
