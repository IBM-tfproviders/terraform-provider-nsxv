package nsx

import (
	"fmt"
	"log"

	"github.com/IBM-tfproviders/govnsx"
	"github.com/IBM-tfproviders/govnsx/nsxresource"
	"github.com/IBM-tfproviders/govnsx/nsxtypes"
	"github.com/hashicorp/terraform/helper/schema"
)

const (
	ApplianceSizeCompact   = "compact"
	ApplianceSizeLarge     = "large"
	ApplianceSizeQuadLarge = "quadlarge"
	ApplianceSizeXLarge    = "xlarge"
)

type mgmtInterfaceCfg struct {
	portgroup string
	ip        string
	mask      string
}

type applianceCfg struct {
	resourcePoolId string
	datastoreId    string
	mgmtInterface  mgmtInterfaceCfg
}

type nsxEdge struct {
	edgeName    string
	edgeType    string
	description string
	datacenter  string
	tenantId    string
	folder      string
	appliances  []applianceCfg
}

func resourceNsxEdge() *schema.Resource {
	return &schema.Resource{
		Create: resourceNsxEdgeCreate,
		Read:   resourceNsxEdgeRead,
		Update: resourceNsxEdgeUpdate,
		Delete: resourceNsxEdgeDelete,

		Schema: map[string]*schema.Schema{
			"type": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"datacenter": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"tenant_id": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				Default:  "Terraform Provider",
				ForceNew: false,
			},
			"folder": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: false,
			},
			"name": &schema.Schema{
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
			"version": &schema.Schema{
				Type:     schema.TypeInt,
				Computed: true,
			},
			"edge_id": &schema.Schema{
				Type:     schema.TypeString,
				Computed: true,
			},
			"appliance": &schema.Schema{
				Type:     schema.TypeSet,
				Required: true,
				ForceNew: false,
				MinItems: 1,
				MaxItems: 2,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"resource_pool_id": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
							ForceNew: true,
						},
						"datastore_id": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
							ForceNew: true,
						},
						"mgmt_interface": &schema.Schema{
							Type:     schema.TypeList,
							Optional: true,
							ForceNew: false,
							MinItems: 1,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"portgroup": &schema.Schema{
										Type:     schema.TypeString,
										Required: true,
										ForceNew: false,
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
								},
							},
						},
					},
				},
			},
		},
	}
}

func resourceNsxEdgeCreate(d *schema.ResourceData, meta interface{}) error {

	edgeCfg := NewNsxEdge(d)

	log.Printf("[INFO] Creating NSX Edge: %#v", edgeCfg)

	client := meta.(*govnsx.Client)

	edge := nsxresource.NewEdge(client)

	applianceList := []nsxtypes.Appliance{}
	for _, value := range edgeCfg.appliances {

		appliance := nsxtypes.Appliance{ResourcePoolId: value.resourcePoolId,
			DatastoreId: value.datastoreId}

		applianceList = append(applianceList, appliance)
	}

	appliances := nsxtypes.Appliances{ApplianceSize: ApplianceSizeCompact,
		DeployAppliances: false, AppliancesList: applianceList}

	edgeInstallSpec := &nsxtypes.EdgeInstallSpec{
		Name:        edgeCfg.edgeName,
		Type:        edgeCfg.edgeType,
		Datacenter:  edgeCfg.datacenter,
		Description: edgeCfg.description,
		Tenant:      edgeCfg.tenantId,
		Appliances:  appliances,
	}

	resp, err := edge.Post(edgeInstallSpec)

	if err != nil {
		log.Printf("[Error] edge.Post() returned error : %v", err)
		return err
	}

	log.Printf("[INFO] Created NSX Edge: %s", resp.EdgeId)

	d.SetId(resp.Location)

	err = d.Set("edge_id", resp.EdgeId)
	if err != nil {
		return fmt.Errorf("Invalid Edge id to set: %#v", resp.EdgeId)
	}
	return resourceNsxEdgeRead(d, meta)
}

func resourceNsxEdgeRead(d *schema.ResourceData, meta interface{}) error {

	client := meta.(*govnsx.Client)
	edge_id := d.Get("edge_id").(string)

	edge := nsxresource.NewEdge(client)

	retEdge, err := edge.Get(edge_id)
	if err != nil {
		log.Printf("[Error] edge.Get() returned error : %v", err)
		d.SetId("")
		d.Set("edge_id", "")
		return err
	}

	log.Printf("[Info] edge.Get() returned Edge: %v", retEdge)

	return nil
}

func resourceNsxEdgeUpdate(d *schema.ResourceData, meta interface{}) error {

	client := meta.(*govnsx.Client)
	edge := nsxresource.NewEdge(client)

	edgeId := d.Get("edge_id").(string)

	retEdge, err := edge.Get(edgeId)

	if err != nil {
		return err
	}

	edgeInstallSpec := &nsxtypes.EdgeInstallSpec{
		Name:        retEdge.Name,
		Description: retEdge.Description,
		Tenant:      retEdge.Tenant,
		Appliances:  retEdge.Appliances,
	}

	if d.HasChange("name") {
		_, v := d.GetChange("name")
		edgeInstallSpec.Name = v.(string)
		log.Printf("[INFO] Updating NsxEdge :name: %s", v)
	}

	if d.HasChange("description") {
		_, v := d.GetChange("description")
		edgeInstallSpec.Description = v.(string)
		log.Printf("[INFO] Updating NsxEdge :description: %s", v)
	}

	err = edge.Put(edgeInstallSpec, edgeId)

	if err != nil {
		return err
	}

	return nil
}

func resourceNsxEdgeDelete(d *schema.ResourceData, meta interface{}) error {

	client := meta.(*govnsx.Client)
	edge := nsxresource.NewEdge(client)

	edgeId := d.Get("edge_id").(string)

	log.Printf("[INFO] Deleting NSX Edge: %s", edgeId)
	err := edge.Delete(edgeId)
	if err != nil {
		log.Printf("[Error] edge.Delete() returned error : %v", err)
		return err
	}
	return nil
}

func NewNsxEdge(d *schema.ResourceData) *nsxEdge {

	edge := &nsxEdge{
		datacenter: d.Get("datacenter").(string),
		edgeType:   d.Get("type").(string),
	}

	if v, ok := d.GetOk("tenant_id"); ok {
		edge.tenantId = v.(string)
	}

	if v, ok := d.GetOk("folder"); ok {
		edge.folder = v.(string)
	}

	if v, ok := d.GetOk("description"); ok {
		edge.description = v.(string)
	}

	if v, ok := d.GetOk("edge_name"); ok {
		edge.edgeName = v.(string)
	}

	vL := d.Get("appliance")
	if appSet, ok := vL.(*schema.Set); ok {

		appCfgs := []applianceCfg{}
		for _, value := range appSet.List() {

			newAppliance := applianceCfg{}

			appliance := value.(map[string]interface{})

			newAppliance.resourcePoolId = appliance["resource_pool_id"].(string)
			newAppliance.datastoreId = appliance["datastore_id"].(string)

			if vL, ok = appliance["mgmt_interface"]; ok {

				mgmt := (vL.([]interface{}))[0].(map[string]interface{})

				newAppliance.mgmtInterface.portgroup = mgmt["portgroup"].(string)
				newAppliance.mgmtInterface.ip = mgmt["ip"].(string)
				newAppliance.mgmtInterface.mask = mgmt["mask"].(string)
			}

			appCfgs = append(appCfgs, newAppliance)
		}
		edge.appliances = appCfgs
	}

	return edge
}