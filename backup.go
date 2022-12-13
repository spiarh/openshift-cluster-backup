package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pkg/errors"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/snapshot"
	"go.etcd.io/etcd/pkg/transport"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

type etcdEnvVars struct {
	endpoints        []string
	selectedEndpoint string
	CAcert           string
	cert             string
	key              string
}

func readEtcdEnvVariableFromFile(logger *zap.Logger, getHostname func() (string, error), path string) (*etcdEnvVars, error) {
	logger.Info("read environment variables from file",
		zap.String("path", path))

	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		return nil, errors.Wrapf(err, fmt.Sprintf("unable to open file: %s", path))
	}

	// snapshot must be requested to only one selected node, so we use
	// one existing variable to build the selected endpoint.
	// The variable are built like: NODE_my_hostname_with_underscore_ETCD_URL_HOST="172.10.1.138"
	hostname, err := getHostname()
	if err != nil {
		return nil, err
	}
	hostname = strings.ReplaceAll(hostname, "-", "_")

	etcdURLHostKeyRegex := "NODE_" + hostname + ".*" + "_ETCD_URL_HOST"

	defer deferWithErrLog(
		logger, func() error { return file.Close() },
		"closing etcd environment variables from file failed")

	etcdEnvVars := etcdEnvVars{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		// Sanitize the line, export ETCDCTL_API="3"
		line = strings.TrimPrefix(line, "export ")
		line = strings.TrimSuffix(line, "\"")
		lineSplit := strings.Split(line, "=\"")

		key := lineSplit[0]
		value := lineSplit[1]

		switch key {
		case etcdCertKey:
			etcdEnvVars.cert = value
		case etcdKeyKey:
			etcdEnvVars.key = value
		case etcdCACertKey:
			etcdEnvVars.CAcert = value
		case etcdEndpointsKey:
			// http://host:2379,http://host2:2379
			etcdEnvVars.endpoints = strings.Split(value, ",")
		}

		re := regexp.MustCompile(etcdURLHostKeyRegex)
		if re.Match([]byte(key)) {
			etcdEnvVars.selectedEndpoint = "https://" + value + ":2379"
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
	case etcdEnvVars.selectedEndpoint == "":
		return nil, fmt.Errorf("current host endpoint not found in env var file")
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

	err := snapshot.Save(ctx, b.logger, etcdV3Config, b.snapshotFileBackup)
	if err != nil {
		return err
	}

	return b.createSHA256HashFileFromFile(b.snapshotFileBackup)
}

func (b *backuper) backupStaticResources() error {
	pods := []string{"kube-apiserver", "kube-controller-manager", "kube-scheduler", "etcd"}
	rscDirs := make([]string, 0, len(pods))

	for _, pod := range pods {
		dir, err := getPodResourceDir(pod)
		if err != nil {
			return err
		}
		rscDirs = append(rscDirs, dir)

		b.logger.Info("back up kube static resources",
			zap.String("source", dir),
			zap.String("archive", b.staticResourcesBackup))
	}

	if err := b.tarDir(b.staticResourcesBackup, hostConfigDir, rscDirs...); err != nil {
		return err
	}

	return b.createSHA256HashFileFromFile(b.staticResourcesBackup)
}

func getPodResourceDir(pod string) (string, error) {
	type podSpec struct {
		Spec struct {
			Volumes []struct {
				Name     string `yaml:"name"`
				HostPath struct {
					Path string `yaml:"path"`
				} `yaml:"hostPath"`
			} `yaml:"volumes"`
		} `yaml:"spec"`
	}

	spec := podSpec{}
	staticPodPath := filepath.Join(manifestsDir, (pod + "-pod.yaml"))

	file, err := ioutil.ReadFile(filepath.Clean(staticPodPath))
	if err != nil {
		return "", err
	}

	if err := yaml.Unmarshal(file, &spec); err != nil {
		return "", err
	}

	for _, vol := range spec.Spec.Volumes {
		if vol.Name == "resource-dir" {
			return vol.HostPath.Path, nil
		}
	}

	return "", fmt.Errorf("pod resource directory not found: %s", pod)
}

func (b *backuper) createBackupArchive() error {
	b.logger.Info("create backup archive to push to s3",
		zap.String("source", b.tmpDir),
		zap.String("archive", b.backupFile))

	return b.tarDir(b.backupFile, "", b.tmpDir)
}

func (b *backuper) backupComponents(ctx context.Context, etcdEnvFile, etcdDialTimeout string) error {
	b.logger.Info("backup local cluster components",
		zap.String("status", "running"))

	// etcd
	etcdEnvVars, err := readEtcdEnvVariableFromFile(b.logger, os.Hostname, etcdEnvFile)
	if err != nil {
		return err
	}

	// The path to the certificate and keys does not always
	// exist at the host level, in this case we need a symlink.
	// actual:   /etc/kubernetes/static-pod-resources/etcd-certs/secrets/etcd-all-peer/master-1.crt
	// expected: /etc/kubernetes/static-pod-certs/secrets/etcd-all-peer/master-1.crt
	if _, err := os.Stat(etcdEnvVars.cert); errors.Is(err, os.ErrNotExist) {
		err := os.Symlink(
			filepath.Join(hostConfigDir, "static-pod-resources/etcd-certs"),
			filepath.Join(hostConfigDir, "static-pod-certs"))
		if err != nil {
			return errors.Wrapf(err, "creating symlink for etcd cert failed")
		}
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

	b.logger.Info("backup local cluster components",
		zap.String("status", "success"))

	return nil
}
