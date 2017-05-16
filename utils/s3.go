package utils

import (
	"bytes"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
)

func TestS3(s3Client *s3.S3, bucket string) error {
	params := &s3.HeadBucketInput{
		Bucket: aws.String(bucket),
	}
	_, err := s3Client.HeadBucket(params)
	if err != nil {
		return err
	}

	return nil
}

func PutS3File(s3Client *s3.S3, bucket string, path string, contentType string, contents []byte) (string, error) {
	params := &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Body:        bytes.NewReader(contents),
		Key:         aws.String(path),
		ContentType: aws.String(contentType),
	}
	_, err := s3Client.PutObject(params)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("https://%s.s3.amazonaws.com%s", bucket, path)
	return url, nil
}
