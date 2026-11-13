/*
Copyright © contributors to CloudNativePG, established as
CloudNativePG a Series of LF Projects, LLC.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

SPDX-License-Identifier: Apache-2.0
*/

// Package configuration contains the configuration of the operator, reading
// if from environment variables and from the ConfigMap
package configuration

import (
	"fmt"
	"os"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/cloudnative-pg/machinery/pkg/log"

	"github.com/cloudnative-pg/cloudnative-pg/pkg/configparser"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/filteredcache"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/metadatarules"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/monitoringdiscovery"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/versions"
)

var configurationLog = log.WithName("configuration")

const (
	// DefaultOperatorPullSecretName is implicitly copied into newly created clusters.
	DefaultOperatorPullSecretName = "cnpg-pull-secret" // #nosec

	// CertificateDuration is the default value for the lifetime of the generated certificates
	CertificateDuration = 90

	// ExpiringCheckThreshold is the default threshold to consider a certificate as expiring
	ExpiringCheckThreshold = 7

	// DefaultKubernetesClusterDomain is the default value used as
	// Kubernetes cluster domain.
	DefaultKubernetesClusterDomain = "cluster.local"

	// DefaultMutatingWebhookConfigurationName is the default name of the mutating webhook configuration
	DefaultMutatingWebhookConfigurationName = "cnpg-mutating-webhook-configuration"

	// DefaultValidatingWebhookConfigurationName is the default name of the validating webhook configuration
	DefaultValidatingWebhookConfigurationName = "cnpg-validating-webhook-configuration"

	// DefaultCaSecretName is the default name of the Secret holding the operator CA
	DefaultCaSecretName = "cnpg-ca-secret" // #nosec

	// DefaultWebhookSecretName is the default name of the Secret holding the webhook serving certificate
	DefaultWebhookSecretName = "cnpg-webhook-cert" // #nosec

	// DefaultWebhookServiceName is the default name of the Service exposing the webhook server
	DefaultWebhookServiceName = "cnpg-webhook-service"

	// DefaultLeaderElectionID is the default name of the Lease used for the leader election
	DefaultLeaderElectionID = "db9c8771.cnpg.io"

	// OperatorStatusIdentityOff writes nothing about the operator in the Cluster status
	OperatorStatusIdentityOff = "off"
	// OperatorStatusIdentityVersion writes the operator version in the Cluster status
	OperatorStatusIdentityVersion = "version"
	// OperatorStatusIdentityName writes the operator version and Deployment name in the Cluster status
	OperatorStatusIdentityName = "name"
	// OperatorStatusIdentityFull writes the operator version, Deployment name and namespace in the Cluster status
	OperatorStatusIdentityFull = "full"

	// leaderElectionIDSuffix is appended to the Deployment name to derive the Lease name
	leaderElectionIDSuffix = ".cnpg.io"
)

// Values of NodeStatusSource
const (
	// NodeStatusSourceAPI reads the cluster-scoped Node API: a Node watch and
	// a list at every reconciliation
	NodeStatusSourceAPI = "api"

	// NodeStatusSourcePods reads the node state that a node bridge echoes
	// onto the instance Pods, under the keys named by NodeStatusPodKeyPrefix
	// and NodeStatusLabelPrefix
	NodeStatusSourcePods = "pods"

	// NodeStatusSourceNone reads no node state at all
	NodeStatusSourceNone = "none"
)

// Values of NodeStatusUnavailablePolicy
const (
	// NodeStatusUnavailablePolicyKeepPDB keeps the PodDisruptionBudgets and
	// reports the degraded mode: a drain is held by the primary
	// PodDisruptionBudget until the primary is moved by hand
	NodeStatusUnavailablePolicyKeepPDB = "keepPDB"

	// NodeStatusUnavailablePolicyDisablePrimaryPDB removes the primary
	// PodDisruptionBudget: a drain evicts the primary, and a failover follows
	NodeStatusUnavailablePolicyDisablePrimaryPDB = "disablePrimaryPDB"

	// NodeStatusUnavailablePolicyFail refuses to start the operator
	NodeStatusUnavailablePolicyFail = "fail"
)

// DefaultNodeStatusEvictionMaxAge is the default age, in seconds, after which
// an eviction request echoed onto a primary Pod is ignored
const DefaultNodeStatusEvictionMaxAge = 600

// DefaultDrainTaints is the default list of taints the operator will watch and treat
// as Unschedule
var DefaultDrainTaints = []string{
	// Kubernetes well-known unschedulable taint
	// See: https://kubernetes.io/docs/reference/labels-annotations-taints/#node-kubernetes-io-unschedulable
	"node.kubernetes.io/unschedulable",

	// Used by the Kubernetes Cluster Autoscaler
	//nolint: lll
	// See: https://github.com/kubernetes/autoscaler/blob/aa1d413ea3bf319b56c7b2e65ade1a028e149439/cluster-autoscaler/cloudprovider/oci/nodepools/consts/annotations.go#L27
	"ToBeDeletedByClusterAutoscaler",

	// Used by Karpenter termination controller
	// See: https://karpenter.sh/docs/concepts/disruption/#termination-controller
	"karpenter.sh/disrupted",

	// Used by Karpenter disruption controller
	// See: https://karpenter.sh/v0.32/concepts/disruption/#disruption-controller
	"karpenter.sh/disruption",
}

