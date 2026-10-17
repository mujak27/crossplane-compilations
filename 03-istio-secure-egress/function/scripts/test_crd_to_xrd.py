from copy import deepcopy
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

import yaml

from crd_to_xrd import convert


class ConverterTest(unittest.TestCase):
    def setUp(self):
        self.crd = {
            "apiVersion": "apiextensions.k8s.io/v1",
            "kind": "CustomResourceDefinition",
            "metadata": {"name": "xistiosecureegresses.platform.mujak.com"},
            "spec": {
                "scope": "Namespaced", "group": "platform.mujak.com",
                "names": {"kind": "XIstioSecureEgress", "plural": "xistiosecureegresses", "singular": "xistiosecureegress"},
                "versions": [{
                    "name": "v1alpha1", "served": True, "storage": True,
                    "schema": {"openAPIV3Schema": {
                        "type": "object",
                        "properties": {
                            "metadata": {"type": "object"},
                            "spec": {"type": "object", "required": ["message"], "properties": {"message": {"type": "string"}}},
                            "status": {"type": "object", "required": ["phase"], "properties": {"phase": {"type": "string", "enum": ["Ready", "Pending"]}}},
                        },
                        "required": ["metadata", "spec"],
                    }},
                }],
            },
        }

    def test_conversion(self):
        before = deepcopy(self.crd)
        xrd = convert(self.crd)
        self.assertEqual(self.crd, before)
        self.assertEqual(xrd["apiVersion"], "apiextensions.crossplane.io/v2")
        self.assertEqual(xrd["spec"]["scope"], "Namespaced")
        self.assertEqual(xrd["spec"]["names"], self.crd["spec"]["names"])
        version = xrd["spec"]["versions"][0]
        self.assertTrue(version["referenceable"])
        self.assertTrue(version["served"])
        self.assertEqual(version["name"], "v1alpha1")
        schema = version["schema"]["openAPIV3Schema"]
        self.assertEqual(set(schema["properties"]), {"spec", "status"})
        self.assertEqual(schema["required"], ["spec"])
        self.assertEqual(schema["properties"]["spec"]["required"], ["message"])
        for key in ("spec", "status"):
            self.assertEqual(schema["properties"][key], self.crd["spec"]["versions"][0]["schema"]["openAPIV3Schema"]["properties"][key])
        self.assertEqual(xrd["metadata"], self.crd["metadata"])
        self.assertEqual(xrd["spec"]["group"], self.crd["spec"]["group"])

    def test_versions_and_filtered_required(self):
        old = deepcopy(self.crd["spec"]["versions"][0])
        old.update(name="v1old", served=False, storage=False)
        old["schema"]["openAPIV3Schema"]["required"] = ["metadata"]
        self.crd["spec"]["versions"].insert(0, old)
        versions = convert(self.crd)["spec"]["versions"]
        self.assertEqual([(v["name"], v["served"], v["referenceable"]) for v in versions], [("v1old", False, False), ("v1alpha1", True, True)])
        self.assertNotIn("required", versions[0]["schema"]["openAPIV3Schema"])

    def test_malformed_inputs(self):
        for crd, message in [(None, "mapping"), ({**self.crd, "kind": "Other"}, "CustomResourceDefinition")]:
            with self.subTest(crd=crd), self.assertRaisesRegex(ValueError, message):
                convert(crd)
        self.crd["spec"]["versions"] = []
        with self.assertRaisesRegex(ValueError, "at least one version"):
            convert(self.crd)

    def test_cli(self):
        script = Path(__file__).with_name("crd_to_xrd.py")
        with tempfile.TemporaryDirectory() as directory:
            source = Path(directory) / "input.yaml"
            output = Path(directory) / "output.yaml"
            source.write_text(yaml.safe_dump(self.crd))
            result = subprocess.run([sys.executable, str(script), str(source), str(output)], capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(yaml.safe_load(output.read_text()), convert(self.crd))
            valid_output = output.read_bytes()
            malformed = ["null", "[", "{}"]
            for field, value in [("versions", {}), ("versions", [])]:
                crd = deepcopy(self.crd)
                crd["spec"][field] = value
                malformed.append(yaml.safe_dump(crd))
            for change in ("flags", "schema"):
                crd = deepcopy(self.crd)
                version = crd["spec"]["versions"][0]
                if change == "flags":
                    version["served"] = "true"
                else:
                    version["schema"]["openAPIV3Schema"] = {"type": "string"}
                malformed.append(yaml.safe_dump(crd))
            for content in malformed:
                source.write_text(content)
                with self.subTest(content=content):
                    result = subprocess.run([sys.executable, str(script), str(source), str(output)], capture_output=True, text=True)
                    self.assertNotEqual(result.returncode, 0)
                    self.assertIn("crd_to_xrd:", result.stderr)
                    self.assertNotIn("Traceback", result.stderr)
                    self.assertEqual(output.read_bytes(), valid_output)
            source.unlink()
            result = subprocess.run([sys.executable, str(script), str(source), str(output)], capture_output=True, text=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("crd_to_xrd:", result.stderr)
            result = subprocess.run([sys.executable, str(script)], capture_output=True, text=True)
            self.assertEqual(result.returncode, 2)
            self.assertIn("usage:", result.stderr)

    def test_cluster_scope_rejected(self):
        self.crd["spec"]["scope"] = "Cluster"
        with self.assertRaisesRegex(ValueError, "Namespaced"):
            convert(self.crd)


if __name__ == "__main__":
    unittest.main()
