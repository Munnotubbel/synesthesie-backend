package services

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go/logging"
	"github.com/synesthesie/backend/internal/config"
)

type S3Service struct {
	mediaClient  *s3.Client
	backupClient *s3.Client
	cfg          *config.Config
}

func NewS3Service(cfg *config.Config) (*S3Service, error) {
	media, err := buildClient(cfg.MediaS3Endpoint, cfg.MediaS3Region, cfg.MediaS3AccessKeyID, cfg.MediaS3SecretAccessKey, cfg.MediaS3UsePathStyle)
	if err != nil {
		return nil, err
	}
	backup, err := buildClient(cfg.BackupS3Endpoint, cfg.BackupS3Region, cfg.BackupS3AccessKeyID, cfg.BackupS3SecretAccessKey, cfg.BackupS3UsePathStyle)
	if err != nil {
		return nil, err
	}
	return &S3Service{mediaClient: media, backupClient: backup, cfg: cfg}, nil
}

func (s *S3Service) GetConfig() *config.Config { return s.cfg }

func buildClient(endpoint, region, key, secret string, pathStyle bool) (*s3.Client, error) {
	resolver := awsconfig.WithEndpointResolverWithOptions(aws.EndpointResolverWithOptionsFunc(
		func(service, rgn string, options ...interface{}) (aws.Endpoint, error) {
			if endpoint != "" {
				return aws.Endpoint{URL: endpoint, SigningRegion: region}, nil
			}
			return aws.Endpoint{}, &aws.EndpointNotFoundError{}
		}))
	cfg, err := awsconfig.LoadDefaultConfig(context.TODO(),
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(key, secret, "")),
		resolver,
		awsconfig.WithLogger(logging.NewStandardLogger(nil)),
	)
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = pathStyle
		if endpoint != "" {
			o.BaseEndpoint = &endpoint
		}
	})
	return client, nil
}

// Upload to media bucket (images or audio)
func (s *S3Service) UploadMedia(ctx context.Context, bucket, key string, body interface{}, ctype string) error {
	uploader := manager.NewUploader(s.mediaClient)
	in := &s3.PutObjectInput{
		Bucket:      &bucket,
		Key:         &key,
		ContentType: &ctype,
		ACL:         s3types.ObjectCannedACLPrivate,
	}
	if r, ok := body.(interface {
		Read(p []byte) (n int, err error)
	}); ok {
		in.Body = r
	}
	_, err := uploader.Upload(ctx, in, func(u *manager.Uploader) { u.PartSize = 10 * 1024 * 1024 })
	return err
}

// Presign GET from media
func (s *S3Service) PresignMediaGet(ctx context.Context, bucket, key string, ttl time.Duration) (string, error) {
	presigner := s3.NewPresignClient(s.mediaClient)
	out, err := presigner.PresignGetObject(ctx, &s3.GetObjectInput{Bucket: &bucket, Key: &key}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", err
	}
	return out.URL, nil
}

// Upload backup object
func (s *S3Service) UploadBackup(ctx context.Context, key string, body interface{}, ctype string) error {
	uploader := manager.NewUploader(s.backupClient)
	in := &s3.PutObjectInput{
		Bucket:      &s.cfg.BackupBucket,
		Key:         &key,
		ContentType: &ctype,
		ACL:         s3types.ObjectCannedACLPrivate,
	}
	if r, ok := body.(interface {
		Read(p []byte) (n int, err error)
	}); ok {
		in.Body = r
	}
	_, err := uploader.Upload(ctx, in, func(u *manager.Uploader) { u.PartSize = 10 * 1024 * 1024 })
	return err
}

func (s *S3Service) MediaURL(bucket, key string) string {
	e := s.mediaClient.Options().BaseEndpoint
	if e == nil {
		return ""
	}
	return fmt.Sprintf("%s/%s/%s", *e, bucket, url.PathEscape(key))
}

// Download media object to memory buffer (for local caching)
func (s *S3Service) DownloadMedia(ctx context.Context, bucket, key string) (*manager.WriteAtBuffer, error) {
	downloader := manager.NewDownloader(s.mediaClient)
	buf := manager.NewWriteAtBuffer([]byte{})
	_, err := downloader.Download(ctx, buf, &s3.GetObjectInput{Bucket: &bucket, Key: &key})
	if err != nil {
		return nil, err
	}
	return buf, nil
}

// Download media object directly to a local file (for large audio caching)
func (s *S3Service) DownloadMediaToFile(ctx context.Context, bucket, key, destPath string) error {
	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()
	downloader := manager.NewDownloader(s.mediaClient)
	_, err = downloader.Download(ctx, f, &s3.GetObjectInput{Bucket: &bucket, Key: &key})
	return err
}

// List media keys with prefix
func (s *S3Service) ListMediaKeys(ctx context.Context, bucket, prefix string, max int32) ([]string, error) {
	keys := []string{}
	var token *string
	for {
		out, err := s.mediaClient.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            &bucket,
			Prefix:            &prefix,
			ContinuationToken: token,
			MaxKeys:           aws.Int32(max),
		})
		if err != nil {
			return nil, err
		}
		for _, o := range out.Contents {
			keys = append(keys, *o.Key)
		}
		if aws.ToBool(out.IsTruncated) && out.NextContinuationToken != nil {
			token = out.NextContinuationToken
			continue
		}
		break
	}
	return keys, nil
}

// GetBackupClient returns the S3 client for backup operations
func (s *S3Service) GetBackupClient() (*s3.Client, error) {
	if s.backupClient == nil {
		return nil, fmt.Errorf("backup S3 client not configured")
	}
	return s.backupClient, nil
}

// DeleteMedia deletes an object from the media bucket
func (s *S3Service) DeleteMedia(ctx context.Context, bucket, key string) error {
	_, err := s.mediaClient.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: &bucket,
		Key:    &key,
	})
	return err
}
