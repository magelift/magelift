package plugin

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/magelift/magelift/internal/awsendpoint"
	"github.com/magelift/magelift/internal/mediasync"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/sdk"
)

func (s *Server) MediaImport(req *sdk.MediaTransferCall, resp *sdk.MediaTransferResult) error {
	return s.mediaTransfer(sdk.OpMediaImport, req, resp)
}

func (s *Server) MediaExport(req *sdk.MediaTransferCall, resp *sdk.MediaTransferResult) error {
	return s.mediaTransfer(sdk.OpMediaExport, req, resp)
}

func (s *Server) mediaTransfer(operation sdk.Operation, req *sdk.MediaTransferCall, resp *sdk.MediaTransferResult) error {
	return withPlan(req.ProtocolVersion, &resp.Error, req.Envelope, req.Plan, func(ctx context.Context, _ platform.StackModule, planned platform.PlannedStack) error {
		if !filepath.IsAbs(req.LocalDir) {
			return invalid(errors.New("media directory must be an absolute path"))
		}
		outputs, err := decodeOutputs(req.OutputsJSON)
		if err != nil {
			return err
		}
		bucket, err := platform.RequireStringOutput(outputs, platform.OutputMediaBucket)
		if err != nil {
			return invalid(err)
		}
		endpoint, err := awsendpoint.FromEnv()
		if err != nil {
			return invalid(err)
		}
		client, err := awsendpoint.S3Client(ctx, planned.Region(), endpoint)
		if err != nil {
			return err
		}
		resp.LocalDir = filepath.Clean(req.LocalDir)
		const prefix = "media/"
		if operation == sdk.OpMediaExport {
			resp.FileCount, resp.ByteCount, err = mediasync.Export(ctx, client, bucket, prefix, resp.LocalDir)
			return err
		}
		result, err := mediasync.Sync(ctx, mediasync.Options{Source: resp.LocalDir, Bucket: bucket, Prefix: prefix, Client: client})
		resp.FileCount, resp.ByteCount = int64(result.Uploaded), result.Bytes
		resp.Listing = &sdk.MediaListing{Keys: result.Keys, Missing: result.Diff.Missing, Extra: result.Diff.Extra}
		return err
	})
}
