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
`FILTERED_CACHE_MODE` | Restricts the operator cache to the objects it manages, the ones carrying the `cnpg.io/reconcile=true` label. `off` (the default) caches every object of the watched kinds; `strict` caches only the managed objects and reads the others from the API server when a reconciliation needs them; `reload` is `strict` plus a metadata-only watch on the Secrets and ConfigMaps carrying `cnpg.io/reload=true`, so that changing them still triggers a reconciliation. See [Operator cache](#operator-cache).
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

## Operator cache

The operator keeps an informer cache of the kinds it works with: Pods,
PersistentVolumeClaims, Jobs, Services, Secrets, ConfigMaps, Leases,
PodDisruptionBudgets, Deployments, ServiceAccounts, Roles, RoleBindings, and
PodMonitors and VolumeSnapshots when their APIs exist. By default the cache
holds every object of those kinds in the namespaces the operator watches,
whoever created them: on a shared cluster, the Secrets and ConfigMaps of
other workloads can make the operator's memory grow far beyond what its own
objects need.

`FILTERED_CACHE_MODE` restricts the cache to the objects the operator
manages, the ones carrying the `cnpg.io/reconcile=true` label that the
operator sets on everything it creates:

- `off`, the default, keeps the behavior described above;
- `strict` caches only the labeled objects. An object the operator needs
  but did not create, such as a certificate, a credential or custom
  monitoring queries referenced by a `Cluster`, is read from the API server
  each time a reconciliation needs it, and changes to it are only noticed at
  the next reconciliation. Label such objects with `cnpg.io/reconcile=true`
  to cache them;
- `reload` is `strict` plus a metadata-only watch on the Secrets and
  ConfigMaps carrying `cnpg.io/reload=true` without the cache label, so that
  changing them triggers a reconciliation exactly as in `off` mode. The
  metadata of each such object costs a few hundred bytes, whatever its size.

For an object referenced by a `Cluster` without the `cnpg.io/reconcile=true`
label, such as a Secret holding a certificate:

| | `off` | `strict` | `reload` |
|---|---|---|---|
| Read | from the cache | from the API server, at each read | from the API server, at each read |
| Change, object labeled `cnpg.io/reload=true` | reconciliation | next reconciliation | reconciliation |
| Change, object without `cnpg.io/reload` | next reconciliation | next reconciliation | next reconciliation |

With the `cnpg.io/reconcile=true` label, the object is read from the cache
in every mode, and, labeled `cnpg.io/reload=true` too, its changes trigger a
reconciliation in every mode.

In the operator namespace, Secrets, ConfigMaps and Services are always
cached in full, because the operator reads objects it did not create there:
the plugin Services and their TLS Secrets, the pull secret and the default
monitoring queries. With `WATCH_NAMESPACE` set, this requires the operator
namespace to be in the list; otherwise these objects are read from the API
server, and, as with the mode `off`, the plugin Services are not watched.

A filtered cache does not hold an object without the label, so an
installation upgraded from a version that did not set it must get the label
on its existing objects before a filtered mode is enabled: run this version
with the mode `off` until every Cluster, Pooler and DatabaseRole has been
reconciled once. Their reconcilers add the label to the objects they
manage; the pod template of each Pooler Deployment gets it too, which rolls
the Pooler out once. Enabling a filtered mode before that leaves the
existing Clusters without visible instances and volumes: the operator reports
them as unrecoverable, keeps reconciling them in a tight loop without touching
them, and only recovers once it runs with the mode `off` again long enough to
label their objects.

With a filtered mode, the Pods matched by `podSelectorRefs` must carry the
`cnpg.io/reconcile=true` label, typically from the pod template of the
application, to be seen by the operator; unlabeled Pods resolve to nothing,
and the Cluster webhook returns a warning on such Clusters.

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
- `Node`: drained or cordoned nodes are not detected, so the primary of a
  node being drained is not switched over to a replica first: its
  `PodDisruptionBudget` holds the drain until the Pod is evicted, and a
  failover follows. Node labels are not resolved in the instance topology:
  when `postgresql.syncReplicaElectionConstraint` or
  `postgresql.synchronous.nodeFailureDomainKeys` relies on them, the
  `Cluster` carries the `TopologyUnavailable` condition and a Warning Event,
  the webhook warns about them, and the failure domain constraints of the
  synchronous replicas are not applied. Pod failure domain keys keep
  working, and `status.topology` still counts the nodes used from the
  instance Pods. The `nodes` rules can be removed from the `ClusterRole`.

The effective state of each API is logged at startup under
`Kubernetes system metadata`.

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
