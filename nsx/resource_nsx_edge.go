package nsx

import (
	"fmt"
	"log"
	"strings"

	"github.com/IBM-tfproviders/govnsx"
	"github.com/IBM-tfproviders/govnsx/nsxresource"
	"github.com/IBM-tfproviders/govnsx/nsxtypes"
	"github.com/hashicorp/terraform/helper/schema"
)

const (
	EdgeTypeGatewayServices   = "gatewayServices"
	EdgeTypeDistributedRouter = "distributedRouter"

	EdgeApplianceSizeCompact   = "compact"
	EdgeApplianceSizeLarge     = "large"
	EdgeApplianceSizeQuadLarge = "quadlarge"
	EdgeApplianceSizeXtraLarge = "xlarge"
)

var edgeTypesList = []string{
	string(EdgeTypeGatewayServices),
	string(EdgeTypeDistributedRouter),
}

var edgeApplianceSizeList = []string{
	string(EdgeApplianceSizeCompact),
	string(EdgeApplianceSizeLarge),
	string(EdgeApplianceSizeQuadLarge),
	string(EdgeApplianceSizeXtraLarge),
}

type mgmtInterfaceCfg struct {
	portgroup string
	ip        string
	mask      string
}

type appliances struct {
	applianceSize string
	applianceList []applianceCfg
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
	tenantId    string
	folder      string
	appliances  appliances
}

