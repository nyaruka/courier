package courier

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
)

func testS3(s *server) error {
	params := &s3.HeadBucketInput{
		Bucket: aws.String(s.config.S3MediaBucket),
	}
	_, err := s.s3Client.HeadBucket(params)
	if err != nil {
		return err
	}

	return nil
}

func putS3File(s *server, filename string, contentType string, contents []byte) (string, error) {
	path := filepath.Join(s.config.S3MediaPrefix, filename[:4], filename)
	if !strings.HasPrefix(path, "/") {
		path = fmt.Sprintf("/%s", path)
	}

	params := &s3.PutObjectInput{
		Bucket:      aws.String(s.config.S3MediaBucket),
		Body:        bytes.NewReader(contents),
		Key:         aws.String(path),
		ContentType: aws.String(contentType),
	}
	_, err := s.s3Client.PutObject(params)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("https://%s.s3.amazonaws.com%s", s.config.S3MediaBucket, path)
	return url, nil
}
