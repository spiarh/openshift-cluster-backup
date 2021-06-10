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
	var err error
	var config s3Config = s3Config{}

	if config.accessKeyID, err = getEnvVar(accessKeyIDEnvKey); err != nil {
		return nil, err
	}
	if config.secretAccessKey, err = getEnvVar(secretAccessKeyEnvKey); err != nil {
		return nil, err
	}
	if config.region, err = getEnvVar(regionEnvKey); err != nil {
		return nil, err
	}
	if config.bucket, err = getEnvVar(bucketEnvKey); err != nil {
		return nil, err
	}

	return &config, nil
}

func (b *backuper) ensureBucket() error {
	b.logger.Info("ensure bucket exists and correctly configured",
		zap.String("bucket", b.s3Config.bucket))

	svc := s3.New(b.s3Session)
	_, err := svc.GetBucketEncryption(&s3.GetBucketEncryptionInput{Bucket: &b.s3Config.bucket})
	if err != nil {
		return err
	}

	return nil
}

func (b *backuper) uploadS3() error {
	b.logger.Info("upload file to s3",
		zap.String("path", b.backupFile),
		zap.String("bucket", b.s3Config.bucket))

	backupArchive, err := os.Open(b.backupFile)
	if err != nil {
		b.logger.Error("open file failed",
			zap.String("path", b.backupFile))
		return err
	}

	defer backupArchive.Close()

	uploader := s3manager.NewUploader(b.s3Session)
	_, err = uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(b.s3Config.bucket),
		Key:    aws.String(path.Base(b.backupFile)),
		Body:   backupArchive,
	})
	if err != nil {
		b.logger.Error("upload to bucket failed",
			zap.String("path", b.backupFile),
			zap.String("bucket", b.s3Config.bucket),
		)
		return err
	}

	b.logger.Info("upload file to s3 finished", zap.String("status", "success"))

	return nil
}
