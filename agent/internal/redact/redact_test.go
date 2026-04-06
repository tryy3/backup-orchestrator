package redact

import (
	"testing"
)

func TestEnv_RedactsSensitiveValues(t *testing.T) {
	input := []string{
		"RESTIC_PASSWORD=abc123",
		"AWS_SECRET_ACCESS_KEY=mykey",
		"HOME=/home/user",
		"RCLONE_CONFIG=/etc/rclone.conf",
		"TOKEN_FILE=/tmp/tok",
		"MY_CREDENTIAL=hunter2",
	}
	got := Env(input)
	want := []string{
		"RESTIC_PASSWORD=*****",
		"AWS_SECRET_ACCESS_KEY=*****",
		"HOME=/home/user",
		"RCLONE_CONFIG=/etc/rclone.conf",
		"TOKEN_FILE=*****",
		"MY_CREDENTIAL=*****",
	}
	if len(got) != len(want) {
		t.Fatalf("length mismatch: got %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("env[%d]: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestEnv_NoEquals(t *testing.T) {
	input := []string{"NOEQUALS"}
	got := Env(input)
	if got[0] != "NOEQUALS" {
		t.Errorf("got %q, want %q", got[0], "NOEQUALS")
	}
}

func TestEnv_EmptySlice(t *testing.T) {
	got := Env([]string{})
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %v", got)
	}
}

func TestEnv_DoesNotMutateInput(t *testing.T) {
	input := []string{"RESTIC_PASSWORD=secret"}
	original := input[0]
	_ = Env(input)
	if input[0] != original {
		t.Error("Env mutated the input slice")
	}
}

func TestArgs_RedactsFlagValues(t *testing.T) {
	input := []string{
		"backup", "--json", "--repo", "/data",
		"--password", "s3cret",
		"--password-file", "/tmp/pw",
		"--tag", "daily",
	}
	got := Args(input)
	want := []string{
		"backup", "--json", "--repo", "/data",
		"--password", "*****",
		"--password-file", "*****",
		"--tag", "daily",
	}
	if len(got) != len(want) {
		t.Fatalf("length mismatch: got %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("args[%d]: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestArgs_SensitiveFlagAtEnd(t *testing.T) {
	// If a sensitive flag is the last arg with no value following, nothing crashes.
	input := []string{"--password"}
	got := Args(input)
	if got[0] != "--password" {
		t.Errorf("got %q, want %q", got[0], "--password")
	}
}

func TestArgs_NonSensitivePassThrough(t *testing.T) {
	input := []string{"--repo", "/data", "--json", "/home/user/docs"}
	got := Args(input)
	for i := range input {
		if got[i] != input[i] {
			t.Errorf("args[%d]: got %q, want %q", i, got[i], input[i])
		}
	}
}

func TestArgs_DoesNotMutateInput(t *testing.T) {
	input := []string{"--password", "s3cret"}
	original := input[1]
	_ = Args(input)
	if input[1] != original {
		t.Error("Args mutated the input slice")
	}
}

func TestArgs_EmptySlice(t *testing.T) {
	got := Args([]string{})
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %v", got)
	}
}

func TestIsSensitive_CaseInsensitive(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"RESTIC_PASSWORD", true},
		{"password", true},
		{"Password", true},
		{"AWS_SECRET_KEY", true},
		{"MY_TOKEN", true},
		{"CREDENTIAL_FILE", true},
		{"HOME", false},
		{"PATH", false},
		{"RCLONE_CONFIG", false},
	}
	for _, tc := range cases {
		if got := isSensitive(tc.name); got != tc.want {
			t.Errorf("isSensitive(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}
