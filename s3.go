package main

import (
	"os"
	"path"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

type s3Config struct {
	accessKeyID     string
	secretAccessKey string
	region          string
	bucket          string
}

func newS3Session(l *zap.Logger, region string) (*session.Session, error) {
	l.Info("create new AWS session", zap.String("region", region))

	sess, err := session.NewSession(&aws.Config{Region: &region})
	if err != nil {
		return nil, err
	}

	if _, err := sess.Config.Credentials.Get(); err != nil {
		return nil, errors.Wrapf(err, "failed to get AWS credentials")
	}

	return sess, nil
}

func newS3Config() (*s3Config, error) {
	accessKeyID, err := getEnvVar(accessKeyIDEnvKey)
	if err != nil {
		return nil, err
	}
	secretAccessKey, err := getEnvVar(secretAccessKeyEnvKey)
	if err != nil {
		return nil, err
	}

	region, err := getEnvVar(regionEnvKey)
	if err != nil {
		return nil, err
	}

	bucket, err := getEnvVar(bucketEnvKey)
	if err != nil {
		return nil, err
	}

	return &s3Config{
		accessKeyID:     accessKeyID,
		secretAccessKey: secretAccessKey,
		region:          region,
		bucket:          bucket,
	}, nil
}

func (b *backuper) ensureBucket() error {
	b.logger.Info("ensure bucket exists and correctly configured",
		zap.String("bucket", b.s3Config.bucket))

	svc := s3.New(b.s3Session)
	_, err := svc.GetBucketEncryption(&s3.GetBucketEncryptionInput{Bucket: &b.s3Config.bucket})
	if err != nil {
		return errors.Wrapf(err, "bucket does not exist or incorrectly configured")
	}

	return nil
}

func (b *backuper) uploadS3() error {
	task := "upload file to s3"
	b.logger.Info(task,
		zap.String("path", b.backupFile),
		zap.String("bucket", b.s3Config.bucket),
		zap.String("status", "running"))

	backupArchive, err := os.Open(b.backupFile)
	if err != nil {
		return errors.Wrapf(err, "open file failed")
	}

	defer deferWithErrLog(
		b.logger, func() error { return backupArchive.Close() },
		"closing backup archive failed")

	uploader := s3manager.NewUploader(b.s3Session)
	_, err = uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(b.s3Config.bucket),
		Key:    aws.String(path.Base(b.backupFile)),
		Body:   backupArchive,
	})
	if err != nil {
		return errors.Wrapf(err, "upload to bucket failed")
	}

	b.logger.Info(task, zap.String("status", "success"))

	return nil
}
