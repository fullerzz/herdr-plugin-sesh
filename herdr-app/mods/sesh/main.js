// Sesh: opens the herdr-sesh picker, a terminal program, over the window. `mod.open`
// shows this mod as a 1px popup; it starts the picker in a herdr-core terminal, shows
// that terminal in a dialog (which closes when the picker exits) and closes itself.

import { View, div } from "gpui-kit";
import { hc, home, notify, openOverlay, closeOverlay, props } from "herdr";

// `herdr-sesh` from the user's login shell PATH. A failed run waits for Enter so its
// error stays readable; a pick or a cancel exits 0 and closes the dialog at once.
const PICKER = `HERDR_SESH_BACKEND=app herdr-sesh picker; s=$?; [ "$s" -eq 0 ] || { printf '\\nherdr-sesh exited %s. Press Enter to close.' "$s"; read -r _; }`;

export default class Sesh extends View {
  init(_props, cx) {
    const self = props()?.overlay;
    cx.spawn(async () => {
      try {
        const r = await hc(["run", "--json", "--bg", "--rm", "--no-parent", "--label", "sesh=picker", "--cwd", home(),
          "--", "/bin/sh", "-lc", PICKER]);
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
