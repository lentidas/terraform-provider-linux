package ssh

import (
	"bufio"
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestSCPSinkCommandTreatsDirectoryAsData(t *testing.T) {
	tests := map[string]string{
		"/tmp":           `scp -vt '/tmp'`,
		"/tmp/a b":       `scp -vt '/tmp/a b'`,
		"/tmp/$(id)":     `scp -vt '/tmp/$(id)'`,
		"/tmp/it's; ls":  `scp -vt '/tmp/it'\''s; ls'`,
		"/tmp/-r /etc/x": `scp -vt '/tmp/-r /etc/x'`,
	}
	for dir, want := range tests {
		if got := scpSinkCommand(dir); got != want {
			t.Errorf("scpSinkCommand(%q) = %s, want %s", dir, got, want)
		}
	}
}

func TestScpUploadFileCreatesPrivateFile(t *testing.T) {
	var sent bytes.Buffer
	acks := bufio.NewReader(bytes.NewReader([]byte{0, 0}))

	if err := scpUploadFile("secret.txt", strings.NewReader("hunter2"), &sent, acks, 7); err != nil {
		t.Fatal(err)
	}
	if header, _, _ := strings.Cut(sent.String(), "\n"); header != "C0600 7 secret.txt" {
		t.Fatalf("unexpected SCP header %q: files must be created private", header)
	}
}

func TestUploadFailedStatIsAnError(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "closed")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	// A failing Stat used to dereference a nil FileInfo.
	if err := (&Communicator{}).Upload("/tmp/x", f); err == nil {
		t.Fatal("expected an error")
	}
}