// DefaultPluginSocketDir is the default directory where the plugin sockets are located.
const DefaultPluginSocketDir = "/plugins"

// Data is the struct containing the configuration of the operator.
// Usually the operator code will use the "Current" configuration.
type Data struct {
	// WebhookCertDir is the directory where the certificates for the webhooks
	// need to written. This is different between plain Kubernetes and OpenShift
	WebhookCertDir string `json:"webhookCertDir" env:"WEBHOOK_CERT_DIR"`

	// MetricsCertDir is the directory where the certificates for the metrics
	// server are stored. If set, the metrics server will use TLS.
	MetricsCertDir string `json:"metricsCertDir" env:"METRICS_CERT_DIR"`

	// PluginSocketDir is the directory where the plugins sockets are to be
	// found
	PluginSocketDir string `json:"pluginSocketDir" env:"PLUGIN_SOCKET_DIR"`

	// WatchNamespace is the namespace where the operator should watch and
	// is configurable via environment variables in the OpenShift console.
	// Multiple namespaces can be specified separated by comma
	WatchNamespace string `json:"watchNamespace" env:"WATCH_NAMESPACE"`

	// OperatorNamespace is the namespace where the operator is installed
	OperatorNamespace string `json:"operatorNamespace" env:"OPERATOR_NAMESPACE"`

	// OperatorDeploymentName is the name of the Deployment running the operator.
	// When set, the operator fetches its Deployment by this name instead of
	// searching it by label, and names the objects it creates for itself after
	// it (see GetCaSecretName and the other getters). When unset, the lookup
	// is by label and every name is the historical literal. It must be a Pod
	// environment variable: the Lease name is needed before the configuration
	// objects are read.
	OperatorDeploymentName string `json:"operatorDeploymentName" env:"OPERATOR_DEPLOYMENT_NAME"`

	// CaSecretName is the name of the Secret holding the operator CA, used as
	// given. When unset, derived from OperatorDeploymentName, or the default.
	CaSecretName string `json:"caSecretName" env:"CA_SECRET_NAME"`

	// WebhookServiceName is the name of the Service exposing the webhook
	// server, the subject of the serving certificate the operator issues,
	// used as given. When unset, derived from OperatorDeploymentName, or the
	// default.
	WebhookServiceName string `json:"webhookServiceName" env:"WEBHOOK_SERVICE_NAME"`

	// LeaderElectionID is the name of the Lease used for the leader election,
	// used as given. When unset, derived from OperatorDeploymentName, or the
	// default. It must be a Pod environment variable.
	LeaderElectionID string `json:"leaderElectionID" env:"LEADER_ELECTION_ID"`

	// OperatorStatusIdentity is how much the operator says about itself in
	// the status of the Clusters it reconciles: "off" nothing, "version" its
	// version, "name" its version and Deployment name, "full" its version,
	// Deployment name and namespace. The Deployment name, when reported, is
	// also the field manager of every write of the operator. It must be a Pod
	// environment variable: the field manager is fixed before the
	// configuration objects are read.
	OperatorStatusIdentity string `json:"operatorStatusIdentity" env:"OPERATOR_STATUS_IDENTITY"`

	// OperatorPullSecretName is the pull secret used to download the
	// pull secret name
	OperatorPullSecretName string `json:"operatorPullSecretName" env:"PULL_SECRET_NAME"`

	// OperatorImageName is the name of the image of the operator, that is
	// used to bootstrap Pods
	OperatorImageName string `json:"operatorImageName" env:"OPERATOR_IMAGE_NAME"`

	// PostgresImageName is the name of the image of PostgreSQL that is
	// used by default for new clusters
	PostgresImageName string `json:"postgresImageName" env:"POSTGRES_IMAGE_NAME"`

	// PgbouncerImageName is the name of the image of PgBouncer that is
	// used by default for new poolers
	PgbouncerImageName string `json:"pgbouncerImageName" env:"PGBOUNCER_IMAGE_NAME"`

	// DefaultStorageClass is the name of the StorageClass written, when a
	// Cluster is created, into every storage section that names none.
	// Empty (the default): the PVCs get the default StorageClass of the
	// Kubernetes cluster.
	DefaultStorageClass string `json:"defaultStorageClass" env:"DEFAULT_STORAGE_CLASS"`

	// InheritedAnnotations is a list of annotations that every resource could inherit from
	// the owning Cluster
	InheritedAnnotations []string `json:"inheritedAnnotations" env:"INHERITED_ANNOTATIONS"`

	// InheritedLabels is a list of labels that every resource could inherit from
	// the owning Cluster
	InheritedLabels []string `json:"inheritedLabels" env:"INHERITED_LABELS"`

	// DefaultMetadata is a YAML list of rules putting labels and annotations
	// on the objects of every Cluster, Pooler, ScheduledBackup and Backup, or
	// of the kinds of resources a rule names, when the resource does not set
	// the key
	DefaultMetadata string `json:"defaultMetadata" env:"DEFAULT_METADATA"`

	// MandatoryMetadata is a YAML list of rules putting labels and
	// annotations on the objects of every Cluster, Pooler, ScheduledBackup
	// and Backup, or of the kinds of resources a rule names, replacing the
	// value set by the resource
	MandatoryMetadata string `json:"mandatoryMetadata" env:"MANDATORY_METADATA"`

	// defaultMetadata and mandatoryMetadata are the compiled forms of the
	// two settings above
	defaultMetadata   metadatarules.Sets
	mandatoryMetadata metadatarules.Sets

	// MonitoringDiscoveryAnnotations is the list of the families of
	// annotations telling monitoring agents how to collect the metrics and
	// the logs of the instance and Pooler Pods: `prometheus` and
	// `opentelemetry`. Empty by default
	MonitoringDiscoveryAnnotations []string `json:"monitoringDiscoveryAnnotations" env:"MONITORING_DISCOVERY_ANNOTATIONS"`

	// monitoringDiscoveryFamilies is the parsed form of
	// MonitoringDiscoveryAnnotations
	monitoringDiscoveryFamilies []monitoringdiscovery.Family

	// MonitoringQueriesConfigmap is the name of the configmap in the operator namespace which contain
	// the monitoring queries. The queries will be read from the data key: "queries".
	MonitoringQueriesConfigmap string `json:"monitoringQueriesConfigmap" env:"MONITORING_QUERIES_CONFIGMAP"`

	// MonitoringQueriesSecret is the name of the secret in the operator namespace which contain
	// the monitoring queries. The queries will be read from the data key: "queries".
	MonitoringQueriesSecret string `json:"monitoringQueriesSecret" env:"MONITORING_QUERIES_SECRET"`

	// EnableInstanceManagerInplaceUpdates enables the instance manager to apply in-place updates,
	// replacing the executable in a pod without restarting
	EnableInstanceManagerInplaceUpdates bool `json:"enableInstanceManagerInplaceUpdates" env:"ENABLE_INSTANCE_MANAGER_INPLACE_UPDATES"` //nolint

	// This is the lifetime of the generated certificates
	CertificateDuration int `json:"certificateDuration" env:"CERTIFICATE_DURATION"`

	// Threshold to consider a certificate as expiring
	ExpiringCheckThreshold int `json:"expiringCheckThreshold" env:"EXPIRING_CHECK_THRESHOLD"`

	// CreateAnyService is true when the user wants the operator to create
	// the <cluster-name>-any service. Defaults to false.
	CreateAnyService bool `json:"createAnyService" env:"CREATE_ANY_SERVICE"`

	// The duration (in seconds) to wait between the roll-outs of different
	// clusters during an operator upgrade. This setting controls the
	// timing of upgrades across clusters, spreading them out to reduce
	// system impact. The default value is 0, which means no delay between
	// PostgreSQL cluster upgrades.
	ClustersRolloutDelay int `json:"clustersRolloutDelay" env:"CLUSTERS_ROLLOUT_DELAY"`

	// The duration (in seconds) to wait between roll-outs of individual
	// PostgreSQL instances within the same cluster during an operator
	// upgrade. The default value is 0, meaning no delay between upgrades
	// of instances in the same PostgreSQL cluster.
	InstancesRolloutDelay int `json:"instancesRolloutDelay" env:"INSTANCES_ROLLOUT_DELAY"`

	// IncludePlugins is a comma-separated list of plugins to always be
	// included in the Cluster reconciliation
	IncludePlugins string `json:"includePlugins" env:"INCLUDE_PLUGINS"`

	// StandbyTCPUserTimeout configuration parameter allows you to
	// specify a custom TCP user timeout for the standby PostgreSQL
	// server's connection to the primary server. This timeout is
	// added as a tcp_user_timeout option to the primary_conninfo
	// string, which is used by the standby server to connect to the
	// primary server in CloudNativePG.
	// When nil, the instance manager will use a default value of 5000ms.
	// Set to 0 explicitly to use the system's default.
	StandbyTCPUserTimeout *int `json:"standbyTcpUserTimeout" env:"STANDBY_TCP_USER_TIMEOUT"`

	// KubernetesClusterDomain defines the domain suffix for service FQDNs
	// within the Kubernetes cluster. If left unset, it defaults to `cluster.local`.
	KubernetesClusterDomain string `json:"kubernetesClusterDomain" env:"KUBERNETES_CLUSTER_DOMAIN"`

	// FilteredCacheMode restricts the operator cache to the objects it
	// manages: "off" (the default) caches every object of the watched kinds,
	// "strict" caches only the objects carrying cnpg.io/reconcile=true and
	// reads the others from the API server when needed, "reload" is
	// "strict" plus a watch on the Secrets and ConfigMaps carrying
	// cnpg.io/reload=true
	FilteredCacheMode string `json:"filteredCacheMode" env:"FILTERED_CACHE_MODE"`

	// EnableWebhookNamespaceSuffix when true appends "-<OperatorNamespace>" to the default
	// webhook configuration names, allowing multiple namespaced operator instances to coexist
	// on the same cluster. The operator will fail to start if this is enabled without OperatorNamespace.
	EnableWebhookNamespaceSuffix bool `json:"enableWebhookNamespaceSuffix" env:"ENABLE_WEBHOOK_NAMESPACE_SUFFIX"`

	// MutatingWebhookConfigurationName is the name of the MutatingWebhookConfiguration
	// the operator injects its CA bundle into, used as given when set. When empty, the
	// default name applies, with the namespace suffix when EnableWebhookNamespaceSuffix is true.
	MutatingWebhookConfigurationName string `json:"mutatingWebhookConfigurationName" env:"MUTATING_WEBHOOK_CONFIGURATION_NAME"` //nolint:lll

	// ValidatingWebhookConfigurationName is the name of the ValidatingWebhookConfiguration
	// the operator injects its CA bundle into, used as given when set. When empty, the
	// default name applies, with the namespace suffix when EnableWebhookNamespaceSuffix is true.
	ValidatingWebhookConfigurationName string `json:"validatingWebhookConfigurationName" env:"VALIDATING_WEBHOOK_CONFIGURATION_NAME"` //nolint:lll

	// DrainTaints is a list of taints the operator will watch and treat as Unschedule
	DrainTaints []string `json:"drainTaints" env:"DRAIN_TAINTS"`

	// NodeStatusSource says where the operator learns the state of the nodes.
	// Unset, it is resolved from APINodeEnabled at startup, see
	// ResolveNodeStatusSource
	// (cordon, drain taints, node labels): "api" reads the cluster-scoped
	// Node API, "pods" reads the keys a node bridge echoes onto the instance
	// Pods, "none" reads nothing
	NodeStatusSource string `json:"nodeStatusSource" env:"NODE_STATUS_SOURCE"`

	// nodeAPIUnavailable tells that NodeStatusSource was left unset and
	// resolved to "none" because the Node API was disabled or not accessible
	// at startup, see ResolveNodeStatusSource
	nodeAPIUnavailable bool

	// NodeStatusPodKeyPrefix is the prefix of the keys read on the instance
	// Pods when NodeStatusSource is "pods": <prefix>unschedulable,
	// <prefix>taints and <prefix>eviction-requested. Required with "pods"
	NodeStatusPodKeyPrefix string `json:"nodeStatusPodKeyPrefix" env:"NODE_STATUS_POD_KEY_PREFIX"`

	// NodeStatusLabelPrefix is the prefix under which the node labels are
	// copied onto the instance Pods when NodeStatusSource is "pods": the
	// node label <key> is read from the Pod label <prefix><key>
	NodeStatusLabelPrefix string `json:"nodeStatusLabelPrefix" env:"NODE_STATUS_LABEL_PREFIX"`

	// NodeStatusEvictionMaxAge is the age, in seconds, after which an
	// eviction request echoed onto an instance Pod is ignored: on the
	// primary it no longer triggers a switchover, and the instance can again
	// be chosen as the new primary. Zero means no limit
	NodeStatusEvictionMaxAge int `json:"nodeStatusEvictionMaxAge" env:"NODE_STATUS_EVICTION_MAX_AGE"`

	// NodeStatusUnavailablePolicy says what the operator does when it reads
	// no node state, that is when NodeStatusSource resolves to "none":
	// "keepPDB" (the default) runs in degraded mode with the
	// PodDisruptionBudgets kept, "disablePrimaryPDB" runs in degraded mode
	// without the primary PodDisruptionBudget, "fail" refuses to start
	NodeStatusUnavailablePolicy string `json:"nodeStatusUnavailablePolicy" env:"NODE_STATUS_UNAVAILABLE_POLICY"`

	// ManageWebhookConfigurations is true when the operator should inject the
	// CA bundle into the mutating and validating webhook configurations. It can
	// be disabled when the webhook configurations are managed externally (e.g.
	// by cert-manager's CA injector or by GitOps).
	ManageWebhookConfigurations bool `json:"manageWebhookConfigurations" env:"MANAGE_WEBHOOK_CONFIGURATIONS"`

	// ScheduledBackupCronFormat tells how the schedule of a ScheduledBackup
	// is read when it has five fields, the standard cron format, instead of
	// the six fields with seconds of robfig/cron: "seconds" (the default)
	// reads it with the seconds field first and warns, "seconds-strict"
	// rejects it when the ScheduledBackup is created, "standard" reads it
	// as standard cron, with the minute field first.
	ScheduledBackupCronFormat string `json:"scheduledBackupCronFormat" env:"SCHEDULED_BACKUP_CRON_FORMAT"`

	// APIPodMonitorEnabled is true when the operator uses the PodMonitor API
	// (monitoring.coreos.com/v1). It is false when disabled by the
	// administrator, or when the API is not installed or the operator is not
	// allowed to list it where it would watch it.
	APIPodMonitorEnabled bool `json:"apiPodMonitorEnabled" env:"API_POD_MONITOR_ENABLED"`

	// APIVolumeSnapshotEnabled is true when the operator uses the
	// VolumeSnapshot API (snapshot.storage.k8s.io/v1), with the same rules as
	// APIPodMonitorEnabled
	APIVolumeSnapshotEnabled bool `json:"apiVolumeSnapshotEnabled" env:"API_VOLUME_SNAPSHOT_ENABLED"`

	// APIClusterImageCatalogEnabled is true when the operator uses the
	// cluster-scoped ClusterImageCatalog API, with the same rules as
	// APIPodMonitorEnabled
	APIClusterImageCatalogEnabled bool `json:"apiClusterImageCatalogEnabled" env:"API_CLUSTER_IMAGE_CATALOG_ENABLED"`

	// APINodeEnabled is true when the operator reads the cluster-scoped Node
	// API, to detect drained nodes and to resolve node labels in the
	// instance topology, with the same rules as APIPodMonitorEnabled
	APINodeEnabled bool `json:"apiNodeEnabled" env:"API_NODE_ENABLED"`

	// PropagateProxyEnv is true when the proxy settings of the operator
	// (HTTP_PROXY, HTTPS_PROXY and NO_PROXY, read from its environment) are
	// set on the workloads it creates, in both the upper and the lower case
	// spelling, unless the resource defines any of them itself. Enabled by
	// default, as the OLM bundle declares the operator proxy-aware.
	PropagateProxyEnv bool `json:"propagateProxyEnv" env:"PROPAGATE_PROXY_ENV"`
}

