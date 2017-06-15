package nsx

import (
	"log"
	"github.com/IBM-tfproviders/govnsx"
	"github.com/IBM-tfproviders/govnsx/nsxresource"
	"github.com/IBM-tfproviders/govnsx/nsxtypes"
	"github.com/hashicorp/terraform/helper/schema"
)

func resourceNsxEdgeDLRInterface() *schema.Resource {
	return &schema.Resource{
		Create: resourceNsxEdgeDLRInterfaceCreate,
		Read:   resourceNsxEdgeDLRInterfaceRead,
		Update: resourceNsxEdgeDLRInterfaceUpdate,
		Delete: resourceNsxEdgeDLRInterfaceDelete,

		Schema: map[string]*schema.Schema{
			"edge_id": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"name": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"index": &schema.Schema{
				Type:     schema.TypeString,
				Computed: true,
			},
			"ip": &schema.Schema{
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validateIP,
			},
			"mask": &schema.Schema{
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validateIP,
			},
			"type": &schema.Schema{
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validateType,
			},
			"is_connected": &schema.Schema{
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
				ForceNew: false,
			},
			"portgroup": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
		},
	}
}

func resourceNsxEdgeDLRInterfaceCreate(d *schema.ResourceData, meta interface{}) error {

	log.Printf("[INFO] Creating NSX Edge Router Interface")

	client := meta.(*govnsx.Client)

	dlrInterfaces := nsxresource.NewEdgeDLRInterfaces(client)

	edgeId := d.Get("edge_id").(string)
	addrGroups := []nsxtypes.AddressGroup{nsxtypes.AddressGroup{
		PrimaryAddress: d.Get("ip").(string),
		SubnetMask:     d.Get("mask").(string)}}

	ifaces := []nsxtypes.EdgeDLRInterface{nsxtypes.EdgeDLRInterface{
		AddressGroups: addrGroups,
		Type:          d.Get("type").(string),
		ConnectedToId: d.Get("portgroup").(string)}}

	
	if v, ok := d.GetOk("name"); ok {
		ifaces[0].Name = v.(string)
	}

	if v, ok := d.GetOk("is_connected"); ok {
		ifaces[0].IsConnected = v.(bool)
	}

	addInterfacesSpec := &nsxtypes.EdgeDLRAddInterfacesSpec{
		EdgeDLRInterfaceList: ifaces,
	}

	addresp, err := dlrInterfaces.Post(addInterfacesSpec, edgeId)
	if err != nil {
		log.Printf("[ERROR] dlrInterfaces.Post () returned error : %v", err)
		return err
	}

	d.Set("index", addresp.EdgeDLRInterfaceList[0].Index)
	return resourceNsxEdgeDLRInterfaceRead(d, meta)
}

func resourceNsxEdgeDLRInterfaceRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*govnsx.Client)
        iface := nsxresource.NewEdgeDLRInterfaces(client)

	edgeId := d.Get("edge_id").(string)
	index  := d.Get("index").(string)
	log.Printf("[INFO] Read NSX Edge Router Interface: ", edgeId, index)

	retInterface, err := iface.Get(edgeId, index)	
	if err != nil {
                log.Printf("[ERROR] Retriving Edge '%s' failed with error : '%v'", edgeId, err)
                d.SetId("")
                d.Set("edge_id", "")
                d.Set("index", "")
                return err
        }

	log.Printf("[INFO] Interface: %v", retInterface)	

	d.SetId(retInterface.Label)
	return nil
}

func resourceNsxEdgeDLRInterfaceUpdate(d *schema.ResourceData, meta interface{}) error {
	log.Printf("[INFO] Update NSX Edge Router Interface TBD")
	return nil
}

func resourceNsxEdgeDLRInterfaceDelete(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*govnsx.Client)
        iface := nsxresource.NewEdgeDLRInterfaces(client)

        edgeId := d.Get("edge_id").(string)
	index  := d.Get("index").(string) 
	log.Printf("[INFO] Deleting NSX EdgeInterface:%s %s\n", edgeId, index)
	
	err := iface.Delete(edgeId, index)
	if err != nil {
                log.Printf("[Error] NSX Edge Interface Delete returned error : %v", err)
                return err
        }
	return nil
}
