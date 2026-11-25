package linux

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/stretchr/testify/require"
)

// recordingPool answers commands like testReadPool and keeps a copy of everything it was asked to run.
func recordingPool(t *testing.T, respond func(string) (string, string, int)) (*linuxPool, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var commands []string
	pool := testReadPool(t, func(command string) (string, string, int) {
		mu.Lock()
		commands = append(commands, command)
		mu.Unlock()
		return respond(command)
	})
	return pool, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), commands...)
	}
}

func TestSetPermissionTreatsModeAndPathAsData(t *testing.T) {
	pool, commands := recordingPool(t, func(string) (string, string, int) { return "", "", 0 })

	err := pool.def.setPermission(context.Background(), "/tmp/a b", permission{owner: 100000, group: 4294967294, mode: "644; id"})
	require.NoError(t, err)
	require.Equal(t, []string{`{ chown 100000:4294967294 '/tmp/a b' && chmod '644; id' '/tmp/a b' ;}`}, commands())
}

func TestGetPermissionKeepsWideIDsAndLeadingZeroModes(t *testing.T) {
	tests := []struct {
		stdout string
		want   permission
	}{
		{"100000 4294967294 644\n", permission{owner: 100000, group: 4294967294, mode: "644"}},
		{"0 0 40\n", permission{mode: "040"}},
		{"0 0 0\n", permission{mode: "000"}},
		{"0 0 4755\n", permission{mode: "4755"}},
	}
	for _, tt := range tests {
		t.Run(strings.TrimSpace(tt.stdout), func(t *testing.T) {
			pool, _ := recordingPool(t, func(string) (string, string, int) { return tt.stdout, "", 0 })
			got, err := pool.def.getPermission(context.Background(), "/tmp/p")
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestPermissionSchemaValidation(t *testing.T) {
	for name, s := range map[string]map[string]*schema.Schema{"file": schemaFileResource, "directory": schemaDirectoryResource} {
		t.Run(name+" mode", func(t *testing.T) {
			validate := s["mode"].ValidateFunc
			for _, ok := range []string{"644", "0644", "755", "4755", "000"} {
				_, errs := validate(ok, "mode")
				require.Empty(t, errs, ok)
			}
			for _, bad := range []string{"644; id #", "x644", "64444", "64", "0o644", "", "$(id)755", "755\n"} {
				_, errs := validate(bad, "mode")
				require.NotEmpty(t, errs, bad)
			}
			suppress := s["mode"].DiffSuppressFunc
			require.True(t, suppress("mode", "644", "0644", nil))
			require.True(t, suppress("mode", "40", "040", nil))
			require.False(t, suppress("mode", "644", "600", nil))
			require.False(t, suppress("mode", "", "644", nil))
		})
		for _, attr := range []string{"owner", "group"} {
			t.Run(name+" "+attr, func(t *testing.T) {
				maxID := int64(maxLinuxID) // not a constant expression, so the test also compiles where int is 32 bits
				if strconv.IntSize < 64 {
					t.Skip("IDs above the int range cannot be expressed")
				}
				validate := s[attr].ValidateFunc
				for _, ok := range []int64{0, 1000, 65535, 65536, 100000, maxID} {
					_, errs := validate(int(ok), attr)
					require.Empty(t, errs, ok)
				}
				for _, bad := range []int64{-1, maxID + 1, 1 << 40} {
					_, errs := validate(int(bad), attr)
					require.NotEmpty(t, errs, bad)
				}
			})
		}
	}
}

func TestModeDiffsEndToEnd(t *testing.T) {
	diff := func(t *testing.T, res *schema.Resource, stateMode, configMode string) *terraform.InstanceDiff {
		t.Helper()
		state := &terraform.InstanceState{ID: "managed", Attributes: map[string]string{
			"id": "managed", "path": "/tmp/x", "mode": stateMode, "owner": "0", "group": "0", "content": "",
			"ignore_content": "false", "overwrite": "false", "recycle_path": "",
		}}
		d, err := res.Diff(context.Background(), state, terraform.NewResourceConfigRaw(map[string]interface{}{
			"path": "/tmp/x", "mode": configMode,
		}), nil)
		require.NoError(t, err)
		return d
	}
	file := fileResource()

	require.Nil(t, diff(t, file, "644", "0644"), "0644 is the same mode as the 644 that stat reports")
	require.Nil(t, diff(t, file, "40", "040"), "state written before Read padded the mode")
	require.NotNil(t, diff(t, file, "755", "4755"), "a setuid bit is a real change")
	require.NotNil(t, diff(t, file, "644", "600"))
}
