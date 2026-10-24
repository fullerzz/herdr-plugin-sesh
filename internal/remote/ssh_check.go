package remote

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
	"unicode"
)

// CheckSSH checks noninteractive SSH access without requiring the peer daemon.
func CheckSSH(ctx context.Context, peer Peer) error {
	if !sshHost.MatchString(peer.SSH) {
		return errors.New("invalid SSH destination")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ssh", // #nosec G204 -- validated destination and fixed remote command.
		"-T", "-n", "-o", "BatchMode=yes", "-o", "ForwardAgent=no",
		"-o", "StrictHostKeyChecking=yes", "-o", "UpdateHostKeys=no", "-o", "ConnectTimeout=5",
		"-o", "ConnectionAttempts=1", "-o", "ClearAllForwardings=yes",
		"-o", "PermitLocalCommand=no", "-o", "ControlPath=none", "--", peer.SSH, "true")
	stderr := &limitedBuffer{limit: 4 << 10}
	cmd.Stderr = stderr
	// Bound Wait even if a configured SSH helper inherits stderr and lingers.
	cmd.WaitDelay = time.Second
	err := cmd.Run()
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	msg := strings.Map(func(r rune) rune {
		if !unicode.IsPrint(r) {
			return ' '
		}
		return r
	}, stderr.String())
	msg = strings.Join(strings.Fields(msg), " ")
	if runes := []rune(msg); len(runes) > 512 {
		msg = string(runes[:512])
	}
	if msg != "" {
		return fmt.Errorf("SSH access: %w (%s)", err, msg)
	}
	return fmt.Errorf("SSH access: %w", err)
}