// Current is the configuration used by the operator
var Current = NewConfiguration()

// ServiceAccountNamespaceFile is where Kubernetes mounts the namespace of a
// Pod, next to its service account token
const ServiceAccountNamespaceFile = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"

// OperatorNamespaceSource tells where the operator namespace comes from
type OperatorNamespaceSource string

const (
	// OperatorNamespaceFromEnvironment means OPERATOR_NAMESPACE gave the
	// operator namespace
	OperatorNamespaceFromEnvironment OperatorNamespaceSource = "OPERATOR_NAMESPACE"

	// OperatorNamespaceFromServiceAccount means the namespace of the service
	// account of the operator Pod gave the operator namespace
	OperatorNamespaceFromServiceAccount OperatorNamespaceSource = ServiceAccountNamespaceFile

	// OperatorNamespaceUnknown means nothing gave the operator namespace, as
	// when the operator runs outside a Pod
	OperatorNamespaceUnknown OperatorNamespaceSource = ""
)

// DiscoverOperatorNamespace sets the operator namespace from the namespace
// file of the service account, when OPERATOR_NAMESPACE did not set it, and
// tells where the namespace comes from
func (config *Data) DiscoverOperatorNamespace(namespaceFile string) OperatorNamespaceSource {
	if config.OperatorNamespace != "" {
		return OperatorNamespaceFromEnvironment
	}

	content, err := os.ReadFile(namespaceFile) // #nosec G304 -- the path is a constant outside tests
	if err != nil {
		return OperatorNamespaceUnknown
	}
	namespace := strings.TrimSpace(string(content))
	if namespace == "" {
		return OperatorNamespaceUnknown
	}

	config.OperatorNamespace = namespace
	return OperatorNamespaceFromServiceAccount
}

