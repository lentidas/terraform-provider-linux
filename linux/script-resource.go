package linux

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform/communicator/remote"
	"github.com/spf13/cast"
)

type haschange interface {
	HasChange(key string) bool
}

const (
	attrScriptProviderOverride = "provider_override"

	attrScriptLifecycleCommands      = "lifecycle_commands"
	attrScriptLifecycleCommandCreate = "create"
	attrScriptLifecycleCommandRead   = "read"
	attrScriptLifecycleCommandUpdate = "update"
	attrScriptLifecycleCommandDelete = "delete"
	attrScriptInterpreter            = "interpreter"

	attrScriptTriggers             = "triggers"
	attrScriptEnvironment          = "environment"
	attrScriptSensitiveEnvironment = "sensitive_environment"
	attrScriptWorkingDirectory     = "working_directory"

	attrScriptOutput = "output"

	attrScriptDirtyOutput  = "__dirty_output__"
	attrScriptFaultyOutput = "__faulty_output__"
)

var schemaScriptResource = map[string]*schema.Schema{
	attrScriptProviderOverride: {
		Description: descProviderOverride,
		Type:        schema.TypeList,
		Optional:    true,
		MaxItems:    1,
		Elem: &schema.Resource{
			Schema: subSchemaProviderOverride,
		},
	},

	attrScriptLifecycleCommands: {
		Description: "Commands uploaded and executed on the remote host in the matching phase of the Terraform " +
			"lifecycle. For long scripts, use the `file` function.",
		Type:     schema.TypeList,
		Required: true,
		MaxItems: 1,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				attrScriptLifecycleCommandCreate: {
					Description: "Commands executed in the create phase.",
					Type:        schema.TypeString,
					Required:    true,
				},
				attrScriptLifecycleCommandUpdate: {
					Description: "Commands executed in the update phase, with the previous `output` available on stdin. " +
						"To keep plans consistent, especially when `output` is a dependency of other objects, they " +
						"should leave `output` as `create` would on a resource that does not exist. When omitted, any " +
						"detected change recreates the resource instead.",
					Type:     schema.TypeString,
					Optional: true,
				},
				attrScriptLifecycleCommandRead: {
					Description: "Commands executed in the read phase and after `create` or `update`. Their standard " +
						"output is recorded in `output`, and a change in it triggers an update or a recreation. If they " +
						"fail, the resource is considered absent and is planned for recreation, and in that case " +
						"`delete` is not executed. Before applying, either accept that the resource is gone, fix the " +
						"host so the commands succeed, or update them. They should not write anything, or at least be " +
						"safe to retry.",
					Type:     schema.TypeString,
					Required: true,
				},
				attrScriptLifecycleCommandDelete: {
					Description: "Commands executed in the delete phase.",
					Type:        schema.TypeString,
					Required:    true,
				},
			},
		},
	},
	attrScriptInterpreter: {
		Description: "Interpreter used to run each lifecycle command, as a program followed by its arguments. The path of " +
			"the uploaded script is appended as the last argument. When empty, the script is executed directly, " +
			"using its `#!` line or `/bin/sh`. The script is uploaded readable by the connecting user only, so an " +
			"interpreter that switches user, such as `sudo -u`, cannot read it. Changing it follows the rules " +
			"described in [Updating the instructions](#updating-the-instructions).",
		Type:     schema.TypeList,
		Optional: true,
		Elem: &schema.Schema{
			Type: schema.TypeString,
		},
	},

	attrScriptTriggers: {
		Description: "Values whose changes force the resource to be recreated, like `triggers` of `null_resource`.",
		Type:        schema.TypeMap,
		Optional:    true,
		ForceNew:    true,
	},
	attrScriptEnvironment: {
		Description: "Environment variables available to each lifecycle command.",
		Type:        schema.TypeMap,
		Optional:    true,
		Elem:        schema.TypeString,
	},
	attrScriptSensitiveEnvironment: {
		Description: "Like `environment`, but marked sensitive so Terraform hides the values in its output. On a " +
			"duplicate name, the value defined here wins. The provider keeps the values out of its own logs, error " +
			"messages and the internal `__faulty_output__` and `__dirty_output__` attributes; the map itself is " +
			"stored in state, marked sensitive. They are still part of the command line executed on the host, " +
			"where other local users can see them in the process list while a lifecycle command runs, and a " +
			"command that prints them can expose them in `output`.",
		Type:      schema.TypeMap,
		Optional:  true,
		Elem:      schema.TypeString,
		Sensitive: true,
	},
	attrScriptWorkingDirectory: {
		Description: "Directory in which each lifecycle command runs. It is created if it does not exist. Defaults to `.`.",
		Type:        schema.TypeString,
		Optional:    true,
		Default:     ".",
	},

	attrScriptOutput: {
		Description: "Standard output of the `read` commands.",
		Type:        schema.TypeString,
		Computed:    true,
	},

	attrScriptDirtyOutput: {
		Description: "Internal state used by the provider to detect changes of `output`. It cannot be set.",
		Type:        schema.TypeString,
		Optional:    true,
		Default:     "",
		ValidateDiagFunc: func(i interface{}, c cty.Path) (d diag.Diagnostics) {
			return diag.Errorf("`%s` should not be set from configuration.", attrScriptDirtyOutput)
		},
	},
	attrScriptFaultyOutput: {
		Description: "Internal state used by the provider to mark failed `read` commands. It cannot be set.",
		Type:        schema.TypeString,
		Optional:    true,
		Default:     "",
		ValidateDiagFunc: func(i interface{}, c cty.Path) (d diag.Diagnostics) {
			return diag.Errorf("`%s` should not be set from configuration.", attrScriptFaultyOutput)
		},
	},
}

