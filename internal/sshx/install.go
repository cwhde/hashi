package sshx

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/ssh"

	"git.juzo.io/juzo/hashi/internal/secrets"
)

// ErrInstall marks a key install that failed for a reason the admin can act on; the text says what to do.
var ErrInstall = errors.New("key install")

// ErrKeyAdded: the line is on the host although the proof login failed (it still belongs in the audit log).
var ErrKeyAdded = fmt.Errorf("%w: the key was added, but Hashi cannot log in with it yet. Check the host's SSH settings, %s", ErrInstall, manualHint)

const manualHint = `or choose "I'll do it myself"`

// InstallCommand adds the key line read from stdin to the login user's authorized_keys, once. A file without a final
// newline gets one first, so the new key never joins the last line. Proxmox's symlinked file is followed.
const InstallCommand = `umask 077; d="$HOME/.ssh"; f="$d/authorized_keys"; mkdir -p "$d" && touch "$f" || exit 1; key=$(cat); ` +
	`grep -qxF "$key" "$f" && exit 0; if [ -s "$f" ] && [ -n "$(tail -c1 "$f")" ]; then echo >> "$f"; fi; printf '%s\n' "$key" >> "$f"`

// Runner is the part of a connection a key install needs.
type Runner interface {
	Run(ctx context.Context, cmd string, stdin io.Reader) (Result, error)
	Close() error
}

// InstallKey logs in with a credential the admin gave once (c.Auth, used for this call only and never stored), adds
// line to authorized_keys and proves that own, Hashi's key, now logs in. c.Pinned is the host key the admin confirmed.
func InstallKey(ctx context.Context, dial func(context.Context, Config) (Runner, error), c Config, line string, own Auth) error {
	password := !c.Auth.Password.IsZero()
	// "Passphrase (if any)": one given for a key that has none must not make a good key look unreadable.
	if !c.Auth.PrivateKey.IsZero() && !c.Auth.Passphrase.IsZero() {
		if _, err := ssh.ParsePrivateKey([]byte(c.Auth.PrivateKey.Reveal())); err == nil {
			c.Auth.Passphrase = secrets.Value{}
		}
	}
	if _, err := c.Auth.methods(); err != nil {
		var missing *ssh.PassphraseMissingError
		switch {
		case errors.As(err, &missing):
			return fmt.Errorf("%w: this key is protected. Enter its passphrase, %s", ErrInstall, manualHint)
		case errors.Is(err, x509.IncorrectPasswordError):
			return fmt.Errorf("%w: that passphrase does not open this key. Try again, %s", ErrInstall, manualHint)
		}
		return fmt.Errorf("%w: this is not a private key Hashi can read. Check it, %s", ErrInstall, manualHint)
	}
	r, err := dial(ctx, c)
	if err != nil {
		if strings.Contains(err.Error(), "unable to authenticate") {
			if password {
				return fmt.Errorf("%w: the host refused this password; many servers do not allow password login for %s. Retry with a private key, %s",
					ErrInstall, c.Auth.User, manualHint)
			}
			return fmt.Errorf("%w: the host refused this key for %s. Retry with another key, %s", ErrInstall, c.Auth.User, manualHint)
		}
		return err
	}
	defer func() { _ = r.Close() }()
	res, err := r.Run(ctx, InstallCommand, strings.NewReader(line))
	if err != nil {
		return err
	}
	if !res.OK() {
		return fmt.Errorf("%w: logged in, but could not write authorized_keys (exit %d: %s). Check the file, %s", ErrInstall,
			res.ExitCode, strings.TrimSpace(string(res.Stderr)), manualHint)
	}
	c.Auth = own
	check, err := dial(ctx, c)
	if err != nil {
		return ErrKeyAdded
	}
	return check.Close()
}
