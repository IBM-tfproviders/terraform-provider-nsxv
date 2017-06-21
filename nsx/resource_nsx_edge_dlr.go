package nsx

import (
	"fmt"
	"github.com/IBM-tfproviders/govnsx"
	"github.com/IBM-tfproviders/govnsx/nsxresource"
	"github.com/IBM-tfproviders/govnsx/nsxtypes"
	"github.com/hashicorp/terraform/helper/schema"
	"log"
	"strings"
)

const (
	DLRResourceIdPrefix = "dlr-"
	InterfaceTypeInternal = "internal"
	InterfaceTypeUplink = "uplink"
)

var interfaceTypesList = []string{
        string(InterfaceTypeInternal),
        string(InterfaceTypeUplink),
}

type ifCfg struct {
	name              string
	ip                string
	mask              string
	logical_switch_id string
}

type dlrCfg struct {
	edgeId    string
	ifCfgList []ifCfg
}

func resourceNsxEdgeDLR() *schema.Resource {
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
			"type" :&schema.Schema{
                                Type:     schema.TypeString,
				Computed: true,
                        },
			"interface": &schema.Schema{
				Type:     schema.TypeSet,
				Required: true,
				MinItems: 1,
				MaxItems: 999,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"name": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
						},
						"index": &schema.Schema{
							Type:     schema.TypeString,
							Computed: true,
						},
						"ip": &schema.Schema{
							Type:         schema.TypeString,
							Required:     true,
							ForceNew:     false,
							ValidateFunc: validateIP,
						},
						"mask": &schema.Schema{
							Type:         schema.TypeString,
							Required:     true,
							ForceNew:     false,
							ValidateFunc: validateIP,
						},
						"logical_switch_id": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
							ForceNew: false,
						},
					},
				},
			},
		},
	}
}

func resourceNsxEdgeDLRInterfaceCreate(d *schema.ResourceData, meta interface{}) error {

	var edgeType string
	var err error
	
	if v, ok := d.GetOk("type"); ok {

		edgeType = v.(string)
	
	} else {
		edgeType, err = getEdgeType(d.Get("edge_id").(string), meta)

		if err != nil {
			log.Printf("[ERROR] Unable to read Edge type %s", err)
			return err
		}
	}

	if edgeType != EdgeTypeDistributedRouter {
		log.Printf("[ERROR] Edge type is not ", EdgeTypeDistributedRouter)
		err := fmt.Errorf("[ERROR] Only Edge type %s is supported for this operation",
			EdgeTypeDistributedRouter)
			return err
	}
	
	dlr, err := parseAndValidateDLRResourceData(d, meta)
	if err != nil {
		log.Printf("[ERROR] Configuration validation failed.")
		return err
	}

	log.Printf("[INFO] Adding DLR Interface '%#v' to Edge '%s'", dlr, dlr.edgeId)

	client := meta.(*govnsx.Client)
	dlrInterfaces := nsxresource.NewEdgeDLRInterfaces(client)

	edgeId := dlr.edgeId
	ifaces := []nsxtypes.EdgeDLRInterface{}

	for _, ifcfg := range dlr.ifCfgList {

		addrGroups := []nsxtypes.AddressGroup{nsxtypes.AddressGroup{
			PrimaryAddress: ifcfg.ip,
			SubnetMask:     ifcfg.mask}}

		iface := nsxtypes.EdgeDLRInterface{
			AddressGroups: addrGroups,
			Name:          ifcfg.name,
			ConnectedToId: ifcfg.logical_switch_id,
			Type:          "internal",
			IsConnected:   true}

		ifaces = append(ifaces, iface)
	}

	addInterfacesSpec := &nsxtypes.EdgeDLRAddInterfacesSpec{
		EdgeDLRInterfaceList: ifaces,
	}

	_, err = dlrInterfaces.Post(addInterfacesSpec, edgeId)
	if err != nil {
		log.Printf("[ERROR] dlrInterfaces.Post () returned error : %v", err)
		return err
	}

	d.SetId(fmt.Sprintf(DLRResourceIdPrefix + dlr.edgeId))

	return resourceNsxEdgeDLRInterfaceRead(d, meta)
}