// newDefaultConfig creates a configuration holding the defaults
func newDefaultConfig() *Data {
	return &Data{
		OperatorPullSecretName:        DefaultOperatorPullSecretName,
		OperatorImageName:             versions.DefaultOperatorImageName,
		PostgresImageName:             versions.DefaultImageName,
		PgbouncerImageName:            versions.DefaultPgbouncerImage,
		PluginSocketDir:               DefaultPluginSocketDir,
		CreateAnyService:              false,
		CertificateDuration:           CertificateDuration,
		ExpiringCheckThreshold:        ExpiringCheckThreshold,
		StandbyTCPUserTimeout:         nil,
		KubernetesClusterDomain:       DefaultKubernetesClusterDomain,
		FilteredCacheMode:             string(filteredcache.ModeOff),
		DrainTaints:                   DefaultDrainTaints,
		ManageWebhookConfigurations:   true,
		APIPodMonitorEnabled:          true,
		APIVolumeSnapshotEnabled:      true,
		APIClusterImageCatalogEnabled: true,
		APINodeEnabled:                true,
		ScheduledBackupCronFormat:     string(utils.CronFormatSeconds),
		PropagateProxyEnv:             true,
		OperatorStatusIdentity:        OperatorStatusIdentityVersion,
		NodeStatusEvictionMaxAge:      DefaultNodeStatusEvictionMaxAge,
		NodeStatusUnavailablePolicy:   NodeStatusUnavailablePolicyKeepPDB,
	}
}

