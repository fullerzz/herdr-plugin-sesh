# Remote workspaces (experimental)

Browse and switch between Herdr workspaces on explicitly configured machines.
Each machine runs an opt-in `herdr-sesh` daemon that shares workspace metadata;
files, running panes, terminal output, and credentials stay on their machines.

The native picker groups **local Herdr workspaces first, remote Herdr workspaces
second, then other sources**, including while searching. Remote rows show their
machine ID. See [Search ranking](config/picker.md#search-ranking) for sorting within
these groups.

Press **F3** in the native picker to show only remote Herdr workspaces; press it
again to show all sources. The search text still filters the visible rows. The
heading shows the active mode, and reopening the picker resets it to all sources.
If preview cycling is bound to F3, use **Ctrl+G** instead. Offline remote rows
remain visible in this mode.

## Configure each machine

Remote support is **disabled by default**. Installing or updating the plugin,
opening the picker, and checking status do not create a service or enable it.
First follow [Run the binary directly](commands.md#run-the-binary-directly) to
set `sesh_bin` in each terminal; plugin installation does not add `herdr-sesh` to
`PATH`. The local commands below use that absolute binary path. After preparing
binaries and SSH below, run this on each participating machine:

```sh
"$sesh_bin" remote enable
"$sesh_bin" remote status
```

On first use, a terminal form asks for the machine ID, Herdr session, and optional
peers. Set `ACCESSIBLE=1` for the screen-reader-friendly form. Canceling setup
does not write configuration or install a service. Existing configurations are
reused without prompting; for unattended setup, create `remote.toml` first.
The command installs and starts a user service and verifies the local daemon
socket. Unavailable sessions and peers are reported separately; the service
does not create Herdr sessions or configure remote machines.

Only one `remote enable` or `remote disable` operation can run per user at a time;
if another is in progress, wait for it to finish and retry. If configuration
changes while the setup form is open, enable stops and preserves those changes.
Config writes also share an adjacent `.lock` file; leave it in place. This lock
coordinates `herdr-sesh` writers, but unrelated editors do not honor it, so avoid
editing the file while an enable or disable operation is saving it.

### 1. Prepare binaries and SSH

Install Herdr and the Sesh plugin on every participating machine, using the same
`herdr-sesh` version. Make both `herdr` and `herdr-sesh` available on each SSH
user's **noninteractive `PATH`**. Plugin installation alone does not put
`herdr-sesh` on `PATH`; see [where to find the binary](commands.md#run-the-binary-directly).
Running `./bin/herdr-sesh` locally does not make it available to incoming SSH commands.

Complete SSH authentication and host-key approval in a normal terminal, then
check every configured direction with the actual destination username:

```sh
# From the laptop to the server:
ssh -o BatchMode=yes -o ForwardAgent=no serveruser@buildbox \
  'command -v herdr && command -v herdr-sesh && herdr-sesh --version'

# From the server to the laptop:
ssh -o BatchMode=yes -o ForwardAgent=no laptopuser@laptop-host \
  'command -v herdr && command -v herdr-sesh && herdr-sesh --version'
```

Replace these example users and hosts. The usernames may differ between
machines. SSH aliases are also supported; configure users, ports, identity files,
and jump hosts in `~/.ssh/config`. Peer connections disable agent forwarding,
so each initiating machine needs its own usable authentication method.

For zsh, put required `PATH` and config environment exports in `~/.zshenv`, not
only `~/.zshrc`. Keep noninteractive shell startup silent: output on stdout
interferes with the daemon protocol.

### 2. Configure `remote.toml`

`remote enable` creates this file during initial setup. You can also create or
edit it manually using the examples below, then run `remote enable` to apply it.

This is separate from the normal picker `config.toml` and has no `version` key.
The first applicable location wins:

| Priority | Location |
| --- | --- |
| 1 | The file named by `HERDR_SESH_REMOTE_CONFIG` |
| 2 | `$XDG_CONFIG_HOME/herdr-sesh/remote.toml`, when `XDG_CONFIG_HOME` is nonempty |
| 3 (Linux) | `~/.config/herdr-sesh/remote.toml` |
| 3 (macOS) | `~/Library/Application Support/herdr-sesh/remote.toml` |

`XDG_CONFIG_HOME` must be absolute. A missing file at priority 2 or 3 disables
remote support; lookup does not fall through to another location. A missing
explicit `HERDR_SESH_REMOTE_CONFIG` file is an error. `enabled = false` also
disables the feature.

Use the same config environment for the daemon, picker, attach wrapper, and
incoming SSH commands. Local overrides are not forwarded to peers. For example,
a macOS config under `~/.config` requires `XDG_CONFIG_HOME` to be set in the
noninteractive environment too. Service managers need their own environment settings.

Create reciprocal configurations, replacing the example SSH destinations:

=== "Laptop"

    ```toml
    enabled = true
    id = "laptop"
    herdr_session = "default"

    [[peers]]
    id = "server"
    ssh = "serveruser@buildbox"
    session = "default"
    ```

=== "Server"

    ```toml
    enabled = true
    id = "server"
    herdr_session = "default"

    [[peers]]
    id = "laptop"
    ssh = "laptopuser@laptop-host"
    session = "default"
    ```

Each peer's `id` must equal that machine's top-level `id`, and its `session` must
match that daemon's `herdr_session`. Session names default to `default` when
omitted. Each daemon represents one Herdr session. Optional `label` fields do
not replace the machine IDs shown in picker rows. Never put passwords or private
keys in this file.

Every machine discovers only its configured peers: there is no transitive
sharing. Switching back requires the server to discover the laptop over SSH.
The attach wrapper's own config must also include every destination it may
switch to; the daemon rejects selections outside that allowlist.

## Run the daemon

### 3. Start and check each daemon

Start the configured Herdr session on each machine, then enable the background
service:

```sh
"$sesh_bin" remote enable
"$sesh_bin" remote status
```

For troubleshooting or a manually managed installation, run in the foreground:

```sh
"$sesh_bin" daemon serve
```

Leave this foreground process running only when using manual startup. Stop a
manually started daemon before enabling the managed service. On each machine:

```sh
"$sesh_bin" daemon list
"$sesh_bin" list
```

Confirm both machines and their workspaces appear without stale/offline status
or `last_error` before testing a switch. The daemon logs peer failures and
recovery. Rerun `remote enable` after changing configuration, upgrading the binary,
or changing the environment needed by the service. Restart attach wrappers after
configuration changes. Use a stable installation path; a service cannot restart
a binary that has been moved or deleted.

Neither installing the plugin nor opening the picker starts a daemon
automatically. Only `remote enable` installs the user service. The daemon does not
start a missing Herdr session, change SSH configuration, or update remote software.

## Switching before native client APIs

### 4. Launch through the attach wrapper

From an interactive **outer terminal, outside Herdr**, run:

```sh
"$sesh_bin" attach
# Or start on a configured peer:
"$sesh_bin" attach server
```

Open the Sesh picker normally and select a remote workspace. The wrapper checks
and focuses the destination, then replaces its foreground Herdr client. The old
server and its running panes stay alive. Open the picker on the destination to
switch back. Herdr's remote attachment may present its normal setup prompts on
first use.

This wrapper is the temporary bridge until native client switching APIs exist.
A plain `herdr` launch does not register a controller. Use one interactive
switching client: only one wrapper can control a machine's daemon at a time,
and picker requests from other clients on that machine target that wrapper.

Remote rows obey the blacklist and offer metadata previews. Closing remote
workspaces and applying local startup/layout settings to them are disabled.
Previous-workspace history stays local to each Herdr session. Offline entries
remain visible with their last known metadata, but cannot be selected for switching.

## Inspect and troubleshoot

Start with:

```sh
"$sesh_bin" remote doctor
```

The command prints `OK`, `FAIL`, or `SKIP` for configuration, background service,
daemon connectivity and identity, catalog health, and each configured peer's SSH
access. Failures include suggested repairs and produce a nonzero exit status.
Independent checks continue after a failure, so a stopped local daemon does not
prevent checking peer SSH access.

Doctor does not change configuration, start services, or repair anything
automatically. Disabled setups skip daemon and SSH probes. For enabled setups,
each SSH probe runs `true` without a terminal, password prompts, agent forwarding,
or accepting new host keys, and has a ten-second timeout. Approve an unknown host
key yourself after verifying it, then rerun doctor. Successful SSH access does
not prove the peer daemon is running or that reverse SSH access works.

Run doctor from the same environment used to enable the service. The current
terminal's SSH agent and environment may differ from the background service's;
rerun `remote enable` after fixing its environment if needed.

`herdr-sesh daemon list` reports the catalog, per-machine `last_error`, and
`switch_error`. `herdr-sesh list --json` exposes typed remote metadata. The plain
`herdr-sesh list` output includes a remote connect key in its final column;
`herdr-sesh connect KEY` submits that selection to the active wrapper. Submission
is not confirmation that the new frontend attached successfully.

Read the nested SSH error first, even if an older build calls the failure a
protocol mismatch:

| Symptom | Check or action |
| --- | --- |
| Doctor SSH passes but daemon peer authentication fails | On Linux, check the systemd user manager environment and restart the service after refreshing its agent socket. On macOS, check the login session's agent and loaded keys. Doctor probes use the current terminal environment. |
| `Permission denied` | Confirm the destination username and SSH authentication from the initiating machine. A working laptop → server login does not establish server → laptop access. |
| `command not found: herdr-sesh` | Install or link the binary into the remote noninteractive `PATH`; rerun the SSH checks above. |
| `stream closed before the protocol handshake` | Inspect the appended stderr: SSH auth, missing/old binary, wrong config environment, or a stopped daemon can prevent a reply. |
| Explicit protocol version mismatch | Install the same `herdr-sesh` build on both machines and restart their daemons. |
| `non-JSON-RPC output` | Remove stdout output from noninteractive shell startup; also check for an older peer binary. |
| `no active attach controller` | Launch with `herdr-sesh attach` from an outer terminal. |
| `another controller is active` | Detach the existing wrapper. A lost stream releases its registration; a half-open connection expires after 30 seconds without keepalives. |
| Machine/session mismatch or allowlist rejection | Match IDs and session names across configs, and include the target in the wrapper's originating config. |
| `a switch is already in progress` | Wait for the current request to finish; inspect wrapper output and `switch_error` if it fails. |
| Offline workspace or `last_error` | Check the peer's SSH access, daemon, and configured Herdr session. Catalog refreshes retry automatically. |

If the controller stream drops, the wrapper warns and resubscribes in the
background while leaving the current Herdr client running. Switching is unavailable
until registration recovers. Failed switches are printed by the wrapper and
recorded as `switch_error`; a successful completion or cross-machine handoff clears
that diagnostic. A lost stream during a switch records an unknown outcome.
If starting the new frontend fails after the old client exits, there is no
automatic rollback; rerun `herdr-sesh attach`.

## Optional background service

`remote enable` manages one service for the current user. It does not require a
system-wide service installation or run the daemon as root. Repeating the command
updates the managed definition and restarts the daemon; it refuses to overwrite
an unrelated service using the same name.

If a macOS update fails while the previous job was stopped, recovery restores
its definition and autostart setting but leaves it unloaded to avoid starting it.
Run `remote enable` when you want to start it again.

| Platform | Service | Lifetime |
| --- | --- | --- |
| macOS | `~/Library/LaunchAgents/com.herdr-sesh.daemon.plist` | Starts at login; survives terminal closure, ends at logout. |
| Linux with systemd | `$XDG_CONFIG_HOME/systemd/user/herdr-sesh-daemon.service` (normally `~/.config/systemd/user/`) | Runs under the systemd user manager. |

The definition uses absolute binary/config paths and explicit environment
settings, including the selected Herdr binary, `PATH`, config/cache locations,
while SSH agent sockets are inherited from the service manager at process start.
The service does not load shell startup files or copy the whole shell environment.
On Linux, arrange for the login session or agent to update the systemd user manager
with `systemctl --user import-environment SSH_AUTH_SOCK` when its socket changes,
then run `systemctl --user restart herdr-sesh-daemon.service`. Prefer an agent with
a stable socket for unattended or lingering sessions. Rerun `remote enable` once
after upgrading to remove any socket saved by an older definition. macOS uses the
login session's launchd agent socket. Incoming SSH
commands still need their own working noninteractive environment, as described above.

`remote status` prints the service path, autostart state, and how to read its logs, plus
whether the local Herdr session and each peer are available. A running daemon
does not mean SSH credentials, peer configuration, or Herdr sessions are ready.

On Linux, starting at boot and continuing after logout requires **separate,
explicit opt-in** to user lingering:

```sh
loginctl enable-linger "$USER"
```

The command may require administrator authorization under local policy.
`remote enable` never changes lingering. `remote disable` also leaves lingering
alone because other user services may rely on it. macOS startup before login and
Linux service managers other than systemd are not supported by this setup command;
`daemon serve` remains available for manual supervision.

## Transport and local socket

Each daemon and attach wrapper maintains its own persistent SSH streams to peers
it contacts, using `herdr-sesh daemon rpc --stream`. Requests share a
newline-delimited JSON-RPC 2.0 stream with an application-protocol handshake;
batches are unsupported. Protocol v3 requires each controller registration to
include the expected machine ID and Herdr session; the daemon validates both
before accepting the controller, including after reconnects. Attach registration
does not fetch the workspace catalog. Upgrade both machines and restart their
daemons together when moving from an older protocol. SSH uses normal host-key checks, `BatchMode=yes`, and
`ForwardAgent=no`. Failed connections retry with backoff up to 30 seconds.
The interactive Herdr attachment uses a separate connection.

Only workspace IDs, labels, working directories, and agent status are shared.
Invalid workspace IDs are dropped and control characters stripped. Catalog
refresh runs in the daemon; the picker makes a bounded local request.

Local requests use a private Unix socket, with no TCP listener. By default it is
under the OS user cache directory at `herdr-sesh/remote/daemon.sock`. An optional
absolute `socket` path overrides it; the parent must belong to your user and have
mode `0700`. A lifetime lock prevents duplicate daemons. Restarting recovers stale
sockets without replacing live listeners, symlinks, or non-socket files. Keep the
adjacent `.lock` file in place.

## Disable

For a managed installation, run:

```sh
"$sesh_bin" remote disable
```

This stops and removes the managed service and sets `enabled = false`, retaining
machine and peer settings for later reuse. Use the same configuration environment
as when enabling it. Editing `enabled` alone does not stop an already-running
daemon, which reads configuration at startup.

`remote enable` and `remote disable` require a regular configuration file and
refuse symlinks before changing the service. To manage a symlinked config,
explicitly select its real target with `HERDR_SESH_REMOTE_CONFIG`. Read-only
commands (`remote status` and `remote doctor`) accept readable symlinks.

For manual startup, stop the daemon (Ctrl+C, SIGTERM, or SIGHUP), then set
`enabled = false` or remove the default config file. If using
`HERDR_SESH_REMOTE_CONFIG`, unset it when removing that file. Herdr sessions and
running panes are left intact.

Generated services use `daemon serve --service`. If configuration disables remote
support before a service starts, it remains idle without opening a daemon socket
or contacting Herdr or SSH. Service status can show running while remote support
is disabled. Run `remote enable` to restart with enabled settings, or `remote disable`
to stop it and remove autostart. Manual `daemon serve` still reports disabled support
as an error. Existing running daemons retain their loaded settings until stopped.
