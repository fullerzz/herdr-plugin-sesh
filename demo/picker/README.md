# Record the Sesh picker

The TUI take opens the installed Herdr plugin in Ghostty. The desktop take opens the Sesh mod in Herdr.app. Both use the same macOS guest and Catppuccin Mocha theme. The picker lists open Herdr workspaces, a configured project, and directories seeded from [the zoxide fixture](zoxide-projects.txt). Each take filters to `atlas`, selects `atlas-api`, scrolls its live terminal preview, widens the divider, and switches to the workspace. The recorder checks the resulting focus with the corresponding Herdr API.

## Re-record

From the repository root on the Apple Silicon recording Mac, run one of these commands. The second argument can be a commit, tag, or branch that contains the chosen integration.

```sh
just record-picker-demo                # both integrations on one clone, using HEAD
just record-picker-demo tui HEAD       # Herdr plugin in Ghostty
just record-picker-demo app HEAD       # Herdr.app mod
```

The recipe clones the stopped `sesh-demo-base`, builds the plugin from a clean export of the selected revision, and uses the recording scripts in the current checkout. It shuts down its working clone on exit and retains the disk for inspection. It writes `master.mkv`, `picker-demo.gif`, and `contact-sheet.png` in `scratch/picker-demo/<clone>/<tui-or-app>/`. The master preserves the RGB framebuffer at 30 fps. The looping GIF uses an adaptive palette at 15 fps. GIF supports 256 colors per color table.

Review the contact sheet and play the GIF at 700 pixels wide. During the scroll, check that later route checks visibly replace the first rows before the divider moves. Check both ends of the loop for first-login prompts, the directory and live previews for fixture-only text, and the final workspace for the 30 passing route checks. After review, copy the TUI GIF to `docs/assets/picker-demo.gif` and the desktop GIF to `docs/assets/picker-demo-app.gif`. Both paths use Git LFS. The recipe leaves replacement of those assets to the reviewer. When you are finished inspecting a take, its stopped VM can be removed with `lume delete <clone>`.

## Recording Mac and template

The prepared local template is sufficient for routine takes. To rebuild it on a different Mac:

1. Install Lume and the host export tools with `brew install lume ffmpeg jq`. Run `mise install` in this repository for Go, just, and uv. Allow disk space for the macOS restore and spare recording clones. [Lume quickstart](https://cua.ai/docs/lume/quickstart)
2. Create `sesh-demo-base` using the Tahoe unattended preset, a 1600×1000 display, 4 CPUs, 8 GiB RAM and a 64 GiB disk. Use the local `lume` user, automatic login, SSH, NAT and zero shared folders. Keep the guest signed out of Apple services. Complete first login in the guest viewer. The template used here ran macOS 26.6.2 (25G83).

   ```sh
   lume create sesh-demo-base --ipsw /path/to/Tahoe_Restore.ipsw --unattended tahoe \
     --cpu 4 --memory 8GB --disk-size 64GB --display 1600x1000 --network nat
   lume run sesh-demo-base --display vnc
   ```

3. Install the signed CuaDriver app and its CLI in the guest. Run `~/.local/bin/cua-driver telemetry disable`, then `~/.local/bin/cua-driver permissions grant`. Grant the guest app Accessibility and Screen Recording. Approve its System Events and direct screen-capture prompts when exercising the app list and screenshot. Confirm `permissions status --json` reports Accessibility and Screen Recording from the guest daemon. [Driver installation](https://cua.ai/docs/cua-driver/quickstart) · [VM permissions](https://cua.ai/docs/cua-driver/guides/vms-and-remote)
4. Install Ghostty in `/Applications`, the Retina and Bold faces of FiraCode Nerd Font, the Xcode Command Line Tools (Git and Swift), Herdr TUI and zoxide in the guest. The command-line programs must resolve under `~/.local/bin` or the macOS system path. The tested versions were Ghostty 1.4.0, Herdr TUI 0.9.5, CuaDriver 0.62.1 and zoxide 0.10.0. Shut the base down before cloning.
5. For desktop takes, the host needs signed Herdr.app and Herdr Core bundles. The tested versions were 0.1.18 and 0.1.22. The workflow copies them from `~/Applications/Herdr.app` and `~/.local/share/herdr-core/active/Herdr Core.app`. Override those paths with `HERDR_DEMO_APP` and `HERDR_DEMO_CORE`. Both demos use the guest's FiraCode Nerd Font.

## Fixtures and capture

`seed.sh` creates local Git repositories with a synthetic author and fixed commit dates. `scene.sh` adds a linked worktree and verifies the names, paths, and sources of every picker entry, including each directory listed in `zoxide-projects.txt`. The app scene creates real local spaces and compares the app catalog with that same verified list. `check-routes.sh` supplies synthetic terminal output. No personal Sesh catalog, workspace history, app profile, or repository content is needed.

The driver fixes the window at 1420×840 on a 2200×1200 desktop, leaving the notification area outside the crop. The recorder samples the clone's loopback VNC framebuffer and crops to the window interior (1404×824). Keyboard input goes to the actual picker terminal. Both takes hide the CuaDriver session cursor before opening the picker. CuaDriver scrolls the TUI preview by dragging its scrollbar and the desktop preview with the mouse wheel. Both takes then drag the divider to widen the preview. The scripts verify each transition in the picker terminal and the final selection through the host application's API. If the font, frame, or picker layout changes, recalibrate the window-local pointer coordinates in `actions.sh` and `app-actions.sh` against a fresh guest screenshot.
