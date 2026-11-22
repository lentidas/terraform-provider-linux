package linux

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

var testAccProviders map[string]*schema.Provider

var testAccProvider = tfmap{
	attrProviderHost:     `"127.0.0.1"`,
	attrProviderPort:     `22`,
	attrProviderUser:     `"root"`,
	attrProviderPassword: `"root"`,
}

var testAccOverridenProvider = tfmap{
	attrProviderHost:     `"8.8.8.8"`,
	attrProviderPort:     `2222`,
	attrProviderUser:     `"something"`,
	attrProviderPassword: `"else"`,
	attrProviderTimeout:  "1",
}

func testAccInit() {
	testAccProviders = map[string]*schema.Provider{
		"linux": Provider(),
	}

	if f, err := ioutil.ReadFile("testdata/id_rsa"); err == nil {
		testAccProvider.With(attrProviderPrivateKey, string(f))
	}
	if v, ok := os.LookupEnv("TEST_ACC_LINUX_PROVIDER_HOST"); ok {
		testAccProvider.With(attrProviderHost, fmt.Sprintf(`"%s"`, v))
	}
	if v, ok := os.LookupEnv("TEST_ACC_LINUX_PROVIDER_PORT"); ok {
		testAccProvider.With(attrProviderPort, v)
	}
	if v, ok := os.LookupEnv("TEST_ACC_LINUX_PROVIDER_USER"); ok {
		testAccProvider.With(attrProviderUser, fmt.Sprintf(`"%s"`, v))
	}
	if v, ok := os.LookupEnv("TEST_ACC_LINUX_PROVIDER_PASSWORD"); ok {
		testAccProvider.With(attrProviderPassword, fmt.Sprintf(`"%s"`, v))
	}
	if v, ok := os.LookupEnv("TEST_ACC_LINUX_PROVIDER_PRIVATE_KEY"); ok {
		testAccProvider.With(attrProviderPrivateKey, fmt.Sprintf(`"%s"`, v))
	}
}

func testAccPreCheckConnection(t *testing.T) func() {
	return func() {
		conf := map[string]string{
			attrProviderHost: strings.ReplaceAll(testAccProvider[attrProviderHost], `"`, ``),
			attrProviderPort: testAccProvider[attrProviderPort],
		}
		err := (&linux{connInfo: conf}).init(context.Background())
		var errNet net.Error
		if errors.As(err, &errNet) {
			t.Fatalf("ssh connection should be available: %v", err)
		}
	}
}

func TestMain(m *testing.M) {
	testAccInit()
	resource.TestMain(m)
}

func testResourceRead(t *testing.T, res *schema.Resource) {
	t.Helper()
	tests := []struct {
		name   string
		stdout string
		stderr string
		status int
		absent bool
	}{
		{name: "missing", stderr: "stat: cannot statx '/tmp/read-test': No such file or directory\n", status: 1, absent: true},
		{name: "permission denied", stderr: "stat: cannot statx '/tmp/read-test': Permission denied\n", status: 1},
		{name: "stat unavailable", stderr: "stat: command not found\n", status: 127},
		{name: "missing exit status", stderr: "stat: cannot statx '/tmp/read-test': No such file or directory\n", status: -1},
		{name: "connection interrupted", stderr: "stat: cannot statx '/tmp/read-test': No such file or directory\n", status: -2},
		{name: "existing", stdout: "1000 1001 640\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := testReadPool(t, func(command string) (string, string, int) {
				if strings.Contains(command, "stat -c ") {
					return tt.stdout, tt.stderr, tt.status
				}
				if command == "cat /tmp/read-test" {
					return "remote content", "", 0
				}
				t.Errorf("unexpected remote command: %q", command)
				return "", "unexpected command", 127
			})
			rd := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{
				"path": "/tmp/read-test", "overwrite": true, "recycle_path": "/tmp/recycle",
			})
			rd.SetId("managed")

			diags := res.ReadContext(context.Background(), rd, pool)
			switch {
			case tt.absent:
				require.False(t, diags.HasError(), "%v", diags)
				require.Empty(t, rd.Id())
			case tt.status != 0:
				require.True(t, diags.HasError(), "%v", diags)
				require.Equal(t, "managed", rd.Id())
				if tt.status > 0 {
					require.Contains(t, diags[0].Summary, strings.TrimSpace(tt.stderr))
				}
			default:
				require.False(t, diags.HasError(), "%v", diags)
				require.Equal(t, "managed", rd.Id())
				require.Equal(t, 1000, rd.Get("owner"))
				require.Equal(t, 1001, rd.Get("group"))
				require.Equal(t, "640", rd.Get("mode"))
				require.Equal(t, true, rd.Get("overwrite"))
				require.Equal(t, "/tmp/recycle", rd.Get("recycle_path"))
				if _, ok := res.Schema["content"]; ok {
					require.Equal(t, "remote content", rd.Get("content"))
				}
			}
		})
	}
}

// Replies to SSH commands without executing them on this machine or an existing host.
func testReadPool(t *testing.T, respond func(string) (string, string, int)) *linuxPool {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	signer, err := ssh.NewSignerFromKey(key)
	require.NoError(t, err)
	config := &ssh.ServerConfig{NoClientAuth: true}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	host, port, err := net.SplitHostPort(listener.Addr().String())
	require.NoError(t, err)
	l := &linux{connInfo: map[string]string{
		"host": host, "port": port, "user": "test", "agent": "false", "timeout": "5s",
		"host_key": strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey()))),
	}}
	done := make(chan struct{})
	t.Cleanup(func() {
		if l.comm != nil {
			if err := l.comm.Disconnect(); err != nil && !errors.Is(err, net.ErrClosed) {
				t.Errorf("disconnect test client: %v", err)
			}
		}
		if err := listener.Close(); err != nil {
			t.Errorf("close test listener: %v", err)
		}
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("test SSH server did not stop")
		}
	})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			if !errors.Is(err, net.ErrClosed) {
				t.Errorf("accept test connection: %v", err)
			}
			return
		}
		defer conn.Close()
		server, channels, requests, err := ssh.NewServerConn(conn, config)
		if err != nil {
			t.Errorf("test SSH handshake: %v", err)
			return
		}
		defer server.Close()
		go ssh.DiscardRequests(requests)
		for incoming := range channels {
			channel, requests, err := incoming.Accept()
			if err != nil {
				t.Errorf("accept test session: %v", err)
				return
			}
			for request := range requests {
				if request.Type != "exec" {
					t.Errorf("unexpected SSH request: %s", request.Type)
					break
				}
				var payload struct{ Command string }
				if err := ssh.Unmarshal(request.Payload, &payload); err != nil {
					t.Errorf("decode test command: %v", err)
					break
				}
				if err := request.Reply(true, nil); err != nil {
					t.Errorf("reply to test command: %v", err)
					break
				}
				stdout, stderr, status := respond(strings.TrimSpace(payload.Command))
				if _, err := io.WriteString(channel, stdout); err != nil {
					t.Errorf("write test stdout: %v", err)
					break
				}
				if _, err := io.WriteString(channel.Stderr(), stderr); err != nil {
					t.Errorf("write test stderr: %v", err)
					break
				}
				if status == -2 {
					if err := server.Close(); err != nil {
						t.Errorf("interrupt test connection: %v", err)
					}
				} else if status >= 0 {
					if _, err := channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{uint32(status)})); err != nil {
						t.Errorf("send test exit status: %v", err)
					}
				}
				break
			}
			channel.Close()
		}
	}()
	return &linuxPool{def: l, pool: make(map[string]*linux)}
}
