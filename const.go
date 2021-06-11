package main

const (
	defaultName           string = "openshift-cluster-backup"
	hostConfigDir         string = "/etc/kubernetes"
	staticResources       string = "/etc/kubernetes/manifests"
	snapshotPrefix        string = "snapshot"
	staticResourcesPrefix string = "static_kuberesources"
)

const (
	defaultEtcdEnvFile       string = "/etc/kubernetes/static-pod-resources/etcd-certs/configmaps/etcd-scripts/etcd.env"
	defaultEtcdDialTimeout   string = "10s"
	defaultEtcdBackupTimeout string = "60s"
	defaultKeepLocalBackup   bool   = false

	etcdEndpointsKey string = "ETCDCTL_ENDPOINTS"
	etcdCACertKey    string = "ETCDCTL_CACERT"
	etcdCertKey      string = "ETCDCTL_CERT"
	etcdKeyKey       string = "ETCDCTL_KEY"
)

const (
	accessKeyIDEnvKey     string = "AWS_ACCESS_KEY_ID"
	secretAccessKeyEnvKey string = "AWS_SECRET_ACCESS_KEY"
	regionEnvKey          string = "AWS_REGION"
	bucketEnvKey          string = "AWS_BUCKET"
)
