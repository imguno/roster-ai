package runner

import (
	"testing"
)

func TestShellSplit(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{
			in:   `powershell -NoProfile -File scripts/run.ps1`,
			want: []string{"powershell", "-NoProfile", "-File", "scripts/run.ps1"},
		},
		{
			in:   `"C:\Program Files\app.exe" --flag value`,
			want: []string{`C:\Program Files\app.exe`, "--flag", "value"},
		},
		{
			in:   `cmd 'arg with spaces'`,
			want: []string{"cmd", "arg with spaces"},
		},
		{
			in:   `simple command`,
			want: []string{"simple", "command"},
		},
		{
			in:   `path/to/"My File.txt" -option`,
			want: []string{`path/to/My File.txt`, "-option"},
		},
	}

	for _, c := range cases {
		got, err := shellSplit(c.in)
		if err != nil {
			t.Errorf("shellSplit(%q) error: %v", c.in, err)
			continue
		}
		if len(got) != len(c.want) {
			t.Errorf("shellSplit(%q) = %v, want %v", c.in, got, c.want)
			continue
		}
		for i, g := range got {
			if g != c.want[i] {
				t.Errorf("shellSplit(%q) = %v, want %v", c.in, got, c.want)
				break
			}
		}
	}
}

func TestShellSplitErrors(t *testing.T) {
	cases := []struct {
		in  string
		err bool
	}{
		{`"unclosed quote`, true},
		{`'unclosed single quote`, true},
		{`"closed "quote`, false}, // This actually parses fine
		{``, false},               // Empty string is fine
	}

	for _, c := range cases {
		_, err := shellSplit(c.in)
		if (err != nil) != c.err {
			t.Errorf("shellSplit(%q) error = %v, want error=%v", c.in, err, c.err)
		}
	}
}
