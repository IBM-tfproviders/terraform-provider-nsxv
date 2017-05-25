package nsx

import (
	"fmt"
	"log"
	"strconv"

	"github.com/IBM-tfproviders/govnsx"
	"github.com/IBM-tfproviders/govnsx/nsxresource"
	"github.com/IBM-tfproviders/govnsx/nsxtypes"
	"github.com/hashicorp/terraform/helper/schema"
)

type edgeSGW struct {
	edgeName       string
	description    string
	edgeId         string
	version        string
	datacenter     string
	tenantId       string
	resourcePoolId string
	datastoreId    string
	folder         string
	mgmtPortgroup  string
	//mgmtAddr       string
}

func resourceNsxEdgeSGW() *schema.Resource {
	return &schema.Resource{
		Create: resourceNsxEdgeSGWCreate,
		Read:   resourceNsxEdgeSGWRead,
		Update: resourceNsxEdgeSGWUpdate,
		Delete: resourceNsxEdgeSGWDelete,

		Schema: map[string]*schema.Schema{
			"version": &schema.Schema{
				Type:     schema.TypeInt,
				Computed: true,
			},
			"datacenter": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: false,
			},
			"tenant_id": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				Default:  "Terraform Provider",
				ForceNew: false,
			},
			"resource_pool_id": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: false,
			},
			"datastore_id": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: false,
			},
			"folder": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: false,
			},
			"edge_name": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: false,
				Default:  "",
			},
			"description": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				Default:  "Created by Terraform",
				ForceNew: false,
			},
			"mgmt_portgroup": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: false,
			},
			//"mgmt_addr": &schema.Schema{
			//	Type:     schema.TypeString,
			//	Required: true,
			//	ForceNew: false,
			//},
		},
	}
}

func resourceNsxEdgeSGWCreate(d *schema.ResourceData, meta interface{}) error {

	sgw := NewNsxEdgeSGW(d)

	log.Printf("[INFO] Creating NSX Edge: %#v", sgw)

	client := meta.(*govnsx.Client)

	edge := nsxresource.NewEdge(client) // TODO: what is common, newcommon. etc ?

	var appliances = []nsxtypes.Appliance{nsxtypes.Appliance{
		ResourcePoolId: sgw.resourcePoolId,
		DatastoreId:    sgw.datastoreId,
	}}

	vnics := []nsxtypes.Vnic{}
	//	addrGroups := []nsxtypes.AddressGroup{}

	//	addrGroup := nsxtypes.AddressGroup{}
	//	addrGroup.PrimaryAddress = sgw.mgmtAddr
	//	addrGroups = append(addrGroups, addrGroup)

	vnic := nsxtypes.Vnic{}
	vnic.Index = strconv.Itoa(0)
	vnic.PortgroupId = sgw.mgmtPortgroup
	//	vnic.AddressGroups = addrGroups
	vnic.IsConnected = true

	vnics = append(vnics, vnic)

	edgeInstallSpec := &nsxtypes.EdgeSGWInstallSpec{
		Name:           sgw.edgeName,
		Datacenter:     sgw.datacenter,
		Description:    sgw.description,
		Tenant:         sgw.tenantId,
		AppliancesList: appliances,
		Vnics:          vnics,
	}

	resp, err := edge.Post(edgeInstallSpec)

	if err != nil {
		log.Printf("[Error] edge.Post() returned error : %v", err)
		return err
	}

	log.Printf("[INFO] Created NSX Edge: %s", resp.EdgeId)

	d.SetId(resp.EdgeId)
	if err != nil {
		return fmt.Errorf("Invalid Edge id to set: %#v", resp.EdgeId)
	}
	return nil
}

func resourceNsxEdgeSGWRead(d *schema.ResourceData, meta interface{}) error {
	// get edgeId d.getId

	// issue Get edge

	log.Printf("[INFO] Reading Nsx Edge SGW")
	log.Printf("[WARN] Yet to be implemented")
	return nil
}

func resourceNsxEdgeSGWUpdate(d *schema.ResourceData, meta interface{}) error {

	// check if dhcp added Update edge vnic and configure dhcp for the new ip range

	// dhcp param update TODO

	log.Printf("[INFO] Updating Nsx Edge SGW")
	log.Printf("[WARN] Yet to be implemented")
	return nil
}

func resourceNsxEdgeSGWDelete(d *schema.ResourceData, meta interface{}) error {

	client := meta.(*govnsx.Client)
	edge := nsxresource.NewEdge(client)

	edgeId := d.Id()

	log.Printf("[INFO] Deleting NSX Edge: %s", edgeId)
	err := edge.Delete(edgeId)
	if err != nil {
		log.Printf("[Error] edge.Delete() returned error : %v", err)
		return err
	}
	return nil
}

func NewNsxEdgeSGW(d *schema.ResourceData) *edgeSGW {

	sgw := &edgeSGW{
		datacenter:     d.Get("datacenter").(string),
		resourcePoolId: d.Get("resource_pool_id").(string),
		datastoreId:    d.Get("datastore_id").(string),
		mgmtPortgroup:  d.Get("mgmt_portgroup").(string),
		//mgmtAddr:       d.Get("mgmt_addr").(string),
	}

	if v, ok := d.GetOk("tenant_id"); ok {
		sgw.tenantId = v.(string)
	}

	if v, ok := d.GetOk("folder"); ok {
		sgw.folder = v.(string)
	}

	if v, ok := d.GetOk("description"); ok {
		sgw.description = v.(string)
	}

	if v, ok := d.GetOk("edge_name"); ok {
		sgw.edgeName = v.(string)
	}

	return sgw
}
