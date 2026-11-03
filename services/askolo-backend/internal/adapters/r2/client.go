package r2

import (
	"bytes"
	"context"
	"fmt"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"

	"askolo/backend/internal/config"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const maxPresignTTL = 15 * time.Minute

type Client struct {
	bucket    string
	s3        *s3.Client
	presigner *s3.PresignClient
}

type PresignedRequest struct {
	URL     string
	Method  string
	Headers http.Header
}

type Object struct {
	Key          string
	Size         int64
	LastModified time.Time
}

func New(cfg config.R2Config) (*Client, error) {
	if !validAccountID(cfg.AccountID) || !validBucketName(cfg.Bucket) ||
		cfg.AccessKeyID == "" || cfg.SecretAccessKey == "" {
		return nil, fmt.Errorf("R2 storage configuration is incomplete")
	}
	endpoint := "https://" + cfg.AccountID + ".r2.cloudflarestorage.com"
	credentials := aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
		return aws.Credentials{
			AccessKeyID:     cfg.AccessKeyID,
			SecretAccessKey: cfg.SecretAccessKey,
			Source:          "Askolo R2",
		}, nil
	})
	s3Client := s3.New(s3.Options{
		BaseEndpoint:     aws.String(endpoint),
		Region:           "auto",
		Credentials:      aws.NewCredentialsCache(credentials),
		UsePathStyle:     true,
		RetryMaxAttempts: 3,
	})
	return &Client{
		bucket:    cfg.Bucket,
		s3:        s3Client,
		presigner: s3.NewPresignClient(s3Client),
	}, nil
}

func (c *Client) PresignPut(
	ctx context.Context,
	key string,
	contentType string,
	expires time.Duration,
) (PresignedRequest, error) {
	if c == nil || c.presigner == nil || !validObjectKey(key) ||
		!validContentType(contentType) || !validPresignTTL(expires) {
		return PresignedRequest{}, fmt.Errorf("invalid R2 upload request")
	}
	request, err := c.presigner.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(c.bucket),
		Key:         aws.String(key),
		ContentType: aws.String(contentType),
	}, func(options *s3.PresignOptions) {
		options.Expires = expires
	})
	if err != nil {
		return PresignedRequest{}, fmt.Errorf("R2 upload request could not be prepared")
	}
	return PresignedRequest{URL: request.URL, Method: request.Method, Headers: request.SignedHeader}, nil
}

func (c *Client) PresignGet(
	ctx context.Context,
	key string,
	expires time.Duration,
) (PresignedRequest, error) {
	return c.presignGet(ctx, key, expires, "attachment")
}

func (c *Client) PresignGetInline(ctx context.Context, key string, expires time.Duration) (PresignedRequest, error) {
	return c.presignGet(ctx, key, expires, "inline")
}

func (c *Client) PresignGetAttachment(ctx context.Context, key string, expires time.Duration, fileName string) (PresignedRequest, error) {
	if strings.TrimSpace(fileName) == "" || fileName == "." || fileName == ".." ||
		len(fileName) > 240 || strings.ContainsAny(fileName, "/\\\r\n\x00") {
		return PresignedRequest{}, fmt.Errorf("invalid attachment filename")
	}
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": fileName})
	if disposition == "" {
		return PresignedRequest{}, fmt.Errorf("invalid attachment filename")
	}
	return c.presignGet(ctx, key, expires, disposition)
}

func (c *Client) presignGet(
	ctx context.Context,
	key string,
	expires time.Duration,
	disposition string,
) (PresignedRequest, error) {
	if c == nil || c.presigner == nil || !validObjectKey(key) || !validPresignTTL(expires) {
		return PresignedRequest{}, fmt.Errorf("invalid R2 download request")
	}
	request, err := c.presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket:                     aws.String(c.bucket),
		Key:                        aws.String(key),
		ResponseContentDisposition: aws.String(disposition),
		ResponseCacheControl:       aws.String("private, no-store"),
	}, func(options *s3.PresignOptions) {
		options.Expires = expires
	})
	if err != nil {
		return PresignedRequest{}, fmt.Errorf("R2 download request could not be prepared")
	}
	return PresignedRequest{URL: request.URL, Method: request.Method, Headers: request.SignedHeader}, nil
}

func (c *Client) PutObject(ctx context.Context, key, contentType string, body []byte) error {
	if c == nil || c.s3 == nil || !validObjectKey(key) ||
		!validContentType(contentType) || len(body) == 0 {
		return fmt.Errorf("invalid R2 upload request")
	}
	if _, err := c.s3.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(c.bucket),
		Key:           aws.String(key),
		Body:          bytes.NewReader(body),
		ContentLength: aws.Int64(int64(len(body))),
		ContentType:   aws.String(contentType),
	}); err != nil {
		return fmt.Errorf("R2 upload operation failed")
	}
	return nil
}

func (c *Client) ListObjects(ctx context.Context, prefix string, limit int32) ([]Object, error) {
	if c == nil || c.s3 == nil || !validObjectKey(strings.TrimSuffix(prefix, "/")) ||
		!strings.HasSuffix(prefix, "/") || limit < 1 || limit > 1000 {
		return nil, fmt.Errorf("invalid R2 list request")
	}
	result, err := c.s3.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket:  aws.String(c.bucket),
		Prefix:  aws.String(prefix),
		MaxKeys: aws.Int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("R2 list operation failed")
	}
	objects := make([]Object, 0, len(result.Contents))
	for _, item := range result.Contents {
		if item.Key == nil || !strings.HasPrefix(*item.Key, prefix) {
			continue
		}
		object := Object{Key: *item.Key}
		if item.Size != nil {
			object.Size = *item.Size
		}
		if item.LastModified != nil {
			object.LastModified = *item.LastModified
		}
		objects = append(objects, object)
	}
	return objects, nil
}

func (c *Client) Delete(ctx context.Context, key string) error {
	if c == nil || c.s3 == nil || !validObjectKey(key) {
		return fmt.Errorf("invalid R2 delete request")
	}
	if _, err := c.s3.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	}); err != nil {
		return fmt.Errorf("R2 delete operation failed")
	}
	return nil
}

func validPresignTTL(value time.Duration) bool {
	return value > 0 && value <= maxPresignTTL
}

func validContentType(value string) bool {
	mediaType, _, err := mime.ParseMediaType(value)
	return err == nil && mediaType != "" && len(value) <= 255 && !strings.ContainsAny(value, "\r\n")
}

func validObjectKey(value string) bool {
	if len(value) == 0 || len(value) > 1024 || strings.HasPrefix(value, "/") ||
		strings.ContainsAny(value, "\\\r\n\x00") || path.Clean(value) != value {
		return false
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "." || segment == ".." || segment == "" {
			return false
		}
		for _, character := range segment {
			if !(character >= 'a' && character <= 'z') &&
				!(character >= 'A' && character <= 'Z') &&
				!(character >= '0' && character <= '9') &&
				!strings.ContainsRune("-_.", character) {
				return false
			}
		}
	}
	return true
}

func validAccountID(value string) bool {
	if len(value) != 32 {
		return false
	}
	for _, character := range value {
		if !(character >= '0' && character <= '9') &&
			!(character >= 'a' && character <= 'f') &&
			!(character >= 'A' && character <= 'F') {
			return false
		}
	}
	return true
}

func validBucketName(value string) bool {
	if len(value) < 3 || len(value) > 63 {
		return false
	}
	for index, character := range value {
		if !(character >= 'a' && character <= 'z') &&
			!(character >= '0' && character <= '9') &&
			!(character == '-' && index > 0 && index < len(value)-1) {
			return false
		}
	}
	return true
}
