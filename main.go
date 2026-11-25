package main

import (
	"github.com/lentidas/terraform-provider-linux/linux"
	"github.com/hashicorp/terraform-plugin-sdk/v2/plugin"
)

func main() {
	plugin.Serve(&plugin.ServeOpts{
		ProviderFunc: linux.Provider,
	})
}
