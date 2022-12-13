package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

func getEnvVar(name string) (string, error) {
	val := os.Getenv(name)
	if val == "" {
		return "", fmt.Errorf("missing environment variable: %s", name)
	}
	return val, nil
}

func createTempDir(pattern string) (string, error) {
	dir, err := ioutil.TempDir("", pattern)
	if err != nil {
		return "", errors.Wrapf(err, "temporary backup dir creation failed")
	}

	return dir, nil
}

func createID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	id := fmt.Sprintf("%x", b[0:4])
	return id, nil
}

func (b *backuper) createSHA256HashFileFromFile(fpath string) error {
	f, err := os.Open(filepath.Clean(fpath))
	if err != nil {
		return err
	}
	defer deferWithErrLog(
		b.logger, func() error { return f.Close() },
		"closing tar file failed")

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}

	hashFile := fpath + ".sha256"
	hashString := fmt.Sprintf("%x", h.Sum(nil))
	content := fmt.Sprintf("%s  %s", hashString, path.Base(fpath))

	return ioutil.WriteFile(hashFile, []byte(content), 0600)
}

func getAPIHostname(kubeconfig string) (string, error) {
	k := struct {
		Clusters []struct {
			Cluster struct {
				Server string `yaml:"server"`
			} `yaml:"cluster"`
			Name string `yaml:"name"`
		} `yaml:"clusters"`
	}{}

	file, err := ioutil.ReadFile(filepath.Clean(kubeconfig))
	if err != nil {
		return "", err
	}

	if err := yaml.Unmarshal(file, &k); err != nil {
		return "", err
	}

	if len(k.Clusters) == 0 {
		return "", fmt.Errorf("external kubernetes api fqdn not found")
	}

	fqdn := k.Clusters[0].Cluster.Server
	fqdn = strings.TrimPrefix(fqdn, "https://")
	fqdn = strings.TrimSuffix(fqdn, ":6443")

	return fqdn, nil
}

func (b *backuper) tarDir(dst, prefix string, dir ...string) error {
	tarfile, err := os.Create(filepath.Clean(dst))
	if err != nil {
		return err
	}
	defer deferWithErrLog(
		b.logger, func() error { return tarfile.Close() },
		"closing tar file failed")

	gw := gzip.NewWriter(tarfile)
	defer deferWithErrLog(
		b.logger, func() error { return gw.Close() },
		"closing gzip writer failed")

	tw := tar.NewWriter(gw)
	defer tw.Close()
	defer deferWithErrLog(
		b.logger, func() error { return tw.Close() },
		"closing tar writer failed")

	for _, d := range dir {
		if err := addToTar(d, prefix, tw); err != nil {
			return err
		}
	}

	return nil
}

func deferWithErrLog(logger *zap.Logger, f func() error, msg string) {
	if err := f(); err != nil {
		logger.Error(msg, zap.Error(err))
	}
}

func addToTar(dir, prefix string, tw *tar.Writer) error {
	if prefix == "" {
		// remove prefix to ensure compatibility with cluster-restore.sh
		prefix = path.Dir(dir)
	}

	fi, err := os.Stat(dir)
	if err != nil {
		return err
	}

	mode := fi.Mode()
	switch {
	case mode.IsRegular():
		// get header
		header, err := tar.FileInfoHeader(fi, dir)
		if err != nil {
			return err
		}
		// write header
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		// get content
		data, err := os.Open(filepath.Clean(dir))
		if err != nil {
			return err
		}
		if _, err := io.Copy(tw, data); err != nil {
			return err
		}
	case mode.IsDir():
		// walk through every file in the folder
		return filepath.Walk(dir, func(file string, fi os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			// generate tar header
			header, err := tar.FileInfoHeader(fi, file)
			if err != nil {
				return err
			}

			header.Name = strings.TrimPrefix(filepath.ToSlash(file), prefix+"/")

			// write header
			if err := tw.WriteHeader(header); err != nil {
				return err
			}
			// if not a dir, write file content
			if !fi.IsDir() {
				data, err := os.Open(filepath.Clean(file))
				if err != nil {
					return err
				}
				if _, err := io.Copy(tw, data); err != nil {
					return err
				}
			}
			return nil
		})
	default:
		return fmt.Errorf("file type not supported")
	}

	return err
}
