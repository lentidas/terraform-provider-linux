package linux

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// A dangling symlink has nothing behind it, so `-e`, which follows links, calls its path free. The provider would then
// write through the link, or leave the link behind on destroy.
func TestPathChecksSeeDanglingSymlinks(t *testing.T) {
	for _, shell := range installedShells() {
		t.Run(filepath.Base(shell), func(t *testing.T) {
			newRoot := func(t *testing.T) (root string, pool *linuxPool) {
				root = t.TempDir()
				require.NoError(t, os.Symlink(filepath.Join(root, "missing"), filepath.Join(root, "dangling")))
				return root, shellPool(t, shell, root)
			}

			t.Run("reservePath treats a dangling symlink as taken", func(t *testing.T) {
				root, pool := newRoot(t)

				err := pool.def.reservePath(context.Background(), filepath.Join(root, "dangling"))
				require.ErrorContains(t, err, "exist")
			})

			t.Run("reservePath still accepts a free path and refuses a file", func(t *testing.T) {
				root, pool := newRoot(t)
				require.NoError(t, os.WriteFile(filepath.Join(root, "file"), nil, 0o644))

				require.NoError(t, pool.def.reservePath(context.Background(), filepath.Join(root, "free")))
				require.ErrorContains(t, pool.def.reservePath(context.Background(), filepath.Join(root, "file")), "exist")
			})

			t.Run("remove deletes a dangling symlink", func(t *testing.T) {
				root, pool := newRoot(t)

				require.NoError(t, pool.def.remove(context.Background(), filepath.Join(root, "dangling"), ""))
				_, err := os.Lstat(filepath.Join(root, "dangling"))
				require.ErrorIs(t, err, os.ErrNotExist)
			})

			t.Run("remove recycles a dangling symlink without touching its target", func(t *testing.T) {
				root, pool := newRoot(t)
				recycle := filepath.Join(root, "recycle")

				require.NoError(t, pool.def.remove(context.Background(), filepath.Join(root, "dangling"), recycle))
				_, err := os.Lstat(filepath.Join(root, "dangling"))
				require.ErrorIs(t, err, os.ErrNotExist)
				entries, err := os.ReadDir(recycle)
				require.NoError(t, err)
				require.Len(t, entries, 1, "the link must be in a recycle folder")
			})

			t.Run("remove of a missing path is still fine", func(t *testing.T) {
				root, pool := newRoot(t)

				require.NoError(t, pool.def.remove(context.Background(), filepath.Join(root, "never-existed"), ""))
			})
		})
	}
}
