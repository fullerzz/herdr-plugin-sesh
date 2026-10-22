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

### 2. Create `remote.toml`

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

Start the configured Herdr session on each machine, then run:

```sh
herdr-sesh daemon serve
```

Leave this foreground process running. In another terminal on each machine:

```sh
herdr-sesh daemon list
herdr-sesh list
```

Confirm both machines and their workspaces appear without stale/offline status
or `last_error` before testing a switch. The daemon logs peer failures and
recovery. Restart daemons and attach wrappers after changing their configuration;
restart daemons after upgrading their binaries.

Neither installing the plugin nor opening the picker starts a daemon
automatically. The daemon does not start a missing Herdr session, install services,
change SSH configuration, or update remote software.

## Switching before native client APIs

### 4. Launch through the attach wrapper

From an interactive **outer terminal, outside Herdr**, run:

```sh
herdr-sesh attach
# Or start on a configured peer:
herdr-sesh attach server
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

`herdr-sesh daemon list` reports the catalog, per-machine `last_error`, and
`switch_error`. `herdr-sesh list --json` exposes typed remote metadata. The plain
`herdr-sesh list` output includes a remote connect key in its final column;
`herdr-sesh connect KEY` submits that selection to the active wrapper. Submission
is not confirmation that the new frontend attached successfully.

Read the nested SSH error first, even if an older build calls the failure a
protocol mismatch:

| Symptom | Check or action |
| --- | --- |
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

After foreground setup works, use a user service manager. For Linux, create
`~/.config/systemd/user/herdr-sesh-daemon.service`:

```ini
[Unit]
Description=Herdr Sesh remote workspace catalog

[Service]
ExecStart=/absolute/path/to/herdr-sesh daemon serve
Restart=on-failure
RestartSec=5
Environment=PATH=/home/YOU/.local/bin:/usr/local/bin:/usr/bin:/bin

[Install]
WantedBy=default.target
```

Replace the executable and `PATH` values for your installation. If needed, add
`Environment=XDG_CONFIG_HOME=/absolute/config/directory` or
`Environment=HERDR_SESH_REMOTE_CONFIG=/absolute/path/to/remote.toml`, then enable it:

```sh
systemctl --user daemon-reload
systemctl --user enable --now herdr-sesh-daemon
```

On macOS, use a per-user LaunchAgent with `ProgramArguments` containing the
absolute executable path, `daemon`, and `serve`. Set `PATH` and any config
overrides in its `EnvironmentVariables`. These services do not start Herdr;
ensure the configured session is running separately.

## Transport and local socket

Each daemon and attach wrapper maintains its own persistent SSH streams to peers
it contacts, using `herdr-sesh daemon rpc --stream`. Requests share a
newline-delimited JSON-RPC 2.0 stream with an application-protocol handshake;
batches are unsupported. SSH uses normal host-key checks, `BatchMode=yes`, and
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

Stop the daemon (Ctrl+C, SIGTERM, or SIGHUP), or stop and disable its user service,
and set `enabled = false` or remove the default config file. If using
`HERDR_SESH_REMOTE_CONFIG`, unset it when removing that file. Herdr sessions and
running panes are left intact.
