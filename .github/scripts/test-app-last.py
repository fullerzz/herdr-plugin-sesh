"""Exercise the installed sesh-last mod in the running app, then restore focus."""

# Assertions are the checks in this standalone live regression script.
# ruff: noqa: S101

import json
import logging
import os
import shutil
import subprocess
import time
from pathlib import Path
from typing import Any

logger = logging.getLogger(__name__)
logging.basicConfig(level=logging.INFO, format="%(message)s")
executable = shutil.which("herdr-app")
assert executable, "herdr-app must be on PATH"


def app(command: str, *args: str) -> dict[str, Any]:
    result = subprocess.run(  # noqa: S603 - fixed app commands; arguments never pass through a shell.
        [executable, command, *args],
        check=True,
        capture_output=True,
        text=True,
        timeout=10,
    )
    return json.loads(result.stdout)


def run(command: str, **args: str) -> dict[str, Any]:
    return app("do", command, json.dumps(args))


def history() -> dict[str, Any]:
    return json.loads(history_path.read_text())


def check(label: str, target: str, previous: str) -> None:
    time.sleep(1.5)
    status = app("status")
    saved = history()
    assert status["space"] == target, (label, status)
    assert not status["overlays"], (label, status)
    assert not status["errors"], (label, status)
    assert saved["workspaces"][:2] == [target, previous], (label, saved)
    logger.info("PASS: %s", label)


original = app("status")
assert not original["overlays"], "Close overlays before running the live test"
app_home = Path(os.environ.get("HERDR_APP_HOME", Path.home() / ".config/herdr-app"))
layout = json.loads((app_home / "layouts" / f"{original['layout']}.json").read_text())


def has_recorder(node: dict[str, Any]) -> bool:
    return node.get("content", {}).get("mod") == "sesh-last" or any(
        has_recorder(child) for child in node.get("children", [])
    )


assert has_recorder(layout), "The current layout needs a sesh-last block"
history_path = Path(os.environ.get("XDG_STATE_HOME", Path.home() / ".local/state"))
history_path /= "herdr-sesh/herdr-app/history.json"
targets = [space for space in app("ls")["spaces"] if space["id"] != original["space"]]
assert targets, "Open at least two spaces before running the live test"
if not any(space["machine"] for space in targets):
    logger.info("NOTE: no other remote space is open; remote toggling not exercised")

try:
    for cycle in range(2):
        for space in targets:
            target = space["id"]
            label = f"{cycle + 1}: {space['name']} ({target})"
            run("space.select", id=target)
            check(f"{label} navigation", target, original["space"])
            run("mod.open", name="sesh-last")
            check(f"{label} last returns", original["space"], target)
            run("mod.open", name="sesh-last")
            check(f"{label} last toggles", target, original["space"])
            run("space.select", id=original["space"])
            check(f"{label} recorder still works", original["space"], target)

    before = history()
    run("space.select", id=targets[0]["id"])
    time.sleep(0.1)
    run("space.select", id=original["space"])
    time.sleep(1.5)
    assert history() == before, "A transient space changed history"
    logger.info("PASS: transient space ignored")
finally:
    run("space.select", id=original["space"])
    time.sleep(1.5)
    run("tab.select", id=original["tab"])
    run("pane.focus", pane=original["focused_pane"])
