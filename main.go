package main

import (
	"github.com/IBM-tfproviders/terraform-provider-nsxv/nsx"
	"github.com/hashicorp/terraform/plugin"
)

func main() {
	printBuildVersion()
	plugin.Serve(&plugin.ServeOpts{
		ProviderFunc: nsx.Provider,
	})
}
