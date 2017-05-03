package main

import (
	"github.com/IBM-tfproviders/vmware-nsx/nsx"
	"github.com/hashicorp/terraform/plugin"
)

func main() {
	plugin.Serve(&plugin.ServeOpts{
		ProviderFunc: nsx.Provider,
	})
}
