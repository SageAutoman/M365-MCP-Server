"""Build the native Windows packages with dependency notices and SHA-256 checksums."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import zipfile


ROOT = Path(__file__).resolve().parents[1]
NATIVE = ROOT / "native"


def json_stream(value: str):
    decoder = json.JSONDecoder()
    while value.strip():
        value = value.lstrip()
        item, index = decoder.raw_decode(value)
        yield item
        value = value[index:]


def dependency_notices(go: str, environment: dict[str, str]) -> str:
    result = subprocess.run(
        [go, "list", "-deps", "-json", "."], cwd=NATIVE, env=environment,
        check=True, text=True, encoding="utf-8", capture_output=True,
    )
    modules = {}
    for package in json_stream(result.stdout):
        module = package.get("Module")
        if module and not module.get("Main"):
            modules[module["Path"]] = module
    notices = []
    for name, module in sorted(modules.items()):
        directory = Path(module["Dir"])
        files = sorted({
            path for pattern in ("LICENSE*", "LICENCE*", "COPYING*", "NOTICE*")
            for path in directory.glob(pattern) if path.is_file()
        })
        if not files:
            raise RuntimeError(f"Missing dependency license: {name}")
        notices.append(f"{name} {module['Version']}\n" + "=" * 72 + "\n")
        for path in files:
            notices.append(f"{path.name}\n{path.read_text(encoding='utf-8', errors='replace')}\n")
    return "\n".join(notices)


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--arch", action="append", choices=("amd64", "arm64"))
    args = parser.parse_args()
    go = shutil.which("go")
    if not go:
        raise SystemExit("Install the Go version declared in native/go.mod and add go to PATH.")
    output = ROOT / "artifacts" / "windows-local-go"
    output.mkdir(parents=True, exist_ok=True)
    checksums = []
    for arch in args.arch or ("amd64", "arm64"):
        environment = {**os.environ, "GOOS": "windows", "GOARCH": arch, "CGO_ENABLED": "0"}
        name = f"m365-mcp-windows-local-go-{arch}"
        executable = output / f"{name}.exe"
        subprocess.run([go, "build", "-trimpath", "-buildvcs=false", "-o", str(executable), "."],
                       cwd=NATIVE, env=environment, check=True)
        notices = dependency_notices(go, environment)
        archive = output / f"{name}.zip"
        with zipfile.ZipFile(archive, "w", compression=zipfile.ZIP_DEFLATED) as package:
            package.write(executable, "m365-mcp-windows-local-go.exe")
            package.writestr("README.md", (ROOT / "docs/windows-local-go.md").read_text(encoding="utf-8")
                             .replace("zh-CN/windows-local-go.md", "README.zh-CN.md"))
            package.writestr("README.zh-CN.md", (ROOT / "docs/zh-CN/windows-local-go.md").read_text(encoding="utf-8")
                             .replace("../windows-local-go.md", "README.md"))
            package.writestr("THIRD_PARTY_NOTICES.txt", notices)
            package.write(ROOT / "LICENSE", "LICENSE")
        for path in (executable, archive):
            digest = hashlib.sha256(path.read_bytes()).hexdigest()
            checksums.append(f"{digest}  {path.name}")
        print(archive.relative_to(ROOT))
    (output / "SHA256SUMS.txt").write_text("\n".join(checksums) + "\n", encoding="ascii")


if __name__ == "__main__":
    main()
