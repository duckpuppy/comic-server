package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/duckpuppy/comic-server/internal/auth"
	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var authPasswordStdin bool

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage the web UI login (works with the server stopped)",
	Long: `The web UI and API have no login until a password is set. These commands
change the login straight in config.db, so they are the way back in if the
password is lost.

Stop the server first, or wait a few seconds for a running server to notice.`,
}

var authSetPasswordCmd = &cobra.Command{
	Use:   "set-password",
	Short: "Set the admin password (turns login on) and sign out every session",
	Args:  cobra.NoArgs,
	RunE:  runAuthSetPassword,
}

var authDisableCmd = &cobra.Command{
	Use:   "disable",
	Short: "Turn login off and sign out every session",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		db, err := openConfigDB()
		if err != nil {
			return err
		}
		defer db.Close()
		if err := db.DisableAuth(); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Login is off. Anyone who can reach the server can use it. Run 'comic-server auth set-password' to turn it back on.")
		return nil
	},
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show whether login is on",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		db, err := openConfigDB()
		if err != nil {
			return err
		}
		defer db.Close()
		h, err := db.AuthPasswordHash()
		if err != nil {
			return err
		}
		if h == "" {
			fmt.Fprintln(cmd.OutOrStdout(), "Login is off.")
		} else {
			fmt.Fprintln(cmd.OutOrStdout(), "Login is on.")
		}
		return nil
	},
}

var authProxiesClear bool

var authTrustedProxiesCmd = &cobra.Command{
	Use:   "trusted-proxies [ip-or-cidr ...]",
	Short: "Show, set or reset the trusted proxy list (works with the server stopped)",
	Long: `Shows the trusted proxy list in force. With addresses, saves them as the list
(this overrides config.yaml and COMIC_SERVER_TRUSTED_PROXIES). With --clear, forgets
the saved list so config.yaml / the env var apply again.

This is the way back if a wrong list makes the web UI refuse your changes.
A running server notices within a few seconds.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		if authProxiesClear && len(args) > 0 {
			return fmt.Errorf("use either --clear or a list of addresses, not both")
		}
		db, err := openConfigDB()
		if err != nil {
			return err
		}
		defer db.Close()
		out := cmd.OutOrStdout()
		switch {
		case authProxiesClear:
			if err := db.ClearTrustedProxies(); err != nil {
				return err
			}
			fmt.Fprintln(out, "Saved list cleared. config.yaml / COMIC_SERVER_TRUSTED_PROXIES apply again.")
		case len(args) > 0:
			if _, err := config.ParseTrustedProxies(args); err != nil {
				return err
			}
			if err := db.SaveTrustedProxies(args); err != nil {
				return err
			}
			fmt.Fprintf(out, "Saved: %s\n", strings.Join(args, ", "))
		default:
			list, saved, err := db.GetTrustedProxies()
			if err != nil {
				return err
			}
			if !saved {
				fmt.Fprintln(out, "No list saved: config.yaml / COMIC_SERVER_TRUSTED_PROXIES apply (none if unset).")
			} else if len(list) == 0 {
				fmt.Fprintln(out, "Saved list: empty (no proxy is trusted).")
			} else {
				fmt.Fprintf(out, "Saved list: %s\n", strings.Join(list, ", "))
			}
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(authCmd)
	authCmd.AddCommand(authSetPasswordCmd, authDisableCmd, authStatusCmd, authTrustedProxiesCmd)
	authTrustedProxiesCmd.Flags().BoolVar(&authProxiesClear, "clear", false, "Forget the saved list so config.yaml / the env var apply again")
	authSetPasswordCmd.Flags().BoolVar(&authPasswordStdin, "password-stdin", false, "Read the password from standard input (one line) instead of asking")
}

func runAuthSetPassword(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true
	pw, err := readNewPassword(cmd, authPasswordStdin)
	if err != nil {
		return err
	}
	if err := auth.ValidatePassword(pw); err != nil {
		return err
	}
	hash, err := auth.HashPassword(pw)
	if err != nil {
		return err
	}
	db, err := openConfigDB()
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.SetAuthPasswordHash(hash); err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), "Password saved. Login is on. Every old session is signed out.")
	return nil
}

func readNewPassword(cmd *cobra.Command, fromStdin bool) (string, error) {
	if fromStdin {
		line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
		if err != nil && err != io.EOF {
			return "", fmt.Errorf("read password: %w", err)
		}
		return strings.TrimRight(line, "\r\n"), nil
	}
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return "", fmt.Errorf("standard input is not a terminal: use --password-stdin")
	}
	fmt.Fprint(os.Stderr, "New password: ")
	a, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	fmt.Fprint(os.Stderr, "Repeat password: ")
	b, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	if string(a) != string(b) {
		return "", fmt.Errorf("the two passwords do not match")
	}
	return string(a), nil
}
