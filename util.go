package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
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
		return "", err
	}

	return dir, nil
}

func tarDir(srcDir, dstFile string) error {
	src, err := os.Open(srcDir)
	if err != nil {
		return err
	}
	defer src.Close()

	// get list of files
	files, err := src.Readdir(0)
	if err != nil {
		return err
	}

	// create tar file
	tarfile, err := os.Create(dstFile)
	if err != nil {
		return err
	}
	defer tarfile.Close()

	gw := gzip.NewWriter(tarfile)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()

	for _, fileInfo := range files {
		if fileInfo.IsDir() {
			continue
		}

		file, err := os.Open(src.Name() + string(filepath.Separator) + fileInfo.Name())
		if err != nil {
			return err
		}
		defer file.Close()

		// prepare the tar header
		header := new(tar.Header)
		header.Name = file.Name()
		header.Size = fileInfo.Size()
		header.Mode = int64(fileInfo.Mode())
		header.ModTime = fileInfo.ModTime()

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		if _, err = io.Copy(tw, file); err != nil {
			return err
		}
	}
	return nil
}

func createSHA256HashFileFromFile(fpath string) error {
	f, err := os.Open(fpath)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}

	hashFile := fpath + ".sha256"
	hashString := fmt.Sprintf("%x", h.Sum(nil))
	content := fmt.Sprintf("%s  %s", hashString, path.Base(fpath))

	return ioutil.WriteFile(hashFile, []byte(content), 0644)
}
