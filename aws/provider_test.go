package aws

import (
	"fmt"
	"log"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go/aws/endpoints"

	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/helper/schema"
	"github.com/hashicorp/terraform/terraform"
	"github.com/terraform-providers/terraform-provider-template/template"
	"github.com/terraform-providers/terraform-provider-tls/tls"
)

var testAccProviders map[string]terraform.ResourceProvider
var testAccProvidersWithTLS map[string]terraform.ResourceProvider
var testAccProviderFactories func(providers *[]*schema.Provider) map[string]terraform.ResourceProviderFactory
var testAccProvider *schema.Provider
var testAccTemplateProvider *schema.Provider

func init() {
	testAccProvider = Provider().(*schema.Provider)
	testAccTemplateProvider = template.Provider().(*schema.Provider)
	testAccProviders = map[string]terraform.ResourceProvider{
		"aws":      testAccProvider,
		"template": testAccTemplateProvider,
	}
	testAccProviderFactories = func(providers *[]*schema.Provider) map[string]terraform.ResourceProviderFactory {
		return map[string]terraform.ResourceProviderFactory{
			"aws": func() (terraform.ResourceProvider, error) {
				p := Provider()
				*providers = append(*providers, p.(*schema.Provider))
				return p, nil
			},
		}
	}
	testAccProvidersWithTLS = map[string]terraform.ResourceProvider{
		"tls": tls.Provider(),
	}

	for k, v := range testAccProviders {
		testAccProvidersWithTLS[k] = v
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
	if v := os.Getenv("AWS_PROFILE"); v == "" {
		if v := os.Getenv("AWS_ACCESS_KEY_ID"); v == "" {
			t.Fatal("AWS_ACCESS_KEY_ID must be set for acceptance tests")
		}
		if v := os.Getenv("AWS_SECRET_ACCESS_KEY"); v == "" {
			t.Fatal("AWS_SECRET_ACCESS_KEY must be set for acceptance tests")
		}
	}

	region := testAccGetRegion()
	log.Printf("[INFO] Test: Using %s as test region", region)
	os.Setenv("AWS_DEFAULT_REGION", region)

	err := testAccProvider.Configure(terraform.NewResourceConfig(nil))
	if err != nil {
		t.Fatal(err)
	}
}

func testAccAwsAlternateAccountPreCheck(t *testing.T) {
	if v := os.Getenv("AWS_ACCESS_KEY_ID_ALTERNATE"); v == "" {
		t.Skip("AWS_ACCESS_KEY_ID_ALTERNATE must be set for this acceptance test")
	}
	if v := os.Getenv("AWS_SECRET_ACCESS_KEY_ALTERNATE"); v == "" {
		t.Skip("AWS_SECRET_ACCESS_KEY_ALTERNATE must be set for this acceptance test")
	}
}

var testAccAwsAlternateAccountProviderConfig = fmt.Sprintf(`
provider "aws" {
  access_key = "%[1]s"
  alias      = "alternate"
  secret_key = "%[2]s"
}
`, os.Getenv("AWS_ACCESS_KEY_ID_ALTERNATE"), os.Getenv("AWS_SECRET_ACCESS_KEY_ALTERNATE"))

func testAccGetRegion() string {
	v := os.Getenv("AWS_DEFAULT_REGION")
	if v == "" {
		return "us-west-2"
	}
	return v
}

func testAccGetPartition() string {
	if partition, ok := endpoints.PartitionForRegion(endpoints.DefaultPartitions(), testAccGetRegion()); ok {
		return partition.ID()
	}
	return "aws"
}

func testAccEC2ClassicPreCheck(t *testing.T) {
	client := testAccProvider.Meta().(*AWSClient)
	platforms := client.supportedplatforms
	region := client.region
	if !hasEc2Classic(platforms) {
		t.Skipf("This test can only run in EC2 Classic, platforms available in %s: %q",
			region, platforms)
	}
}

func testAccHasServicePreCheck(service string, t *testing.T) {
	if partition, ok := endpoints.PartitionForRegion(endpoints.DefaultPartitions(), testAccGetRegion()); ok {
		if _, ok := partition.Services()[service]; !ok {
			t.Skip(fmt.Sprintf("skipping tests; partition does not support %s service", service))
		}
	}
}

func testAccMultipleRegionsPreCheck(t *testing.T) {
	if partition, ok := endpoints.PartitionForRegion(endpoints.DefaultPartitions(), testAccGetRegion()); ok {
		if len(partition.Regions()) < 2 {
			t.Skip("skipping tests; partition only includes a single region")
		}
	}
}

func testAccAwsAccountProviderFunc(accountID string, providers *[]*schema.Provider) func() *schema.Provider {
	return func() *schema.Provider {
		if accountID == "" {
			log.Println("[DEBUG] No account ID given")
			return nil
		}
		if providers == nil {
			log.Println("[DEBUG] No providers given")
			return nil
		}

		log.Printf("[DEBUG] Checking providers for AWS account ID: %s", accountID)
		for _, provider := range *providers {
			// Ignore if Meta is empty, this can happen for validation providers
			if provider == nil || provider.Meta() == nil {
				log.Printf("[DEBUG] Skipping empty provider")
				continue
			}

			// Ignore if Meta is not AWSClient, this will happen for other providers
			client, ok := provider.Meta().(*AWSClient)
			if !ok {
				log.Printf("[DEBUG] Skipping non-AWS provider")
				continue
			}

			clientAccountID := client.accountid
			log.Printf("[DEBUG] Checking AWS provider account ID %q against %q", clientAccountID, accountID)
			if clientAccountID == accountID {
				log.Printf("[DEBUG] Found AWS provider with region: %s", accountID)
				return provider
			}
		}

		log.Printf("[DEBUG] No suitable provider found for %q in %d providers", accountID, len(*providers))
		return nil
	}
}

func testAccAwsRegionProviderFunc(region string, providers *[]*schema.Provider) func() *schema.Provider {
	return func() *schema.Provider {
		if region == "" {
			log.Println("[DEBUG] No region given")
			return nil
		}
		if providers == nil {
			log.Println("[DEBUG] No providers given")
			return nil
		}

		log.Printf("[DEBUG] Checking providers for AWS region: %s", region)
		for _, provider := range *providers {
			// Ignore if Meta is empty, this can happen for validation providers
			if provider == nil || provider.Meta() == nil {
				log.Printf("[DEBUG] Skipping empty provider")
				continue
			}

			// Ignore if Meta is not AWSClient, this will happen for other providers
			client, ok := provider.Meta().(*AWSClient)
			if !ok {
				log.Printf("[DEBUG] Skipping non-AWS provider")
				continue
			}

			clientRegion := client.region
			log.Printf("[DEBUG] Checking AWS provider region %q against %q", clientRegion, region)
			if clientRegion == region {
				log.Printf("[DEBUG] Found AWS provider with region: %s", region)
				return provider
			}
		}

		log.Printf("[DEBUG] No suitable provider found for %q in %d providers", region, len(*providers))
		return nil
	}
}

func testAccCheckWithProviders(f func(*terraform.State, *schema.Provider) error, providers *[]*schema.Provider) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		numberOfProviders := len(*providers)
		for i, provider := range *providers {
			if provider.Meta() == nil {
				log.Printf("[DEBUG] Skipping empty provider %d (total: %d)", i, numberOfProviders)
				continue
			}
			log.Printf("[DEBUG] Calling check with provider %d (total: %d)", i, numberOfProviders)
			if err := f(s, provider); err != nil {
				return err
			}
		}
		return nil
	}
}

// Check sweeper API call error for reasons to skip sweeping
// These include missing API endpoints and unsupported API calls
func testSweepSkipSweepError(err error) bool {
	// Ignore missing API endpoints
	if isAWSErr(err, "RequestError", "send request failed") {
		return true
	}
	// Ignore unsupported API calls
	if isAWSErr(err, "UnsupportedOperation", "") {
		return true
	}
	// Ignore more unsupported API calls
	// InvalidParameterValue: Use of cache security groups is not permitted in this API version for your account.
	if isAWSErr(err, "InvalidParameterValue", "not permitted in this API version for your account") {
		return true
	}
	return false
}
