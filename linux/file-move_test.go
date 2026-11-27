package linux

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/stretchr/testify/require"
)

// failChown makes every ownership change fail, which is the last step of an update after the file was moved.
func failChown(command string) (string, string, int) {
	if strings.HasPrefix(command, "{ chown ") {
		return "", "chown: changing ownership: Operation not permitted", 1
	}
	return "", "", 0
}

func movedFiles() (*file, *file) {
	old := &file{path: "/srv/a/app.conf", ignoreContent: true, permission: permission{mode: "644"}}
	new := &file{path: "/srv/b/new/app.conf", ignoreContent: true, overwrite: true, permission: permission{mode: "644"}}
	return old, new
}

func TestUpdateFileCreatesTheParentBeforeMoving(t *testing.T) {
	pool, commands := recordingPool(t, func(string) (string, string, int) { return "", "", 0 })
	old, new := movedFiles()

	moved, err := pool.def.updateFile(context.Background(), old, new)
	require.NoError(t, err)
	require.True(t, moved)

	all := strings.Join(commands(), "\n")
	mkdir := strings.Index(all, "mkdir -p /srv/b/new")
	mv := strings.Index(all, "mv /srv/a/app.conf /srv/b/new/app.conf")
	require.GreaterOrEqual(t, mkdir, 0, "the destination directory must be created: %s", all)
	require.Greater(t, mv, mkdir, "the move must follow the creation of the directory: %s", all)
}

func TestUpdateFileReportsWhetherTheFileWasMoved(t *testing.T) {
	t.Run("a failure after the move", func(t *testing.T) {
		pool, _ := recordingPool(t, failChown)
		old, new := movedFiles()

		moved, err := pool.def.updateFile(context.Background(), old, new)
		require.Error(t, err)
		require.True(t, moved, "the file is already at its new path")
	})

	t.Run("a failure of the move itself", func(t *testing.T) {
		pool, _ := recordingPool(t, func(command string) (string, string, int) {
			if strings.HasPrefix(command, "mv ") {
				return "", "mv: cannot move", 1
			}
			return "", "", 0
		})
		old, new := movedFiles()

		moved, err := pool.def.updateFile(context.Background(), old, new)
		require.Error(t, err)
		require.False(t, moved, "the file never left its old path")
	})
}

func TestFileUpdateRecordsTheNewPathWhenALaterStepFails(t *testing.T) {
	pool, _ := recordingPool(t, failChown)
	res := fileResource()
	state := &terraform.InstanceState{ID: "managed", Attributes: map[string]string{
		"id": "managed", "path": "/srv/a/app.conf", "content": "", "ignore_content": "true", "overwrite": "true",
		"owner": "0", "group": "0", "mode": "644", "recycle_path": "",
	}}
	diff := &terraform.InstanceDiff{Attributes: map[string]*terraform.ResourceAttrDiff{
		"path": {Old: "/srv/a/app.conf", New: "/srv/b/new/app.conf"},
	}}

	got, diags := res.Apply(context.Background(), state, diff, pool)
	require.True(t, diags.HasError(), "%v", diags)
	require.Equal(t, "/srv/b/new/app.conf", got.Attributes["path"],
		"the file was moved, so the next plan must start from its real path")
	require.Equal(t, "managed", got.ID)
}
