# cloudnative-pg

CloudNativePG Operator Helm Chart

![Version: 0.0.0-SNAPSHOT](https://img.shields.io/badge/Version-0.0.0--SNAPSHOT-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 0.0.0-SNAPSHOT](https://img.shields.io/badge/AppVersion-0.0.0--SNAPSHOT-informational?style=flat-square)

## Installing the Chart

To install the chart with the release name `my-release`:

```console
$ helm repo add fyannk-cnpg https://fyannk.github.io/cloudnative-pg
$ helm install my-release fyannk-cnpg/cloudnative-pg
```

## Requirements

Kubernetes: `>=1.21.0-0`

| Repository | Name | Version |
|------------|------|---------|
| https://fyannk.github.io/cloudnative-pg | crds | 0.0.0-SNAPSHOT |

## Values

<h3>Capabilities</h3>
<table>
	<thead>
		<th>Key</th>
		<th>Type</th>
		<th>Default</th>
		<th>Description</th>
	</thead>
	<tbody>
		<tr>
			<td>capabilities.clusterImageCatalog</td>
			<td>bool</td>
			<td><pre lang="json">
true
</pre>
</td>
			<td>Specifies whether the operator can have ClusterImageCatalog(Get,List,Watch) privileges at Cluster Level.</td>
		</tr>
		<tr>
			<td>capabilities.nodeList</td>
			<td>bool</td>
			<td><pre lang="json">
true
</pre>
</td>
			<td>Specifies whether the operator can have Node(Get,List,Watch) privileges at Cluster Level.</td>
		</tr>
		<tr>
			<td>capabilities.podMonitor</td>
			<td>bool</td>
			<td><pre lang="json">
true
</pre>
</td>
			<td>Specifies whether the operator can manage PodMonitor.</td>
		</tr>
		<tr>
			<td>capabilities.secureNamespace</td>
			<td>bool</td>
			<td><pre lang="json">
false
</pre>
</td>
			<td>Specifies whether the operator is installed in a secure namespace.</td>
		</tr>
		<tr>
			<td>capabilities.volumeSnapshot</td>
			<td>bool</td>
			<td><pre lang="json">
true
</pre>
</td>
			<td>Specifies whether VolumeSnapshot is available.</td>
		</tr>
	</tbody>
</table>
<h3>Cluster</h3>
<table>
	<thead>
		<th>Key</th>
		<th>Type</th>
		<th>Default</th>
		<th>Description</th>
	</thead>
	<tbody>
		<tr>
			<td>cluster.user.aggregateClusterRoles</td>
			<td>bool</td>
			<td><pre lang="json">
false
</pre>
</td>
			<td>Aggregate ClusterRoles to Kubernetes default user-facing roles. Ref: https://kubernetes.io/docs/reference/access-authn-authz/rbac/#user-facing-roles</td>
		</tr>
		<tr>
			<td>cluster.webhooks.certManagerCert</td>
			<td>string</td>
			<td><pre lang="json">
null
</pre>
</td>
			<td>name of the cert-manager certificate to use with the webhook.</td>
		</tr>
		<tr>
			<td>cluster.webhooks.manageCerts</td>
			<td>bool</td>
			<td><pre lang="json">
true
</pre>
</td>
			<td>whether the operator manages the webhook certificates automatically.</td>
		</tr>
		<tr>
			<td>cluster.webhooks.mutatingfailurePolicy</td>
			<td>string</td>
			<td><pre lang="json">
"Fail"
</pre>
</td>
			<td>MutatingWebHook failure policy</td>
		</tr>
		<tr>
			<td>cluster.webhooks.port</td>
			<td>int</td>
			<td><pre lang="json">
9443
</pre>
</td>
			<td>Listening port of the webhook.</td>
		</tr>
		<tr>
			<td>cluster.webhooks.validatingfailurePolicy</td>
			<td>string</td>
			<td><pre lang="json">
"Fail"
</pre>
</td>
			<td>ValidatingWebHook failure policy</td>
		</tr>
	</tbody>
</table>
<h3>Common Parameters</h3>
<table>
	<thead>
		<th>Key</th>
		<th>Type</th>
		<th>Default</th>
		<th>Description</th>
	</thead>
	<tbody>
		<tr>
			<td>commonAnnotations</td>
			<td>object</td>
			<td><pre lang="json">
{}
</pre>
</td>
			<td>Add annotations to all resources.</td>
		</tr>
		<tr>
			<td>fullnameOverride</td>
			<td>string</td>
			<td><pre lang="json">
""
</pre>
</td>
			<td>Generic FullnameOverride</td>
		</tr>
		<tr>
			<td>installCRDs</td>
			<td>bool</td>
			<td><pre lang="json">
true
</pre>
</td>
			<td>installCRDs determines whether Custom Resource Definitions (CRD) are installed by the chart.</td>
		</tr>
		<tr>
			<td>installMode</td>
			<td>string</td>
			<td><pre lang="json">
"ClusterWide"
</pre>
</td>
			<td>Helm install Mode : ClusterWide, Namespaced, AdminForNamespaced, NamespacedWithWebhooks, Free</td>
		</tr>
		<tr>
			<td>kubeAPIServerIP</td>
			<td>string</td>
			<td><pre lang="json">
null
</pre>
</td>
			<td>kubeAPIServerIP is the IP address of the Kubernetes API server from inside cluster.</td>
		</tr>
		<tr>
			<td>managedNamespaces</td>
			<td>list</td>
			<td><pre lang="json">
[]
</pre>
</td>
			<td>Namespaces where the operator should manage its CRDs.</td>
		</tr>
		<tr>
			<td>nameOverride</td>
			<td>string</td>
			<td><pre lang="json">
""
</pre>
</td>
			<td>Generic NameOverride</td>
		</tr>
		<tr>
			<td>podAnnotations</td>
			<td>object</td>
			<td><pre lang="json">
{}
</pre>
</td>
			<td>add annotations to pods</td>
		</tr>
		<tr>
			<td>podLabels</td>
			<td>object</td>
			<td><pre lang="json">
{}
</pre>
</td>
			<td>add labels to pods</td>
		</tr>
	</tbody>
</table>
<h3>Features</h3>
<table>
	<thead>
		<th>Key</th>
		<th>Type</th>
		<th>Default</th>
		<th>Description</th>
	</thead>
	<tbody>
		<tr>
			<td>features.cluster.enabled</td>
			<td>bool</td>
			<td><pre lang="json">
true
</pre>
</td>
			<td>Global flag to enable/disable everything at cluster level.</td>
		</tr>
		<tr>
			<td>features.cluster.role</td>
			<td>bool</td>
			<td><pre lang="json">
true
</pre>
</td>
			<td>Enable ClusterRole for Operator.</td>
		</tr>
		<tr>
			<td>features.cluster.rolebinding</td>
			<td>bool</td>
			<td><pre lang="json">
true
</pre>
</td>
			<td>Enable ClusterRoleBinding for Operator.</td>
		</tr>
		<tr>
			<td>features.cluster.user</td>
			<td>bool</td>
			<td><pre lang="json">
true
</pre>
</td>
			<td>Enable ClusterRole / ClusterRoleBinding for User RBAC</td>
		</tr>
		<tr>
			<td>features.cluster.webhooks</td>
			<td>bool</td>
			<td><pre lang="json">
true
</pre>
</td>
			<td>Enable Mutating / Validating WebHooks.</td>
		</tr>
		<tr>
			<td>features.operator.configuration</td>
			<td>bool</td>
			<td><pre lang="json">
true
</pre>
</td>
			<td>Enable Operator ConfigMap or Secret creation.</td>
		</tr>
		<tr>
			<td>features.operator.deployment</td>
			<td>bool</td>
			<td><pre lang="json">
true
</pre>
</td>
			<td>Enable Operator Deployment creation.</td>
		</tr>
		<tr>
			<td>features.operator.enabled</td>
			<td>bool</td>
			<td><pre lang="json">
true
</pre>
</td>
			<td>Global flag to enable/disable everything inside Operator's Namespace.</td>
		</tr>
		<tr>
			<td>features.operator.monitoring</td>
			<td>bool</td>
			<td><pre lang="json">
true
</pre>
</td>
			<td>Enable Monitoring ConfigMap creation.</td>
		</tr>
		<tr>
			<td>features.operator.networkpolicy</td>
			<td>bool</td>
			<td><pre lang="json">
true
</pre>
</td>
			<td>Enable NetworkPolicy creation.</td>
		</tr>
		<tr>
			<td>features.operator.podmonitor</td>
			<td>bool</td>
			<td><pre lang="json">
true
</pre>
</td>
			<td>Enable PodMonitor creation.</td>
		</tr>
		<tr>
			<td>features.operator.role</td>
			<td>bool</td>
			<td><pre lang="json">
true
</pre>
</td>
			<td>Enable Role creation. </td>
		</tr>
		<tr>
			<td>features.operator.rolebinding</td>
			<td>bool</td>
			<td><pre lang="json">
true
</pre>
</td>
			<td>Enable RoleBinding creation.</td>
		</tr>
		<tr>
			<td>features.operator.service</td>
			<td>bool</td>
			<td><pre lang="json">
true
</pre>
</td>
			<td>Enable Service creation for WebHooks.</td>
		</tr>
		<tr>
			<td>features.operator.serviceaccount</td>
			<td>bool</td>
			<td><pre lang="json">
true
</pre>
</td>
			<td>Enable ServiceAccount for Operator.</td>
		</tr>
	</tbody>
</table>
<h3>Operator Configuration</h3>
<table>
	<thead>
		<th>Key</th>
		<th>Type</th>
		<th>Default</th>
		<th>Description</th>
	</thead>
	<tbody>
		<tr>
			<td>operator.configuration.data.CA_SECRET_NAME</td>
			<td>string</td>
			<td><pre lang="json">
"cnpg-ca-secret"
</pre>
</td>
			<td>The name of the secret containing the CA certificate.</td>
		</tr>
		<tr>
			<td>operator.configuration.data.CERTIFICATE_DURATION</td>
			<td>string</td>
			<td><pre lang="json">
"365"
</pre>
</td>
			<td>Determines the lifetime of the generated certificates in days. Default is 90.</td>
		</tr>
		<tr>
			<td>operator.configuration.data.CLUSTERS_ROLLOUT_DELAY</td>
			<td>int</td>
			<td><pre lang="json">
0
</pre>
</td>
			<td>delay in seconds between the rollout of two clusters. Default is 0    The duration (in seconds) to wait between the roll-outs of different    clusters during an operator upgrade. This setting controls the    timing of upgrades across clusters, spreading them out to reduce    system impact. The default value is 0, which means no delay between    PostgreSQL cluster upgrades.</td>
		</tr>
		<tr>
			<td>operator.configuration.data.CLUSTER_WIDE_CACHE_FILTER</td>
			<td>string</td>
			<td><pre lang="json">
"true"
</pre>
</td>
			<td>If set to true, the operator will use filter "cnpg.io/reconcile=true" for objects in cache.</td>
		</tr>
		<tr>
			<td>operator.configuration.data.CREATE_ANY_SERVICE</td>
			<td>string</td>
			<td><pre lang="json">
"false"
</pre>
</td>
			<td>when set to true, will create -any service for the cluster. Default is false</td>
		</tr>
		<tr>
			<td>operator.configuration.data.DEFAULT_STORAGE_CLASS</td>
			<td>string</td>
			<td><pre lang="json">
""
</pre>
</td>
			<td>The default storage class to use for new clusters, default is Kubernetes' default.</td>
		</tr>
		<tr>
			<td>operator.configuration.data.ENABLE_AZURE_PVC_UPDATES</td>
			<td>string</td>
			<td><pre lang="json">
"false"
</pre>
</td>
			<td>Enables to delete Postgres pod if its PVC is stuck in Resizing condition.    This feature is mainly for the Azure environment (default false)</td>
		</tr>
		<tr>
			<td>operator.configuration.data.ENABLE_INSTANCE_MANAGER_INPLACE_UPDATES</td>
			<td>string</td>
			<td><pre lang="json">
"false"
</pre>
</td>
			<td>when set to true, enables in-place updates of the instance manager after an update    of the operator, avoiding rolling updates of the cluster (default false)</td>
		</tr>
		<tr>
			<td>operator.configuration.data.EXPIRING_CHECK_THRESHOLD</td>
			<td>string</td>
			<td><pre lang="json">
"30"
</pre>
</td>
			<td>Determines the threshold, in days, for identifying a certificate as expiring. Default is 7.</td>
		</tr>
		<tr>
			<td>operator.configuration.data.INCLUDE_PLUGINS</td>
			<td>string</td>
			<td><pre lang="json">
""
</pre>
</td>
			<td>comma-separated list of plugins to always be included in the Cluster reconciliation</td>
		</tr>
		<tr>
			<td>operator.configuration.data.INHERITED_ANNOTATIONS</td>
			<td>string</td>
			<td><pre lang="json">
"prometheus.io/scrape, prometheus.io/path, prometheus.io/port"
</pre>
</td>
			<td>List of annotations that should be inherited from the parent object to the child object (not only cluster).</td>
		</tr>
		<tr>
			<td>operator.configuration.data.INHERITED_LABELS</td>
			<td>string</td>
			<td><pre lang="json">
""
</pre>
</td>
			<td>List of labels that should be inherited from the parent object to the child object (not only cluster).</td>
		</tr>
		<tr>
			<td>operator.configuration.data.INSTANCES_ROLLOUT_DELAY</td>
			<td>int</td>
			<td><pre lang="json">
0
</pre>
</td>
			<td>delay in seconds between the rollout of two instances. Default is 0    The duration (in seconds) to wait between roll-outs of individual    PostgreSQL instances within the same cluster during an operator    upgrade. The default value is 0, meaning no delay between upgrades    of instances in the same PostgreSQL cluster.</td>
		</tr>
		<tr>
			<td>operator.configuration.data.MANDATORY_ANNOTATIONS</td>
			<td>string</td>
			<td><pre lang="json">
"prometheus.io/port=9187, prometheus.io/scrape=true, prometheus.io/path=/metrics"
</pre>
</td>
			<td>List of annotations that MUST be present on every resource managed by the operator.</td>
		</tr>
		<tr>
			<td>operator.configuration.data.MANDATORY_LABELS</td>
			<td>string</td>
			<td><pre lang="json">
""
</pre>
</td>
			<td>List of labels that MUST be present on every resource managed by the operator.</td>
		</tr>
		<tr>
			<td>operator.configuration.data.PULL_SECRET_NAME</td>
			<td>string</td>
			<td><pre lang="json">
""
</pre>
</td>
			<td>name of an additional pull secret to be defined in the operator's namespace and to be used to download images</td>
		</tr>
		<tr>
			<td>operator.configuration.data.WEBHOOK_ENABLED</td>
			<td>string</td>
			<td><pre lang="json">
"true"
</pre>
</td>
			<td>If set to true, the operator will manage Webhooks certificates. (default true)</td>
		</tr>
		<tr>
			<td>operator.configuration.data.WEBHOOK_SECRET_NAME</td>
			<td>string</td>
			<td><pre lang="json">
"cnpg-webhook-cert"
</pre>
</td>
			<td>The name of the secret to use for the Webhook.</td>
		</tr>
		<tr>
			<td>operator.configuration.name</td>
			<td>string</td>
			<td><pre lang="json">
""
</pre>
</td>
			<td>The name of the configmap/secret to use.</td>
		</tr>
		<tr>
			<td>operator.configuration.secret</td>
			<td>bool</td>
			<td><pre lang="json">
false
</pre>
</td>
			<td>Specifies whether it should be stored in a secret, instead of a configmap.</td>
		</tr>
		<tr>
			<td>operator.deployment.additionalArgs</td>
			<td>list</td>
			<td><pre lang="json">
[]
</pre>
</td>
			<td>Additinal arguments to be added to the operator's args list.</td>
		</tr>
		<tr>
			<td>operator.deployment.additionalEnv</td>
			<td>list</td>
			<td><pre lang="json">
[]
</pre>
</td>
			<td>Array containing extra environment variables which can be templated. For example:  - name: RELEASE_NAME    value: "{{ .Release.Name }}"  - name: MY_VAR    value: "mySpecialKey"</td>
		</tr>
		<tr>
			<td>operator.deployment.affinity</td>
			<td>object</td>
			<td><pre lang="json">
{}
</pre>
</td>
			<td>Affinity for the operator to be installed.</td>
		</tr>
		<tr>
			<td>operator.deployment.containerSecurityContext</td>
			<td>object</td>
			<td><pre lang="json">
{
  "allowPrivilegeEscalation": false,
  "capabilities": {
    "drop": [
      "ALL"
    ]
  },
  "readOnlyRootFilesystem": true,
  "runAsGroup": 10001,
  "runAsUser": 10001,
  "seccompProfile": {
    "type": "RuntimeDefault"
  }
}
</pre>
</td>
			<td>Container Security Context.</td>
		</tr>
		<tr>
			<td>operator.deployment.image.pullPolicy</td>
			<td>string</td>
			<td><pre lang="json">
"IfNotPresent"
</pre>
</td>
			<td>Options: Always, Never, IfNotPresent</td>
		</tr>
		<tr>
			<td>operator.deployment.image.repository</td>
			<td>string</td>
			<td><pre lang="json">
"ghcr.io/fyannk/cloudnative-pg-testing"
</pre>
</td>
			<td>The image repository to pull from.</td>
		</tr>
		<tr>
			<td>operator.deployment.image.tag</td>
			<td>string</td>
			<td><pre lang="json">
""
</pre>
</td>
			<td>Overrides the image tag whose default is the chart appVersion.</td>
		</tr>
		<tr>
			<td>operator.deployment.imagePullSecrets</td>
			<td>list</td>
			<td><pre lang="json">
[
  {
    "name": "cnpg-pull-secret"
  }
]
</pre>
</td>
			<td>This secret should exist, but MUST HAVE label cnpg.io/reconcile=true</td>
		</tr>
		<tr>
			<td>operator.deployment.livenessProbe</td>
			<td>object</td>
			<td><pre lang="json">
{
  "initialDelaySeconds": 3
}
</pre>
</td>
			<td>Operator's probes configuration.</td>
		</tr>
		<tr>
			<td>operator.deployment.nodeSelector</td>
			<td>object</td>
			<td><pre lang="json">
{}
</pre>
</td>
			<td>Nodeselector for the operator to be installed.</td>
		</tr>
		<tr>
			<td>operator.deployment.podSecurityContext</td>
			<td>object</td>
			<td><pre lang="json">
{
  "runAsNonRoot": true,
  "seccompProfile": {
    "type": "RuntimeDefault"
  }
}
</pre>
</td>
			<td>Security Context for the whole pod.</td>
		</tr>
		<tr>
			<td>operator.deployment.port</td>
			<td>int</td>
			<td><pre lang="json">
9443
</pre>
</td>
			<td>Listening port for the webhooks.</td>
		</tr>
		<tr>
			<td>operator.deployment.priorityClassName</td>
			<td>string</td>
			<td><pre lang="json">
""
</pre>
</td>
			<td>Priority indicates the importance of a Pod relative to other Pods.</td>
		</tr>
		<tr>
			<td>operator.deployment.readinessProbe</td>
			<td>object</td>
			<td><pre lang="json">
{
  "initialDelaySeconds": 3
}
</pre>
</td>
			<td>Operator's readiness probe configuration.</td>
		</tr>
		<tr>
			<td>operator.deployment.replicaCount</td>
			<td>int</td>
			<td><pre lang="json">
1
</pre>
</td>
			<td>Number of replicas.</td>
		</tr>
		<tr>
			<td>operator.deployment.resources</td>
			<td>object</td>
			<td><pre lang="json">
{
  "limits": {
    "cpu": "100m",
    "memory": "200Mi"
  },
  "requests": {
    "cpu": "100m",
    "memory": "100Mi"
  }
}
</pre>
</td>
			<td>Resources to allocate for the operator.</td>
		</tr>
		<tr>
			<td>operator.deployment.tolerations</td>
			<td>list</td>
			<td><pre lang="json">
[]
</pre>
</td>
			<td>Tolerations for the operator to be installed.</td>
		</tr>
		<tr>
			<td>operator.monitoring.name</td>
			<td>string</td>
			<td><pre lang="json">
""
</pre>
</td>
			<td>The name of the default monitoring configmap.</td>
		</tr>
		<tr>
			<td>operator.monitoring.queries</td>
			<td>string</td>
			<td><pre lang="json">
null
</pre>
</td>
			<td>A string representation of a YAML defining monitoring queries.</td>
		</tr>
		<tr>
			<td>operator.podmonitor.additionalLabels</td>
			<td>object</td>
			<td><pre lang="json">
{}
</pre>
</td>
			<td>Additional labels for the podMonitor</td>
		</tr>
		<tr>
			<td>operator.podmonitor.grafanaDashboard.annotations</td>
			<td>object</td>
			<td><pre lang="json">
{}
</pre>
</td>
			<td>Annotations that ConfigMaps can have to get configured in Grafana.</td>
		</tr>
		<tr>
			<td>operator.podmonitor.grafanaDashboard.configMapName</td>
			<td>string</td>
			<td><pre lang="json">
"cnpg-grafana-dashboard"
</pre>
</td>
			<td>The name of the ConfigMap containing the dashboard.</td>
		</tr>
		<tr>
			<td>operator.podmonitor.grafanaDashboard.labels</td>
			<td>object</td>
			<td><pre lang="json">
{}
</pre>
</td>
			<td>Labels that ConfigMaps should have to get configured in Grafana.</td>
		</tr>
		<tr>
			<td>operator.podmonitor.grafanaDashboard.namespace</td>
			<td>string</td>
			<td><pre lang="json">
""
</pre>
</td>
			<td>Allows overriding the namespace where the ConfigMap will be created, defaulting to the same one as the Release.</td>
		</tr>
		<tr>
			<td>operator.podmonitor.grafanaDashboard.sidecarLabel</td>
			<td>string</td>
			<td><pre lang="json">
"grafana_dashboard"
</pre>
</td>
			<td>Label that ConfigMaps should have to be loaded as dashboards.  DEPRECATED: Use labels instead.</td>
		</tr>
		<tr>
			<td>operator.podmonitor.grafanaDashboard.sidecarLabelValue</td>
			<td>string</td>
			<td><pre lang="json">
"1"
</pre>
</td>
			<td>Label value that ConfigMaps should have to be loaded as dashboards.  DEPRECATED: Use labels instead.</td>
		</tr>
		<tr>
			<td>operator.podmonitor.metricRelabelings</td>
			<td>list</td>
			<td><pre lang="json">
[]
</pre>
</td>
			<td>Metrics relabel configurations to apply to samples before ingestion.</td>
		</tr>
		<tr>
			<td>operator.podmonitor.relabelings</td>
			<td>list</td>
			<td><pre lang="json">
[]
</pre>
</td>
			<td>Relabel configurations to apply to samples before scraping.</td>
		</tr>
		<tr>
			<td>operator.service.name</td>
			<td>string</td>
			<td><pre lang="json">
""
</pre>
</td>
			<td>The name of the service to use.</td>
		</tr>
		<tr>
			<td>operator.service.port</td>
			<td>int</td>
			<td><pre lang="json">
443
</pre>
</td>
			<td>The port to use.</td>
		</tr>
		<tr>
			<td>operator.service.type</td>
			<td>string</td>
			<td><pre lang="json">
"ClusterIP"
</pre>
</td>
			<td>The type of service to use.</td>
		</tr>
		<tr>
			<td>operator.serviceAccount.name</td>
			<td>string</td>
			<td><pre lang="json">
""
</pre>
</td>
			<td>The name of the service account to use. If not set and create is true, a name is generated using the fullname template.</td>
		</tr>
	</tbody>
</table>

<h3>Other Values</h3>
<table>
	<thead>
		<th>Key</th>
		<th>Type</th>
		<th>Default</th>
		<th>Description</th>
	</thead>
	<tbody>
	<tr>
		<td>operator.deployment.dnsPolicy</td>
		<td>string</td>
		<td><pre lang="json">
""
</pre>
</td>
		<td></td>
	</tr>
	<tr>
		<td>operator.deployment.hostNetwork</td>
		<td>bool</td>
		<td><pre lang="json">
false
</pre>
</td>
		<td></td>
	</tr>
	</tbody>
</table>

