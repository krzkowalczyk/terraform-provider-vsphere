package main

import (
	"github.com/hashicorp/terraform/plugin"
	"github.com/krzkowalczyk/terraform-provider-vsphere/vsphere"
)

func main() {
	plugin.Serve(&plugin.ServeOpts{
		ProviderFunc: vsphere.Provider})
}
