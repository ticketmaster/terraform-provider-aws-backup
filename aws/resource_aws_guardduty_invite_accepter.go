package aws

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/guardduty"
	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/helper/schema"
)

func resourceAwsGuardDutyInviteAccepter() *schema.Resource {
	return &schema.Resource{
		Create: resourceAwsGuardDutyInviteAccepterCreate,
		Read:   resourceAwsGuardDutyInviteAccepterRead,
		Delete: resourceAwsGuardDutyInviteAccepterDelete,

		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Schema: map[string]*schema.Schema{
			"detector_id": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"master_id": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validateAwsAccountId,
			},
		},
		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(60 * time.Second),
		},
	}
}

func resourceAwsGuardDutyInviteAccepterCreate(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*AWSClient).guarddutyconn

	detectorID := d.Get("detector_id").(string)
	invitationID := ""
	masterID := d.Get("master_id").(string)

	listInvitationsInput := &guardduty.ListInvitationsInput{}

	err := resource.Retry(d.Timeout(schema.TimeoutCreate), func() *resource.RetryError {
		log.Printf("[DEBUG] Listing GuardDuty Invitations: %s", listInvitationsInput)
		err := conn.ListInvitationsPages(listInvitationsInput, func(page *guardduty.ListInvitationsOutput, lastPage bool) bool {
			for _, invitation := range page.Invitations {
				if aws.StringValue(invitation.AccountId) == masterID {
					invitationID = aws.StringValue(invitation.InvitationId)
					return false
				}
			}
			return !lastPage
		})

		if err != nil {
			return resource.NonRetryableError(fmt.Errorf("error listing GuardDuty Invitations: %s", err))
		}

		if invitationID == "" {
			return resource.RetryableError(fmt.Errorf("unable to find pending GuardDuty Invitation for detector ID %q from master account ID %q", detectorID, masterID))
		}

		return nil
	})
	if err != nil {
		return err
	}

	acceptInvitationInput := &guardduty.AcceptInvitationInput{
		DetectorId:   aws.String(detectorID),
		InvitationId: aws.String(invitationID),
		MasterId:     aws.String(masterID),
	}

	log.Printf("[INFO] Accepting GuardDuty Invitation: %s", acceptInvitationInput)
	_, err = conn.AcceptInvitation(acceptInvitationInput)
	if err != nil {
		return fmt.Errorf("error accepting GuardDuty Invitation %q: %s", invitationID, err)
	}

	d.SetId(fmt.Sprintf("%s:%s", detectorID, masterID))

	return resourceAwsGuardDutyInviteAccepterRead(d, meta)
}

func resourceAwsGuardDutyInviteAccepterRead(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*AWSClient).guarddutyconn

	detectorID, _, err := decodeGuardDutyInviteAccepterID(d.Id())
	if err != nil {
		return err
	}

	input := guardduty.GetMasterAccountInput{
		DetectorId: aws.String(detectorID),
	}

	log.Printf("[DEBUG] Reading GuardDuty Master Account: %s", input)
	output, err := conn.GetMasterAccount(&input)
	if err != nil {
		if isAWSErr(err, guardduty.ErrCodeBadRequestException, "The request is rejected because the input detectorId is not owned by the current account.") {
			log.Printf("[WARN] GuardDuty Master Account %q not found, removing from state", d.Id())
			d.SetId("")
			return nil
		}
		return fmt.Errorf("error reading GuardDuty Master Account %q: %s", d.Id(), err)
	}

	if output.Master == nil {
		log.Printf("[WARN] GuardDuty Master Account %q not found, removing from state", d.Id())
		d.SetId("")
		return nil
	}

	d.Set("detector_id", detectorID)
	d.Set("master_id", output.Master.AccountId)

	return nil
}

func resourceAwsGuardDutyInviteAccepterDelete(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*AWSClient).guarddutyconn

	detectorID, _, err := decodeGuardDutyInviteAccepterID(d.Id())
	if err != nil {
		return err
	}

	input := &guardduty.DisassociateFromMasterAccountInput{
		DetectorId: aws.String(detectorID),
	}

	log.Printf("[DEBUG] Disassociating from GuardDuty Master Account: %s", input)
	_, err = conn.DisassociateFromMasterAccount(input)
	if err != nil {
		return fmt.Errorf("error disassociating %q from GuardDuty Master Account: %s", d.Id(), err)
	}
	return nil
}

func decodeGuardDutyInviteAccepterID(id string) (string, string, error) {
	parts := strings.Split(id, ":")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("GuardDuty Invite Accepter ID must be of the form <Member Detector ID>:<Master AWS Account ID>, was provided: %s", id)
	}
	return parts[0], parts[1], nil
}
