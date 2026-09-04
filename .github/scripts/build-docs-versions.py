"""Build a complete Pages artifact without pushing a deployment branch."""

import io
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tarfile
import tempfile
import tomllib
from contextlib import chdir

from mike import commands, utils


def git(repo, *args):
    return subprocess.check_output(["git", "-C", str(repo), *args])


def extract(repo, ref, destination, *paths):
    archive = git(repo, "archive", ref, *paths)
    with tarfile.open(fileobj=io.BytesIO(archive)) as files:
        files.extractall(destination, filter="data")


def build(source, output):
    source, output = source.resolve(), output.resolve()
    tags = git(source, "tag", "--list", "v*", "--sort=version:refname").decode().splitlines()
    versions = []
    for tag in tags:
        if not re.fullmatch(r"v[0-9A-Za-z][0-9A-Za-z._+-]*", tag):
            raise ValueError(f"Unsafe documentation version: {tag}")
        ref = f"refs/tags/{tag}"
        paths = git(source, "ls-tree", "--name-only", ref).decode().splitlines()
        if "zensical.toml" in paths and "docs" in paths:
            versions.append((tag, ref))
        else:
            print(f"Skipping {tag}: predates the Zensical wiki", flush=True)

    # ponytail: rebuild all tags; cache immutable snapshots if release builds get slow.
    with tempfile.TemporaryDirectory(prefix="herdr-sesh-docs-") as directory:
        work = Path(directory)
        git(work, "init", "--quiet")
        git(work, "config", "user.name", "Docs builder")
        git(work, "config", "user.email", "docs@localhost")
        git(work, "commit", "--quiet", "--allow-empty", "-m", "docs: initialize build")
        for version, ref in [*versions, ("latest", None)]:
            snapshot = work / "snapshot"
            shutil.rmtree(snapshot, ignore_errors=True)
            snapshot.mkdir()
            if ref:
                extract(source, ref, snapshot, "docs", "zensical.toml")
            else:
                shutil.copytree(source / "docs", snapshot / "docs")
                shutil.copy2(source / "zensical.toml", snapshot / "zensical.toml")

            # git archive preserves LFS pointers; hydrate them from fetched objects.
            for path in (snapshot / "docs").rglob("*"):
                if path.is_file():
                    with path.open("rb") as file:
                        pointer = file.read(1024)
                    if pointer.startswith(b"version https://git-lfs.github.com/spec/v1\n"):
                        result = subprocess.run(
                            ["git", "-C", str(source), "lfs", "smudge"],
                            input=pointer, capture_output=True, check=True,
                        )
                        if result.stdout.startswith(b"version https://git-lfs.github.com/spec/v1\n"):
                            raise RuntimeError(f"Unresolved LFS pointer: {path}")
                        path.write_bytes(result.stdout)

            config = tomllib.loads((snapshot / "zensical.toml").read_text())["project"]
            config.setdefault("extra", {})["version"] = {"provider": "mike", "default": "latest"}
            config["edit_uri"] = "edit/main/docs/" if ref is None else ""
            # JSON is YAML: retain each tag's config without adding a TOML writer.
            config_file = snapshot / "mkdocs.yml"
            config_file.write_text(json.dumps(config))
            print(f"Building documentation: {version}", flush=True)
            with chdir(work):
                cfg = utils.load_config(str(config_file))
                with commands.deploy(cfg, version, message=f"docs: build {version}"):
                    subprocess.run(
                        ["zensical", "build", "--clean", "--strict", "-f", str(config_file)],
                        env={**os.environ, "MIKE_DOCS_VERSION": version}, check=True,
                    )
        with chdir(work):
            commands.set_default("latest")
        # Replace the previous artifact only after every version has built successfully.
        if output.exists():
            shutil.rmtree(output)
        output.mkdir(parents=True)
        extract(work, "refs/heads/gh-pages", output)
        # Preserve unversioned page links using mike's query/fragment-aware redirect.
        template = commands._redirect_template()
        for page in (output / "latest").rglob("*.html"):
            relative = page.relative_to(output / "latest")
            redirect = output / relative
            if redirect.exists() or relative == Path("404.html"):
                continue
            target = page.parent if page.name == "index.html" else page
            href = Path(os.path.relpath(target, redirect.parent)).as_posix()
            if page.name == "index.html":
                href += "/"
            redirect.parent.mkdir(parents=True, exist_ok=True)
            redirect.write_text(template.render(href=href))


if __name__ == "__main__":
    root = Path(__file__).resolve().parents[2]
    build(root, root / "site")
