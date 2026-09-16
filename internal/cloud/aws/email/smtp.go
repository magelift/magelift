// Package email provisions managed SES sending for Magento: a verified
// domain identity with DKIM records, an SMTP credential pair in SSM Parameter
// Store, and the Magento SMTP wiring values.
package email

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
)

// SES SMTP derivation constants from the AWS SES documentation ("Obtaining
// SES SMTP credentials by converting existing AWS credentials"). Do not change.
const (
	sesSMTPDate     = "11111111"
	sesSMTPService  = "ses"
	sesSMTPMessage  = "SendRawEmail"
	sesSMTPTerminal = "aws4_request"
	sesSMTPVersion  = 0x04
)

// SmtpPort is the SES STARTTLS submission port.
const SmtpPort = 587

// SmtpEndpoint returns the regional SES SMTP endpoint.
func SmtpEndpoint(region string) string {
	return "email-smtp." + region + ".amazonaws.com"
}

// SmtpCredentialsJSON renders the {username, password} JSON stored in the
// managed credential parameter. Pure so the shape is unit-testable; the live
// cell proves the resolved values end to end.
func SmtpCredentialsJSON(username, password string) (string, error) {
	encoded, err := json.Marshal(map[string]string{"username": username, "password": password})
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// SmtpPasswordFromSecretAccessKey converts an IAM secret access key into the
// SES SMTP password for region, following the AWS-documented SigV4 chain
// (version byte 0x04 + HMAC-SHA256 over date, region, service, terminal,
// message, base64). It intentionally duplicates the provider's computed
// SesSmtpPassword attribute: that attribute documents support only for six
// regions (excluding eu-west-3), while this conversion is region-agnostic.
func SmtpPasswordFromSecretAccessKey(secretAccessKey, region string) string {
	sign := func(key []byte, message string) []byte {
		mac := hmac.New(sha256.New, key)
		mac.Write([]byte(message))
		return mac.Sum(nil)
	}
	signature := sign([]byte("AWS4"+secretAccessKey), sesSMTPDate)
	signature = sign(signature, region)
	signature = sign(signature, sesSMTPService)
	signature = sign(signature, sesSMTPTerminal)
	signature = sign(signature, sesSMTPMessage)
	versioned := append([]byte{sesSMTPVersion}, signature...)
	return base64.StdEncoding.EncodeToString(versioned)
}
