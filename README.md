# Terraform Provider for NSXv

This is the repository for the Terraform NSV Provider, which one can use with
Terraform to work with [VMware NSX-V](https://www.vmware.com/products/nsx.html).

For general information about Terraform, visit the [official
website](https://terraform.io/) and the [GitHub project page](tf-github).

Documentation on the NSX platform can be found at the [NSX-V Documentation page](https://docs.vmware.com/en/VMware-NSX-Data-Center-for-vSphere/index.html)

# Using the Provider

This provider is tested only on Terraform 0.11.x and 0.12.x. 

# Building the provider

In order to reduce the security risk build this provider using the latest version of golang. 
You may also need to upgrade the version of required modules in go.mod and regenerate the go.sum for any security issues.
This provider depends on [govnsx](https://github.com/IBM-tfproviders/govnsx). You would have to first update the 
[govnsx](https://github.com/IBM-tfproviders/govnsx) to have the latest modules before rebuilding the provider.

## Steps to rebuild this provider

export GOPATH=<YOUR_GO_PATH>
export GO_BIN_LOCATION=<YOUR_GO_BIN_PATH>
export NSXV_VERSION=1.0.2
mkdir -p $GOPATH/src/github.com/IBM-tfproviders
git clone -b v$NSXV_VERSION https://github.com/IBM-tfproviders/terraform-provider-nsxv
cd $GOPATH/src/github.com/IBM-tfproviders/terraform-provider-nsxv
$GO_BIN_LOCATION mod tidy
$GO_BIN_LOCATION install