// NewConfiguration create a new CNPG configuration by reading
// the environment variables
func NewConfiguration() *Data {
	configuration := newDefaultConfig()
	configuration.ReadConfigMap(nil)
	return configuration
}

// NewFromData creates a configuration from the defaults and the passed in
// data map only, without reading the environment of the current process: the
// way the kubectl plugin reads the environment of the operator container
func NewFromData(data map[string]string) *Data {
	configuration := newDefaultConfig()
	configparser.ReadData(configuration, newDefaultConfig(), data)
	return configuration
}

// ReadConfigMap reads the configuration from the environment and the passed in data map
func (config *Data) ReadConfigMap(data map[string]string) {
	configparser.ReadConfigMap(config, newDefaultConfig(), data)
}

// IsAnnotationInherited checks if an annotation with a certain name should
// be inherited from the Cluster specification to the generated objects
func (config *Data) IsAnnotationInherited(name string) bool {
	return evaluateGlobPatterns(config.InheritedAnnotations, name)
}

// IsLabelInherited checks if a label with a certain name should
// be inherited from the Cluster specification to the generated objects
func (config *Data) IsLabelInherited(name string) bool {
	return evaluateGlobPatterns(config.InheritedLabels, name)
}

// GetClustersRolloutDelay gets the delay between roll-outs of different clusters
func (config *Data) GetClustersRolloutDelay() time.Duration {
	return time.Duration(config.ClustersRolloutDelay) * time.Second
}

