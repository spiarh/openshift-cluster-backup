package main

import (
	"context"
	"flag"
	"fmt"
	"path"
	"time"

	"go.uber.org/zap"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/pkg/errors"
)

type backuper struct {
	logger *zap.Logger

	etcdEnvFile       string
	etcdDialTimeout   time.Duration
	etcdBackupTimeout time.Duration

	tmpDir                string
	snapshotFileBackup    string
	staticResourcesBackup string
	backupFile            string

	s3Config  *s3Config
	s3Session *session.Session
}

func newBackuper(l *zap.Logger, tmpDir, etcdEnvFile, etcdDialTimeout, etcdBackupTimeout string) (*backuper, error) {
	now := time.Now().Format("2006-01-02T15-04-05")

	s3Config, err := newS3Config()
	if err != nil {
		return nil, errors.Wrapf(err, "retrieve AWS S3 configuration failed")
	}

	s3Session, err := newS3Session(l, s3Config.region)
	if err != nil {
		return nil, err
	}

	etcdDialTimeoutDuration, err := time.ParseDuration(etcdDialTimeout)
	if err != nil {
		return nil, err
	}
	etcdBackupTimeoutDuration, err := time.ParseDuration(etcdBackupTimeout)
	if err != nil {
		return nil, err
	}

	return &backuper{
		logger:                l,
		tmpDir:                tmpDir,
		etcdEnvFile:           etcdEnvFile,
		etcdDialTimeout:       etcdDialTimeoutDuration,
		etcdBackupTimeout:     etcdBackupTimeoutDuration,
		snapshotFileBackup:    path.Join(tmpDir, fmt.Sprintf("%s_%s.db", snapshotPrefix, now)),
		staticResourcesBackup: path.Join(tmpDir, fmt.Sprintf("%s_%s.tgz", staticResourcesPrefix, now)),
		backupFile:            path.Join(tmpDir, fmt.Sprintf("%s_%s.tgz", defaultName, now)),
		s3Session:             s3Session,
		s3Config:              s3Config,
	}, nil
}

func main() {
	var (
		etcdEnvFile       string
		etcdDialTimeout   string
		etcdBackupTimeout string
	)

	flag.StringVar(&etcdEnvFile, "etcd-env-file", defaultEtcdEnvFile, "The path to the etcd environment variable path.")
	flag.StringVar(&etcdDialTimeout, "etcd-dial-timeout", defaultEtcdDialTimeout, "The timeout for failing to establish a connection to etcd")
	flag.StringVar(&etcdBackupTimeout, "etcd-backup-timeout", defaultEtcdBackupTimeout, "The timeout for backing up etcd")
	flag.Parse()

	// Logging
	logger, err := zap.NewProduction()
	if err != nil {
		panic(fmt.Sprintf("create logger failed: %v", err))
	}
	logger = logger.With(zap.String("name", defaultName))
	defer logger.Sync()

	// Create backup configuration.
	// TODO: add deletion
	tmpDir, err := createTempDir(defaultName)
	if err != nil {
		logger.Fatal("temporary backup dir creation failed", zap.Error(err))
	}

	backuper, err := newBackuper(logger, tmpDir, etcdEnvFile, etcdDialTimeout, etcdBackupTimeout)
	if err != nil {
		logger.Fatal("create backuper failed", zap.Error(err))
	}

	logger.Debug("temporary backup directory", zap.String("path", tmpDir))

	if err := backuper.ensureBucket(); err != nil {
		logger.Fatal("bucket does not exist or incorrectly configured", zap.Error(err))
	}

	ctx, ctxCancel := context.WithTimeout(context.Background(), backuper.etcdBackupTimeout)
	defer ctxCancel()
	if err := backuper.backup(ctx, etcdEnvFile, etcdDialTimeout); err != nil {
		logger.Fatal("backup finished",
			zap.String("status", "failed"),
			zap.Error(err))
	}

	if err := backuper.uploadS3(); err != nil {
		logger.Fatal("upload finished",
			zap.String("status", "failed"),
			zap.Error(err))
		logger.Fatal("", zap.Error(err))
	}
}
