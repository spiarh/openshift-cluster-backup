package main

import (
	"reflect"
	"testing"
)

func TestReadEtcdEnvVariableFromFile(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected *etcdEnvVars
	}{
		{
			name: "valid etcd env var file",
			path: "./testdata/etcd.env",
			expected: &etcdEnvVars{
				endpoints:        []string{"https://192.168.126.11:2379", "https://192.168.126.10:2379", "https://192.168.126.9:2379"},
				selectedEndpoint: "https://192.168.126.11:2379",
				CAcert:           "/etc/kubernetes/static-pod-certs/configmaps/etcd-serving-ca/ca-bundle.crt",
				cert:             "/etc/kubernetes/static-pod-certs/secrets/etcd-all-peer/etcd-peer-crc-rsppg-master-0.crt",
				key:              "/etc/kubernetes/static-pod-certs/secrets/etcd-all-peer/etcd-peer-crc-rsppg-master-0.key",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			logger, err := createLogger("./testdata/kubeconfig")
			if err != nil {
				t.Error(err)
			}

			getHostnameF := func() (string, error) { return "crc_rsppg_master_0", nil }

			result, err := readEtcdEnvVariableFromFile(logger, getHostnameF, test.path)
			if err != nil {
				t.Error(err)
			}

			if !reflect.DeepEqual(test.expected, result) {
				t.Errorf("Expected %+v, got %+v", test.expected, result)
			}
		})
	}
}