// GetInstancesRolloutDelay gets the delay between roll-outs of pods belonging
// to the same cluster
func (config *Data) GetInstancesRolloutDelay() time.Duration {
	return time.Duration(config.InstancesRolloutDelay) * time.Second
}

// GetScheduledBackupCronFormat gets how the schedule of a ScheduledBackup is read
func (config *Data) GetScheduledBackupCronFormat() utils.CronFormat {
	return utils.CronFormat(config.ScheduledBackupCronFormat)
}

// ReadsNodeAPI reports whether the operator watches and lists the Nodes
func (config *Data) ReadsNodeAPI() bool {
	return config.NodeStatusSource == NodeStatusSourceAPI
}

// ReadsNodeStatusFromPods reports whether the operator reads the node state
// echoed onto the instance Pods
func (config *Data) ReadsNodeStatusFromPods() bool {
	return config.NodeStatusSource == NodeStatusSourcePods
}

// GetNodeStatusEvictionMaxAge returns the age after which an echoed eviction
// request is ignored, zero for no limit
func (config *Data) GetNodeStatusEvictionMaxAge() time.Duration {
	return time.Duration(config.NodeStatusEvictionMaxAge) * time.Second
}

// ResolveNodeStatusSource settles NodeStatusSource once APINodeEnabled is
// known, after the probe of the Node API. Unset, the source follows the API:
// "api" when the operator reads it, "none" when it is disabled or not
// accessible. Set to "api" while the Node API is disabled, the source is
// refused: a stated source that cannot work. "pods" and "none" never depend
// on the Node API. "pods" needs NodeStatusPodKeyPrefix.
func (config *Data) ResolveNodeStatusSource() error {
	switch config.NodeStatusSource {
	case "":
		config.NodeStatusSource = NodeStatusSourceNone
		config.nodeAPIUnavailable = !config.APINodeEnabled
		if config.APINodeEnabled {
			config.NodeStatusSource = NodeStatusSourceAPI
		}
		return nil
	case NodeStatusSourceAPI:
		if !config.APINodeEnabled {
			return fmt.Errorf("NODE_STATUS_SOURCE=%s requires the Node API, which is disabled or not "+
				"accessible (API_NODE_ENABLED): leave NODE_STATUS_SOURCE unset, or set it to %s or %s",
				NodeStatusSourceAPI, NodeStatusSourcePods, NodeStatusSourceNone)
		}
		return nil
	case NodeStatusSourceNone:
		return nil
	case NodeStatusSourcePods:
		if config.NodeStatusPodKeyPrefix == "" {
			return fmt.Errorf("NODE_STATUS_SOURCE=%s requires NODE_STATUS_POD_KEY_PREFIX, "+
				"the prefix of the keys echoed onto the instance Pods", NodeStatusSourcePods)
		}
		if config.NodeStatusEvictionMaxAge < 0 {
			return fmt.Errorf("NODE_STATUS_EVICTION_MAX_AGE must not be negative, got %d",
				config.NodeStatusEvictionMaxAge)
		}
		return nil
	default:
		return fmt.Errorf("unknown NODE_STATUS_SOURCE %q, expected %s, %s or %s",
			config.NodeStatusSource, NodeStatusSourceAPI, NodeStatusSourcePods, NodeStatusSourceNone)
	}
}

// GetNodeStatusUnavailablePolicy returns the policy applied when the
// operator reads no node state, keepPDB when unset
func (config *Data) GetNodeStatusUnavailablePolicy() string {
	if config.NodeStatusUnavailablePolicy == "" {
		return NodeStatusUnavailablePolicyKeepPDB
	}
	return config.NodeStatusUnavailablePolicy
}

