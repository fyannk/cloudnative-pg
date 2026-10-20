---
id: operator_conf
sidebar_position: 260
title: Operator configuration
---

# Operator configuration
<!-- SPDX-License-Identifier: CC-BY-4.0 -->

The operator for CloudNativePG is installed from a standard
deployment manifest and follows the convention over configuration paradigm.
While this is fine in most cases, there are some scenarios where you want
to change the default behavior, such as:

- defining annotations and labels to be inherited by all resources created
  by the operator and that are set in the cluster resource
- defining a different default image for PostgreSQL or an additional pull secret

By default, the operator is installed in the `cnpg-system`
namespace as a Kubernetes `Deployment` called `cnpg-controller-manager`.

:::note
    In the examples below we assume the default name and namespace for the operator deployment.
:::

The behavior of the operator can be customized through a `ConfigMap`/`Secret` that
is located in the same namespace of the operator deployment and with
`cnpg-controller-manager-config` as the name.

:::info[Important]
    Any change to the config's `ConfigMap`/`Secret` will not be automatically
    detected by the operator, - and as such, it needs to be reloaded (see below).
    Moreover, changes only apply to the resources created after the configuration
    is reloaded.
:::

:::info[Important]
    The operator first processes the ConfigMap values and then the Secret’s, in this order.
    As a result, if a parameter is defined in both places, the one in the Secret will be used.
:::

## Available options

The operator looks for the following environment variables to be defined in the `ConfigMap`/`Secret`:

