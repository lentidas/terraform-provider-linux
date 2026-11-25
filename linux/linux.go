package linux

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"al.essio.dev/pkg/shellescape"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform/communicator/remote"
	"github.com/hashicorp/terraform/communicator/ssh"
	"github.com/hashicorp/terraform/terraform"
)

var (
	errPathNotExist = errors.New("Path doesn't exist") // asuming read permission is allowed
	errNil          = errors.New("unexpected nil object")
)

type linux struct {
	connInfo map[string]string

	comm      *ssh.Communicator
	commErr   error
	commOnce  sync.Once
	commMutex sync.Mutex
}

func (l *linux) Equal(li *linux) (eq bool) {
	if l == nil || li == nil {
		return l == li
	}

	for k, v := range l.connInfo {
		if li.connInfo[k] != v {
			return false
		}
	}
	return true
}

func (l *linux) init(ctx context.Context) error {
	l.comm, l.commErr = ssh.NewNoPty(&terraform.InstanceState{Ephemeral: terraform.EphemeralState{
		ConnInfo: l.connInfo,
	}})
	if l.commErr != nil {
		return l.commErr
	}

	l.commErr = l.comm.Connect(nil)
	return l.commErr
}

func (l *linux) communicator(ctx context.Context) (*ssh.Communicator, error) {
	l.commOnce.Do(func() {
		err := resource.RetryContext(ctx, 5*time.Minute, func() *resource.RetryError {
			var errNet net.Error
			switch err := l.init(ctx); {
			default:
				return nil

			case errors.As(err, &errNet):
				return resource.RetryableError(errNet)

			case err != nil:
				return resource.NonRetryableError(err)
			}
		})

		if l.commErr == nil {
			l.commErr = err
		}
	})

	return l.comm, l.commErr
}

func (l *linux) exec(ctx context.Context, cmd *remote.Cmd) (err error) {
	l.commMutex.Lock()
	defer l.commMutex.Unlock()

	c, err := l.communicator(ctx)
	if err != nil {
		return
	}
	if err = c.Start(cmd); err != nil {
		return
	}
	return cmd.Wait()
}

func (l *linux) upload(ctx context.Context, path string, input io.Reader) (err error) {
	l.commMutex.Lock()
	defer l.commMutex.Unlock()

	c, err := l.communicator(ctx)
	if err != nil {
		return
	}
	return c.Upload(path, input)
}

func (l *linux) uploadScript(ctx context.Context, path string, input io.Reader) (err error) {
	l.commMutex.Lock()
	defer l.commMutex.Unlock()

	c, err := l.communicator(ctx)
	if err != nil {
		return
	}
	return c.UploadScript(path, input)
}

func (l *linux) scriptPath(ctx context.Context) string {
	c, err := l.communicator(ctx)
	if err != nil {
		return ""
	}
	return c.ScriptPath()
}

type permission struct {
	owner uint32
	group uint32
	mode  string
}

// Linux reserves the maximum uint32 value as the "no change" ID (-1) of chown.
const maxLinuxID = 4294967294

var modePattern = regexp.MustCompile(`^[0-7]{3,4}$`)

func validateID(v interface{}, k string) (ws []string, errs []error) {
	if id, ok := v.(int); !ok || id < 0 || int64(id) > maxLinuxID {
		errs = append(errs, fmt.Errorf("%q must be between 0 and %d, got %v", k, int64(maxLinuxID), v))
	}
	return
}

func validateMode(v interface{}, k string) (ws []string, errs []error) {
	if s, ok := v.(string); !ok || !modePattern.MatchString(s) {
		errs = append(errs, fmt.Errorf("%q must be three or four octal digits, for example 644 or 0644", k))
	}
	return
}

// The remote reports modes without leading zeros, so 0644 and 644 must not be a difference.
func suppressEquivalentMode(k, old, new string, d *schema.ResourceData) bool {
	o, errOld := strconv.ParseUint(old, 8, 16)
	n, errNew := strconv.ParseUint(new, 8, 16)
	return errOld == nil && errNew == nil && o == n
}

func (l *linux) setPermission(ctx context.Context, path string, p permission) (err error) {
	pathSafe := shellescape.Quote(path)
	cmd := fmt.Sprintf(`{ chown %d:%d %s && chmod %s %s ;}`,
		p.owner, p.group, pathSafe, shellescape.Quote(p.mode), pathSafe)
	return l.exec(ctx, &remote.Cmd{Command: cmd})
}