type handlerScriptResource struct {
}

func (h handlerScriptResource) attrCommands() map[string]bool {
	return map[string]bool{
		attrScriptLifecycleCommands: true,
		attrScriptInterpreter:       true,
	}
}
func (h handlerScriptResource) attrInputs() map[string]bool {
	return map[string]bool{
		attrScriptEnvironment:          true,
		attrScriptSensitiveEnvironment: true,
		attrScriptWorkingDirectory:     true,
	}
}
func (h handlerScriptResource) attrOutputs() map[string]bool {
	return map[string]bool{
		attrScriptOutput: true,
	}
}
func (h handlerScriptResource) attrInternal() map[string]bool {
	return map[string]bool{
		attrScriptDirtyOutput:  true,
		attrScriptFaultyOutput: true,
	}
}
func (h handlerScriptResource) attrs(source ...map[string]bool) (m map[string]bool) {
	m = make(map[string]bool)
	if len(source) > 0 {
		for _, s := range source {
			for k, v := range s {
				m[k] = v
			}
		}
		return
	}
	for k := range h.attrCommands() {
		m[k] = true
	}
	for k := range h.attrInputs() {
		m[k] = true
	}
	for k := range h.attrOutputs() {
		m[k] = true
	}
	for k := range h.attrInternal() {
		m[k] = true
	}
	return
}

func (h handlerScriptResource) changed(rd haschange, attrs map[string]bool) (changed []string) {
	for k := range attrs {
		if rd.HasChange(k) {
			changed = append(changed, k)
		}
	}
	return
}
func (h handlerScriptResource) changedAttrInputs(rd haschange) (changed []string) {
	return h.changed(rd, h.attrInputs())
}
func (h handlerScriptResource) changedAttrCommands(rd haschange) (changed []string) {
	return h.changed(rd, h.attrCommands())
}
func (h handlerScriptResource) changedAttrInternal(rd haschange) (changed []string) {
	return h.changed(rd, h.attrInternal())
}

func (h handlerScriptResource) setNewComputed(rd *schema.ResourceDiff) (err error) {
	for k := range h.attrOutputs() {
		err = rd.SetNewComputed(k) // assume all computed value will changedAttrInputs
		if err != nil {
			return
		}
	}
	return
}
func (h handlerScriptResource) newScript(rd *schema.ResourceData, l *linux, attrLifeCycle string) (s *script) {
	if rd == nil {
		return
	}

	lc := cast.ToSlice(rd.Get(attrScriptLifecycleCommands))[0]
	s = &script{
		l: l,

		workdir:      cast.ToString(rd.Get(attrScriptWorkingDirectory)),
		env:          cast.ToStringMapString(rd.Get(attrScriptEnvironment)),
		sensitiveEnv: cast.ToStringMapString(rd.Get(attrScriptSensitiveEnvironment)),
		interpreter:  cast.ToStringSlice(rd.Get(attrScriptInterpreter)),
		body:         cast.ToStringMapString(lc)[attrLifeCycle],
	}
	return
}

func (h handlerScriptResource) read(ctx context.Context, rd *schema.ResourceData, l *linux) (err error) {
	sc := h.newScript(rd, l, attrScriptLifecycleCommandRead)
	res, err := sc.exec(ctx)
	if err != nil {
		return
	}
	if err = rd.Set(attrScriptOutput, res); err != nil {
		return
	}
	return
}

func (h handlerScriptResource) Read(ctx context.Context, rd *schema.ResourceData, meta interface{}) (d diag.Diagnostics) {
	_ = rd.Set(attrScriptDirtyOutput, "")
	_ = rd.Set(attrScriptFaultyOutput, "")
	old := cast.ToString(rd.Get(attrScriptOutput))
	defer func() { _ = rd.Set(attrScriptOutput, old) }() // never change output here, since it will be corrected by create or update.

	l, err := getLinux(meta.(*linuxPool), rd)
	if err != nil {
		return diag.FromErr(err)
	}
	err = h.read(ctx, rd, l)
	if errExit := (*remote.ExitError)(nil); errors.As(err, &errExit) {
		_ = rd.Set(attrScriptFaultyOutput, fmt.Sprintf("Faulty output produced:\n\n%s", err))
		return
	}
	if err != nil {
		return diag.FromErr(err)
	}

	new := cast.ToString(rd.Get(attrScriptOutput))
	if old != new {
		_ = rd.Set(attrScriptDirtyOutput, fmt.Sprintf("Dirty output detected:\n\n%s", new))
	}

	return
}

