package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/pkg/errors"
	"go.etcd.io/etcd/clientv3"
	"go.etcd.io/etcd/clientv3/snapshot"
	"go.etcd.io/etcd/pkg/transport"
	"go.uber.org/zap"
)

type etcdEnvVars struct {
	endpoints        []string
	selectedEndpoint string
	CAcert           string
	cert             string
	key              string
}

func readEtcdEnvVariableFromFile(l *zap.Logger, filepath string) (*etcdEnvVars, error) {
	l.Info("read environment variables from file",
		zap.String("path", filepath))

	etcdEnvVars := etcdEnvVars{}
	file, err := os.Open(filepath)
	if err != nil {
		return nil, errors.Wrapf(err, fmt.Sprintf("unable to open file: %s", filepath))
	}

	// snapshot must be requested to only one selected node, so we use
	// one existing variable to build the selected endpoint.
	// The variable are built like: NODE_my_hostname_with_underscore_ETCD_URL_HOST="172.10.1.138"
	hostname, err := os.Hostname()
	if err != nil {
		return nil, err
	}
	hostname = strings.ReplaceAll(hostname, "-", "_")
	etcdURLHostKey := "NODE_" + hostname + "_ETCD_URL_HOST"

	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		// Sanitize the line, export ETCDCTL_API="3"
		line = strings.TrimPrefix(line, "export ")
		line = strings.TrimSuffix(line, "\"")
		lineSplit := strings.Split(line, "=\"")

		switch lineSplit[0] {
		case etcdCertKey:
			etcdEnvVars.cert = lineSplit[1]
		case etcdKeyKey:
			etcdEnvVars.key = lineSplit[1]
		case etcdCACertKey:
			etcdEnvVars.CAcert = lineSplit[1]
		case etcdEndpointsKey:
			// http://host:2379,http://host2:2379
			etcdEnvVars.endpoints = strings.Split(lineSplit[1], ",")
		case etcdURLHostKey:
			etcdEnvVars.selectedEndpoint = "https://" + lineSplit[1] + ":2379"
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	switch {
	case etcdEnvVars.cert == "":
		return nil, fmt.Errorf("certificate not found in env var file")
	case etcdEnvVars.key == "":
		return nil, fmt.Errorf("keynot found in env var file")
	case etcdEnvVars.CAcert == "":
		return nil, fmt.Errorf("CA certificate not found in env var file")
	case etcdEnvVars.endpoints == nil:
		return nil, fmt.Errorf("endpoints not found in env var file")
	}

	return &etcdEnvVars, nil
}

// generateTLSConfig reads the local certificates and creates
// the TLS Config for the etcdv3 client.
func generateTLSConfig(etcdEnvVars *etcdEnvVars) (*tls.Config, error) {
	TLSInfo := transport.TLSInfo{
		CertFile:      etcdEnvVars.cert,
		KeyFile:       etcdEnvVars.key,
		TrustedCAFile: etcdEnvVars.CAcert,
	}

	TLSConfig, err := TLSInfo.ClientConfig()
	if err != nil {
		return nil, err
	}

	return TLSConfig, nil
}

func (b *backuper) newEtcdV3Config(etcdEnvVars *etcdEnvVars) (clientv3.Config, error) {
	b.logger.Debug("create etcd client configuration")

	etcdV3Config := clientv3.Config{
		DialTimeout: b.etcdDialTimeout,
		Endpoints:   []string{etcdEnvVars.selectedEndpoint},
	}

	TLSConfig, err := generateTLSConfig(etcdEnvVars)
	if err != nil {
		return etcdV3Config, err
	}
	etcdV3Config.TLS = TLSConfig

	return etcdV3Config, nil
}

func (b *backuper) snapshotEtcd(ctx context.Context, etcdV3Config clientv3.Config) error {
	b.logger.Info("snaphot etcd")
	etcdManager := snapshot.NewV3(b.logger)
	if err := etcdManager.Save(ctx, etcdV3Config, b.snapshotFileBackup); err != nil {
		return err
	}

	return createSHA256HashFileFromFile(b.snapshotFileBackup)
}

func (b *backuper) backupStaticResources() error {
	b.logger.Info("back up Kube Static Resources",
		zap.String("source", staticResources),
		zap.String("archive", b.staticResourcesBackup))

	if err := tarDir(staticResources, b.staticResourcesBackup); err != nil {
		return err
	}

	return createSHA256HashFileFromFile(b.staticResourcesBackup)
}

func (b *backuper) createBackupArchive() error {
	b.logger.Info("create backup archive to push to s3",
		zap.String("source", b.tmpDir),
		zap.String("archive", b.backupFile))

	return tarDir(b.tmpDir, b.backupFile)
}

func (b *backuper) backupComponents(ctx context.Context, etcdEnvFile, etcdDialTimeout string) error {
	b.logger.Info("backup local cluster components")

	// etcd
	etcdEnvVars, err := readEtcdEnvVariableFromFile(b.logger, etcdEnvFile)
	if err != nil {
		return err
	}

	// The path to the certificate and keys does not always
	// exist at the host level, in this case we need a symlink.
	// actual:   /etc/kubernetes/static-pod-resources/etcd-certs/secrets/etcd-all-peer/master-1.crt
	// expected: /etc/kubernetes/static-pod-certs/secrets/etcd-all-peer/master-1.crt
	if _, err := os.Stat(etcdEnvVars.cert); os.IsNotExist(err) {
		os.Symlink(
			path.Join(hostConfigDir, "static-pod-resources/etcd-certs"),
			path.Join(hostConfigDir, "static-pod-certs"))
	}

	etcdV3Config, err := b.newEtcdV3Config(etcdEnvVars)
	if err != nil {
		return err
	}

	if err := b.snapshotEtcd(ctx, etcdV3Config); err != nil {
		return errors.Wrapf(err, "snapshot failed")
	}

	// Kube Static Resources.
	if err := b.backupStaticResources(); err != nil {
		return errors.Wrapf(err, "backup of kube static resources failed")
	}

	// Create final archive to push
	if err := b.createBackupArchive(); err != nil {
		return errors.Wrapf(err, "create final backup archive failed")
	}

	b.logger.Info("backup local cluster components finished",
		zap.String("status", "success"))

	return nil
}
