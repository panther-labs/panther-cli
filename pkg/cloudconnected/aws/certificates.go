package aws

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/acm"
	"github.com/aws/aws-sdk-go-v2/service/acm/types"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/panther-labs/panther-cli/pkg/cloudconnected/config"
	"github.com/panther-labs/panther-cli/pkg/util"
	"github.com/pkg/errors"
)

type CertificateRegistrationHelper struct {
	ctx           context.Context
	cfg           *config.Config
	client        *acm.Client
	route53Client *route53.Client
}

// CertificateValidationDetails contains the DNS record information needed for certificate validation
type CertificateValidationDetails struct {
	DomainNames []string
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

	// Create ACM client
	client := acm.NewFromConfig(awsCfg)

	// Create Route 53 client
	route53Client := route53.NewFromConfig(awsCfg)

	return &CertificateRegistrationHelper{
		ctx:           ctx,
		cfg:           cfg,
		client:        client,
		route53Client: route53Client,
	}, nil
}

// getACMClientForRegion returns an ACM client for a specific region
func (c *CertificateRegistrationHelper) getACMClientForRegion(region string) (*acm.Client, error) {
	awsCfg, err := util.GetAWSConfig(
		c.ctx,
		region,
		c.cfg.AWSConfig.AccessKeyID,
		c.cfg.AWSConfig.SecretAccessKey,
		c.cfg.AWSConfig.SessionToken,
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create AWS config")
	}

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

		// Collect all domain names from the validation options
		domainNames := make([]string, len(result.Certificate.DomainValidationOptions))
		for i, opt := range result.Certificate.DomainValidationOptions {
			domainNames[i] = aws.ToString(opt.DomainName)
		}

		return CertificateValidationDetails{
			DomainNames: domainNames,
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
	pantherSubdomain := c.cfg.AWSConfig.DomainCertificateConfiguration.PantherSubdomain
	wildcardDomain := "*." + pantherSubdomain

	input := &acm.RequestCertificateInput{
		DomainName:              aws.String(wildcardDomain),
		ValidationMethod:        types.ValidationMethodDns,
		SubjectAlternativeNames: []string{pantherSubdomain},
	}

	result, err := c.client.RequestCertificate(c.ctx, input)
	if err != nil {
		return CertificateRegistrationResult{}, errors.Wrapf(
			err,
			"failed to request certificate for panther subdomain (%s)",
			pantherSubdomain,
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

// RegisterValidationDomains automatically creates DNS records for certificate validation
func (c *CertificateRegistrationHelper) RegisterValidationDomains(validationDetails CertificateValidationDetails) error {
	if !c.cfg.AWSConfig.DomainCertificateConfiguration.AutoRegisterValidationDomains {
		return nil // Skip if auto-registration is disabled
	}

	log.Printf("Attempting to auto-register DNS validation record: %s -> %s\n", 
		validationDetails.RecordName, validationDetails.RecordValue)

	// Find the hosted zone for the domain
	hostedZoneID, err := c.findHostedZoneForDomain(validationDetails.RecordName)
	if err != nil {
		return errors.Wrapf(err, "failed to find Route 53 hosted zone for domain %s. Ensure the domain is hosted in Route 53 in this AWS account", validationDetails.RecordName)
	}

	log.Printf("Found hosted zone %s for domain %s\n", hostedZoneID, validationDetails.RecordName)

	// Create the DNS record
	err = c.createDNSRecord(hostedZoneID, validationDetails)
	if err != nil {
		return errors.Wrap(err, "failed to create DNS validation record")
	}

	log.Printf("Successfully created DNS validation record %s (%s) -> %s\n", 
		validationDetails.RecordName, validationDetails.RecordType, validationDetails.RecordValue)
	return nil
}

// findHostedZoneForDomain finds the Route 53 hosted zone for a given domain
func (c *CertificateRegistrationHelper) findHostedZoneForDomain(domain string) (string, error) {
	// Remove trailing dot if present
	domain = strings.TrimSuffix(domain, ".")

	// List all hosted zones
	input := &route53.ListHostedZonesInput{}
	result, err := c.route53Client.ListHostedZones(c.ctx, input)
	if err != nil {
		return "", errors.Wrap(err, "failed to list hosted zones")
	}

	// Find the most specific zone that matches the domain
	var bestMatch string
	var bestMatchLength int

	for _, zone := range result.HostedZones {
		zoneName := strings.TrimSuffix(aws.ToString(zone.Name), ".")

		// Check if domain ends with the zone name
		if domain == zoneName || strings.HasSuffix(domain, "."+zoneName) {
			if len(zoneName) > bestMatchLength {
				bestMatch = aws.ToString(zone.Id)
				bestMatchLength = len(zoneName)
			}
		}
	}

	if bestMatch == "" {
		return "", errors.Errorf("no hosted zone found for domain %s", domain)
	}

	// Clean the hosted zone ID (remove /hostedzone/ prefix if present)
	bestMatch = strings.TrimPrefix(bestMatch, "/hostedzone/")

	return bestMatch, nil
}

// createDNSRecord creates a DNS record in Route 53 for certificate validation
func (c *CertificateRegistrationHelper) createDNSRecord(hostedZoneID string, validationDetails CertificateValidationDetails) error {
	// Prepare the change batch
	change := &route53types.Change{
		Action: route53types.ChangeActionUpsert,
		ResourceRecordSet: &route53types.ResourceRecordSet{
			Name: aws.String(validationDetails.RecordName),
			Type: route53types.RRType(validationDetails.RecordType),
			TTL:  aws.Int64(300), // 5 minutes TTL for validation records
			ResourceRecords: []route53types.ResourceRecord{
				{
					Value: aws.String(validationDetails.RecordValue),
				},
			},
		},
	}

	changeBatch := &route53types.ChangeBatch{
		Comment: aws.String("Certificate validation record created by panther-cli"),
		Changes: []route53types.Change{*change},
	}

	// Create the change request
	input := &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(hostedZoneID),
		ChangeBatch:  changeBatch,
	}

	_, err := c.route53Client.ChangeResourceRecordSets(c.ctx, input)
	if err != nil {
		return errors.Wrapf(err, "failed to create DNS record %s", validationDetails.RecordName)
	}

	return nil
}
