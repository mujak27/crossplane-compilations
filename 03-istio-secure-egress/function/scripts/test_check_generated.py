"""Exercise drift checks only in disposable repositories, never the task index."""

from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest


class GeneratedTest(unittest.TestCase):
    def test_drift(self):
        script = Path(__file__).with_name("check_generated.py")
        artifacts = ["xrd.yaml", "function/api/v1alpha1/zz_generated.deepcopy.go"]
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "function/scripts").mkdir(parents=True)
            (root / "function/api/v1alpha1").mkdir(parents=True)
            shutil.copy(script, root / "function/scripts/check_generated.py")
            (root / "Makefile").write_text("generate:\n\tcp expected-xrd xrd.yaml\n\tcp expected-go function/api/v1alpha1/zz_generated.deepcopy.go\n")
            for path in [*artifacts, "expected-xrd", "expected-go"]:
                (root / path).write_text("generated\n")

            def git(*args):
                return subprocess.run(["git", *args], cwd=root, check=True, capture_output=True)

            def check(ok):
                result = subprocess.run([sys.executable, "function/scripts/check_generated.py"], cwd=root, capture_output=True, text=True)
                self.assertEqual(result.returncode == 0, ok, result.stdout + result.stderr)
                if not ok:
                    self.assertIn("Generated artifacts differ", result.stderr)

            git("init")
            git("add", ".")
            git("-c", "user.name=Fixture", "-c", "user.email=fixture@example.org", "-c", "commit.gpgsign=false", "commit", "-m", "fixture")
            check(True)
            for path, expected in zip(artifacts, ["expected-xrd", "expected-go"]):
                with self.subTest(path=path):
                    # A source change makes regenerated output differ from HEAD.
                    (root / expected).write_text("changed\n")
                    check(False)
                    # Even staging the drift must not make the check pass.
                    git("add", path)
                    check(False)
                    (root / expected).write_text("generated\n")
                    (root / path).write_text("generated\n")
                    git("add", path)
                    check(True)
                    (root / path).write_text("manual drift\n")
                    check(False)
                    check(True)
                    (root / path).unlink()
                    check(False)
                    check(True)
                    git("rm", "--cached", path)
                    check(False)
                    git("add", path)
                    check(True)
            # Newly generated Go output must also be tracked.
            with (root / "Makefile").open("a") as makefile:
                makefile.write("\tmkdir -p function/api/v1alpha1/extra\n\tcp expected-go function/api/v1alpha1/extra/zz_generated.deepcopy.go\n")
            check(False)


if __name__ == "__main__":
    unittest.main()
