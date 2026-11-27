package linux

import (
	"context"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRemoveRecyclesIntoUniqueFolders(t *testing.T) {
	pool, commands := recordingPool(t, func(string) (string, string, int) { return "", "", 0 })

	for range 2 {
		require.NoError(t, pool.def.remove(context.Background(), "/srv/a/app.conf", "/tmp/recycle"))
	}

	// Two removals in the same second must not share a folder, or the second move would overwrite the first.
	folder := regexp.MustCompile(`mkdir (/tmp/recycle/\d+-[0-9a-f]{8}) && mv /srv/a/app\.conf (/tmp/recycle/\S+) ;`)
	var folders []string
	for _, c := range commands() {
		m := folder.FindStringSubmatch(c)
		require.NotNil(t, m, "unexpected command %q", c)
		require.Equal(t, m[1], m[2], "the file must be moved into the folder that was just created")
		folders = append(folders, m[1])
	}
	require.Len(t, folders, 2)
	require.NotEqual(t, folders[0], folders[1])
}
