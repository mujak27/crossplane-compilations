"""Run the real pinned Function through Crossplane's local render engine."""

from copy import deepcopy
from pathlib import Path
import subprocess
import tempfile
import unittest

import yaml


ROOT = Path(__file__).resolve().parent.parent
COMPOSITION = ROOT / "composition.yaml"
NOW = "2030-01-01T00:00:00Z"


class ReadinessTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.xr = yaml.safe_load((ROOT / "examples/xr.yaml").read_text())
        cls.healthy = list(yaml.safe_load_all(
            (ROOT / "examples/observed/healthy.yaml").read_text()
        ))
        cls.composition = yaml.safe_load(COMPOSITION.read_text())
        # Only replace the clock; every case executes the actual production template
        # and installed Function package, including readiness annotation processing.
        inline = cls.composition["spec"]["pipeline"][0]["input"]["inline"]
        clock = "{{- $now := now -}}"
        if clock in inline["template"]:
            assert inline["template"].count(clock) == 1
            inline["template"] = inline["template"].replace(
                clock, '{{- $now := toDate "2006-01-02T15:04:05Z" "' + NOW + '" -}}'
            )
        cls.functions = [f for f in yaml.safe_load_all(
            (ROOT / "functions.yaml").read_text()
        ) if f["metadata"]["name"] == "function-go-templating"]

    def render(self, observed, certificate=True, ingress=True, health="Healthy", xr=None):
        with tempfile.TemporaryDirectory(prefix="tls-ingress-test-") as directory:
            path = Path(directory)
            inputs = {"xr": xr or self.xr, "composition": self.composition}
            for name, document in inputs.items():
                (path / f"{name}.yaml").write_text(yaml.safe_dump(document))
            (path / "functions.yaml").write_text(yaml.safe_dump_all(self.functions))
            (path / "observed.yaml").write_text(yaml.safe_dump_all(observed))
            command = [
                "crossplane", "composition", "render", str(path / "xr.yaml"),
                str(path / "composition.yaml"), str(path / "functions.yaml"),
                "--crossplane-version=v2.4.2", "--include-full-xr",
            ]
            if observed:
                command += ["--observed-resources", str(path / "observed.yaml")]
            result = subprocess.run(command, text=True, capture_output=True, timeout=90)
            self.assertEqual(result.returncode, 0, result.stderr)
            rendered = list(yaml.safe_load_all(result.stdout))

        self.assertEqual(len(rendered), 3)
        composite = next(r for r in rendered if r["kind"] == "XIngressTLS")
        cert = next(r for r in rendered if r["kind"] == "Certificate")
        ing = next(r for r in rendered if r["kind"] == "Ingress")
        ready = next(c for c in composite["status"]["conditions"] if c["type"] == "Ready")
        self.assertEqual(ready["status"], "True" if certificate and ingress else "False")
        # Crossplane derives this message from Function response readiness, not
        # annotations left in YAML. Check each composed resource independently.
        unready = set(ready.get("message", "").removeprefix("Unready resources: ").split(", "))
        self.assertEqual("certificate" in unready, not certificate)
        self.assertEqual("ingress" in unready, not ingress)
        status = composite["status"]["certificate"]
        self.assertEqual(status["ready"], certificate)
        self.assertEqual(status["health"], health)
        source = xr or self.xr
        name, namespace = source["metadata"]["name"], source["metadata"]["namespace"]
        for resource, key in ((cert, "certificate"), (ing, "ingress")):
            self.assertEqual(resource["metadata"]["name"], name)
            self.assertEqual(resource["metadata"]["namespace"], namespace)
            self.assertEqual(resource["metadata"]["annotations"][
                "crossplane.io/composition-resource-name"], key)
        self.assertEqual(cert["spec"], self.healthy[0]["spec"])
        expected_spec = deepcopy(self.healthy[1]["spec"])
        if not source["spec"]["ingress"].get("className"):
            expected_spec.pop("ingressClassName")
        self.assertEqual(ing["spec"], expected_spec)
        expected_expiry = next(((r.get("status") or {}).get("notAfter")
                                for r in observed if r["kind"] == "Certificate"), None)
        self.assertEqual(status.get("notAfter"), expected_expiry or None)
        self.assertEqual(ing["metadata"]["annotations"].get(
            "platform.mujak.com/cert-not-after"), expected_expiry or None)
        return composite

    def test_healthy(self):
        self.render(deepcopy(self.healthy))

    def test_missing_resources(self):
        for observed, cert, ing in (([], False, False),
                                    ([self.healthy[0]], True, False),
                                    ([self.healthy[1]], False, True)):
            with self.subTest(resources=[r["kind"] for r in observed]):
                self.render(observed, cert, ing, "Healthy" if cert else "Pending")

    def test_certificate_conditions(self):
        changes = [
            ("false", {"type": "Ready", "status": "False", "observedGeneration": 1}),
            ("unknown", {"type": "Ready", "status": "Unknown", "observedGeneration": 1}),
            ("missing observedGeneration", {"type": "Ready", "status": "True"}),
            ("stale", {"type": "Ready", "status": "True", "observedGeneration": 0}),
            ("missing status", {"type": "Ready", "observedGeneration": 1}),
            ("unrelated", {"type": "Issuing", "status": "True", "observedGeneration": 1}),
        ]
        for name, condition in changes:
            with self.subTest(name=name):
                observed = deepcopy(self.healthy)
                observed[0]["status"]["conditions"] = [condition]
                self.render(observed, certificate=False, health="Pending")

    def test_certificate_missing_fields(self):
        for parent, key in (("status", "conditions"), ("status", "notAfter"),
                            ("metadata", "generation"), (None, "status")):
            with self.subTest(parent=parent, key=key):
                observed = deepcopy(self.healthy)
                target = observed[0][parent] if parent else observed[0]
                del target[key]
                self.render(observed, certificate=False, health="Pending")
        for key in ("status", "conditions", "generations"):
            with self.subTest(null=key):
                observed = deepcopy(self.healthy)
                if key == "status":
                    observed[0]["status"] = None
                elif key == "conditions":
                    observed[0]["status"]["conditions"] = None
                else:
                    observed[0]["metadata"]["generation"] = None
                    observed[0]["status"]["conditions"][0]["observedGeneration"] = None
                self.render(observed, certificate=False, health="Pending")

    def test_certificate_expiry_boundaries(self):
        for expiry, ready, health, days in (
            ("not-a-date", False, "Expired", None),
            ("2029-12-31T23:59:59Z", False, "Expired", 0),
            (NOW, False, "Expired", 0),
            ("2030-01-01T00:00:01Z", True, "Warning", 0),
            ("2030-01-16T23:59:59Z", True, "Warning", 15),
            ("2030-01-17T00:00:00Z", True, "Healthy", 16),
            ("2030-01-01T01:00:01+01:00", True, "Warning", 0),
            ("2030-01-01T00:00:00.000000001Z", True, "Warning", 0),
        ):
            with self.subTest(expiry=expiry):
                observed = deepcopy(self.healthy)
                observed[0]["status"]["notAfter"] = expiry
                composite = self.render(observed, certificate=ready, health=health)
                if days is not None:
                    self.assertEqual(composite["status"]["certificate"]["daysRemaining"], days)

    def test_ingress_mismatches(self):
        changes = [
            (("rules", 0, "host"), "other.example"),
            (("rules", 0, "http", "paths", 0, "path"), "/other"),
            (("rules", 0, "http", "paths", 0, "pathType"), "Exact"),
            (("rules", 0, "http", "paths", 0, "backend", "service", "name"), "other"),
            (("rules", 0, "http", "paths", 0, "backend", "service", "port", "number"), 80),
            (("tls", 0, "hosts", 0), "other.example"),
            (("tls", 0, "secretName"), "other-tls"),
            (("ingressClassName",), "other"),
            (("rules",), []),
            (("tls",), []),
            (("rules", 0, "http"), {}),
            (("rules", 0, "http", "paths", 0, "backend"), {}),
        ]
        for fields, value in changes:
            with self.subTest(fields=fields):
                observed = deepcopy(self.healthy)
                target = observed[1]["spec"]
                for field in fields[:-1]:
                    target = target[field]
                target[fields[-1]] = value
                self.render(observed, ingress=False)

    def test_ingress_missing_and_extra_fields(self):
        for key in ("rules", "tls", "ingressClassName", "spec"):
            with self.subTest(missing=key):
                observed = deepcopy(self.healthy)
                target = observed[1] if key == "spec" else observed[1]["spec"]
                del target[key]
                self.render(observed, ingress=False)
        for fields in (("rules",), ("tls",), ("rules", 0, "http", "paths"),
                       ("tls", 0, "hosts")):
            with self.subTest(extra=fields):
                observed = deepcopy(self.healthy)
                target = observed[1]["spec"]
                for field in fields:
                    target = target[field]
                target.append(deepcopy(target[0]))
                self.render(observed, ingress=False)

    def test_defaulted_class_and_numeric_representations(self):
        xr = deepcopy(self.xr)
        del xr["spec"]["ingress"]["className"]
        observed = deepcopy(self.healthy)
        observed[1]["spec"]["ingressClassName"] = "admission-default"
        observed[1]["metadata"]["annotations"]["controller.example/key"] = "ignored"
        observed[0]["metadata"]["generation"] = 1.0
        observed[1]["spec"]["rules"][0]["http"]["paths"][0]["backend"]["service"]["port"]["number"] = 8080.0
        self.render(observed, xr=xr)

    def test_readiness_transitions(self):
        composite = self.render([], False, False, "Pending")
        composite = self.render(deepcopy(self.healthy), xr=composite)
        observed = deepcopy(self.healthy)
        observed[0]["status"]["conditions"][0]["status"] = "False"
        observed[1]["spec"]["tls"][0]["secretName"] = "changed"
        self.render(observed, False, False, "Pending", xr=composite)


if __name__ == "__main__":
    unittest.main()