// ValidateNodeStatusUnavailablePolicy refuses an unknown
// NODE_STATUS_UNAVAILABLE_POLICY when the configuration is loaded, before
// the optional APIs are probed
func (config *Data) ValidateNodeStatusUnavailablePolicy() error {
	switch config.GetNodeStatusUnavailablePolicy() {
	case NodeStatusUnavailablePolicyKeepPDB,
		NodeStatusUnavailablePolicyDisablePrimaryPDB,
		NodeStatusUnavailablePolicyFail:
		return nil
	default:
		return fmt.Errorf("unknown NODE_STATUS_UNAVAILABLE_POLICY %q, expected %s, %s or %s",
			config.NodeStatusUnavailablePolicy, NodeStatusUnavailablePolicyKeepPDB,
			NodeStatusUnavailablePolicyDisablePrimaryPDB, NodeStatusUnavailablePolicyFail)
	}
}

// ReadsNoNodeStatus reports whether the operator reads no node state: the
// resolved NodeStatusSource is "none", because it was set so or because the
// Node API was disabled or not accessible at startup
func (config *Data) ReadsNoNodeStatus() bool {
	return config.NodeStatusSource == NodeStatusSourceNone
}

// CheckNodeStatusUnavailablePolicy applies the fail policy once the source
// of node state is resolved: the operator refuses to start when it reads no
// node state because NODE_STATUS_SOURCE was left unset and the Node API was
// not accessible at startup. An explicit NODE_STATUS_SOURCE=none is the
// choice of the administrator and never refuses to start. Called after
// ResolveNodeStatusSource
func (config *Data) CheckNodeStatusUnavailablePolicy() error {
	if !config.nodeAPIUnavailable ||
		config.GetNodeStatusUnavailablePolicy() != NodeStatusUnavailablePolicyFail {
		return nil
	}

	return fmt.Errorf("NODE_STATUS_UNAVAILABLE_POLICY=%s: the operator would read no node state "+
		"(NODE_STATUS_SOURCE resolved to %s, API_NODE_ENABLED=%t after the startup probe); grant the "+
		"operator the right to list and watch the Nodes and restart it, set NODE_STATUS_SOURCE=%s "+
		"with a node bridge, or set NODE_STATUS_UNAVAILABLE_POLICY to %s or %s to run in degraded mode",
		NodeStatusUnavailablePolicyFail, NodeStatusSourceNone, config.APINodeEnabled, NodeStatusSourcePods,
		NodeStatusUnavailablePolicyKeepPDB, NodeStatusUnavailablePolicyDisablePrimaryPDB)
}

// NodeStatusUnavailableMessage says what the operator gives up when it reads
// no node state, for the operator log, the Cluster Events and the webhook
const NodeStatusUnavailableMessage = "The operator reads no node state (NODE_STATUS_SOURCE=none): " +
	"the primary of a node being drained is not switched over before the drain, and the node " +
	"labels of the instance topology are not resolved; set NODE_STATUS_SOURCE=pods with a node " +
	"bridge to recover them without the Node API"

// WatchedNamespaces get the list of additional watched namespaces.
// The result is a list of namespaces specified in the WATCHED_NAMESPACE where
// each namespace is separated by comma
func (config *Data) WatchedNamespaces() []string {
	return cleanNamespaceList(config.WatchNamespace)
}

// ownObjectName returns the name of an object the operator creates or uses
// for itself: the explicit name when set, "<OperatorDeploymentName>-<suffix>"
// when the Deployment name is known, the historical literal otherwise.
func (config *Data) ownObjectName(explicit, suffix, literal string) string {
	if explicit != "" {
		return explicit
	}
	if config.OperatorDeploymentName != "" {
		return config.OperatorDeploymentName + "-" + suffix
	}
	return literal
}

// GetCaSecretName returns the name of the Secret holding the operator CA
func (config *Data) GetCaSecretName() string {
	return config.ownObjectName(config.CaSecretName, "ca-secret", DefaultCaSecretName)
}

// GetWebhookSecretName returns the name of the Secret holding the webhook
// serving certificate: there is no explicit setting, the Secret mounted in
// the operator Pod takes precedence over this name when it can be discovered
func (config *Data) GetWebhookSecretName() string {
	return config.ownObjectName("", "webhook-cert", DefaultWebhookSecretName)
}

// GetWebhookServiceName returns the name of the Service exposing the webhook
// server, the subject of the serving certificate
func (config *Data) GetWebhookServiceName() string {
	return config.ownObjectName(config.WebhookServiceName, "webhook-service", DefaultWebhookServiceName)
}

