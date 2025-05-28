package aws

import (
	"context"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/acm"
	"github.com/aws/aws-sdk-go-v2/service/acm/types"
	"github.com/panther-labs/panther-cli/pkg/cloudconnected/config"
	"github.com/panther-labs/panther-cli/pkg/util"
	"github.com/pkg/errors"
)

type CertificateRegistrationHelper struct {
	ctx    context.Context
	cfg    *config.Config
	client *acm.Client
}

// CertificateValidationDetails contains the DNS record information needed for certificate validation
type CertificateValidationDetails struct {
	DomainName  string
	RecordName  string
	RecordValue string
	RecordType  string
}

// CertificateRegistrationResult contains the certificate ARN and validation details
type CertificateRegistrationResult struct {
	CertificateArn    string
	ValidationDetails CertificateValidationDetails
}

func NewCertificateRegistrationHelper(
	ctx context.Context,
	cfg *config.Config,
) (*CertificateRegistrationHelper, error) {
	// Get AWS config using the utility helper
	awsCfg, err := util.GetAWSConfig(
		ctx,
		cfg.AWSConfig.Region,
		cfg.AWSConfig.AccessKeyID,
		cfg.AWSConfig.SecretAccessKey,
		cfg.AWSConfig.SessionToken,
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create AWS config")
	}

	// Override region from our config
	awsCfg.Region = cfg.AWSConfig.Region

	// Create ACM client
	client := acm.NewFromConfig(awsCfg)

	return &CertificateRegistrationHelper{
		ctx:    ctx,
		cfg:    cfg,
		client: client,
	}, nil
}

// getACMClientForRegion returns an ACM client for a specific region
func (c *CertificateRegistrationHelper) getACMClientForRegion(region string) (*acm.Client, error) {
	awsCfg, err := util.GetAWSConfig(
		c.ctx,
		c.cfg.AWSConfig.Region,
		c.cfg.AWSConfig.AccessKeyID,
		c.cfg.AWSConfig.SecretAccessKey,
		c.cfg.AWSConfig.SessionToken,
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create AWS config")
	}

	// Override region
	awsCfg.Region = region

	return acm.NewFromConfig(awsCfg), nil
}

const (
	retryAttempts     = 10
	sleepDurationSecs = 5
)

// getValidationDetails retrieves the DNS validation details for a certificate
func (c *CertificateRegistrationHelper) getValidationDetails(
	certificateArn string,
	client *acm.Client,
) (CertificateValidationDetails, error) {
	input := &acm.DescribeCertificateInput{
		CertificateArn: aws.String(certificateArn),
	}

	log.Println(
		"Attempting to get domain ownership details for DNS validation for certificate. This may take multiple attempts...",
	)

	for ii := range 10 {
		result, err := client.DescribeCertificate(c.ctx, input)
		if err != nil {
			return CertificateValidationDetails{}, errors.Wrapf(
				err,
				"failed to describe certificate (%s)",
				certificateArn,
			)
		}

		if len(result.Certificate.DomainValidationOptions) == 0 {
			if ii < retryAttempts {
				log.Printf("Did not find domain validation options, retrying... (attempt %d)\n", ii+1)
				time.Sleep(sleepDurationSecs * time.Second)
				continue
			}
			return CertificateValidationDetails{}, errors.New("no domain validation options found")
		}

		// Get the first validation option (there should only be one for our use case)
		validation := result.Certificate.DomainValidationOptions[0]
		if validation.ResourceRecord == nil {
			if ii < retryAttempts {
				log.Printf("Did not find domain validation options, retrying... (attempt %d)\n", ii+1)
				time.Sleep(sleepDurationSecs * time.Second)
				continue
			}
			return CertificateValidationDetails{}, errors.New("no validation record found")
		}

		return CertificateValidationDetails{
			DomainName:  aws.ToString(validation.DomainName),
			RecordName:  aws.ToString(validation.ResourceRecord.Name),
			RecordValue: aws.ToString(validation.ResourceRecord.Value),
			RecordType:  string(validation.ResourceRecord.Type),
		}, nil
	}

	return CertificateValidationDetails{}, errors.New(
		"failed to get validation details after multiple attempts, please contact Panther Support",
	)
}

func (c *CertificateRegistrationHelper) RegisterPantherSubdomainCertificate() (CertificateRegistrationResult, error) {
	input := &acm.RequestCertificateInput{
		DomainName:       aws.String(c.cfg.AWSConfig.DomainCertificateConfiguration.PantherSubdomain),
		ValidationMethod: types.ValidationMethodDns,
	}

	result, err := c.client.RequestCertificate(c.ctx, input)
	if err != nil {
		return CertificateRegistrationResult{}, errors.Wrapf(
			err,
			"failed to request certificate for panther subdomain (%s)",
			c.cfg.AWSConfig.DomainCertificateConfiguration.PantherSubdomain,
		)
	}

	// Get validation details using the configured region's client
	validationDetails, err := c.getValidationDetails(*result.CertificateArn, c.client)
	if err != nil {
		return CertificateRegistrationResult{}, errors.Wrap(err, "failed to get validation details")
	}

	return CertificateRegistrationResult{
		CertificateArn:    *result.CertificateArn,
		ValidationDetails: validationDetails,
	}, nil
}

func (c *CertificateRegistrationHelper) RegisterWildcardSubdomainCertificate() (CertificateRegistrationResult, error) {
	// Get a us-east-1 specific client for the wildcard certificate. This is due to AWS's CloudFront control plane
	// living in us-east-1, so we need the cert to be there so it can work its magic.
	usEast1Client, err := c.getACMClientForRegion("us-east-1")
	if err != nil {
		return CertificateRegistrationResult{}, errors.Wrap(err, "failed to create us-east-1 ACM client")
	}

	// Create wildcard domain name from PantherSubdomain
	wildcardDomain := "*." + c.cfg.AWSConfig.DomainCertificateConfiguration.PantherSubdomain

	input := &acm.RequestCertificateInput{
		DomainName:       aws.String(wildcardDomain),
		ValidationMethod: types.ValidationMethodDns,
	}

	result, err := usEast1Client.RequestCertificate(c.ctx, input)
	if err != nil {
		return CertificateRegistrationResult{}, errors.Wrap(err, "failed to request wildcard certificate")
	}

	// Get validation details using the us-east-1 client
	validationDetails, err := c.getValidationDetails(*result.CertificateArn, usEast1Client)
	if err != nil {
		return CertificateRegistrationResult{}, errors.Wrap(err, "failed to get validation details")
	}

	return CertificateRegistrationResult{
		CertificateArn:    *result.CertificateArn,
		ValidationDetails: validationDetails,
	}, nil
}

// IsCertificateIssued checks if the certificate with the given ARN has been issued
func (c *CertificateRegistrationHelper) IsCertificateIssued(certificateArn string, isWildcard bool) (bool, error) {
	var client *acm.Client
	var err error

	if isWildcard {
		// Use us-east-1 client for wildcard certificate
		client, err = c.getACMClientForRegion("us-east-1")
		if err != nil {
			return false, errors.Wrap(err, "failed to create us-east-1 ACM client")
		}
	} else {
		// Use configured region's client for other certificates
		client = c.client
	}

	input := &acm.DescribeCertificateInput{
		CertificateArn: aws.String(certificateArn),
	}

	result, err := client.DescribeCertificate(c.ctx, input)
	if err != nil {
		return false, errors.Wrapf(err, "failed to describe certificate (%s)", certificateArn)
	}

	// Check if certificate status is ISSUED
	return result.Certificate.Status == types.CertificateStatusIssued, nil
}
