package versions

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Versions holds the detected tool versions.
type Versions struct {
	Restic string
	Rclone string
}

// Detect runs restic and rclone version commands and returns the detected
// versions. It returns an error if either binary is not found or fails to
// report its version. This is intended to be called at agent startup as a
// readiness check.
func Detect(ctx context.Context) (*Versions, error) {
	resticVer, err := detectRestic(ctx)
	if err != nil {
		return nil, fmt.Errorf("detecting restic version: %w", err)
	}

	rcloneVer, err := detectRclone(ctx)
	if err != nil {
		return nil, fmt.Errorf("detecting rclone version: %w", err)
	}

	return &Versions{
		Restic: resticVer,
		Rclone: rcloneVer,
	}, nil
}

func detectRestic(ctx context.Context) (string, error) {
	out, err := runCommand(ctx, "restic", "version")
	if err != nil {
		return "", fmt.Errorf("restic binary not found or failed to execute: %w", err)
	}
	return parseResticVersion(out)
}

func detectRclone(ctx context.Context) (string, error) {
	out, err := runCommand(ctx, "rclone", "version")
	if err != nil {
		return "", fmt.Errorf("rclone binary not found or failed to execute: %w", err)
	}
	return parseRcloneVersion(out)
}

// parseResticVersion extracts the version string from restic version output.
// Example output: "restic 0.17.3 compiled with go1.22.3 on linux/amd64"
func parseResticVersion(output string) (string, error) {
	parts := strings.Fields(strings.TrimSpace(output))
	if len(parts) < 2 {
		return "", fmt.Errorf("unexpected restic version output: %q", output)
	}
	return parts[1], nil
}

// parseRcloneVersion extracts the version string from rclone version output.
// The first line of the output is: "rclone v1.68.0"
func parseRcloneVersion(output string) (string, error) {
	firstLine := strings.SplitN(strings.TrimSpace(output), "\n", 2)[0]
	parts := strings.Fields(firstLine)
	if len(parts) < 2 {
		return "", fmt.Errorf("unexpected rclone version output: %q", output)
	}
	return parts[1], nil
}

func runCommand(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		if errOut.Len() > 0 {
			return "", fmt.Errorf("running %q: %w: %s", name, err, strings.TrimSpace(errOut.String()))
		}
		return "", fmt.Errorf("running %q: %w", name, err)
	}
	return out.String(), nil
}
