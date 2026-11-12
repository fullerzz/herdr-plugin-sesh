#!/usr/bin/env -S uv run --frozen python
"""Drive the built CLI with private fixtures and an unavailable Herdr boundary."""

# CLI status output is part of the verification protocol.
# ruff: noqa: T201

import argparse
import hashlib
import json
import shutil
import subprocess
import uuid
from pathlib import Path


def digest(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def write_json(path: Path, value: object) -> None:
    path.write_text(json.dumps(value, indent=2) + "\n")


def environment(runtime: Path) -> dict[str, str]:
    return {
        "PATH": "/usr/bin:/bin",
        "HOME": str(runtime / "home"),
        "XDG_CONFIG_HOME": str(runtime / "config"),
        "XDG_STATE_HOME": str(runtime / "state"),
        "XDG_CACHE_HOME": str(runtime / "cache"),
        "HERDR_SESH_CONFIG": str(runtime / "config.toml"),
        "HERDR_SESH_BACKEND": "herdr",
        "HERDR_BIN_PATH": "/usr/bin/false",
        "GIT_CONFIG_NOSYSTEM": "1",
        "GIT_CEILING_DIRECTORIES": str(runtime),
        "TERM": "xterm-256color",
        "LANG": "en_US.UTF-8",
    }


def launch(evidence: Path) -> None:
    repo = Path(__file__).resolve().parents[4]
    binary = repo / "bin/herdr-sesh"
    if not binary.is_file():
        raise RuntimeError("Run just build from the checkout first")
    evidence.mkdir(parents=True, exist_ok=False)
    runtime = evidence / "runtime"
    runtime.mkdir()
    token = uuid.uuid4().hex
    (runtime / ".owner").write_text(token)
    write_json(evidence / "manifest.json", {"token": token, "binary_sha256": digest(binary)})
    shutil.copy2(binary, evidence / "herdr-sesh")
    for folder in ("home", "config", "state", "cache", "alpha", "beta"):
        (runtime / folder).mkdir()
    (runtime / "alpha/README.md").write_text("Alpha verification fixture\n")
    config = 'version = 1\n[list]\ncache = false\n[picker]\nshow_preview = false\nherdr_theme_inherit = false\n[workspace_defaults]\npreview = "/bin/ls -1 {}"\n'
    for name in ("alpha", "beta"):
        config += f'\n[[workspace]]\nname = "verify-{name}"\npath = {json.dumps(str(runtime / name))}\n'
    (runtime / "config.toml").write_text(config)
    (evidence / "fixture.toml").write_text(config)
    for label, args in (("revision", ["git", "rev-parse", "HEAD"]), ("source-diff", ["git", "diff", "HEAD"])):
        result = subprocess.run(args, cwd=repo, capture_output=True, check=True)  # noqa: S603 -- fixed git argv above
        (evidence / f"{label}.txt").write_bytes(result.stdout)
    write_json(evidence / "environment.json", environment(runtime))


def check(evidence: Path) -> Path:
    manifest = json.loads((evidence / "manifest.json").read_text())
    runtime = evidence / "runtime"
    if runtime.is_symlink() or (runtime / ".owner").read_text() != manifest["token"]:
        raise RuntimeError("Runtime ownership marker mismatch")
    if digest(evidence / "herdr-sesh") != manifest["binary_sha256"]:
        raise RuntimeError("Run binary changed; launch a new run")
    return runtime


def cli(evidence: Path, label: str, args: list[str], tty: bool = False) -> bytes:
    runtime = check(evidence)
    if not label or not all(c.isalnum() or c in "-_" for c in label):
        raise ValueError("Use an alphanumeric evidence label")
    prefix = evidence / label
    action = prefix.with_suffix(".command.json")
    if action.exists():
        raise ValueError("Evidence label already used; choose a new label")
    command = [str(evidence / "herdr-sesh"), *args]
    write_json(action, command)
    try:
        result = subprocess.run(  # noqa: S603 -- hash-checked binary, explicit user CLI args, no shell
            command,
            cwd=runtime,
            env=environment(runtime),
            capture_output=not tty,
            timeout=180 if tty else 30,
            check=False,
        )
    except subprocess.TimeoutExpired as exc:
        prefix.with_suffix(".stdout").write_bytes(exc.stdout or b"")
        prefix.with_suffix(".stderr").write_bytes(exc.stderr or b"")
        prefix.with_suffix(".exit").write_text("timeout\n")
        raise
    if not tty:
        prefix.with_suffix(".stdout").write_bytes(result.stdout)
        prefix.with_suffix(".stderr").write_bytes(result.stderr)
    prefix.with_suffix(".exit").write_text(f"{result.returncode}\n")
    if result.returncode:
        raise RuntimeError(f"{label} exited {result.returncode}; inspect evidence")
    return result.stdout or b""


def catalog(evidence: Path) -> None:
    runtime = check(evidence)
    before = digest(runtime / "config.toml")
    listed = json.loads(cli(evidence, "list-json", ["list", "--json"]))
    picked = json.loads(cli(evidence, "picker-json", ["picker", "--json"]))
    expected = {"verify-alpha", "verify-beta"}
    for rows in (listed, picked):
        if len(rows) != 2 or {r["name"] for r in rows} != expected:
            raise RuntimeError(f"Unexpected catalog: {rows}")
        if any(r["source"] != "config" or r.get("machine") for r in rows):
            raise RuntimeError("External source leaked into catalog")
    text = cli(evidence, "list-text", ["list"]).decode()
    if not all(name in text for name in expected):
        raise RuntimeError("Text list omitted a seeded workspace")
    preview = cli(evidence, "preview-alpha", ["preview", "verify-alpha"])
    if preview.strip() != b"README.md":
        raise RuntimeError(f"Unexpected directory preview: {preview!r}")
    cli(evidence, "config-valid", ["config", "validate"])
    if before != digest(runtime / "config.toml"):
        raise RuntimeError("Read-only drive modified configuration")
    write_json(
        evidence / "catalog-proof.json",
        {
            "passed": True,
            "names": sorted(expected),
            "config_unchanged": True,
            "backend": "unavailable; live integration not exercised",
        },
    )


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("launch", "doctor", "catalog", "cli", "tui", "cleanup"))
    parser.add_argument("evidence", type=Path)
    parser.add_argument("args", nargs=argparse.REMAINDER)
    opts = parser.parse_args()
    evidence = opts.evidence.absolute()
    if opts.action == "launch":
        launch(evidence)
    elif opts.action == "cleanup":
        if (evidence / "cleanup.json").exists() and not (evidence / "runtime").exists():
            print("Already cleaned")
            return
        runtime = check(evidence)
        shutil.rmtree(runtime)
        write_json(evidence / "cleanup.json", {"runtime_removed": not runtime.exists(), "evidence_retained": True})
    elif opts.action == "doctor":
        runtime = check(evidence)
        result = subprocess.run(  # noqa: S603 -- hash-checked binary with fixed version argument
            [str(evidence / "herdr-sesh"), "--version"],
            cwd=runtime,
            env=environment(runtime),
            capture_output=True,
            check=True,
            timeout=10,
        )
        if not (runtime / "config.toml").is_file():
            raise RuntimeError("Fixture config missing")
        print(result.stdout.decode().strip(), "— private fixture ready; Herdr intentionally unavailable")
    elif opts.action == "catalog":
        catalog(evidence)
    else:
        if len(opts.args) < 2:
            parser.error("cli/tui requires LABEL COMMAND [ARGS...]")
        cli(evidence, opts.args[0], opts.args[1:], tty=opts.action == "tui")
    print(f"{opts.action}: {evidence}")


if __name__ == "__main__":
    main()
