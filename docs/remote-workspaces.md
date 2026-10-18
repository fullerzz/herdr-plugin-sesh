# Remote workspaces (experimental)

Opt in to a combined workspace picker across machines by running a small
`herdr-sesh` daemon on each participating machine. Each daemon reads its own
Herdr session and caches the workspaces exported by explicitly configured peers.
Files, pane processes, terminal output, and credentials stay on their machines.

## Switching before native client APIs

Start Herdr from an outer terminal with:

```sh
herdr-sesh attach
```

Open the Sesh picker normally. Remote entries display their machine ID; selecting
one asks the wrapper to replace its foreground Herdr connection with an attachment
to that machine. The old Herdr server and its running panes stay alive. Open the
picker on the remote machine to switch back, or to another configured machine.
You can start directly on a configured peer with `herdr-sesh attach server`.
The wrapper first asks its foreground client to exit gracefully so Herdr restores
terminal modes. It force-stops that client only if it does not exit within two seconds.

This is a temporary attachment fallback, not Herdr's native in-window machine
switching. A plain `herdr` launch can display remote picker entries but cannot
accept switching requests without a wrapper. One wrapper may control a machine's
daemon at a time. The wrapper subscribes over a persistent stream to the daemon
of the machine it is attached to, and that stream is the registration: the daemon
releases the controller as soon as the stream closes, including when the
terminal closes and the wrapper exits, or after 30 seconds without the wrapper's
keepalive when a network drop leaves it half-open. Another subscription is
rejected until then. Other Herdr clients can still attach, but
picker requests on that machine target its subscribed wrapper, so use this mode
with one interactive switching client.

## Configure each machine

Install compatible Herdr and the same version of `herdr-sesh` on every
participating machine. Streams begin with a protocol handshake, so a machine
running an incompatible `herdr-sesh` fails with a version mismatch error instead
of misreading requests. Both executables must be available on the SSH user's
noninteractive `PATH`. Start the configured Herdr session before starting the
daemon. Peer streams use noninteractive SSH; complete authentication and host-key
approval in a normal terminal first.

Configuration is separate from the regular picker configuration:

- Linux: `$XDG_CONFIG_HOME/herdr-sesh/remote.toml`, or
  `~/.config/herdr-sesh/remote.toml` when unset.
- macOS: `~/Library/Application Support/herdr-sesh/remote.toml`.
- Set `HERDR_SESH_REMOTE_CONFIG` to use an explicit file. The remote SSH entry
  point reads the remote machine's default file; local overrides are not forwarded.

Example for a laptop:

```toml
enabled = true
id = "laptop"
label = "Laptop"
herdr_session = "default"

[[peers]]
id = "server"
label = "Build server"
ssh = "buildbox"
session = "default"
```

On the server, use `id = "server"` and add a reciprocal peer with `id = "laptop"`
and an SSH target reachable **from the server**. IDs must match across catalogs.
Every device configures the machines it is allowed to discover and switch to;
there is no transitive peer discovery. Each daemon represents one Herdr session.
The session names default to `default` when omitted, and peer session names must
match the peer daemon's `herdr_session`.

An SSH target is a hostname, `user@hostname`, or an SSH-config alias. Configure
ports, identity files, and jump hosts in normal SSH config. Passwords and private
keys never go in `remote.toml`.

Each daemon, and each attach wrapper, keeps one long-lived `ssh` process per peer
that runs the fixed command `herdr-sesh daemon rpc --stream`. Requests to that
peer share the stream as newline-delimited JSON-RPC 2.0 messages, and the peer
daemon pushes switch requests over it as `switch` notifications, so a peer costs
one SSH handshake instead of one per request. Batch requests are not supported. If the stream drops,
the next request reconnects, backing off up to 30 seconds while the peer stays
unreachable. These connections use your `ssh` binary and SSH config, keep
host-key checks, set `BatchMode=yes`, and disable agent forwarding.

Absent configuration or `enabled = false` leaves remote integration disabled.
Neither installing the plugin nor opening the picker starts a daemon or SSH
connection automatically.

## Run the daemon

Run this on each participating machine:

```sh
herdr-sesh daemon serve
```

It runs in the foreground for supervision by a user service manager. Stop it with
Ctrl+C, SIGTERM, or SIGHUP. A private Unix socket serves local requests; no TCP listener
is opened. Catalog refreshes happen in the daemon, so the picker only makes a
bounded local request. Unreachable peers retain their last catalog marked offline,
and switching to offline entries is rejected. Peers share only each workspace's
ID, label, working directory, and agent status. Received entries with invalid IDs
are dropped, and control characters are removed from their text.

For a Linux user service, create `~/.config/systemd/user/herdr-sesh-daemon.service`:

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

Replace both paths for your installation, then explicitly enable it:

```sh
systemctl --user daemon-reload
systemctl --user enable --now herdr-sesh-daemon
```

On macOS, use a per-user LaunchAgent with `ProgramArguments` containing the
absolute executable path, `daemon`, and `serve`; set its `PATH` to include Herdr.
The daemon does not install services, change SSH configuration, start a missing
Herdr server, or update remote software. Herdr's interactive `--remote` attachment
may present its normal setup prompts; prepare your machines before unattended use.

## Inspect and troubleshoot

```sh
herdr-sesh daemon list
herdr-sesh list
herdr-sesh list --json
```

The plain list prints a remote connect key in its final column. Pass it to
`herdr-sesh connect KEY` to send a switch to the active wrapper. The daemon pushes
it over the wrapper's stream; successful submission means it was handed to that
stream. Selections the wrapper could never complete are rejected immediately: the
wrapper tells its daemon which machines its own `remote.toml` allows, so a
machine known only to the daemon's catalog fails in `connect` and the picker.
Another selection is rejected until the wrapper finishes the current one. If the
switch fails later, the wrapper prints the error and the daemon keeps it as
`switch_error` in `herdr-sesh daemon list` until the next switch succeeds. Remote rows obey
the normal blacklist and provide metadata previews only. Missing remote keys fail
explicitly instead of being interpreted as local paths. Closing a remote workspace
and applying local startup/layout configuration to remote entries are disabled.
Local previous-workspace history remains local to each Herdr session.

If switching reports no active controller, start with `herdr-sesh attach`.
If it reports another controller, detach the existing wrapper; after a crash or
network drop the daemon releases it within 30 seconds. When its stream drops,
for example because the daemon restarts, the wrapper warns once, keeps the
attached Herdr client running, and resubscribes in the background; only
switching is unavailable until then. `herdr-sesh daemon list` reports each
machine's `last_error`, and `herdr-sesh daemon serve` logs when a machine
becomes unavailable or recovers. SSH failures include the bounded `ssh` error
output. A protocol handshake failure means the machines run incompatible
`herdr-sesh` versions, or a daemon started before an upgrade is still running:
update both and restart their daemons. Older wrappers that still use the
one-request-per-SSH-call mode can read catalogs for one more release but cannot
control a newer daemon.

The default socket is under the OS user cache directory in
`herdr-sesh/remote/daemon.sock`. Set an absolute `socket` in the config when needed;
its parent must belong to your user and have mode `0700`. Existing socket paths
are protected by a lifetime lock. After an unclean exit, restarting the daemon
recovers its stale socket automatically. Live listeners, symlinks, and non-socket
files are never replaced. Keep the adjacent `.lock` file in place; its inode is
reused across restarts to prevent concurrent daemons.

Disable the feature by stopping the daemon/service and setting `enabled = false`
or removing its configuration. The daemon does not modify or stop Herdr sessions.
