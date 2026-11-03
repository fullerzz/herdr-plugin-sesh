// Sesh: opens the herdr-sesh picker, a terminal program, over the window. `mod.open`
// shows this mod as a 1px popup; it starts the picker in a herdr-core terminal, shows
// that terminal in a dialog (which closes when the picker exits) and closes itself.

import { View, div } from "gpui-kit";
import { exec, hc, home, notify, openOverlay, closeOverlay, props } from "herdr";

const LABEL = "sesh=picker";

// Runs the picker at the absolute path in $1. A failed run waits for Enter so its error
// stays readable; a pick or a cancel exits 0 and closes the dialog at once.
const PICKER = `HERDR_SESH_BACKEND=app "$1" picker; s=$?; [ "$s" -eq 0 ] || { printf '\\nherdr-sesh exited %s. Press Enter to close.' "$s"; read -r _; }`;

// `exec` runs with the user's real environment, including their login shell's PATH,
// which the core's own terminals may not have.
async function findPicker() {
  const r = await exec("/bin/sh", ["-c", "command -v herdr-sesh"]);
  const path = r.stdout.trim();
  if (r.code !== 0 || !path) throw new Error("herdr-sesh is not on your PATH. See the herdr-sesh Herdr app docs.");
  return path;
}

// Esc reaches the picker, which exits and closes its dialog. Any other way of closing
// the dialog leaves the picker running unseen, so a new open ends earlier ones first.
async function endEarlierPickers() {
  const r = await hc(["ls", "--json", "--label", LABEL]);
  if (r.code !== 0) return;
  for (const t of JSON.parse(r.stdout).terminals ?? []) await hc(["kill", t.address]);
}

export default class Sesh extends View {
  init(_props, cx) {
    const self = props()?.overlay;
    cx.spawn(async () => {
      try {
        const picker = await findPicker();
        await endEarlierPickers();
        const r = await hc(["run", "--json", "--bg", "--rm", "--no-parent", "--label", LABEL, "--cwd", home(),
          "--", "/bin/sh", "-c", PICKER, "sh", picker]);
        if (r.code !== 0) throw new Error(r.stderr.trim() || `hc run exited ${r.code}`);
        const { address } = JSON.parse(r.stdout);
        openOverlay({ kind: "dialog", modal: true, width: 0.7, height: 0.75,
          layout: { type: "block", content: { terminal: address } } });
      } catch (e) {
        notify({ title: "Sesh picker didn't start", body: String(e?.message ?? e), level: "error" });
      }
      if (self) closeOverlay(self);
    });
  }

  render() {
    return div();
  }
}