func (l *linux) getPermission(ctx context.Context, path string) (p permission, err error) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	// Only ENOENT means absence; other stat failures must not cause Terraform to forget a managed resource.
	cmd := "LC_ALL=C " + shellescape.QuoteCommand([]string{"stat", "-c", "%u %g %a", "--", path})
	err = l.exec(ctx, &remote.Cmd{Command: cmd, Stdout: stdout, Stderr: stderr})
	var exitError *remote.ExitError
	if errors.As(err, &exitError) && exitError.ExitStatus == 1 &&
		strings.HasSuffix(stderr.String(), ": No such file or directory\n") {
		err = errPathNotExist
		return
	}
	if err != nil {
		if stderr.Len() > 0 {
			err = fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
		}
		return
	}

	out, err := stdout.ReadString('\n')
	if err != nil {
		return
	}
	parts := strings.Split(strings.TrimSpace(out), " ")
	if len(parts) != 3 {
		err = fmt.Errorf("malformed output of %q: %q", cmd, out)
		return
	}
	owner, err := strconv.ParseUint(parts[0], 10, 32)
	if err != nil {
		err = fmt.Errorf("while parsing owner id %q: %w", parts[0], err)
		return
	}
	group, err := strconv.ParseUint(parts[1], 10, 32)
	if err != nil {
		err = fmt.Errorf("while parsing group id %q: %w", parts[1], err)
		return
	}
	p = permission{
		owner: uint32(owner),
		group: uint32(group),
		mode:  strings.Repeat("0", max(0, 3-len(parts[2]))) + parts[2], // stat drops the leading zeros of "%a"
	}
	return
}

func (l *linux) reservePath(ctx context.Context, path string) (err error) {
	var exitError *remote.ExitError
	cmd := fmt.Sprintf("[ ! -e %s ]", shellescape.Quote(path))
	if err = l.exec(ctx, &remote.Cmd{Command: cmd}); errors.As(err, &exitError) {
		return fmt.Errorf("path '%s' exist", path)
	}
	return
}

func (l *linux) mkdirp(ctx context.Context, path string) (err error) {
	cmd := shellescape.QuoteCommand([]string{"mkdir", "-p", path})
	return l.exec(ctx, &remote.Cmd{Command: cmd})
}

func (l *linux) cat(ctx context.Context, path string) (s string, err error) {
	stdout := new(bytes.Buffer)
	cmd := shellescape.QuoteCommand([]string{"cat", path})
	if err = l.exec(ctx, &remote.Cmd{Command: cmd, Stdout: stdout}); err != nil {
		return
	}
	return stdout.String(), nil
}

func (l *linux) mv(ctx context.Context, old, new string) (err error) {
	cmd := shellescape.QuoteCommand([]string{"mv", old, new})
	return l.exec(ctx, &remote.Cmd{Command: cmd})
}

func (l *linux) remove(ctx context.Context, path, recyclePath string) (err error) {
	if path == "" {
		return
	}

	var cmd string
	if recyclePath != "" {
		recycleFolder := fmt.Sprintf("%s/%d", recyclePath, time.Now().Unix())
		cmd = fmt.Sprintf(`{ [ ! -e %s ] || { %s && %s ;} ;}`,
			shellescape.Quote(path),
			shellescape.QuoteCommand([]string{"mkdir", "-p", recycleFolder}),
			shellescape.QuoteCommand([]string{"mv", path, recycleFolder}),
		)
	} else {
		cmd = fmt.Sprintf(`{ [ ! -e %s ] || %s ;}`,
			shellescape.Quote(path),
			shellescape.QuoteCommand([]string{"rm", "-rf", path}),
		)
	}
	return l.exec(ctx, &remote.Cmd{Command: cmd})
}

func (l *linux) lforwardTCP(ctx context.Context, local string, remote string) (err error) {
	comm, err := l.communicator(ctx)
	if err != nil {
		return err
	}

	listener, err := net.Listen("tcp", local)
	if err != nil {
		return err
	}

	go func(ctx context.Context) {
		defer listener.Close()
		for {
			select {
			case <-ctx.Done():
			default:
			}

			lconn, err := listener.Accept()
			if err != nil {
				continue
			}

			rconn, err := comm.Dial("tcp", remote)
			if err != nil {
				lconn.Close()
				continue
			}

			// What to do here if its error
			go io.Copy(lconn, rconn)
			go io.Copy(rconn, lconn)
		}
	}(ctx)

	return
}
