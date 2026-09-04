"""Integration regression: release snapshots, latest, selector and redirects."""

# Assertions are the checks in this standalone regression script, not input validation.
# ruff: noqa: S101

import importlib.util
import json
import logging
import re
import tempfile
from pathlib import Path

logger = logging.getLogger(__name__)
logging.basicConfig(level=logging.INFO, format="%(message)s")

spec = importlib.util.spec_from_file_location("docs_versions", Path(__file__).with_name("build-docs-versions.py"))
builder = importlib.util.module_from_spec(spec)
spec.loader.exec_module(builder)

with tempfile.TemporaryDirectory() as directory:
    source = Path(directory) / "repo"
    source.mkdir()
    builder.git(source, "init", "--quiet")
    builder.git(source, "config", "user.name", "Docs test")
    builder.git(source, "config", "user.email", "docs@localhost")
    builder.git(source, "commit", "--quiet", "--allow-empty", "-m", "before wiki")
    builder.git(source, "tag", "v0.1.0")
    (source / "docs").mkdir()
    (source / "zensical.toml").write_text(
        '[project]\nsite_name = "Test"\nsite_url = "https://example.com/wiki/"\n'
        'repo_url = "https://github.com/example/test"\nedit_uri = "edit/main/docs/"\n'
        'nav = [{"Overview" = "index.md"}, {"Guide" = "guide.md"}, '
        '{"Benchmarks" = "development/benchmarks.md"}]\n'
    )
    (source / "docs/index.md").write_text("# Released documentation\n\n[Guide](guide.md)\n")
    (source / "docs/guide.md").write_text("# Released guide\n")
    (source / "docs/development").mkdir()
    (source / "docs/development/benchmarks.md").write_text("# Benchmarks\n")
    builder.git(source, "add", ".")
    builder.git(source, "commit", "--quiet", "-m", "release docs")
    builder.git(source, "tag", "-a", "v1.0.0", "-m", "release")
    # A same-named branch must never replace the release tag's snapshot.
    builder.git(source, "checkout", "--quiet", "-b", "v1.0.0")
    (source / "docs/index.md").write_text("# Latest documentation\n\n[Guide](guide.md)\n")
    builder.git(source, "add", ".")
    builder.git(source, "commit", "--quiet", "-m", "main docs")
    (source / "docs/guide.md").write_text("# Uncommitted preview guide\n")
    before = builder.git(source, "status", "--porcelain")
    output = Path(directory) / "site"
    builder.build(source, output)
    versions = json.loads((output / "versions.json").read_text())
    assert {item["version"] for item in versions} == {"latest", "v1.0.0"}
    release = (output / "v1.0.0/index.html").read_text()
    latest = (output / "latest/index.html").read_text()
    assert "Released documentation" in release
    assert "Latest documentation" not in release
    assert "Latest documentation" in latest
    assert "Uncommitted preview guide" in (output / "latest/guide/index.html").read_text()
    assert "Released guide" in (output / "v1.0.0/guide/index.html").read_text()
    assert 'href="guide/"' in release
    assert 'rel="canonical" href="https://example.com/wiki/v1.0.0/"' in release
    assert 'rel="canonical" href="https://example.com/wiki/latest/"' in latest
    runtime_config = json.loads(
        re.search(r'<script id="__config" type="application/json">(.*?)</script>', release).group(1)
    )
    assert runtime_config["version"] == {"default": "latest", "provider": "mike"}
    assert "edit/main/docs/" not in release
    assert 'href="latest/"' in (output / "index.html").read_text()
    for page, href in (
        ("guide/index.html", "../latest/guide/"),
        ("development/benchmarks/index.html", "../../latest/development/benchmarks/"),
    ):
        redirect = (output / page).read_text()
        assert f'href="{href}"' in redirect
        assert "window.location.replace(" in redirect
        assert "window.location.search + window.location.hash" in redirect
        assert (output / page).parent.joinpath(href).resolve().joinpath("index.html").is_file()
    assert builder.git(source, "status", "--porcelain") == before
    assert not builder.git(source, "branch", "--list", "gh-pages").strip()
    logger.info("Versioned documentation checks passed")
