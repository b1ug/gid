#!/usr/bin/env python3
"""Print an offline GID source manifest and compare its tracked file fingerprints."""
import argparse
import hashlib
import json
from pathlib import Path, PurePosixPath
import re
import sys

DEFAULT_ROOT = Path(__file__).resolve().parents[1]


def audit(root, manifest):
    if manifest.get("schema_version") != 1 or not isinstance(manifest.get("files"), list) or not manifest["files"]:
        raise ValueError("expected source manifest schema_version=1 and a nonempty files array")
    rows, data = [], {}
    seen = set()
    for entry in manifest["files"]:
        relative = entry.get("path")
        if not isinstance(relative, str) or not relative or relative in seen:
            raise ValueError("manifest file paths must be unique, nonempty strings")
        seen.add(relative)
        parts = PurePosixPath(relative)
        if parts.is_absolute() or ".." in parts.parts or "\\" in relative:
            raise ValueError("manifest file path must stay within the repository: " + relative)
        expected = entry.get("sha256", "")
        if not re.fullmatch(r"[0-9a-f]{64}", expected):
            raise ValueError("invalid SHA256 for " + relative)
        row = {"path": relative, "expected_sha256": expected,
               "expected_bytes": entry.get("bytes"), "expected_lines": entry.get("lines")}
        target = (root / relative).resolve()
        try:
            target.relative_to(root)
        except ValueError:
            row.update({"status": "outside-root", "error": "resolved path leaves the selected repository"})
            rows.append(row)
            continue
        try:
            content = target.read_bytes()
        except OSError as exc:
            row.update({"status": "missing-or-unreadable", "error": str(exc)})
            rows.append(row)
            continue
        actual = hashlib.sha256(content).hexdigest()
        row.update({"actual_sha256": actual, "actual_bytes": len(content),
                    "actual_lines": len(content.splitlines())})
        row["status"] = "match" if (actual == expected and len(content) == entry.get("bytes")
            and len(content.splitlines()) == entry.get("lines")) else "changed"
        rows.append(row)
        data[relative] = content
    header = data.get("hidapi_linux.h", b"").decode("utf-8", errors="replace")
    macros = {}
    for name, value in re.findall(r"^\s*#\s*define\s+(LIBUSB_(?:MAJOR|MINOR|MICRO|NANO))\s+([^\s/]+)", header, re.MULTILINE):
        macros.setdefault(name, []).append(value)
    return {"audit": "gid-upstream-source-audit", "root": str(root),
            "ok": all(row["status"] == "match" for row in rows),
            "network_used": False, "writes_performed": False,
            "files": rows, "observed_libusb_macro_definitions": macros,
            "observed_embedded_source_paths": sorted(set(re.findall(r'^// line \d+ "([^"]+)"', header, re.MULTILINE))),
            "scope": "Fingerprint and recorded provenance comparison only; not an upstream-release identity, native build or hardware verification.",
            "manifest": manifest}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, default=DEFAULT_ROOT, help="repository or controlled source-copy directory")
    parser.add_argument("--check", action="store_true", help="exit 1 if a tracked file is changed, missing or outside the root")
    args = parser.parse_args()
    root = args.root.expanduser().resolve()
    try:
        manifest = json.loads((root / "upstream-sources.json").read_text(encoding="utf-8"))
        if not isinstance(manifest, dict):
            raise ValueError("source manifest must be a JSON object")
        result = audit(root, manifest)
    except (OSError, ValueError, TypeError, AttributeError) as exc:
        parser.error(str(exc))
    json.dump(result, sys.stdout, ensure_ascii=False, indent=2)
    sys.stdout.write("\n")
    return 1 if args.check and not result["ok"] else 0


if __name__ == "__main__":
    raise SystemExit(main())