Name | Description
---- | -----------
`API_CLUSTER_IMAGE_CATALOG_ENABLED` | When set to `false`, the operator does not use the cluster-scoped `ClusterImageCatalog` API: a `Cluster` or `Pooler` referencing one is refused by the webhook, or fails its reconciliation with an explicit error if admitted before. Default is `true`, and the API is probed at startup (see [Restricted installations](#restricted-installations)).
`API_NODE_ENABLED` | When set to `false`, the operator does not read the cluster-scoped `Node` API: drained nodes are not detected, and node labels are not resolved in the instance topology. Default is `true`, and the API is probed at startup (see [Restricted installations](#restricted-installations)).
`API_POD_MONITOR_ENABLED` | When set to `false`, the operator does not use the `PodMonitor` API: no `PodMonitor` is created for the clusters and poolers asking for one, and the webhook warns about them. Default is `true`, and the API is probed at startup (see [Restricted installations](#restricted-installations)).
`API_VOLUME_SNAPSHOT_ENABLED` | When set to `false`, the operator does not use the `VolumeSnapshot` API: volume snapshot backups and recoveries are refused as when the API is not installed. Default is `true`, and the API is probed at startup (see [Restricted installations](#restricted-installations)).
`CERTIFICATE_DURATION` | Determines the lifetime of the generated certificates in days. Default is 90.
`CLUSTERS_ROLLOUT_DELAY` | The duration (in seconds) to wait between the roll-outs of different clusters during an operator upgrade. This setting controls the timing of upgrades across clusters, spreading them out to reduce system impact. The default value is `0` which means no delay between PostgreSQL cluster upgrades.
`CREATE_ANY_SERVICE` | When set to `true`, will create `-any` service for the cluster. Default is `false`
`NODE_STATUS_SOURCE` | Where the operator learns the state of the nodes (cordon, drain taints, node labels): `api` reads the cluster-scoped `Node` API; `pods` reads the keys a node bridge echoes onto the instance Pods, see [Node state from the instance Pods](#node-state-from-the-instance-pods); `none` reads nothing, see the same section for what is lost. Unset, it follows `API_NODE_ENABLED`: `api` when the operator reads the Node API, `none` when the API is disabled or not accessible. An explicit `api` while the Node API is disabled refuses to start.
`NODE_STATUS_POD_KEY_PREFIX` | With `NODE_STATUS_SOURCE=pods`, the prefix of the keys read on the instance Pods: `<prefix>unschedulable`, `<prefix>taints` and `<prefix>eviction-requested`. Required with `pods`.
`NODE_STATUS_LABEL_PREFIX` | With `NODE_STATUS_SOURCE=pods`, the prefix under which the node labels are copied onto the instance Pods: the node label `<key>` is read from the Pod label `<prefix><key>`. Empty by default: the node label keys themselves.
`NODE_STATUS_EVICTION_MAX_AGE` | With `NODE_STATUS_SOURCE=pods`, the age in seconds after which an eviction request echoed onto a primary Pod no longer triggers a switchover. Default `600`; `0` means no limit.
`DRAIN_TAINTS` | Specifies the taint keys that should be interpreted as indicators of node drain. By default, it includes the taints commonly applied by [kubectl](https://kubernetes.io/docs/concepts/scheduling-eviction/taint-and-toleration/), [Cluster Autoscaler](https://github.com/kubernetes/autoscaler), and [Karpenter](https://github.com/aws/karpenter-provider-aws): `node.kubernetes.io/unschedulable`, `ToBeDeletedByClusterAutoscaler`, `karpenter.sh/disrupted`, `karpenter.sh/disruption`.
`ENABLE_INSTANCE_MANAGER_INPLACE_UPDATES` | When set to `true`, enables in-place updates of the instance manager after an update of the operator, avoiding rolling updates of the cluster (default `false`)
`ENABLE_WEBHOOK_NAMESPACE_SUFFIX` | When set to `true`, the operator looks up its `MutatingWebhookConfiguration` and `ValidatingWebhookConfiguration` under names suffixed with `-<OPERATOR_NAMESPACE>` (e.g. `cnpg-mutating-webhook-configuration-cnpg-team-a` and `cnpg-validating-webhook-configuration-cnpg-team-a`). Use this when running multiple namespaced operator instances on the same cluster to avoid name collisions. The operator does not create the suffixed configurations; you must create them before starting the operator with this flag enabled. If `OPERATOR_NAMESPACE` is not set when this flag is enabled, the operator will refuse to start. Default is `false`.
`EXPIRING_CHECK_THRESHOLD` | Determines the threshold, in days, for identifying a certificate as expiring. Default is 7.
`INCLUDE_PLUGINS` | A comma-separated list of plugins to be always included in the Cluster's reconciliation.
`INHERITED_ANNOTATIONS` | List of annotation names that, when defined in a `Cluster` metadata, will be inherited by all the generated resources, including pods
`INHERITED_LABELS` | List of label names that, when defined in a `Cluster` metadata, will be inherited by all the generated resources, including pods
`INSTANCES_ROLLOUT_DELAY` | The duration (in seconds) to wait between roll-outs of individual PostgreSQL instances within the same cluster during an operator upgrade. The default value is `0`, meaning no delay between upgrades of instances in the same PostgreSQL cluster.
`KUBERNETES_CLUSTER_DOMAIN` | Defines the domain suffix for service FQDNs within the Kubernetes cluster. If left unset, it defaults to "cluster.local".
`MANAGE_WEBHOOK_CONFIGURATIONS` | When set to `true` (default), the operator injects its CA bundle into the `MutatingWebhookConfiguration` and `ValidatingWebhookConfiguration`. Set to `false` when the CA bundle is injected externally, for example by cert-manager's CA injector or by GitOps. This is independent of the webhook serving certificate, which the operator always manages when it owns its PKI.
`METRICS_CERT_DIR` | The directory where TLS certificates for the operator metrics server are stored. When set, enables TLS for the metrics endpoint on port 8080. The directory must contain `tls.crt` and `tls.key` files following standard Kubernetes TLS secret conventions. If not set, the metrics server operates without TLS (default behavior).
`MONITORING_QUERIES_CONFIGMAP` | The name of a ConfigMap in the operator's namespace with a set of default queries (to be specified under the key `queries`) to be applied to all created Clusters
`MONITORING_QUERIES_SECRET` | The name of a Secret in the operator's namespace with a set of default queries (to be specified under the key `queries`) to be applied to all created Clusters
`OPERATOR_IMAGE_NAME` | The name of the operator image used to bootstrap Pods. Defaults to the image specified during installation.
`PGBOUNCER_IMAGE_NAME` | The name of the PgBouncer image used by default for new poolers. Defaults to the version specified in the operator.
`POSTGRES_IMAGE_NAME` | The name of the PostgreSQL image used by default for new clusters. Defaults to the version specified in the operator.
`PULL_SECRET_NAME` | Name of an additional pull secret to be defined in the operator's namespace and to be used to download images
`STANDBY_TCP_USER_TIMEOUT` | Defines the [`TCP_USER_TIMEOUT` socket option](https://www.postgresql.org/docs/current/runtime-config-connection.html#GUC-TCP-USER-TIMEOUT) in milliseconds for replication connections from standby instances to the primary. Default is 5000 (5 seconds). Set to `0` to use the system's default.
`WATCH_NAMESPACE` | Specifies the namespace(s) where the operator should watch for resources. Multiple namespaces can be specified separated by commas. If not set, the operator watches all namespaces (cluster-wide mode).

:::warning
    CloudNativePG does NOT currently support running multiple operators on
    the same cluster. While this can be achieved through the use of namespaced
    deployments using `ENABLE_WEBHOOK_NAMESPACE_SUFFIX` and `WATCH_NAMESPACE`,
    multiple operators still use the same set of shared CRDs. We cannot
    guarantee a backwards compatible upgrade across multiple concurrently
    running operators.
:::

Values in `INHERITED_ANNOTATIONS` and `INHERITED_LABELS` support path-like wildcards. For example, the value `example.com/*` will match
both the value `example.com/one` and `example.com/two`.

When you specify an additional pull secret name using the `PULL_SECRET_NAME` parameter,
the operator will use that secret to create a pull secret for every created PostgreSQL
cluster. That secret will be named `<cluster-name>-pull`.

The namespace where the operator looks for the `PULL_SECRET_NAME` secret is where
you installed the operator. If the operator is not able to find that secret, it
will ignore the configuration parameter.

## Defining an operator config map

The example below customizes the behavior of the operator, by defining
the label/annotation names to be inherited by the resources created by
any `Cluster` object that is deployed at a later time, by enabling
[in-place updates for the instance
manager](installation_upgrade.md#in-place-updates-of-the-instance-manager),
and by spreading upgrades.

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: cnpg-controller-manager-config
  namespace: cnpg-system
data:
  CLUSTERS_ROLLOUT_DELAY: '60'
  ENABLE_INSTANCE_MANAGER_INPLACE_UPDATES: 'true'
  INHERITED_ANNOTATIONS: categories
  INHERITED_LABELS: environment, workload, app
  INSTANCES_ROLLOUT_DELAY: '10'
```

## Defining an operator secret

The example below customizes the behavior of the operator, by defining
the label/annotation names to be inherited by the resources created by
any `Cluster` object that is deployed at a later time, and by enabling
[in-place updates for the instance
manager](installation_upgrade.md#in-place-updates-of-the-instance-manager),
and by spreading upgrades.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: cnpg-controller-manager-config
  namespace: cnpg-system
type: Opaque
stringData:
  CLUSTERS_ROLLOUT_DELAY: '60'
  ENABLE_INSTANCE_MANAGER_INPLACE_UPDATES: 'true'
  INHERITED_ANNOTATIONS: categories
  INHERITED_LABELS: environment, workload, app
  INSTANCES_ROLLOUT_DELAY: '10'
```

## Restarting the operator to reload configs

For the change to be effective, you need to recreate the operator pods to
reload the config map. If you have installed the operator on Kubernetes
using the manifest you can do that by issuing:

```shell
kubectl rollout restart deployment \
    -n cnpg-system \
    cnpg-controller-manager
```

In general, given a specific namespace, you can delete the operator pods with
the following command:

```shell
kubectl delete pods -n [NAMESPACE_NAME_HERE] \
  -l app.kubernetes.io/name=cloudnative-pg
```

:::warning
    Customizations will be applied only to `Cluster` resources created
    after the reload of the operator deployment.
:::

Following the above example, if the `Cluster` definition contains a `categories`
annotation and any of the `environment`, `workload`, or `app` labels, these will
be inherited by all the resources generated by the deployment.

## Restricted installations

Some of the APIs the operator uses are optional for PostgreSQL: the
`PodMonitor` API of the Prometheus operator, the `VolumeSnapshot` API of the
CSI snapshotter, and the cluster-scoped `ClusterImageCatalog` and `Node`
APIs. An
installation on a cluster where they are absent, or where the operator is not
granted access to them, can run without them, with the corresponding
capabilities switched off.

At startup, for each of these APIs left enabled, the operator checks that it
can list the kind where it will watch it: in every namespace of
`WATCH_NAMESPACE`, or cluster-wide. An API that is not installed, or that the
operator is forbidden to list, is disabled for the life of the process, with a
line in the operator log. Any other error, such as a timeout, stops the
operator so that the check runs again on the next start. `API_*_ENABLED`
settings disable an API explicitly, without probing it.

The webhook tells which objects ask for a disabled API. An object that cannot
work without it is refused when it is created, or when an update sets the
field asking for it; when an update leaves that field unchanged, as for an
object admitted before the API was disabled, the webhook only warns, so that
the object can still be updated. An object that works without the API only
gets a warning.

What a disabled API costs:

- `PodMonitor`: no `PodMonitor` object is created; a `Cluster` or `Pooler`
  with `monitoring.enablePodMonitor` set, deprecated, gets a warning from the
  webhook and in the operator log. The `monitoring.coreos.com` rules can be
  removed from the operator `ClusterRole`.
- `VolumeSnapshot`: `volumeSnapshot` backups and scheduled backups, and
  bootstrapping from a `VolumeSnapshot`, are refused by the webhook;
  `backup.volumeSnapshot` gets a warning, and replicas are created with
  `pg_basebackup`. The `snapshot.storage.k8s.io` rules can be removed from
  the `ClusterRole`.
- `ClusterImageCatalog`: a `Cluster` or `Pooler` whose `imageCatalogRef` is a
  `ClusterImageCatalog` is refused by the webhook, and one admitted before
  fails its reconciliation with an explicit error; use a namespaced
  `ImageCatalog` instead. The `clusterimagecatalogs` rules can be removed
  from the `ClusterRole`.
- `Node`: unless `NODE_STATUS_SOURCE=pods` brings the node state through
  the instance Pods (see [Node state from the instance Pods](#node-state-from-the-instance-pods)),
  the operator reads no node state (`NODE_STATUS_SOURCE=none`): drained or
  cordoned nodes are not detected, so the primary of a node being drained is
  not switched over to a replica first: its `PodDisruptionBudget` holds the
  drain until the Pod is evicted, and a failover follows. Node labels are not
  resolved in the instance topology: when
  `postgresql.syncReplicaElectionConstraint` or
  `postgresql.synchronous.nodeFailureDomainKeys` relies on them, the
  `Cluster` carries the `TopologyUnavailable` condition and a Warning Event,
  the webhook warns about them, and the failure domain constraints of the
  synchronous replicas are not applied. Pod failure domain keys keep
  working, and `status.topology` still counts the nodes used from the
  instance Pods. The operator log says so at startup, every `Cluster` gets a
  Warning Event and a webhook warning when it is created. The `nodes` rules
  can be removed from the `ClusterRole`.

The effective state of each API is logged at startup under
`Kubernetes system metadata`.
## Node state from the instance Pods

The operator reacts to the state of the nodes: the primary of a node that is
cordoned or carries one of the `DRAIN_TAINTS` is switched over to a replica
before the node is drained, replicas on such nodes are not chosen as the new
primary, and the node labels named by
`.spec.postgresql.synchronous.nodeFailureDomainKeys` or the legacy
`.spec.postgresql.syncReplicaElectionConstraint.nodeLabelsAntiAffinity` define
the failure domains of the synchronous replicas. It reads that state from the
cluster-scoped `Node` API when it has access to it (`API_NODE_ENABLED`), which
an installation confined to a namespace has not.

`NODE_STATUS_SOURCE=pods` reads the same facts from the instance Pods instead,
where a node bridge echoes them. The bridge holds the `Node` rights, the
operator does not need them any more, and the `nodes` rules can be removed
from its role. The operator keeps every decision; the bridge only publishes
facts. It expects, on each instance Pod:

| Key on the Pod | Kind | Value | Meaning |
|---|---|---|---|
| `<prefix>unschedulable` | label or annotation | `"true"`, absent otherwise | the Node is cordoned (`spec.unschedulable`) |
| `<prefix>taints` | annotation | JSON array of the Node taints the bridge copies (`key`, `effect`, `value`, `timeAdded`), absent when none | matched against `DRAIN_TAINTS` on the key, as the Node taints are |
| `<labelPrefix><node label key>` | label | the Node label value | resolves the node failure domain keys and `nodeLabelsAntiAffinity` |
| `<prefix>eviction-requested` | annotation | RFC 3339 time of the first eviction request received for the Pod | on the primary, and when not older than `NODE_STATUS_EVICTION_MAX_AGE`, triggers the same switchover as a draining node |

`<prefix>` is `NODE_STATUS_POD_KEY_PREFIX` and `<labelPrefix>` is
`NODE_STATUS_LABEL_PREFIX`. The bridge must copy at least the taint keys listed
in `DRAIN_TAINTS`, and the node labels the Clusters name, under the same keys
or under `<labelPrefix>`. Kubernetes 1.35 and later copy
`topology.kubernetes.io/zone` and `topology.kubernetes.io/region` onto every
Pod at binding by itself (`PodTopologyLabelsAdmission`); other keys, such as
`kubernetes.io/hostname`, need the bridge. An absent key means that the node is
not cordoned or tainted, or that the bridge has not echoed the state yet: the
operator cannot tell the two apart, and behaves as with `none` until the keys
arrive. No new watch is needed, the instance Pods already wake their Cluster
up on any change.

[NodeEcho](https://github.com/fyannk/NodeEcho) is one such bridge. This rule
pair echoes what the operator reads, and holds the eviction of a primary with
an HTTP 429, as a PodDisruptionBudget does, until the operator has switched it
over, which covers the case where the Cluster disables its PodDisruptionBudget:

```yaml
rules:
- name: cnpg-instances
  podSelector: {matchLabels: {cnpg.io/podRole: instance}}
  unschedulable: {}
  taints: {copy: [node.kubernetes.io/unschedulable, ToBeDeletedByClusterAutoscaler,
                  karpenter.sh/disrupted, karpenter.sh/disruption]}
  labels: {copy: [kubernetes.io/hostname, topology.kubernetes.io/zone]}
- name: cnpg-primaries
  podSelector: {matchLabels: {cnpg.io/podRole: instance, cnpg.io/instanceRole: primary}}
  eviction: {response: deny, message: "switch over before evicting the primary"}
```

with `NODE_STATUS_SOURCE=pods` and
`NODE_STATUS_POD_KEY_PREFIX=nodeecho.fyannk.dev/` on the operator. Keep the
`deny` answer for primaries only: the operator moves the primary role away,
nobody moves a replica, and a replica nobody can evict blocks the drain. A
single-instance Cluster has no replica to switch over to: its eviction is held
as its PodDisruptionBudget holds it today.

`NODE_STATUS_SOURCE=none`, which is what an unset `NODE_STATUS_SOURCE` resolves
to when the Node API is disabled or not accessible, reads no node state: see
the `Node` entry of [Restricted installations](#restricted-installations) for
what is lost and how it is reported.

## Defaulting and validation without admission webhooks

CloudNativePG normally defaults and validates its resources through admission
webhooks. When the webhooks are not installed, or are temporarily unable to
respond, the operator performs the same defaulting and validation during
reconciliation as a fallback.

If a resource fails validation, reconciliation stops and the reason is surfaced
on the resource status: `Cluster` and `Backup` move to the `Invalid cluster
definition` and `invalid backup definition` phases respectively, while `Pooler`
and `ScheduledBackup` report it in `.status.error`. The status lists only the
offending field paths; the full validation error, which may contain field
values, is written to the operator logs only. Correcting the spec clears the
error and resumes reconciliation.

Validation performed during reconciliation honors the `cnpg.io/validation:
disabled` annotation, exactly as the admission webhook does.

## Profiling tools

The operator can expose a pprof HTTP server on `localhost:6060`.
To enable it, edit the operator deployment and add the flag
`--pprof-server=true` to the container args:

```shell
kubectl edit deployment -n cnpg-system cnpg-controller-manager
```

Add `--pprof-server=true` to the args list, for example:

```yaml
      containers:
      - args:
        - controller
        - --leader-elect
        - --config-map-name=cnpg-controller-manager-config
        - --secret-name=cnpg-controller-manager-config
        - --log-level=info
        - --pprof-server=true # relevant line
        command:
        - /manager
```

After saving, the deployment will roll out and the new pod will
have the pprof server enabled.

:::info[Important]
    The pprof server only serves plain HTTP on port `6060`.
:::

To access the pprof endpoints from your local machine, use
port-forwarding:

```shell
kubectl port-forward -n cnpg-system deploy/cnpg-controller-manager 6060
curl -sS http://localhost:6060/debug/pprof/
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30
```

You can also access pprof using the browser at [http://localhost:6060/debug/pprof/](http://localhost:6060/debug/pprof/).

:::warning
    The example above uses `kubectl port-forward` for local testing only.
    This is **not** the intended way to expose the feature in production.
    Treat pprof as a sensitive debugging interface and never expose it publicly.
    If you must access it remotely, secure it with proper network policies and access controls.
:::