func resourceNsxEdgeDLRInterfaceRead(d *schema.ResourceData, meta interface{}) error {

	client := meta.(*govnsx.Client)
	dlrInterfaces := nsxresource.NewEdgeDLRInterfaces(client)

	edgeId := d.Get("edge_id").(string)

	if v, ok := d.GetOk("type"); ok {

		edgeType := v.(string)

		if edgeType != EdgeTypeDistributedRouter {
			log.Printf("[ERROR] Edge type is not ", EdgeTypeDistributedRouter)
			err := fmt.Errorf(
				"[ERROR] Only Edge type %s is supported for this operation",
				EdgeTypeDistributedRouter)
			return err
		}
	} else {

		edgeType, err := getEdgeType(d.Get("edge_id").(string), meta)
        
		if err != nil {
			log.Printf("[ERROR] Unable to read Edge type %s", err)
			return err
		}
        
		if edgeType != EdgeTypeDistributedRouter {
			log.Printf("[ERROR] Edge type is not ", EdgeTypeDistributedRouter) 
			err := fmt.Errorf("[ERROR] Only Edge type %s is supported for this operation",
			EdgeTypeDistributedRouter)
			return err
		}

		d.Set("type", EdgeTypeDistributedRouter)
	}

	log.Printf("[INFO] Read NSX Edge Router Interface: ", edgeId)
	resp, err := dlrInterfaces.Get(edgeId)

	if err != nil {
		log.Printf("[ERROR] Retriving Edge Interfaces %s failed with error : '%v'", edgeId, err)
		d.SetId("")
		return err
	}

	log.Printf("[DEBUG] Retrieved Edge Interfaces %v", resp) 

	ifaces := make([]map[string]interface{}, 0)

	for _, curIface := range resp.EdgeDLRInterfaceList {

		if prvIfaces, ok := d.Get("interface").(*schema.Set); ok {

		 	for _, val := range prvIfaces.List() {
				prvIface := val.(map[string]interface{})
				
				log.Printf("[DEBUG] curIface.ConnectedToName %s,prvIface[logical_switch_id] %s\n",
					curIface.ConnectedToName,
					prvIface["logical_switch_id"].(string))

				log.Printf("[DEBUG] curIface.AddressGroups[0].PrimaryAddress %s, prvIface[ip] %s\n",
					curIface.AddressGroups[0].PrimaryAddress,
					prvIface["ip"].(string) )

				if curIface.Name == prvIface["name"] &&
					curIface.ConnectedToId == prvIface["logical_switch_id"].(string) &&
					curIface.AddressGroups[0].PrimaryAddress == prvIface["ip"].(string) {

						prvIface["index"] = curIface.Index
						ifaces = append(ifaces, prvIface)
						log.Printf("[DEBUG] Updated index %s", 
							curIface.Index)
						break
				}
			}
		}
	}

	err = d.Set("interface", ifaces)
	if err != nil {
		return fmt.Errorf("Invalid interfaces to set: %#v", ifaces)
	}
	return nil
}

func resourceNsxEdgeDLRInterfaceUpdate(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*govnsx.Client)
        edlrIface := nsxresource.NewEdgeDLRInterfaces(client)

        edgeId := d.Get("edge_id").(string)

	if d.HasChange("interface") {

		oldIface, newIface := d.GetChange("interface")

		oldIfaceSet := oldIface.(*schema.Set)
		newIfaceSet := newIface.(*schema.Set)

		addedIfaceSet := newIfaceSet.Difference(oldIfaceSet)
		removedIfaceSet := oldIfaceSet.Difference(newIfaceSet)
		
		log.Printf("[DEBUG] added Interface : %#v\n", addedIfaceSet)
               	log.Printf("[DEBUG] removed Interface : %#v\n", removedIfaceSet)	
		
		for _, removedIfaceRaw := range removedIfaceSet.List() {
			removedIface  := removedIfaceRaw.(map[string]interface{})	
			removedIfaceIndex := removedIface["index"].(string)
			err := edlrIface.Delete(edgeId, removedIfaceIndex)
			if err != nil {
				log.Printf("[Error]  NSX Edge Interface Delete returned error : %v", err)
				return err
			}
		}
	
		ifaces := []nsxtypes.EdgeDLRInterface{}
		for _, addedIfaceRaw := range addedIfaceSet.List() {
			addedIface := addedIfaceRaw.(map[string]interface{})

			addrGroups := []nsxtypes.AddressGroup{nsxtypes.AddressGroup{
                        	PrimaryAddress: addedIface["ip"].(string),
                        	SubnetMask:     addedIface["mask"].(string),
				},
			}	
			
			iface := nsxtypes.EdgeDLRInterface{
				AddressGroups: addrGroups,
				Name:          addedIface["name"].(string),
				ConnectedToId: addedIface["logical_switch_id"].(string),
				Type:          "internal",
				IsConnected:   true,
			}

			ifaces = append(ifaces, iface)	
		}
			
		if len(ifaces) > 0 {
			addInterfacesSpec := &nsxtypes.EdgeDLRAddInterfacesSpec{
				EdgeDLRInterfaceList: ifaces,
			}

			_, err := edlrIface.Post(addInterfacesSpec, edgeId)
			if err != nil {
				log.Printf("[ERROR] dlrInterfaces.Post () returned error : %v", err)
			return err
			}
		}
        }
	return resourceNsxEdgeDLRInterfaceRead(d, meta)
}

func resourceNsxEdgeDLRInterfaceDelete(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*govnsx.Client)
	iface := nsxresource.NewEdgeDLRInterfaces(client)

	edgeId := d.Get("edge_id").(string)
	log.Printf("[INFO] Deleting NSX EdgeInterface:%s %s\n", edgeId)

	err := iface.Delete(edgeId)
	if err != nil {
		log.Printf("[Error] NSX Edge Interface Delete returned error : %v", err)
		return err
	}
	return nil
}

func parseAndValidateDLRResourceData(d *schema.ResourceData, meta interface{}) (*dlrCfg, error) {

	dlr := &dlrCfg{
		edgeId: d.Get("edge_id").(string),
	}

	ifCfgs := []ifCfg{}
	vL := d.Get("interface")

	if ifSet, ok := vL.(*schema.Set); ok {
		for _, value := range ifSet.List() {

			newInterface := ifCfg{}
			iface := value.(map[string]interface{})

			newInterface.name = iface["name"].(string)
			newInterface.ip = iface["ip"].(string)
			newInterface.mask = iface["mask"].(string)
			newInterface.logical_switch_id = iface["logical_switch_id"].(string)
			ifCfgs = append(ifCfgs, newInterface)
		}
	}
	dlr.ifCfgList = ifCfgs
	return dlr, nil
}

func validateInterfaceType(v interface{}, k string) (ws []string, errors []error) {
	value := v.(string)
        found := false
	
	for _, t := range interfaceTypesList {
                if t == value {
                        found = true
                }
        }

	if !found {
                errors = append(errors, fmt.Errorf(
                        "%s: Supported values are %s", k, strings.Join(interfaceTypesList, ", ")))
        }

        return
}
