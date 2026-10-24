package remoteservice

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const label = "com.herdr-sesh.daemon"
const unit = "herdr-sesh-daemon.service"
const marker = "Managed by herdr-sesh remote service"

type Options struct {
	Executable  string
	ConfigPath  string
	HerdrBinary string
	Environment []string
}

func validPath(name, value string) error {
	if !filepath.IsAbs(value) || !utf8.ValidString(value) || strings.ContainsFunc(value, unicode.IsControl) {
		return fmt.Errorf("%s must be an absolute path without control characters", name)
	}
	return nil
}

func executable(name, path string) error {
	if err := validPath(name, path); err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("%s is not an executable file: %s", name, path)
	}
	return nil
}

func environment(opts Options) ([]string, error) {
	if err := executable("herdr-sesh executable", opts.Executable); err != nil {
		return nil, err
	}
	if err := executable("Herdr executable", opts.HerdrBinary); err != nil {
		return nil, err
	}
	if err := validPath("remote config", opts.ConfigPath); err != nil {
		return nil, err
	}
	values := map[string]string{"PATH": "/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"}
	for _, entry := range opts.Environment {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		switch key {
		case "PATH", "XDG_CONFIG_HOME", "XDG_CACHE_HOME", "HERDR_CONFIG_PATH", "SSH_AUTH_SOCK":
			if !utf8.ValidString(value) || strings.ContainsFunc(value, unicode.IsControl) {
				return nil, fmt.Errorf("invalid control character in %s", key)
			}
			if value == "" {
				continue
			}
			if key != "PATH" {
				if err := validPath(key, value); err != nil {
					return nil, err
				}
			}
			values[key] = value
		}
	}
	values["HERDR_SESH_REMOTE_CONFIG"] = opts.ConfigPath
	values["HERDR_BIN_PATH"] = opts.HerdrBinary
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, key+"="+values[key])
	}
	return result, nil
}

func xmlText(value string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(value))
	return b.String()
}

func systemdQuote(value string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, `%`, `%%`).Replace(value) + `"`
}

func (m *Manager) definition(opts Options) ([]byte, error) {
	env, err := environment(opts)
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	switch m.platform {
	case "darwin":
		fmt.Fprintf(&b, "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<!-- %s -->\n<!DOCTYPE plist PUBLIC \"-//Apple//DTD PLIST 1.0//EN\" \"http://www.apple.com/DTDs/PropertyList-1.0.dtd\">\n<plist version=\"1.0\"><dict>\n", marker)
		fmt.Fprintf(&b, "<key>Label</key><string>%s</string>\n<key>ProgramArguments</key><array><string>%s</string><string>daemon</string><string>serve</string></array>\n", label, xmlText(opts.Executable))
		b.WriteString("<key>EnvironmentVariables</key><dict>\n")
		for _, entry := range env {
			key, value, _ := strings.Cut(entry, "=")
			fmt.Fprintf(&b, "<key>%s</key><string>%s</string>\n", key, xmlText(value))
		}
		b.WriteString("</dict>\n<key>RunAtLoad</key><true/>\n<key>KeepAlive</key><true/>\n<key>ThrottleInterval</key><integer>5</integer>\n")
		fmt.Fprintf(&b, "<key>StandardOutPath</key><string>%s</string>\n<key>StandardErrorPath</key><string>%s</string>\n</dict></plist>\n", xmlText(m.logPath), xmlText(m.logPath))
	case "linux":
		// The ':' prefix disables systemd's environment expansion in arguments.
		fmt.Fprintf(&b, "# %s\n[Unit]\nDescription=Herdr Sesh remote workspace daemon\n\n[Service]\nType=exec\nExecStart=:%s daemon serve\nRestart=always\nRestartSec=5\n", marker, systemdQuote(opts.Executable))
		for _, entry := range env {
			fmt.Fprintf(&b, "Environment=%s\n", systemdQuote(entry))
		}
		b.WriteString("StandardOutput=journal\nStandardError=journal\n\n[Install]\nWantedBy=default.target\n")
	default:
		return nil, errors.New("unsupported service platform")
	}
	return []byte(b.String()), nil
}
