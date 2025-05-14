// Copyright (C) 2025 once contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.
//
// SPDX-License-Identifier: GPL-3.0

// Package main implements the once CLI.
package main

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"

	"github.com/jaredallard/cmdexec"
	"github.com/rogpeppe/go-internal/lockedfile"

	"github.com/spf13/cobra"
)

const developmentVersion = "v0.0.0-dev"

// Version contains the current version of the CLI, which gets overwritten during releases (see .goreleaser.yml).
var Version = developmentVersion

// entrypoint is the main logic of the CLI.
func entrypoint(c *cobra.Command, args []string) error {
	// Force show usage if --help or -h are passed as the only flags we
	// support.
	if args[0] == "--help" || args[0] == "-h" {
		return c.Help()
	}

	if args[0] == "--version" || args[0] == "-v" {
		fmt.Printf("%s version %s\n", c.DisplayName(), c.Version)
		return nil
	}

	cmdPath := args[0]
	cmdArgs := args[1:]

	// Always prefer the full path for the command if possible.
	if absPath, err := exec.LookPath(cmdPath); err == nil && absPath != "" {
		cmdPath = absPath
	}

	unlock, err := obtainLock(cmdPath, cmdArgs)
	if err != nil {
		return err
	}
	defer unlock()

	cmd := cmdexec.CommandContext(c.Context(), cmdPath, cmdArgs...)
	cmd.UseOSStreams(true)
	return cmd.Run()
}

// getCacheDir returns the directory where extensions are cached in
func getCacheDir() (string, error) {
	cacheDir := os.Getenv("XDG_CACHE_HOME")
	if cacheDir == "" { // default to $HOME/.cache as per XDG spec
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		cacheDir = filepath.Join(homeDir, ".cache")
	}
	return filepath.Join(cacheDir, "once"), nil
}

// obtainLock gets a lock for the provided command and args. This
// function blocks until a lock can be obtained. Returned is a function
// to unlock the lock that must be called.
//
// Note that the lock is also released if the program exits, but not
// removed from disk in that case.
func obtainLock(cmdPath string, cmdArgs []string) (func(), error) {
	lockDir, err := getCacheDir()
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(lockDir, 0o750); err != nil {
		return nil, fmt.Errorf("failed to ensure lock dir exists %s: %w", lockDir, err)
	}

	// Hash the arguments
	hasher := sha512.New()
	if _, err := fmt.Fprintf(hasher, "%s|%v", cmdPath, cmdArgs); err != nil {
		return nil, fmt.Errorf("failed to create hash from path and args: %w", err)
	}
	hash := hex.EncodeToString(hasher.Sum(nil))

	var waitedForLock bool
	lockPath := filepath.Join(lockDir, hash+".lock")
	if _, err := os.Stat(lockPath); err == nil {
		// Note that we're likely going to be waiting for a lock below.
		fmt.Fprintf(os.Stderr, "[once] Waiting for lock ... (%s)\n", lockPath)
		waitedForLock = true
	}

	unlock, err := lockedfile.MutexAt(lockPath).Lock()
	if err != nil {
		return nil, err
	}

	if waitedForLock {
		fmt.Fprintf(os.Stderr, "[once] Obtained lock\n")
	}

	return func() {
		os.Remove(lockPath) //nolint:errcheck,gosec // Why: best effort
		unlock()
	}, nil
}

// main wraps cobra command creation and other specific logic. See
// [entrypoint] for the logic.
func main() {
	exitCode := 0
	defer func() { os.Exit(exitCode) }()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()

	rootCmd := &cobra.Command{
		Use:   "once [flags] <command> [arguments]",
		Short: "Run the provided command at most once at a time",
		Long: "The provided command is ran as provided to once with a " +
			"file-backed mutex to ensure that other instances of once do not " +
			"run the same command+args combination at the same time. When " +
			"another instance of once fails to lock the mutex, it will wait " +
			"until the lock is released. Locks are created using a hashed " +
			"version of the command+args to prevent leaking information about " +
			"the command being ran.",
		Args:    cobra.MinimumNArgs(1),
		Version: Version,

		// If we add flags we should do a v2 where we switch to -- as the
		// syntax. Otherwise, it's better UX to not require it.
		DisableFlagParsing: true,
		DisableSuggestions: true,
		SilenceErrors:      true,
		SilenceUsage:       true,

		RunE: entrypoint,
	}
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		exitCode = 1
	}
}
