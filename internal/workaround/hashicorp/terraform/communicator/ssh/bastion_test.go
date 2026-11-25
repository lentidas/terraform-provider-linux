package ssh

import (
	"net"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform/terraform"
	"golang.org/x/crypto/ssh"
)

// newTestBastion accepts SSH handshakes without authentication and returns its address. Channel requests are ignored,
// so a connection can only get as far as the host key verification of the client.
func newTestBastion(t *testing.T, signer ssh.Signer) (host string, port int) {
	t.Helper()

	cfg := &ssh.ServerConfig{NoClientAuth: true}
	cfg.AddHostKey(signer)

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })

	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				conn, chans, reqs, err := ssh.NewServerConn(c, cfg)
				if err != nil {
					return
				}
				go ssh.DiscardRequests(reqs)
				for ch := range chans {
					_ = ch.Reject(ssh.Prohibited, "test bastion")
				}
				conn.Close()
			}()
		}
	}()

	h, p, _ := net.SplitHostPort(l.Addr().String())
	port, _ = strconv.Atoi(p)
	return h, port
}

func TestBastionHostKeyDefaultsToHostKey(t *testing.T) {
	parse := func(t *testing.T, bastionHostKey string) *connectionInfo {
		t.Helper()
		info, err := parseConnectionInfo(&terraform.InstanceState{Ephemeral: terraform.EphemeralState{
			ConnInfo: map[string]string{
				"host": "target", "host_key": "target-key", "bastion_host": "bastion", "bastion_host_key": bastionHostKey,
			},
		}})
		if err != nil {
			t.Fatal(err)
		}
		return info
	}

	// A CA that signs both hosts is configured once, as host_key, and must keep verifying the bastion.
	if got := parse(t, "").BastionHostKey; got != "target-key" {
		t.Fatalf("unset bastion_host_key = %q, want the host_key", got)
	}
	if got := parse(t, "bastion-key").BastionHostKey; got != "bastion-key" {
		t.Fatalf("explicit bastion_host_key = %q, want it kept", got)
	}
}

func TestPrepareSSHConfig_bastionVerifiesBastionHostKey(t *testing.T) {
	_, bastionSigner := newTestKey(t)
	_, otherSigner := newTestKey(t)
	host, port := newTestBastion(t, bastionSigner)

	connect := func(t *testing.T, hostKey, bastionHostKey string) error {
		t.Helper()
		cfg, err := prepareSSHConfig(&connectionInfo{
			User:           "user",
			Host:           "127.0.0.1",
			Port:           22,
			HostKey:        hostKey,
			BastionHost:    host,
			BastionPort:    port,
			BastionUser:    "user",
			BastionHostKey: bastionHostKey,
		})
		if err != nil {
			t.Fatal(err)
		}
		conn, err := cfg.connection()
		if conn != nil {
			conn.Close()
		}
		return err
	}

	t.Run("wrong bastion pin is rejected", func(t *testing.T) {
		err := connect(t, "", authorizedKey(otherSigner))
		if err == nil || !strings.Contains(err.Error(), "mismatch") {
			t.Fatalf("expected a host key mismatch, got: %v", err)
		}
	})

	t.Run("destination pin is not used for the bastion", func(t *testing.T) {
		// The bastion passes verification; the later failure is the rejected channel to the destination.
		err := connect(t, authorizedKey(otherSigner), authorizedKey(bastionSigner))
		if err == nil || strings.Contains(err.Error(), "mismatch") {
			t.Fatalf("expected a failure other than a host key mismatch, got: %v", err)
		}
	})
}
