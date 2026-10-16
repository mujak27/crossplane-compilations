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
    assert isinstance(xr["spec"]["message"], str)
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
        self.assertIn("message", schema["properties"]["spec"]["required"])
        self.assertEqual(schema["properties"]["spec"]["properties"]["message"]["type"], "string")

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
