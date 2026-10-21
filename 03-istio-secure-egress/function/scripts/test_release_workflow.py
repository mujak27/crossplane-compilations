import os
from pathlib import Path
import subprocess
import tempfile
import unittest

import yaml


WORKFLOW = Path(__file__).resolve().parents[3] / ".github/workflows/istio-secure-egress.yaml"


class ReleaseWorkflowTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.workflow = yaml.load(WORKFLOW.read_text(), Loader=yaml.BaseLoader)

    def test_runtime_release_tags_are_the_only_publish_trigger(self):
        self.assertEqual(set(self.workflow["on"]), {"pull_request", "push", "workflow_dispatch"})
        self.assertEqual(self.workflow["on"]["push"]["branches"], ["main"])
        self.assertEqual(
            self.workflow["on"]["push"]["tags"],
            ["function-istio-secure-egress-v*"],
        )
        publish = self.workflow["jobs"]["publish"]
        self.assertIn("github.event_name == 'push'", publish["if"])
        self.assertIn("startsWith(github.ref, 'refs/tags/function-istio-secure-egress-v')", publish["if"])
        self.assertIn("needs.test.outputs.release_version != ''", publish["if"])
        self.assertEqual(publish["needs"], "test")
        self.assertEqual(publish["permissions"]["packages"], "write")
        self.assertEqual(self.workflow["permissions"]["contents"], "read")

        steps = self.workflow["jobs"]["test"]["steps"]
        docker_build = next(step["run"] for step in steps if step.get("name") == "Build runtime image")
        xpkg_build = next(step["run"] for step in steps if step.get("name") == "Build Crossplane package")
        upload = next(step for step in steps if step.get("name") == "Upload release package")
        self.assertIn("--platform linux/amd64", docker_build)
        self.assertIn("--embed-runtime-image function-istio-secure-egress:runtime", xpkg_build)
        self.assertEqual(upload["if"], "steps.release.outputs.version != ''")

    def test_release_resolver_accepts_strict_versions_and_rejects_other_tags(self):
        build = self.workflow["jobs"]["test"]
        step = next(step for step in build["steps"] if step.get("name") == "Resolve runtime release version")

        for tag in ("function-istio-secure-egress-v0.1.0", "function-istio-secure-egress-v12.34.56"):
            with self.subTest(tag=tag), tempfile.TemporaryDirectory() as directory:
                output = Path(directory) / "output"
                result = subprocess.run(
                    ["bash", "-euo", "pipefail", "-c", step["run"]],
                    env={**os.environ, "RELEASE_TAG": tag, "GITHUB_OUTPUT": str(output)},
                    capture_output=True,
                    text=True,
                )
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(output.read_text().strip(), f"version={tag.removeprefix('function-istio-secure-egress-')}")

        for tag in (
            "function-istio-secure-egress-v01.1.0",
            "function-istio-secure-egress-v1.01.0",
            "function-istio-secure-egress-v1.1.00",
            "function-istio-secure-egress-v1.1",
            "istio-secure-egress-v1.2.3",
        ):
            with self.subTest(tag=tag), tempfile.TemporaryDirectory() as directory:
                result = subprocess.run(
                    ["bash", "-euo", "pipefail", "-c", step["run"]],
                    env={**os.environ, "RELEASE_TAG": tag, "GITHUB_OUTPUT": str(Path(directory) / "output")},
                    capture_output=True,
                    text=True,
                )
                self.assertNotEqual(result.returncode, 0, tag)


if __name__ == "__main__":
    unittest.main()
