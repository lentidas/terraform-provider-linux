package linux

import (
	"context"
	"fmt"
	"path"
	"strings"

	"al.essio.dev/pkg/shellescape"
	"github.com/hashicorp/terraform/communicator/remote"
)

type directory struct {
	path       string
	permission permission

	overwrite   bool
	recyclePath string
}

func (l *linux) readDirectory(ctx context.Context, path string) (d *directory, err error) {
	perm, err := l.getPermission(ctx, path)
	if err != nil {
		return
	}
	d = &directory{path: path, permission: perm}
	return
}

func (l *linux) createDirectory(ctx context.Context, d *directory) (err error) {
	if d == nil {
		return errNil
	}

	if !d.overwrite {
		if err = l.reservePath(ctx, d.path); err != nil {
			return
		}
	}

	err = l.mkdirp(ctx, d.path)
	if err != nil {
		return
	}

	return l.setPermission(ctx, d.path, d.permission)
}

func (l *linux) deleteDirectory(ctx context.Context, f *directory) (err error) {
	if f == nil {
		return
	}
	return l.remove(ctx, f.path, f.recyclePath)
}

// updateDirectory reports whether the directory was moved to its new path, so that a caller can record where the
// directory really is when a later step fails.
func (l *linux) updateDirectory(ctx context.Context, old, new *directory) (moved bool, err error) {
	if old == nil {
		return false, l.createDirectory(ctx, new)
	}
	if new == nil {
		return false, l.deleteDirectory(ctx, old)
	}
	if path.Clean(old.path) != path.Clean(new.path) { // a trailing slash or a double one is not a move
		if !new.overwrite {
			if err = l.reservePath(ctx, new.path); err != nil {
				return
			}
		}
		if err = l.moveDirectory(ctx, old.path, new.path); err != nil {
			return
		}
		moved = true
	}

	d := &directory{}
	*d = *new
	d.overwrite = true
	return moved, l.createDirectory(ctx, d)
}

// moveDirectory renames the directory when the destination does not exist. When it exists, which the caller only
// allows with overwrite, the contents are merged into it, the entries of the source win over entries with the same
// name, and the source is only removed after everything was copied.
func (l *linux) moveDirectory(ctx context.Context, oldPath, newPath string) error {
	o, n := path.Clean(oldPath), path.Clean(newPath)
	if o == n {
		return nil
	}
	if o == "/" || o == "." || n == "/" || n == "." {
		return fmt.Errorf("refusing to move directory %q to %q", oldPath, newPath)
	}
	if strings.HasPrefix(n, o+"/") || strings.HasPrefix(o, n+"/") {
		return fmt.Errorf("cannot move directory %q to %q: one is inside the other", oldPath, newPath)
	}

	// The paths differ as strings, but they can still be one directory, through a symlink or a bind mount. Merging a
	// directory into itself and then removing it would delete it, so that is checked here on the host and not left to
	// the behavior of cp. A shell without -ef fails the test, which only means the guard does nothing there.
	oldQ, newQ := shellescape.Quote(o), shellescape.Quote(n)
	cmd := fmt.Sprintf(`{ if [ -e %[2]s ] || [ -L %[2]s ]; then `+
		`if [ %[1]s -ef %[2]s ]; then echo 'the old and the new path are the same directory' >&2; false; `+
		`else %[3]s && %[4]s; fi; `+
		`else %[5]s && %[6]s; fi ;}`,
		oldQ, newQ,
		shellescape.QuoteCommand([]string{"cp", "-a", o + "/.", n + "/"}),
		shellescape.QuoteCommand([]string{"rm", "-rf", o}),
		shellescape.QuoteCommand([]string{"mkdir", "-p", path.Dir(n)}),
		shellescape.QuoteCommand([]string{"mv", o, n}),
	)
	return l.exec(ctx, &remote.Cmd{Command: cmd})
}
