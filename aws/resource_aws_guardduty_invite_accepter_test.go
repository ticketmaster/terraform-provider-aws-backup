package aws

import (
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/guardduty"
	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/helper/schema"
	"github.com/hashicorp/terraform/terraform"
)

func testAccAwsGuardDutyInviteAccepter_basic(t *testing.T) {
	var providers []*schema.Provider
	resourceName := "aws_guardduty_invite_accepter.test"
	accountID, email := testAccAWSGuardDutyMemberFromEnv(t)

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccAwsAlternateAccountPreCheck(t)
		},
		ProviderFactories: testAccProviderFactories(&providers),
		CheckDestroy:      testAccCheckWithProviders(testAccCheckAwsGuardDutyInviteAccepterDestroyWithProvider, &providers),
		Steps: []resource.TestStep{
			{
				Config: testAccAwsGuardDutyInviteAccepterConfig_basic(email),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckAwsGuardDutyInviteAccepterExistsWithProvider(resourceName, testAccAwsAccountProviderFunc(accountID, &providers)),
					resource.TestCheckResourceAttrSet(resourceName, "detector_id"),
					resource.TestCheckResourceAttrSet(resourceName, "master_id"),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccCheckAwsGuardDutyInviteAccepterDestroy(s *terraform.State) error {
	return testAccCheckAwsGuardDutyInviteAccepterDestroyWithProvider(s, testAccProvider)
}

func testAccCheckAwsGuardDutyInviteAccepterDestroyWithProvider(s *terraform.State, provider *schema.Provider) error {
	conn := provider.Meta().(*AWSClient).guarddutyconn

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "aws_guardduty_invite_accepter" {
			continue
		}

		detectorID, masterID, err := decodeGuardDutyInviteAccepterID(rs.Primary.ID)
		if err != nil {
			return err
		}

		input := &guardduty.GetMasterAccountInput{
			DetectorId: aws.String(detectorID),
		}

		output, err := conn.GetMasterAccount(input)
		if err != nil {
			if isAWSErr(err, guardduty.ErrCodeBadRequestException, "The request is rejected because the input detectorId is not owned by the current account.") {
				return nil
			}
			return err
		}

		if output.Master == nil || aws.StringValue(output.Master.AccountId) != masterID {
			continue
		}

		return fmt.Errorf("Expected GuardDuty Invite Accepter to be destroyed, %s found", rs.Primary.ID)
	}

	return nil
}

func testAccCheckAwsGuardDutyInviteAccepterExists(resourceName string) resource.TestCheckFunc {
	return testAccCheckAwsGuardDutyInviteAccepterExistsWithProvider(resourceName, func() *schema.Provider { return testAccProvider })
}

func testAccCheckAwsGuardDutyInviteAccepterExistsWithProvider(resourceName string, providerF func() *schema.Provider) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("Not found: %s", resourceName)
		}

		detectorID, masterID, err := decodeGuardDutyInviteAccepterID(rs.Primary.ID)
		if err != nil {
			return err
		}

		conn := providerF().Meta().(*AWSClient).guarddutyconn

		input := &guardduty.GetMasterAccountInput{
			DetectorId: aws.String(detectorID),
		}

		output, err := conn.GetMasterAccount(input)
		if err != nil {
			return err
		}

		if output.Master == nil {
			return fmt.Errorf("no master account found for: %s", resourceName)
		}

		if aws.StringValue(output.Master.AccountId) != masterID {
			return fmt.Errorf("expected master account %q, received %q", masterID, aws.StringValue(output.Master.AccountId))
		}

		return nil
	}
}

func testAccAwsGuardDutyInviteAccepterConfig_basic(email string) string {
	return testAccAwsAlternateAccountProviderConfig + fmt.Sprintf(`
resource "aws_guardduty_detector" "master" {}

resource "aws_guardduty_detector" "member" {
  provider = "aws.alternate"
}

resource "aws_guardduty_member" "member" {
  account_id                 = "${aws_guardduty_detector.member.account_id}"
  detector_id                = "${aws_guardduty_detector.master.id}"
  disable_email_notification = true
  email                      = "%s"
  invite                     = true
}

resource "aws_guardduty_invite_accepter" "member" {
  depends_on = ["aws_guardduty_member.member"]
  provider   = "aws.alternate"

  detector_id        = "${aws_guardduty_detector.member.id}"
  master_id          = "${aws_guardduty_detector.master.account_id}"
}
`, email)
}