func (h handlerScriptResource) Create(ctx context.Context, rd *schema.ResourceData, meta interface{}) (d diag.Diagnostics) {
	l, err := getLinux(meta.(*linuxPool), rd)
	if err != nil {
		return diag.FromErr(err)
	}
	sc := h.newScript(rd, l, attrScriptLifecycleCommandCreate)
	if _, err := sc.exec(ctx); err != nil {
		return diag.FromErr(err)
	}

	id, err := uuid.NewRandom()
	if err != nil {
		return diag.FromErr(err)
	}
	rd.SetId(id.String())

	if err := h.read(ctx, rd, l); err != nil {
		return diag.FromErr(err)
	}
	return
}

// WARN: see https://github.com/hashicorp/terraform-plugin-sdk/issues/476
func (h handlerScriptResource) restoreOldResourceData(rd *schema.ResourceData, except map[string]bool) (err error) {
	for k := range h.attrs() {
		if except != nil && except[k] {
			continue
		}
		o, _ := rd.GetChange(k)
		err = rd.Set(k, o)
		if err != nil {
			return
		}
	}
	return
}

func (h handlerScriptResource) UpdateCommands(ctx context.Context, rd *schema.ResourceData, meta interface{}) (d diag.Diagnostics) {
	l, err := getLinux(meta.(*linuxPool), rd)
	if err != nil {
		return diag.FromErr(err)
	}

	if rd.HasChange(attrScriptLifecycleCommands + ".0." + attrScriptLifecycleCommandRead) {
		err := h.read(ctx, rd, l)
		if err != nil {
			_ = h.restoreOldResourceData(rd, nil)
			return diag.FromErr(err)
		}
	}

	return
}

func (h handlerScriptResource) Update(ctx context.Context, rd *schema.ResourceData, meta interface{}) (d diag.Diagnostics) {
	if len(h.changedAttrCommands(rd)) > 0 {
		return h.UpdateCommands(ctx, rd, meta)
	}

	l, err := getLinux(meta.(*linuxPool), rd)
	if err != nil {
		return diag.FromErr(err)
	}
	sc := h.newScript(rd, l, attrScriptLifecycleCommandUpdate)
	oldOutput := cast.ToString(rd.Get(attrScriptOutput))
	sc.stdin = strings.NewReader(oldOutput)
	if _, err := sc.exec(ctx); err != nil {
		_ = h.restoreOldResourceData(rd, nil)
		return diag.FromErr(err)
	}

	if err := h.read(ctx, rd, l); err != nil {
		return diag.FromErr(err)
	}
	return
}

func (h handlerScriptResource) Delete(ctx context.Context, rd *schema.ResourceData, meta interface{}) (d diag.Diagnostics) {
	if cast.ToString(rd.Get(attrScriptFaultyOutput)) != "" {
		return
	}
	l, err := getLinux(meta.(*linuxPool), rd)
	if err != nil {
		return diag.FromErr(err)
	}
	sc := h.newScript(rd, l, attrScriptLifecycleCommandDelete)
	if _, err := sc.exec(ctx); err != nil {
		return diag.FromErr(err)
	}
	return
}

func (h handlerScriptResource) CustomizeDiff(c context.Context, rd *schema.ResourceDiff, meta interface{}) (err error) {
	if rd.Id() == "" {
		return // no state
	}

	if cmd := h.changedAttrCommands(rd); len(cmd) > 0 {
		if fbd := h.changedAttrInputs(rd); len(fbd) > 0 {
			return fmt.Errorf("update to '%s' should not be combined with update to other arguments: %s",
				strings.Join(cmd, ","), strings.Join(fbd, ","))
		}

		if rd.HasChange(attrScriptLifecycleCommands + ".0." + attrScriptLifecycleCommandRead) {
			_ = h.setNewComputed(rd) // assume all computed will change
		}
		return // updated commands. let Update handle it.
	}

	if f, _ := rd.GetChange(attrScriptFaultyOutput); cast.ToString(f) != "" {
		_ = rd.ForceNew(attrScriptFaultyOutput) // faulty output, force recreation
		return
	}

	if _, ok := rd.GetOk(attrScriptLifecycleCommands + ".0." + attrScriptLifecycleCommandUpdate); ok {
		if len(h.changedAttrInputs(rd)) > 0 {
			_ = h.setNewComputed(rd) // assume all computed will change
		}
		return // updateable
	}

	for _, key := range h.changed(rd, h.attrs(h.attrInputs(), h.attrInternal())) {
		err = rd.ForceNew(key)
		if err != nil {
			return
		}
	}
	return
}

func scriptResource() *schema.Resource {
	var h handlerScriptResource
	return &schema.Resource{
		Description: "Manages an arbitrary resource through commands that are uploaded to the host and executed there " +
			"in each phase of the Terraform lifecycle.",
		Schema:        schemaScriptResource,
		CreateContext: h.Create,
		ReadContext:   h.Read,
		UpdateContext: h.Update,
		DeleteContext: h.Delete,
		CustomizeDiff: h.CustomizeDiff,
	}
}
