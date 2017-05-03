package nsx

import (
	//"fmt"
	//"log"

	"github.com/hashicorp/terraform/helper/schema"
	//"github.com/IBM-tfproviders/govnsx"
)

func resourceNsxNetwork() *schema.Resource {
	return &schema.Resource{
		Create: resourceNsxNetworkCreate,
		Read:   resourceNsxNetworkRead,
		Delete: resourceNsxNetworkDelete,

		Schema: map[string]*schema.Schema{
			"name": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
		},
	}
}

func resourceNsxNetworkCreate(d *schema.ResourceData, meta interface{}) error {

	//client := meta.(*govnsx.Client)

	return resourceNsxNetworkRead(d, meta)
}

func resourceNsxNetworkRead(d *schema.ResourceData, meta interface{}) error {
	return nil
}

func resourceNsxNetworkDelete(d *schema.ResourceData, meta interface{}) error {
	d.SetId("")
	return nil
}