// GetWebhookServiceNames returns the names the webhook Service may have, the
// subject of the serving certificate first: the configured or derived name,
// then the derived one and the historical literal, without duplicates
func (config *Data) GetWebhookServiceNames() []string {
	names := []string{config.GetWebhookServiceName()}
	for _, name := range []string{
		config.ownObjectName("", "webhook-service", DefaultWebhookServiceName),
		DefaultWebhookServiceName,
	} {
		if !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	return names
}

// GetLeaderElectionID returns the name of the Lease used for the leader
// election: the explicit name, "<OperatorDeploymentName>.cnpg.io", or the
// default
func (config *Data) GetLeaderElectionID() string {
	if config.LeaderElectionID != "" {
		return config.LeaderElectionID
	}
	if config.OperatorDeploymentName != "" {
		return config.OperatorDeploymentName + leaderElectionIDSuffix
	}
	return DefaultLeaderElectionID
}

// webhookConfigurationName resolves the name of a webhook configuration: the
// explicit name is used as given; otherwise the name derived from
// OperatorDeploymentName, or the default, with "-<OperatorNamespace>" appended

// operatorStatusIdentityLevels orders the OperatorStatusIdentity values from
// the least to the most detailed
var operatorStatusIdentityLevels = []string{
	OperatorStatusIdentityOff,
	OperatorStatusIdentityVersion,
	OperatorStatusIdentityName,
	OperatorStatusIdentityFull,
}

// ValidateOperatorStatusIdentity returns an error when OperatorStatusIdentity
// is not one of the known values; empty stands for the default
func (config *Data) ValidateOperatorStatusIdentity() error {
	if config.OperatorStatusIdentity == "" ||
		slices.Contains(operatorStatusIdentityLevels, config.OperatorStatusIdentity) {
		return nil
	}
	return fmt.Errorf("OPERATOR_STATUS_IDENTITY %q is not one of %s",
		config.OperatorStatusIdentity, strings.Join(operatorStatusIdentityLevels, ", "))
}

// reportsOperatorIdentity tells whether OperatorStatusIdentity reaches the
// passed level; empty stands for the default, an unknown value reports nothing
func (config *Data) reportsOperatorIdentity(level string) bool {
	mode := config.OperatorStatusIdentity
	if mode == "" {
		mode = OperatorStatusIdentityVersion
	}
	current := slices.Index(operatorStatusIdentityLevels, mode)
	return current >= slices.Index(operatorStatusIdentityLevels, level)
}

// ReportsOperatorVersion tells whether the operator version is written in
// the status of the reconciled Clusters
func (config *Data) ReportsOperatorVersion() bool {
	return config.reportsOperatorIdentity(OperatorStatusIdentityVersion)
}

// ReportsOperatorName tells whether the operator Deployment name is written
// in the status of the reconciled Clusters and used as the field manager
func (config *Data) ReportsOperatorName() bool {
	return config.reportsOperatorIdentity(OperatorStatusIdentityName)
}

// ReportsOperatorNamespace tells whether the operator namespace is written
// in the status of the reconciled Clusters
func (config *Data) ReportsOperatorNamespace() bool {
	return config.reportsOperatorIdentity(OperatorStatusIdentityFull)
}

// when EnableWebhookNamespaceSuffix is true and OperatorNamespace is set
func (config *Data) webhookConfigurationName(explicit, suffix, literal string) string {
	if explicit != "" {
		return explicit
	}
	name := config.ownObjectName("", suffix, literal)
	if config.EnableWebhookNamespaceSuffix && config.OperatorNamespace != "" {
		return name + "-" + config.OperatorNamespace
	}
	return name
}

// GetMutatingWebhookConfigurationName returns the name of the MutatingWebhookConfiguration
// the operator injects its CA bundle into
func (config *Data) GetMutatingWebhookConfigurationName() string {
	return config.webhookConfigurationName(
		config.MutatingWebhookConfigurationName,
		"mutating-webhook-configuration",
		DefaultMutatingWebhookConfigurationName)
}

// GetValidatingWebhookConfigurationName returns the name of the ValidatingWebhookConfiguration
// the operator injects its CA bundle into
func (config *Data) GetValidatingWebhookConfigurationName() string {
	return config.webhookConfigurationName(
		config.ValidatingWebhookConfigurationName,
		"validating-webhook-configuration",
		DefaultValidatingWebhookConfigurationName)
}

// GetIncludePlugins gets the list of plugins to be always
// included in the operator reconciliation
func (config *Data) GetIncludePlugins() []string {
	rawList := strings.Split(config.IncludePlugins, ",")
	result := make([]string, 0, len(rawList))
	for _, pluginName := range rawList {
		if trimmedPluginName := strings.TrimSpace(pluginName); trimmedPluginName != "" {
			result = append(result, trimmedPluginName)
		}
	}

	return result
}

func cleanNamespaceList(namespaces string) (result []string) {
	unfilteredList := strings.Split(namespaces, ",")
	result = make([]string, 0, len(unfilteredList))

	for _, elem := range unfilteredList {
		elem = strings.TrimSpace(elem)
		if len(elem) != 0 {
			result = append(result, elem)
		}
	}

	return result
}

func evaluateGlobPatterns(patterns []string, value string) (result bool) {
	var err error

	for _, pattern := range patterns {
		if result, err = path.Match(pattern, value); err != nil {
			configurationLog.Info(
				"Skipping invalid glob pattern during labels/annotations inheritance",
				"pattern", pattern)
			continue
		}

		if result {
			return result
		}
	}

	return result
}

// GetFilteredCacheMode returns the mode of the operator cache. An invalid
// value is reported at startup; here it counts as off.
func (config *Data) GetFilteredCacheMode() filteredcache.Mode {
	mode, err := filteredcache.ParseMode(config.FilteredCacheMode)
	if err != nil {
		return filteredcache.ModeOff
	}
	return mode
}
