from copy import deepcopy
from pathlib import Path
import unittest

import yaml


def check_contract(xrd, composition, xr, function):
    assert xrd["apiVersion"] == "apiextensions.crossplane.io/v2"
    assert xrd["kind"] == "CompositeResourceDefinition"
    spec = xrd["spec"]
    assert spec["scope"] == "Namespaced"
    assert xrd["metadata"]["name"] == f'{spec["names"]["plural"]}.{spec["group"]}'
    version = next(v for v in spec["versions"] if v["referenceable"])
    assert version["served"]
    identity = {"apiVersion": f'{spec["group"]}/{version["name"]}', "kind": spec["names"]["kind"]}
    assert composition["kind"] == "Composition"
    assert composition["spec"]["compositeTypeRef"] == identity
    assert {key: xr[key] for key in identity} == identity
    assert xr["metadata"]["name"] and xr["metadata"]["namespace"]
    assert xr["spec"]["crossplane"]["compositionRef"]["name"] == composition["metadata"]["name"]
    assert isinstance(xr["spec"]["hostname"], str)
    assert isinstance(xr["spec"]["gatewayProfile"], str)
    assert identity == {"apiVersion": "platform.mujak.com/v1alpha1", "kind": "XIstioSecureEgress"}
    assert composition["metadata"]["name"] == "istio-secure-egress.platform.mujak.com"
    assert function["metadata"]["name"] == "function-istio-secure-egress"
    assert set(composition["spec"]["pipeline"][0]) == {"step", "functionRef"}
    assert function["kind"] == "Function"
    assert composition["spec"]["pipeline"][0]["functionRef"]["name"] == function["metadata"]["name"]
    for definition in (xrd, composition, function):
        assert "namespace" not in definition["metadata"]


class ManifestTest(unittest.TestCase):
    def setUp(self):
        self.root = Path(__file__).resolve().parents[2]
        self.manifests = [yaml.safe_load((self.root / path).read_text()) for path in ("xrd.yaml", "composition.yaml", "examples/xr.yaml", "functions.yaml")]

    def test_contract(self):
        check_contract(*self.manifests)
        kustomization = yaml.safe_load((self.root / "kustomization.yaml").read_text())
        self.assertEqual(kustomization["resources"], ["xrd.yaml", "composition.yaml"])
        schema = self.manifests[0]["spec"]["versions"][0]["schema"]["openAPIV3Schema"]
        self.assertIn("spec", schema["required"])
        spec = schema["properties"]["spec"]
        self.assertEqual(set(spec["required"]), {"hostname", "gatewayProfile"})
        self.assertEqual(set(spec["properties"]), {"hostname", "gatewayProfile"})
        host, profile = (spec["properties"][key] for key in ("hostname", "gatewayProfile"))
        self.assertEqual(host["type"], "string")
        self.assertEqual(host["minLength"], 1)
        self.assertEqual(host["maxLength"], 253)
        self.assertEqual(host["x-kubernetes-validations"][0]["rule"], "!isIP(self)")
        self.assertEqual(profile["type"], "string")
        self.assertEqual(profile["minLength"], 1)
        self.assertEqual(profile["maxLength"], 50)
        import re
        for value in ("api.example.com", "a", "1.example", "123.abc"):
            self.assertRegex(value, host["pattern"])
        for value in ("*.example.com", "https://api.example.com", "a:443", "a..com", "a." , "a" * 64 + ".com"):
            self.assertIsNone(re.fullmatch(host["pattern"], value))
        self.assertRegex("east-west", profile["pattern"])
        self.assertIsNone(re.fullmatch(profile["pattern"], "east.west"))
        fixture = yaml.safe_load((self.root / "examples/environmentconfig.yaml").read_text())
        self.assertEqual(fixture["metadata"], {"name": "istio-egress-east-west"})
        self.assertEqual(fixture["kind"], "EnvironmentConfig")
        self.assertNotEqual(self.manifests[2]["metadata"]["namespace"], "default")
        self.assertIn("--required-resources", (self.root / "render.sh").read_text())

    def test_contract_rejects_drift(self):
        for index, path, value in [
            (0, ["metadata", "name"], "wrong"),
            (0, ["spec", "scope"], "Cluster"),
            (0, ["apiVersion"], "apiextensions.crossplane.io/v1"),
            (1, ["spec", "compositeTypeRef", "kind"], "Wrong"),
            (2, ["apiVersion"], "wrong/v1"),
            (2, ["metadata", "namespace"], None),
            (2, ["spec", "crossplane", "compositionRef"], None),
            (2, ["spec", "crossplane", "compositionRef", "name"], "wrong"),
            *[(i, ["metadata", "namespace"], "default") for i in (0, 1, 3)],
        ]:
            manifests = deepcopy(self.manifests)
            obj = manifests[index]
            for key in path[:-1]:
                obj = obj[key]
            if value is None:
                del obj[path[-1]]
            else:
                obj[path[-1]] = value
            with self.subTest(index=index, path=path), self.assertRaises((AssertionError, KeyError)):
                check_contract(*manifests)


if __name__ == "__main__":
    unittest.main()
