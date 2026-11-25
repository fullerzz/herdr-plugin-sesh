# /// script
# requires-python = ">=3.13,<3.14"
# dependencies = ["vncdotool==1.3.0", "cryptography<48"]
# ///
"""Sample the clone's live framebuffer while Cua Driver operates the picker."""

import argparse
import json
import shutil
import subprocess
import threading
import time
from pathlib import Path
from typing import override
from urllib.parse import SplitResult, unquote, urlsplit

from vncdotool import api
from vncdotool.client import VNCDoToolClient, VNCDoToolFactory


def executable(name: str) -> str:
    path = shutil.which(name)
    if path is None:
        raise FileNotFoundError(f"Required recording tool not found: {name}")
    return path


def clone_display(lume: str, vm: str) -> SplitResult:
    # The validated clone name is a single argv element.
    (info,) = json.loads(subprocess.check_output([lume, "get", vm, "-f", "json"]))  # noqa: S603
    url = urlsplit(info["vncUrl"])
    if info["sharedDirectories"] or url.hostname != "127.0.0.1":
        raise ValueError("expected an isolated clone with a loopback display")
    return url


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("vm")
    parser.add_argument("output", type=Path)
    parser.add_argument("--pilot", action="store_true")
    parser.add_argument("--surface", choices=("tui", "app"), default="tui")
    args = parser.parse_args()
    if not args.vm.startswith("sesh-demo-take-"):
        parser.error("expected a demo recording clone")
    lume = executable("lume")
    ffmpeg = executable("ffmpeg")
    url = clone_display(lume, args.vm)

    args.output.mkdir(parents=True, exist_ok=False)
    first_frame = threading.Event()
    latest = None
    # The fixed app or Ghostty frame begins at (85,64). Inset its rounded corners.
    crop = (93, 72, 1497, 896)
    width, height = crop[2] - crop[0], crop[3] - crop[1]

    class FrameClient(VNCDoToolClient):
        @override
        def vncConnectionMade(self) -> None:
            super().vncConnectionMade()
            self.framebufferUpdateRequest()

        @override
        def commitUpdate(self, rectangles: list[tuple[int, int, int, int]] | None = None) -> None:
            nonlocal latest
            super().commitUpdate(rectangles)
            if self.screen is not None:
                latest = self.screen.crop(crop).convert("RGB").tobytes()
                first_frame.set()
            self.framebufferUpdateRequest(incremental=True)

    class FrameFactory(VNCDoToolFactory):
        protocol = FrameClient

    client = api.connect(
        f"127.0.0.1::{url.port}",
        password=unquote(url.password or url.username),
        factory_class=FrameFactory,
        timeout=10,
    )
    try:
        if not first_frame.wait(10):
            raise RuntimeError("No guest framebuffer received")
        process = subprocess.Popen(  # noqa: S603 - fixed encoder arguments and local output path
            [
                ffmpeg,
                "-nostdin",
                "-v",
                "error",
                "-f",
                "rawvideo",
                "-pixel_format",
                "rgb24",
                "-video_size",
                f"{width}x{height}",
                "-framerate",
                "30",
                "-i",
                "pipe:0",
                "-c:v",
                "libx264rgb",
                "-crf",
                "0",
                "-preset",
                "ultrafast",
                str(args.output / "master.mkv"),
            ],
            stdin=subprocess.PIPE,
        )
        try:
            started = time.monotonic()
            mode = "pilot" if args.pilot else "full"
            script = "actions.sh" if args.surface == "tui" else "desktop/app-actions.sh"
            command = [lume, "ssh", args.vm, f"~/picker-demo/{script} {mode}"]
            with subprocess.Popen(command) as actions:  # noqa: S603 - fixed guest scripts and modes
                index = 0
                while actions.poll() is None:
                    if time.monotonic() - started > 75:
                        actions.terminate()
                        raise TimeoutError("Guest storyboard exceeded 75 seconds")
                    process.stdin.write(latest)
                    index += 1
                    time.sleep(max(0, started + index / 30 - time.monotonic()))
                if actions.returncode:
                    raise RuntimeError(f"Guest storyboard failed ({actions.returncode})")
        finally:
            process.stdin.close()
            if process.wait():
                raise RuntimeError("FFmpeg could not finish the master")
    finally:
        client.disconnect()
        api.shutdown()


if __name__ == "__main__":
    main()
