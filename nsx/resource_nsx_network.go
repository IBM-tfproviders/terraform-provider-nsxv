package nsx

import (
	"fmt"
	"log"

	"github.com/IBM-tfproviders/govnsx"
	"github.com/IBM-tfproviders/govnsx/nsxresource"
	"github.com/IBM-tfproviders/govnsx/nsxtypes"
	"github.com/hashicorp/terraform/helper/schema"
)

func resourceLogicalSwitch() *schema.Resource {
	return &schema.Resource{
		Create: resourceLogicalSwitchCreate,
		Read:   resourceLogicalSwitchRead,
		Update: resourceLogicalSwitchUpdate,
		Delete: resourceLogicalSwitchDelete,

		Schema: map[string]*schema.Schema{
			"name": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: false,
			},

			"scope_id": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"description": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: false,
			},

			"tenant_id": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: false,
			},

			"control_plane_mode": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: false,
			},

			"guest_vlan_allowed": &schema.Schema{
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
				ForceNew: true,
			},

			"network_label": &schema.Schema{
				Type:     schema.TypeString,
				Computed: true,
			},

			"virtual_wire_id": &schema.Schema{
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func resourceLogicalSwitchCreate(d *schema.ResourceData, meta interface{}) error {

	nsxclient := meta.(*govnsx.Client)

	netobj := nsxresource.NewNetwork(nsxclient)
	vWspec := NewVWCreateSpec(d)

	scopeId := d.Get("scope_id").(string)

	vWpostresp, err := netobj.Post(vWspec, scopeId)

	if err != nil {
		return err
	}
	d.Set("virtual_wire_id", vWpostresp.VirtualWireOID)
	d.SetId(vWpostresp.Location)
	log.Printf("[INFO] Logical Switch %s created at:%s", vWspec.Name,
		vWpostresp.Location)

	return resourceLogicalSwitchRead(d, meta)
}

func resourceLogicalSwitchRead(d *schema.ResourceData, meta interface{}) error {
	nsxclient := meta.(*govnsx.Client)

	netobj := nsxresource.NewNetwork(nsxclient)
	location := d.Id()
	vwire, err := netobj.Get(location)
	if err != nil {
		return err
	}

	//Need to contruct the network_label only for the first time
	//This is required since vSphere port group doesn't change when we update
	//the Virtualwire name. This is good otherwiase terraform redeploy the VMs.
	if _, ok := d.GetOk("network_label"); ok == false {
		net_label := fmt.Sprintf(nsxtypes.NetworkLableFormat, vwire.SwitchOId,
			vwire.ObjectId, vwire.VdnId, vwire.Name)
		d.Set("network_label", net_label)
		log.Printf("[INFO] Logical Switch portgroup name set to:%s", net_label)
	}

	return nil
}

func resourceLogicalSwitchUpdate(d *schema.ResourceData, meta interface{}) error {
	nsxclient := meta.(*govnsx.Client)

	updateVW := nsxtypes.NewUpdateVirtualWire()
	if d.HasChange("name") {
		_, v := d.GetChange("name")
		updateVW.Name = v.(string)
		log.Printf("[INFO] Updating Logical switch :name: %s", v)
	}
	if d.HasChange("description") {
		_, v := d.GetChange("description")
		updateVW.Description = v.(string)
		log.Printf("[INFO] Updating Logical switch :description: %s", v)
	}
	if d.HasChange("tenant_id") {
		_, v := d.GetChange("tenant_id")
		updateVW.TenantId = v.(string)
		log.Printf("[INFO] Updating Logical switch :tenant_id: %s", v)
	}
	if d.HasChange("control_plane_mode") {
		_, v := d.GetChange("control_plane_mode")
		updateVW.ControlPlaneMode = v.(string)
		log.Printf("[INFO] Updating Logical switch :control_plane_mode: %s", v)
	} else {
		updateVW.ControlPlaneMode = d.Get("control_plane_mode").(string)
	}

	location := d.Id()
	netobj := nsxresource.NewNetwork(nsxclient)
	err := netobj.Put(updateVW, location)
	if err != nil {
		return err
	}

	return nil
}

func resourceLogicalSwitchDelete(d *schema.ResourceData, meta interface{}) error {
	nsxclient := meta.(*govnsx.Client)

	netobj := nsxresource.NewNetwork(nsxclient)
	err := netobj.Delete(d.Id())
	if err != nil {
		return err
	}
	log.Printf("[INFO] Logical Switch deleted :%s", d.Id())
	d.SetId("")
	return nil
}

func NewVWCreateSpec(d *schema.ResourceData) *nsxtypes.VWCreateSpec {

	vWspec := nsxtypes.NewVWCreateSpec()

	if v, ok := d.GetOk("name"); ok {
		vWspec.Name = v.(string)
	}

	if v, ok := d.GetOk("description"); ok {
		vWspec.Description = v.(string)
	}

	if v, ok := d.GetOk("tenant_id"); ok {
		vWspec.TenantId = v.(string)
	}

	if v, ok := d.GetOk("control_plane_mode"); ok {
		vWspec.ControlPlaneMode = v.(string)
	}

	if v, ok := d.GetOk("guest_vlan_allowed"); ok {
		vWspec.GuestVlanAllowed = v.(bool)
	}
	return vWspec
}
