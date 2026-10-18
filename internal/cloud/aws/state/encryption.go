package state

import (
	"errors"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// EncryptionMode selects the PutObject/CopyObject SSE policy for DIY state.
type EncryptionMode int

const (
	// EncryptionKMS requires a customer-managed KMS key ARN (AWS production).
	EncryptionKMS EncryptionMode = iota + 1
	// EncryptionAES256 uses S3 AES256 SSE without a KMS key (OVH/SCW/Floci).
	EncryptionAES256
	// EncryptionNone omits SSE headers. Only for emulators that reject SSE.
	EncryptionNone
)

// ObjectEncryption is the SSE policy passed to Manager and Archive constructors.
type ObjectEncryption struct {
	Mode      EncryptionMode
	KMSKeyARN string
}

func (e ObjectEncryption) validate() error {
	switch e.Mode {
	case EncryptionKMS:
		if !kmsKeyARN.MatchString(e.KMSKeyARN) {
			return errors.New("KMS encryption requires a customer-managed KMS key ARN")
		}
		return nil
	case EncryptionAES256, EncryptionNone:
		if e.KMSKeyARN != "" {
			return errors.New("AES256 and none encryption modes must not set a KMS key ARN")
		}
		return nil
	default:
		return errors.New("unknown object encryption mode")
	}
}

func (e ObjectEncryption) applyPut(input *s3.PutObjectInput) {
	switch e.Mode {
	case EncryptionKMS:
		input.ServerSideEncryption = s3types.ServerSideEncryptionAwsKms
		input.SSEKMSKeyId = awssdk.String(e.KMSKeyARN)
	case EncryptionAES256:
		input.ServerSideEncryption = s3types.ServerSideEncryptionAes256
	case EncryptionNone:
		// Emulator-compatible path: no SSE headers.
	}
}

func (e ObjectEncryption) applyCopy(input *s3.CopyObjectInput) {
	switch e.Mode {
	case EncryptionKMS:
		input.ServerSideEncryption = s3types.ServerSideEncryptionAwsKms
		input.SSEKMSKeyId = awssdk.String(e.KMSKeyARN)
	case EncryptionAES256:
		input.ServerSideEncryption = s3types.ServerSideEncryptionAes256
	case EncryptionNone:
		// Emulator-compatible path: no SSE headers.
	}
}
