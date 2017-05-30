package nsx

import (
	//"fmt"

	"github.com/hashicorp/terraform/helper/schema"
	"github.com/hashicorp/terraform/terraform"
)

// Provider returns a terraform.ResourceProvider.
func Provider() terraform.ResourceProvider {
	return &schema.Provider{
		Schema: map[string]*schema.Schema{
			"user": &schema.Schema{
				Type:        schema.TypeString,
				Required:    true,
				DefaultFunc: schema.EnvDefaultFunc("NSX_USER", nil),
				Description: "The user name for NSX API operations.",
			},

			"password": &schema.Schema{
				Type:        schema.TypeString,
				Required:    true,
				Sensitive:   true,
				DefaultFunc: schema.EnvDefaultFunc("NSX_PASSWORD", nil),
				Description: "The user password for NSX API operations.",
			},

			"nsx_manager_uri": &schema.Schema{
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("NSX_MANAGER_URI", nil),
				Description: "The NSX Manager URI for API operations.",
			},

			"allow_unverified_ssl": &schema.Schema{
				Type:        schema.TypeBool,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("NSX_ALLOW_UNVERIFIED_SSL", false),
				Description: "If set, NSX client will permit unverifiable SSL certificates.",
			},

			"client_debug": &schema.Schema{
				Type:        schema.TypeBool,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("NSX_CLIENT_DEBUG", false),
				Description: "govnsx debug",
			},

			"client_debug_path_run": &schema.Schema{
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("NSX_CLIENT_DEBUG_PATH_RUN", ""),
				Description: "govnsx debug path for a single run",
			},

			"client_debug_path": &schema.Schema{
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("NSX_CLIENT_DEBUG_PATH", ""),
				Description: "govnsx debug path for debug",
			},

			"user_agent_name": &schema.Schema{
				Type:        schema.TypeString,
				Optional:    true,
				Default:     "Terraform-Nsx-Provider",
				Description: "NSX Clinet user agent name",
			},
		},

		ResourcesMap: map[string]*schema.Resource{
			"nsxv_logical_switch": resourceLogicalSwitch(),
		},

		ConfigureFunc: providerConfigure,
	}
}

func providerConfigure(d *schema.ResourceData) (interface{}, error) {

	config := Config{
		User:          d.Get("user").(string),
		Password:      d.Get("password").(string),
		NsxManagerUri: d.Get("nsx_manager_uri").(string),
		UserAgentName: d.Get("user_agent_name").(string),
		InsecureFlag:  d.Get("allow_unverified_ssl").(bool),
		Debug:         d.Get("client_debug").(bool),
		DebugPathRun:  d.Get("client_debug_path_run").(string),
		DebugPath:     d.Get("client_debug_path").(string),
	}

	return config.Client()
}
