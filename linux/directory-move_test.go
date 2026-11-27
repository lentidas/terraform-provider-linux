package linux

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/stretchr/testify/require"
)

// shellPool answers commands by running them in a real shell on this machine, so the shell code of a directory move
// is exercised for real. It refuses to run anything that does not stay inside root.
func shellPool(t *testing.T, shell, root string) *linuxPool {
	t.Helper()
	return testReadPool(t, func(command string) (string, string, int) {
		if !strings.Contains(command, root) {
			t.Errorf("refusing to run a command outside %s: %s", root, command)
			return "", "refused", 126
		}
		cmd := exec.Command(shell, "-c", command)
		var stdout, stderr strings.Builder
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		status := 0
		if err := cmd.Run(); err != nil {
			exitErr, ok := err.(*exec.ExitError)
			if !ok {
				t.Fatalf("running %s: %v", shell, err)
			}
			status = exitErr.ExitCode()
		}
		return stdout.String(), stderr.String(), status
	})
}

// installedShells lists the POSIX shells to run the move under. dash is the strictest one and the Debian default.
func installedShells() (shells []string) {
	for _, s := range []string{"sh", "dash", "bash"} {
		if p, err := exec.LookPath(s); err == nil {
			shells = append(shells, p)
		}
	}
	return
}

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		p := filepath.Join(root, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	}
}

func readTree(t *testing.T, root string) map[string]string {
	t.Helper()
	tree := map[string]string{}
	require.NoError(t, filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		rel, _ := filepath.Rel(root, p)
		tree[rel] = string(b)
		return err
	}))
	return tree
}

func TestMoveDirectory(t *testing.T) {
	for _, shell := range installedShells() {
		t.Run(filepath.Base(shell), func(t *testing.T) {
			t.Run("renames the directory and keeps hidden entries", func(t *testing.T) {
				root := t.TempDir()
				writeTree(t, root, map[string]string{
					"old/visible": "v", "old/.hidden": "h", "old/sub/.deep/file": "d", "old/sub/other": "o",
				})
				pool := shellPool(t, shell, root)

				err := pool.def.moveDirectory(context.Background(), filepath.Join(root, "old"), filepath.Join(root, "missing/parent/new"))
				require.NoError(t, err)

				require.Equal(t,
					map[string]string{"visible": "v", ".hidden": "h", "sub/.deep/file": "d", "sub/other": "o"},
					readTree(t, filepath.Join(root, "missing/parent/new")))
				require.NoDirExists(t, filepath.Join(root, "old"))
			})

			t.Run("merges into an existing directory", func(t *testing.T) {
				root := t.TempDir()
				writeTree(t, root, map[string]string{
					"old/same": "from old", "old/.hidden": "h", "old/fresh": "f",
					"new/same": "from new", "new/keep": "k",
				})
				pool := shellPool(t, shell, root)

				err := pool.def.moveDirectory(context.Background(), filepath.Join(root, "old"), filepath.Join(root, "new"))
				require.NoError(t, err)

				require.Equal(t,
					map[string]string{"same": "from old", ".hidden": "h", "fresh": "f", "keep": "k"},
					readTree(t, filepath.Join(root, "new")), "the entries only in the destination must stay")
				require.NoDirExists(t, filepath.Join(root, "old"))
			})

			t.Run("treats unusual names as data", func(t *testing.T) {
				root := t.TempDir()
				oldName, newName := `it's a dir *`, `new $(touch pwned) "q"`
				writeTree(t, root, map[string]string{oldName + "/file": "x"})
				pool := shellPool(t, shell, root)

				err := pool.def.moveDirectory(context.Background(), filepath.Join(root, oldName), filepath.Join(root, newName))
				require.NoError(t, err)

				require.Equal(t, map[string]string{"file": "x"}, readTree(t, filepath.Join(root, newName)))
				require.NoFileExists(t, filepath.Join(root, "pwned"))
				require.NoFileExists(t, "pwned")
			})

			t.Run("leaves the source alone when the destination is a file", func(t *testing.T) {
				root := t.TempDir()
				writeTree(t, root, map[string]string{"old/data": "precious", "new": "i am a file"})
				pool := shellPool(t, shell, root)

				err := pool.def.moveDirectory(context.Background(), filepath.Join(root, "old"), filepath.Join(root, "new"))
				require.Error(t, err)

				require.Equal(t, map[string]string{"data": "precious"}, readTree(t, filepath.Join(root, "old")))
				require.Equal(t, "i am a file", readTree(t, root)["new"])
			})

			t.Run("refuses nested paths without running anything", func(t *testing.T) {
				root := t.TempDir()
				writeTree(t, root, map[string]string{"a/data": "precious"})
				pool := shellPool(t, shell, root)

				for _, pair := range [][2]string{{"a", "a/b"}, {"a/b", "a"}, {"a", "a/../a/c"}} {
					err := pool.def.moveDirectory(context.Background(), filepath.Join(root, pair[0]), filepath.Join(root, pair[1]))
					require.Error(t, err, "%v", pair)
				}
				require.Equal(t, map[string]string{"a/data": "precious"}, readTree(t, root))
			})
		})
	}
}

func TestDirectoryUpdateRecordsTheNewPathWhenALaterStepFails(t *testing.T) {
	pool, _ := recordingPool(t, failChown)
	res := directoryResource()
	state := &terraform.InstanceState{ID: "managed", Attributes: map[string]string{
		"id": "managed", "path": "/srv/a", "overwrite": "true", "owner": "0", "group": "0", "mode": "755",
		"recycle_path": "",
	}}
	diff := &terraform.InstanceDiff{Attributes: map[string]*terraform.ResourceAttrDiff{
		"path": {Old: "/srv/a", New: "/srv/b"},
	}}

	got, diags := res.Apply(context.Background(), state, diff, pool)
	require.True(t, diags.HasError(), "%v", diags)
	require.Equal(t, "/srv/b", got.Attributes["path"], "the directory was already moved")
}
