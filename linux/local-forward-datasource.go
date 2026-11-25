package linux

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/phayes/freeport"
)

const (
	attrLocalForwardProviderOverride = "provider_override"
	attrLocalForwardRHost            = "remote_host"
	attrLocalForwardRPort            = "remote_port"
	attrLocalForwardLHost            = "local_host"
	attrLocalForwardLPort            = "local_port"
	attrLocalForwardHost             = "host"
	attrLocalForwardPort             = "port"
)

var schemaLocalForwardDataSource = map[string]*schema.Schema{
	attrLocalForwardProviderOverride: {
		Description: descProviderOverride,
		Type:        schema.TypeList,
		Optional:    true,
		MaxItems:    1,
		Elem: &schema.Resource{
			Schema: subSchemaProviderOverride,
		},
	},

	attrLocalForwardRHost: {
		Description: "Host that the SSH server forwards connections to, resolved from the SSH server.",
		Type:        schema.TypeString,
		Required:    true,
	},
	attrLocalForwardRPort: {
		Description: "Port that the SSH server forwards connections to.",
		Type:        schema.TypeInt,
		Required:    true,
	},

	attrLocalForwardLHost: {
		Description: "Local address to listen on. The default, `0.0.0.0`, accepts connections on every network interface.",
		Type:        schema.TypeString,
		Optional:    true,
		Default:     "0.0.0.0",
	},
	attrLocalForwardLPort: {
		Type:        schema.TypeInt,
		Optional:    true,
		Description: "Local port to listen on. The default, `0`, picks a random free port.",
		Default:     0,
	},

	attrLocalForwardHost: {
		Type:        schema.TypeString,
		Description: "Local address that is listening for connections. It equals `local_host`.",
		Computed:    true,
	},
	attrLocalForwardPort: {
		Type:        schema.TypeInt,
		Description: "Local port that is listening for connections. It equals `local_port`, or the port that was picked when `local_port` is `0`.",
		Computed:    true,
	},
}

type handlerLocalForwardDataSource struct {
}

func (h handlerLocalForwardDataSource) Read(ctx context.Context, rd *schema.ResourceData, meta interface{}) (d diag.Diagnostics) {
	l, err := getLinux(meta.(*linuxPool), rd)
	if err != nil {
		return diag.FromErr(err)
	}

	host := rd.Get(attrLocalForwardLHost)
	port := rd.Get(attrLocalForwardLPort)
	if port == 0 {
		port, err = freeport.GetFreePort()
		if err != nil {
			return diag.FromErr(err)
		}
	}

	err = l.lforwardTCP(ctx,
		fmt.Sprintf("%s:%d", host, port),
		fmt.Sprintf("%s:%d", rd.Get(attrLocalForwardRHost), rd.Get(attrLocalForwardRPort)),
	)
	if err != nil {
		return diag.FromErr(err)
	}

	rd.Set(attrLocalForwardHost, host)
	rd.Set(attrLocalForwardPort, port)
	rd.SetId("static")
	return
}

func localforwardDataSource() *schema.Resource {
	h := handlerLocalForwardDataSource{}
	return &schema.Resource{
		Description: "Establishes SSH local port forwarding through the connection of the provider.",
		Schema:      schemaLocalForwardDataSource,
		ReadContext: h.Read,
	}
}
