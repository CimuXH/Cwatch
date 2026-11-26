package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"slices"
	"strings"
	"time"

	madmin "github.com/minio/madmin-go/v3"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const (
	endpoint   = "minio:9000"
	bucket     = "cwatch"
	policyName = "cwatch-media"
	policy     = `{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": ["s3:GetBucketLocation", "s3:ListBucket", "s3:ListBucketMultipartUploads"],
      "Resource": ["arn:aws:s3:::cwatch"]
    },
    {
      "Effect": "Allow",
      "Action": ["s3:GetObject", "s3:PutObject", "s3:DeleteObject", "s3:AbortMultipartUpload", "s3:ListMultipartUploadParts"],
      "Resource": ["arn:aws:s3:::cwatch/*"]
    }
  ]
}`
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if err := bootstrap(ctx); err != nil {
		log.Fatal(err)
	}
	log.Print("Cwatch MinIO bucket and application permissions are ready")
}

func bootstrap(ctx context.Context) error {
	rootUser, rootPassword := os.Getenv("MINIO_ROOT_USER"), os.Getenv("MINIO_ROOT_PASSWORD")
	appUser, appPassword := os.Getenv("CWATCH_MINIO_ACCESS_KEY"), os.Getenv("CWATCH_MINIO_SECRET_KEY")
	root, err := client(rootUser, rootPassword)
	if err != nil {
		return err
	}
	for {
		exists, err := root.BucketExists(ctx, bucket)
		if err == nil {
			if !exists {
				if err := root.MakeBucket(ctx, bucket, minio.MakeBucketOptions{Region: "us-east-1"}); err != nil &&
					minio.ToErrorResponse(err).Code != "BucketAlreadyOwnedByYou" {
					return fmt.Errorf("create bucket: %w", err)
				}
			}
			break
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for MinIO: %w", err)
		case <-time.After(2 * time.Second):
		}
	}

	admin, err := madmin.New(endpoint, rootUser, rootPassword, false)
	if err != nil {
		return err
	}
	if err := admin.AddCannedPolicy(ctx, policyName, []byte(policy)); err != nil {
		return fmt.Errorf("create application policy: %w", err)
	}
	if err := admin.SetUser(ctx, appUser, appPassword, madmin.AccountEnabled); err != nil {
		return fmt.Errorf("create application account: %w", err)
	}
	info, err := admin.GetUserInfo(ctx, appUser)
	if err != nil {
		return fmt.Errorf("read application permissions: %w", err)
	}
	if !slices.Contains(strings.Split(info.PolicyName, ","), policyName) {
		if _, err := admin.AttachPolicy(ctx, madmin.PolicyAssociationReq{Policies: []string{policyName}, User: appUser}); err != nil {
			return fmt.Errorf("attach application policy: %w", err)
		}
	}

	app, err := client(appUser, appPassword)
	if err != nil {
		return err
	}
	exists, err := app.BucketExists(ctx, bucket)
	if err != nil {
		return fmt.Errorf("verify application account: %w", err)
	}
	if !exists {
		return fmt.Errorf("application account cannot see bucket %s", bucket)
	}
	return nil
}

func client(accessKey, password string) (*minio.Client, error) {
	return minio.New(endpoint, &minio.Options{
		Creds: credentials.NewStaticV4(accessKey, password, ""), Region: "us-east-1",
		BucketLookup: minio.BucketLookupPath, MaxRetries: 1,
	})
}
