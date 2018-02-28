package utils

import (
	"bytes"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
)

var s3BucketURL = "https://%s.s3.amazonaws.com%s"

// TestS3 tests whether the passed in s3 client is properly configured and the passed in bucket is accessible
func TestS3(s3Client s3iface.S3API, bucket string) error {
	params := &s3.HeadBucketInput{
		Bucket: aws.String(bucket),
	}
	_, err := s3Client.HeadBucket(params)
	if err != nil {
		return err
	}

	return nil
}

// PutS3File writes the passed in file to the bucket with the passed in content type
func PutS3File(s3Client s3iface.S3API, bucket string, path string, contentType string, contents []byte) (string, error) {
	params := &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Body:        bytes.NewReader(contents),
		Key:         aws.String(path),
		ContentType: aws.String(contentType),
		ACL:         aws.String(s3.BucketCannedACLPublicRead),
	}
	_, err := s3Client.PutObject(params)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf(s3BucketURL, bucket, path)
	return url, nil
}
