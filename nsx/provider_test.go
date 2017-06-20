package nsx

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform/helper/schema"
	"github.com/hashicorp/terraform/terraform"
)

var testAccProviders map[string]terraform.ResourceProvider
var testAccProvider *schema.Provider

func init() {
	testAccProvider = Provider().(*schema.Provider)
	testAccProviders = map[string]terraform.ResourceProvider{
		"nsxv": testAccProvider,
	}
}

func TestProvider(t *testing.T) {
	if err := Provider().(*schema.Provider).InternalValidate(); err != nil {
		t.Fatalf("err: %s", err)
	}

}

func TestProvider_impl(t *testing.T) {
	var _ terraform.ResourceProvider = Provider()
}

func testAccPreCheck(t *testing.T) {
	if v := os.Getenv("NSXV_USER"); v == "" {
		t.Fatal("NSXV_USER must be set for acceptance tests")
	}

	if v := os.Getenv("NSXV_PASSWORD"); v == "" {
		t.Fatal("NSXV_PASSWORD must be set for acceptance tests")
	}

	if v := os.Getenv("NSXV_MANAGER_URI"); v == "" {
		t.Fatal("NSXV_MANAGER_URI must be set for acceptance tests")
	}
}