func resourceNsxEdge() *schema.Resource {
	return &schema.Resource{
		Create: resourceNsxEdgeCreate,
		Read:   resourceNsxEdgeRead,
		Update: resourceNsxEdgeUpdate,
		Delete: resourceNsxEdgeDelete,

		Schema: map[string]*schema.Schema{
			"type": &schema.Schema{
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validateEdgeType,
			},
			"tenant_id": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				Default:  "Terraform Provider",
				ForceNew: true,
			},
			"folder": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			},
			"name": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
			},
			"description": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				Default:  "Created by Terraform",
			},
			"version": &schema.Schema{
				Type:     schema.TypeInt,
				Computed: true,
			},
			"edge_id": &schema.Schema{
				Type:     schema.TypeString,
				Computed: true,
			},
			"appliances": &schema.Schema{
				Type:     schema.TypeList,
				Required: true,
				MinItems: 1,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"size": &schema.Schema{
							Type:         schema.TypeString,
							Optional:     true,
							Default:      EdgeApplianceSizeCompact,
							ValidateFunc: validateEdgeApplianceSize,
						},
						"appliance": &schema.Schema{
							Type:     schema.TypeList,
							Required: true,
							MinItems: 1,
							MaxItems: 2,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"resource_pool_id": &schema.Schema{
										Type:     schema.TypeString,
										Required: true,
									},
									"datastore_id": &schema.Schema{
										Type:     schema.TypeString,
										Required: true,
									},
									"mgmt_interface": &schema.Schema{
										Type:     schema.TypeList,
										Optional: true,
										MinItems: 1,
										Elem: &schema.Resource{
											Schema: map[string]*schema.Schema{
												"portgroup": &schema.Schema{
													Type:     schema.TypeString,
													Required: true,
												},
												"ip": &schema.Schema{
													Type:         schema.TypeString,
													Required:     true,
													ValidateFunc: validateIP,
												},
												"mask": &schema.Schema{
													Type:         schema.TypeString,
													Required:     true,
													ValidateFunc: validateIP,
												},
											},
										},
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

	edgeCfg := parseResourceData(d)

	log.Printf("[INFO] Creating NSX Edge: %#v", edgeCfg)

	client := meta.(*govnsx.Client)

	edge := nsxresource.NewEdge(client)

	edgeInstallSpec := &nsxtypes.EdgeInstallSpec{
		Name:        edgeCfg.edgeName,
		Type:        edgeCfg.edgeType,
		Description: edgeCfg.description,
		Tenant:      edgeCfg.tenantId,
		Appliances:  createAppliancesSpec(edgeCfg.appliances),
	}

	resp, err := edge.Post(edgeInstallSpec)

	if err != nil {
		log.Printf("[ERROR] Edge Creation failed. %v", err)
		return err
	}

	log.Printf("[INFO] Created NSX Edge: %s", resp.EdgeId)

	d.SetId(resp.Location)
	d.Set("edge_id", resp.EdgeId)

	return resourceNsxEdgeRead(d, meta)
}

func resourceNsxEdgeRead(d *schema.ResourceData, meta interface{}) error {

	client := meta.(*govnsx.Client)
	edgeId := d.Get("edge_id").(string)

	edge := nsxresource.NewEdge(client)

	retEdge, err := edge.Get(edgeId)
	if err != nil {
		log.Printf("[ERROR] Retriving Edge '%s' failed with error : '%v'", edgeId, err)
		d.SetId("")
		d.Set("edge_id", "")
		return err
	}

	log.Printf("[INFO] The Edge: %v", retEdge)

	return nil
}

func resourceNsxEdgeUpdate(d *schema.ResourceData, meta interface{}) error {

	client := meta.(*govnsx.Client)
	edge := nsxresource.NewEdge(client)

	edgeId := d.Get("edge_id").(string)

	retEdge, err := edge.Get(edgeId)

	if err != nil {
		log.Printf("[ERROR] Retriving Edge '%s' failed with error : '%v'", edgeId, err)
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
		log.Printf("[DEBUG] Updating NsxEdge %s : name: '%s'", edgeId, v)
	}

	if d.HasChange("description") {
		_, v := d.GetChange("description")
		edgeInstallSpec.Description = v.(string)
		log.Printf("[DEBUG] Updating NsxEdge %s : description: '%s'", edgeId, v)
	}

	if d.HasChange("appliances") {
		appliances := parseAppliances(d)
		edgeInstallSpec.Appliances = createAppliancesSpec(appliances)
		log.Printf("[DEBUG] Updating NsxEdge %s : Appliances: '%s'", edgeId,
			appliances)
	}

	err = edge.Put(edgeInstallSpec, edgeId)

	if err != nil {
		log.Printf("[ERROR] Updating Edge '%s' failed with error : '%v'", edgeId, err)
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
		log.Printf("[ERROR] Deleting Edge '%s' failed with error : %v", edgeId, err)
		return err
	}

	return nil
}

func parseResourceData(d *schema.ResourceData) *nsxEdge {

	edge := &nsxEdge{
		edgeType:    d.Get("type").(string),
		edgeName:    d.Get("name").(string),
		tenantId:    d.Get("tenant_id").(string),
		description: d.Get("description").(string),
	}

	if v, ok := d.GetOk("folder"); ok {
		edge.folder = v.(string)
	}

	edge.appliances = parseAppliances(d)

	return edge
}

func parseAppliances(d *schema.ResourceData) appliances {

	newAppliances := appliances{}
	vL := d.Get("appliances")

	for _, value := range vL.([]interface{}) {

		appliances := value.(map[string]interface{})

		log.Printf("[INFO] KAVI appliances = %s", appliances)
		newAppliances.applianceSize = appliances["size"].(string)

		vL = appliances["appliance"]

		appCfgs := []applianceCfg{}
		for _, value := range vL.([]interface{}) {

			newAppliance := applianceCfg{}

			appliance := value.(map[string]interface{})
			log.Printf("[INFO] KAVI appliance = %s", appliance)

			newAppliance.resourcePoolId = appliance["resource_pool_id"].(string)
			newAppliance.datastoreId = appliance["datastore_id"].(string)

			if vL, ok := appliance["mgmt_interface"]; ok {

				for _, value := range vL.([]interface{}) {

					mgmt := value.(map[string]interface{})

					newAppliance.mgmtInterface.portgroup = mgmt["portgroup"].(string)
					newAppliance.mgmtInterface.ip = mgmt["ip"].(string)
					newAppliance.mgmtInterface.mask = mgmt["mask"].(string)

				}
			}
			appCfgs = append(appCfgs, newAppliance)
		}
		newAppliances.applianceList = appCfgs
	}
	return newAppliances
}

func validateEdgeType(v interface{}, k string) (ws []string, errors []error) {
	value := v.(string)
	found := false

	for _, t := range edgeTypesList {
		if t == value {
			found = true
		}
	}
	if !found {
		errors = append(errors, fmt.Errorf(
			"%s: Supported values are %s", k, strings.Join(edgeTypesList, ", ")))
	}

	return
}

func validateEdgeApplianceSize(v interface{}, k string) (ws []string, errors []error) {
	value := v.(string)
	found := false

	for _, t := range edgeApplianceSizeList {
		if t == value {
			found = true
		}
	}
	if !found {
		errors = append(errors, fmt.Errorf(
			"%s: Supported values are %s", k, strings.Join(edgeApplianceSizeList, ", ")))
	}

	return
}

func createAppliancesSpec(appInfo appliances) nsxtypes.Appliances {

	log.Printf("[INFO] KAVI appInfo = %s", appInfo)
	applianceList := []nsxtypes.Appliance{}
	for _, value := range appInfo.applianceList {

		appliance := nsxtypes.Appliance{ResourcePoolId: value.resourcePoolId,
			DatastoreId: value.datastoreId}

		applianceList = append(applianceList, appliance)
	}

	appliances := nsxtypes.Appliances{ApplianceSize: appInfo.applianceSize,
		DeployAppliances: false, AppliancesList: applianceList}

	return appliances
}
