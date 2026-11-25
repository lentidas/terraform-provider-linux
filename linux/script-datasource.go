package linux

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

var schemaScriptDataSource = func() (m map[string]*schema.Schema) {
	m = make(map[string]*schema.Schema)
	for k, v := range schemaScriptResource {
		switch k {
		default:
			m[k] = v

		case attrScriptLifecycleCommands:
			read := *v.Elem.(*schema.Resource).Schema[attrScriptLifecycleCommandRead]
			read.Description = "Commands executed to obtain the data. Their standard output is recorded in `output`."

			m[k] = new(schema.Schema)
			*m[k] = *v
			m[k].Description = "Commands uploaded and executed on the remote host. For long scripts, use the `file` function."
			m[k].Elem = &schema.Resource{
				Schema: map[string]*schema.Schema{
					attrScriptLifecycleCommandRead: &read,
				},
			}

		case attrScriptInterpreter:
			m[k] = new(schema.Schema)
			*m[k] = *v
			m[k].Description = "Interpreter used to run the `read` commands, as a program followed by its arguments. The " +
				"path of the uploaded script is appended as the last argument. When empty, the script is executed " +
				"directly, using its `#!` line or `/bin/sh`. The script is uploaded readable by the connecting user " +
				"only, so an interpreter that switches user, such as `sudo -u`, cannot read it."

		case attrScriptTriggers:
		case attrScriptDirtyOutput:
		case attrScriptFaultyOutput:
		}
	}
	return
}()

type handlerScriptDataSource struct {
	hsr handlerScriptResource
}

func (h handlerScriptDataSource) Read(ctx context.Context, rd *schema.ResourceData, meta interface{}) (d diag.Diagnostics) {
	l, err := getLinux(meta.(*linuxPool), rd)
	if err != nil {
		return diag.FromErr(err)
	}

	err = h.hsr.read(ctx, rd, l)
	if err != nil {
		d = diag.FromErr(err)
	}
	rd.SetId("static")
	return
}

func scriptDataSource() *schema.Resource {
	h := handlerScriptDataSource{hsr: handlerScriptResource{}}
	return &schema.Resource{
		Description: "Reads arbitrary data by executing commands on the remote host.",
		Schema:      schemaScriptDataSource,
		ReadContext: h.Read,
	}
}
