package linux

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"al.essio.dev/pkg/shellescape"
	"github.com/hashicorp/terraform/communicator/remote"
)

type env map[string]string

var envNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func (e env) validate() error {
	for k := range e {
		if !envNamePattern.MatchString(k) {
			return fmt.Errorf("invalid environment variable name %q", k)
		}
	}
	return nil
}

// overlay returns a copy of e in which the entries of o win.
func (e env) overlay(o env) env {
	m := make(env, len(e)+len(o))
	for k, v := range e {
		m[k] = v
	}
	for k, v := range o {
		m[k] = v
	}
	return m
}

// mask keeps the names and hides the values.
func (e env) mask() env {
	m := make(env, len(e))
	for k := range e {
		m[k] = "***"
	}
	return m
}

func (e env) serialize(sep string) string {
	keys := make([]string, 0, len(e))
	for k := range e {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	b := strings.Builder{}
	for i, k := range keys {
		if i > 0 {
			b.WriteString(sep)
		}
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(shellescape.Quote(e[k]))
	}
	return b.String()
}

func (e env) inline() string {
	return e.serialize(" ")
}

func (e env) multiline() string {
	return e.serialize("\n")
}

type script struct {
	l *linux

	workdir string
	env     env
	// sensitiveEnv wins over env on a duplicate name, and its values never reach logs or error messages.
	sensitiveEnv env
	interpreter  []string
	body         string

	stdin io.Reader
}

func (sc *script) scriptForUpload() (io.Reader, error) {
	if len(sc.interpreter) == 0 {
		return strings.NewReader(sc.body), nil
	}

	// prevent communicator from appending #!/bin/sh
	reader := bufio.NewReader(strings.NewReader(sc.body))
	prefix, err := reader.Peek(2)
	if err != nil {
		return nil, fmt.Errorf("Error reading script: %s", err)
	}
	var script bytes.Buffer
	if string(prefix) != "#!" {
		script.WriteString("#!\n")
	}
	_, _ = script.ReadFrom(reader)
	return &script, nil
}

func (sc *script) upload(ctx context.Context) (path string, err error) {
	path = shellescape.Quote(sc.l.scriptPath(ctx))
	s, err := sc.scriptForUpload()
	if err != nil {
		return "", err
	}
	if err := sc.l.uploadScript(ctx, path, s); err != nil {
		return "", err
	}
	return
}

func (sc *script) exec(ctx context.Context) (res string, err error) {
	if err = sc.env.validate(); err != nil {
		return
	}
	if err = sc.sensitiveEnv.validate(); err != nil {
		return
	}

	path, err := sc.upload(ctx)
	if err != nil {
		return
	}
	defer func() { _ = sc.l.remove(ctx, path, "") }()

	command := func(e env) string {
		return fmt.Sprintf(`{ %s && %s && %s %s ;}`,
			shellescape.QuoteCommand([]string{"mkdir", "-p", sc.workdir}),
			shellescape.QuoteCommand([]string{"cd", sc.workdir}),
			e.inline(), shellescape.QuoteCommand(append(sc.interpreter, path)),
		)
	}
	cmd := command(sc.env.overlay(sc.sensitiveEnv))
	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	err = sc.l.exec(ctx, &remote.Cmd{
		Command:  cmd,
		Redacted: command(sc.env.overlay(sc.sensitiveEnv.mask())),
		Stdin:    sc.stdin,
		Stdout:   stdout,
		Stderr:   stderr,
	})
	if err != nil {
		err = fmt.Errorf("stderr: %s\nerror: %w", stderr, err)
		return
	}
	return stdout.String(), nil
}
