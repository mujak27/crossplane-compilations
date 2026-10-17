"""Convert this template's generated namespaced CRD into a Crossplane v2 XRD."""

import argparse
from copy import deepcopy
from pathlib import Path

import yaml


def convert(crd):
    if not isinstance(crd, dict):
        raise ValueError("expected a CRD mapping")
    if crd["apiVersion"] != "apiextensions.k8s.io/v1" or crd["kind"] != "CustomResourceDefinition":
        raise ValueError("expected an apiextensions.k8s.io/v1 CustomResourceDefinition")
    spec = crd["spec"]
    if spec["scope"] != "Namespaced":
        raise ValueError("only Namespaced CRDs are supported")
    versions = []
    if not isinstance(spec["versions"], list):
        raise ValueError("CRD versions must be a list")
    for version in spec["versions"]:
        if not isinstance(version["served"], bool) or not isinstance(version["storage"], bool):
            raise ValueError("version served and storage must be booleans")
        schema = deepcopy(version["schema"]["openAPIV3Schema"])
        if schema.get("type") != "object" or not isinstance(schema.get("properties"), dict):
            raise ValueError("version schema must be an object with properties")
        schema["properties"] = {
            key: value for key, value in schema["properties"].items()
            if key in ("spec", "status")
        }
        if "required" in schema:
            schema["required"] = [key for key in schema["required"] if key in schema["properties"]]
            if not schema["required"]:
                del schema["required"]
        versions.append({
            "name": version["name"],
            "served": version["served"],
            "referenceable": version["storage"],
            "schema": {"openAPIV3Schema": schema},
        })
    if not versions:
        raise ValueError("CRD must contain at least one version")
    return {
        "apiVersion": "apiextensions.crossplane.io/v2",
        "kind": "CompositeResourceDefinition",
        "metadata": {"name": crd["metadata"]["name"]},
        "spec": {
            "scope": "Namespaced",
            "group": spec["group"],
            "names": {key: spec["names"][key] for key in ("kind", "plural", "singular")},
            "versions": versions,
        },
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("input", type=Path)
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    try:
        xrd = convert(yaml.safe_load(args.input.read_text()))
        args.output.write_text("# Generated from function/api/v1alpha1/types.go; DO NOT EDIT.\n" + yaml.safe_dump(xrd, sort_keys=False))
    except (OSError, yaml.YAMLError, KeyError, TypeError, ValueError, AttributeError) as err:
        parser.exit(1, f"crd_to_xrd: {err}\n")


if __name__ == "__main__":
    main()
