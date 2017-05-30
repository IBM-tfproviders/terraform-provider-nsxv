package main

import (
	"log"

	"github.com/IBM-tfproviders/terraform-provider-nsxv/nsx"
	"github.com/hashicorp/terraform/plugin"
)

func main() {
	printBuildVersion()
	plugin.Serve(&plugin.ServeOpts{
		ProviderFunc: nsx.Provider,
	})
}

func printBuildVersion() {
	log.Printf("[INFO] nsxv provider build version = %s", BuildVersion)

}
