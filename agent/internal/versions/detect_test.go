package versions

import "testing"

func TestParseResticVersion(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "standard output",
			input: "restic 0.17.3 compiled with go1.22.3 on linux/amd64",
			want:  "0.17.3",
		},
		{
			name:  "with trailing newline",
			input: "restic 0.17.3 compiled with go1.22.3 on linux/amd64\n",
			want:  "0.17.3",
		},
		{
			name:    "empty output",
			input:   "",
			wantErr: true,
		},
		{
			name:    "only one word",
			input:   "restic",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseResticVersion(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseResticVersion(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("parseResticVersion(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseRcloneVersion(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "standard output single line",
			input: "rclone v1.68.0",
			want:  "v1.68.0",
		},
		{
			name: "multi-line output",
			input: `rclone v1.68.0
- os/version: linux 6.1.0 (64 bit)
- os/kernel: 6.1.0 #1
- os/type: linux`,
			want: "v1.68.0",
		},
		{
			name:  "with trailing newline",
			input: "rclone v1.68.0\n",
			want:  "v1.68.0",
		},
		{
			name:    "empty output",
			input:   "",
			wantErr: true,
		},
		{
			name:    "only one word",
			input:   "rclone",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseRcloneVersion(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseRcloneVersion(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("parseRcloneVersion(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
