// Sesh: opens the herdr-sesh picker, a terminal program, over the window. `mod.open`
// shows this mod as a 1px popup; it starts the picker in a herdr-core terminal, closes
// itself and shows that terminal in a dialog (which closes when the picker exits).

import { View, div } from "gpui-kit";
import { exec, hc, home, notify, openOverlay, closeOverlay, props } from "herdr";

const LABEL = "sesh=picker";

// Runs the picker at the absolute path in $1 with the login PATH in $2, so the programs
// it runs (zoxide, herdr-app, hc) resolve as they do in the user's shell. A failed run
// waits for Enter so its error stays readable; a pick or a cancel exits 0 at once.
const PICKER = `PATH="\${2:-$PATH}" HERDR_SESH_BACKEND=app "$1" picker; s=$?; [ "$s" -eq 0 ] || { printf '\\nherdr-sesh exited %s. Press Enter to close.' "$s"; read -r _; }`;

// `exec` runs with the user's real environment, including their login shell's PATH,
// which the core's own terminals may not have. Returns herdr-sesh's path and that PATH.
async function findPicker() {
  const r = await exec("/bin/sh", ["-c", 'command -v herdr-sesh && printf "%s" "$PATH"']);
  const [path, loginPath] = r.stdout.split("\n");
  if (r.code !== 0 || !path) throw new Error("herdr-sesh is not on your PATH. See the herdr-sesh Herdr app docs.");
  return { path, loginPath: loginPath ?? "" };
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
      let address;
      try {
        const { path, loginPath } = await findPicker();
        await endEarlierPickers();
        const r = await hc(["run", "--json", "--bg", "--rm", "--no-parent", "--label", LABEL, "--cwd", home(),
          "--", "/bin/sh", "-c", PICKER, "sh", path, loginPath]);
        if (r.code !== 0) throw new Error(r.stderr.trim() || `hc run exited ${r.code}`);
        ({ address } = JSON.parse(r.stdout));
      } catch (e) {
        notify({ title: "Sesh picker didn't start", body: String(e?.message ?? e), level: "error" });
      }
      // Closing a popup hands the keyboard back to whatever had it before the popup, so
      // this popup closes before the dialog opens; the other order leaves the picker deaf.
      if (self) closeOverlay(self);
      if (address) openOverlay({ kind: "dialog", modal: true, width: 0.7, height: 0.75,
        layout: { type: "block", content: { terminal: address } } });
    });
  }

  render() {
    return div();
  }
}
