// Sesh last: shows the space you were in before this one, as `herdr-sesh last` does.
// `mod.open` shows this mod as a 1px popup that runs `herdr-sesh last` and closes.
// Placed as a block in a layout, a copy also records every space switch, including
// ones made in the app itself, which herdr-sesh can't see on its own; that copy
// switches on `sesh-last.last` too.

import { View, div } from "gpui-kit";
import { exec, notify, closeOverlay, nextCommand, props, spaces } from "herdr";

// `exec` can't set environment variables, so sh selects the app backend. It runs with
// the user's real environment, so herdr-sesh, herdr-app and hc resolve on their PATH.
const SESH = `command -v herdr-sesh >/dev/null || { echo "herdr-sesh is not on your PATH. See the herdr-sesh Herdr app docs." >&2; exit 127; }
HERDR_SESH_BACKEND=app exec herdr-sesh "$@"`;

async function sesh(args) {
  const r = await exec("/bin/sh", ["-c", SESH, "sh", ...args]);
  if (r.code !== 0) throw new Error(r.stderr.trim() || `herdr-sesh ${args[0]} exited ${r.code}`);
}

function last() {
  return sesh(["last"]).catch((e) =>
    notify({ title: "Sesh last didn't switch", body: String(e?.message ?? e), level: "error" }));
}

const shownSpace = () => spaces().find((s) => s.active)?.id ?? null;

// herdr-app shows every space it creates, so `connect --no-focus` flashes its new
// space before herdr-sesh shows the old one again. Only a space that stays shown
// this long counts as a switch.
const SETTLE_MS = 500;

export default class SeshLast extends View {
  init(_props, cx) {
    const overlay = props()?.overlay;
    if (overlay) {
      cx.spawn(async () => {
        await last();
        closeOverlay(overlay);
      });
      return;
    }
    this.space = shownSpace();
    this.seen = this.space;
    this.changedAt = 0;
    this.recording = Promise.resolve();
    cx.spawn(async () => {
      for (;;) if ((await nextCommand()) === "last") await last();
    });
    // A native callback keeps the delay owned by this block, even when a popup
    // closes; the zero-width block need not render to observe navigation.
    cx.timer.every(100, (cx) => {
      this.track();
      if (this.changedAt && Date.now() - this.changedAt >= SETTLE_MS) {
        this.changedAt = 0;
        cx.spawn(() => this.settle());
      }
    });
  }

  track() {
    const space = shownSpace();
    if (space === this.seen) return;
    this.seen = space;
    this.changedAt = Date.now();
  }

  settle() {
    const space = shownSpace();
    if (!space || space === this.space) return;
    const from = this.space;
    this.space = space;
    if (!from) return;
    // Chained so quick switches reach the history in the order they happened.
    this.recording = this.recording
      .then(() => sesh(["app", "record-switch", from, space]))
      .then(() => { this.warned = false; })
      .catch((e) => {
        if (this.warned) return;
        this.warned = true;
        notify({ title: "Sesh couldn't record a space switch", body: String(e?.message ?? e), level: "warning" });
      });
    return this.recording;
  }

  render() {
    if (!props()?.overlay) this.track();
    return div();
  }
}
