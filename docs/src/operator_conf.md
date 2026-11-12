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
`CA_SECRET_NAME` | The name of the Secret holding the CA the operator generates for its webhooks, used as given. When unset, `<OPERATOR_DEPLOYMENT_NAME>-ca-secret` when the Deployment name is set, `cnpg-ca-secret` otherwise. See [Several operators in one namespace](#several-operators-in-one-namespace).
`CERTIFICATE_DURATION` | Determines the lifetime of the generated certificates in days. Default is 90.
`CLUSTERS_ROLLOUT_DELAY` | The duration (in seconds) to wait between the roll-outs of different clusters during an operator upgrade. This setting controls the timing of upgrades across clusters, spreading them out to reduce system impact. The default value is `0` which means no delay between PostgreSQL cluster upgrades.
`CREATE_ANY_SERVICE` | When set to `true`, will create `-any` service for the cluster. Default is `false`
`DEFAULT_METADATA` | YAML list of rules putting labels and annotations on the objects of every `Cluster`, `Pooler`, `ScheduledBackup`, and `Backup`, or of the kinds of resources a rule names, when the resource does not set the key. See [Default and mandatory labels and annotations](labels_annotations.md#default-and-mandatory-labels-and-annotations).
`NODE_STATUS_SOURCE` | Where the operator learns the state of the nodes (cordon, drain taints, node labels): `api` reads the cluster-scoped `Node` API; `pods` reads the keys a node bridge echoes onto the instance Pods, see [Node state from the instance Pods](#node-state-from-the-instance-pods); `none` reads nothing, see the same section for what is lost. Unset, it follows `API_NODE_ENABLED`: `api` when the operator reads the Node API, `none` when the API is disabled or not accessible. An explicit `api` while the Node API is disabled refuses to start.
`NODE_STATUS_POD_KEY_PREFIX` | With `NODE_STATUS_SOURCE=pods`, the prefix of the keys read on the instance Pods: `<prefix>unschedulable`, `<prefix>taints` and `<prefix>eviction-requested`. Required with `pods`.
`NODE_STATUS_LABEL_PREFIX` | With `NODE_STATUS_SOURCE=pods`, the prefix under which the node labels are copied onto the instance Pods: the node label `<key>` is read from the Pod label `<prefix><key>`. Empty by default: the node label keys themselves.
`NODE_STATUS_EVICTION_MAX_AGE` | With `NODE_STATUS_SOURCE=pods`, the age in seconds after which an eviction request echoed onto a primary Pod no longer triggers a switchover. Default `600`; `0` means no limit.
`DRAIN_TAINTS` | Specifies the taint keys that should be interpreted as indicators of node drain. By default, it includes the taints commonly applied by [kubectl](https://kubernetes.io/docs/concepts/scheduling-eviction/taint-and-toleration/), [Cluster Autoscaler](https://github.com/kubernetes/autoscaler), and [Karpenter](https://github.com/aws/karpenter-provider-aws): `node.kubernetes.io/unschedulable`, `ToBeDeletedByClusterAutoscaler`, `karpenter.sh/disrupted`, `karpenter.sh/disruption`.
`ENABLE_INSTANCE_MANAGER_INPLACE_UPDATES` | When set to `true`, enables in-place updates of the instance manager after an update of the operator, avoiding rolling updates of the cluster (default `false`)
`ENABLE_WEBHOOK_NAMESPACE_SUFFIX` | When set to `true`, the operator looks up its `MutatingWebhookConfiguration` and `ValidatingWebhookConfiguration` under names suffixed with `-<OPERATOR_NAMESPACE>` (e.g. `cnpg-mutating-webhook-configuration-cnpg-team-a` and `cnpg-validating-webhook-configuration-cnpg-team-a`). Use this when running multiple namespaced operator instances on the same cluster to avoid name collisions. The operator does not create the suffixed configurations; you must create them before starting the operator with this flag enabled. If `OPERATOR_NAMESPACE` is not set when this flag is enabled, the operator will refuse to start. An explicit `MUTATING_WEBHOOK_CONFIGURATION_NAME` or `VALIDATING_WEBHOOK_CONFIGURATION_NAME` is used as given, without the suffix. Default is `false`.
`EXPIRING_CHECK_THRESHOLD` | Determines the threshold, in days, for identifying a certificate as expiring. Default is 7.
`INCLUDE_PLUGINS` | A comma-separated list of plugins to be always included in the Cluster's reconciliation.
`INHERITED_ANNOTATIONS` | List of annotation names that, when defined in a `Cluster` metadata, will be inherited by all the generated resources, including pods
`INHERITED_LABELS` | List of label names that, when defined in a `Cluster` metadata, will be inherited by all the generated resources, including pods
`INSTANCES_ROLLOUT_DELAY` | The duration (in seconds) to wait between roll-outs of individual PostgreSQL instances within the same cluster during an operator upgrade. The default value is `0`, meaning no delay between upgrades of instances in the same PostgreSQL cluster.
`KUBERNETES_CLUSTER_DOMAIN` | Defines the domain suffix for service FQDNs within the Kubernetes cluster. If left unset, it defaults to "cluster.local".
`MANDATORY_METADATA` | YAML list of rules putting labels and annotations on the objects of every `Cluster`, `Pooler`, `ScheduledBackup`, and `Backup`, or of the kinds of resources a rule names, replacing the value set by the resource. See [Default and mandatory labels and annotations](labels_annotations.md#default-and-mandatory-labels-and-annotations).
`MANAGE_WEBHOOK_CONFIGURATIONS` | When set to `true` (default), the operator injects its CA bundle into the `MutatingWebhookConfiguration` and `ValidatingWebhookConfiguration`. Set to `false` when the CA bundle is injected externally, for example by cert-manager's CA injector or by GitOps. This is independent of the webhook serving certificate, which the operator always manages when it owns its PKI.
`METRICS_CERT_DIR` | The directory where TLS certificates for the operator metrics server are stored. When set, enables TLS for the metrics endpoint on port 8080. The directory must contain `tls.crt` and `tls.key` files following standard Kubernetes TLS secret conventions. If not set, the metrics server operates without TLS (default behavior).
`MONITORING_DISCOVERY_ANNOTATIONS` | Comma-separated list of the families of annotations telling monitoring agents how to collect the metrics and the logs of the instance and pooler pods: `prometheus`, `opentelemetry`. Empty by default. See [Collecting metrics and logs without the Prometheus Operator](monitoring.md#collecting-metrics-and-logs-without-the-prometheus-operator).
`LEADER_ELECTION_ID` | The name of the Lease used for the leader election, used as given. When unset, `<OPERATOR_DEPLOYMENT_NAME>.cnpg.io` when the Deployment name is set, `db9c8771.cnpg.io` otherwise. It must be a Pod environment variable, the Lease is taken before the operator configuration objects are read.
`MUTATING_WEBHOOK_CONFIGURATION_NAME` | The name of the `MutatingWebhookConfiguration` the operator injects its CA bundle into, used as given. When unset, `<OPERATOR_DEPLOYMENT_NAME>-mutating-webhook-configuration` when the Deployment name is set, `cnpg-mutating-webhook-configuration` otherwise, with the namespace suffix when `ENABLE_WEBHOOK_NAMESPACE_SUFFIX` is set. See [Webhook certificates and configurations](#webhook-certificates-and-configurations).
`MONITORING_QUERIES_CONFIGMAP` | The name of a ConfigMap in the operator's namespace with a set of default queries (to be specified under the key `queries`) to be applied to all created Clusters
`MONITORING_QUERIES_SECRET` | The name of a Secret in the operator's namespace with a set of default queries (to be specified under the key `queries`) to be applied to all created Clusters
`OPERATOR_DEPLOYMENT_NAME` | The name of the Deployment running the operator. When set, the operator fetches its Deployment by this name instead of searching it by the `app.kubernetes.io/name=cloudnative-pg` label, and names the objects it creates for itself after it: `<name>-ca-secret`, `<name>-webhook-cert`, `<name>-webhook-service`, `<name>-mutating-webhook-configuration`, `<name>-validating-webhook-configuration` and the Lease `<name>.cnpg.io`, each one still overridable by its own setting. When unset, the lookup is by label and every name keeps its historical value. It must be a Pod environment variable. See [Several operators in one namespace](#several-operators-in-one-namespace).
`OPERATOR_IMAGE_NAME` | The name of the operator image used to bootstrap Pods. Defaults to the image specified during installation.
`OPERATOR_STATUS_IDENTITY` | How much the operator says about itself in the `status.operator` block of the Clusters it reconciles: `off` nothing, `version` (the default) its version, `name` its version and its Deployment name, `full` its version, its Deployment name and its namespace. With `name` and `full`, the Deployment name is also the field manager of every write of the operator, visible in the `managedFields` of every object it creates or updates. It must be a Pod environment variable, the field manager is fixed before the operator configuration objects are read. See [Several operators in one namespace](#several-operators-in-one-namespace).
`PGBOUNCER_IMAGE_NAME` | The name of the PgBouncer image used by default for new poolers. Defaults to the version specified in the operator.
`POSTGRES_IMAGE_NAME` | The name of the PostgreSQL image used by default for new clusters. Defaults to the version specified in the operator.
`FILTERED_CACHE_MODE` | Restricts the operator cache to the objects it manages, the ones carrying the `cnpg.io/reconcile=true` label. `off` (the default) caches every object of the watched kinds; `strict` caches only the managed objects and reads the others from the API server when a reconciliation needs them; `reload` is `strict` plus a metadata-only watch on the Secrets and ConfigMaps carrying `cnpg.io/reload=true`, so that changing them still triggers a reconciliation. See [Operator cache](#operator-cache).
`PROPAGATE_PROXY_ENV` | When set to `true` (default), the proxy settings of the operator (`HTTP_PROXY`, `HTTPS_PROXY` and `NO_PROXY`, read from its environment) are set on the PostgreSQL instances, bootstrap included, on the major upgrade jobs and on the PgBouncer poolers, unless the `Cluster` or the `Pooler` defines any of them. Set to `false` to keep the proxy settings to the operator. See [Proxy settings](#proxy-settings).
`PULL_SECRET_NAME` | Name of an additional pull secret to be defined in the operator's namespace and to be used to download images
`SCHEDULED_BACKUP_CRON_FORMAT` | How the `schedule` of a `ScheduledBackup` is read when it has five fields, the standard cron format, instead of the six fields with seconds described in [Backup](backup.md#cron-schedule). `seconds` (default): the five fields are read with the seconds field first, with an admission warning and a `ScheduleFieldCount` Warning event on the object. `seconds-strict`: a five-field schedule is rejected when the `ScheduledBackup` is created, with its six-field equivalent in the message; existing objects can still be updated. `standard`: the five fields are read as standard cron, with the minute field first. Six-field schedules and descriptors such as `@daily` are read the same way whatever the value.
`STANDBY_TCP_USER_TIMEOUT` | Defines the [`TCP_USER_TIMEOUT` socket option](https://www.postgresql.org/docs/current/runtime-config-connection.html#GUC-TCP-USER-TIMEOUT) in milliseconds for replication connections from standby instances to the primary. Default is 5000 (5 seconds). Set to `0` to use the system's default.
`VALIDATING_WEBHOOK_CONFIGURATION_NAME` | The name of the `ValidatingWebhookConfiguration` the operator injects its CA bundle into, used as given. When unset, `<OPERATOR_DEPLOYMENT_NAME>-validating-webhook-configuration` when the Deployment name is set, `cnpg-validating-webhook-configuration` otherwise, with the namespace suffix when `ENABLE_WEBHOOK_NAMESPACE_SUFFIX` is set. See [Webhook certificates and configurations](#webhook-certificates-and-configurations).
`WATCH_NAMESPACE` | Specifies the namespace(s) where the operator should watch for resources. Multiple namespaces can be specified separated by commas. If not set, the operator watches all namespaces (cluster-wide mode).
`WEBHOOK_CERT_DIR` | The directory holding an externally provided certificate for the webhook server, as `apiserver.crt`/`apiserver.key` (what OLM writes) or `tls.crt`/`tls.key` (a Kubernetes TLS Secret, for example from cert-manager). When set, the operator serves the webhooks with that certificate, manages no certificate of its own and leaves the webhook configurations untouched; it refuses to start when the directory holds no complete, readable pair. When unset, the operator manages its own PKI. See [Webhook certificates and configurations](#webhook-certificates-and-configurations).
`WEBHOOK_SERVICE_NAME` | The name of the Service exposing the webhook server, the subject of the serving certificate the operator issues, used as given. When unset, `<OPERATOR_DEPLOYMENT_NAME>-webhook-service` when the Deployment name is set, `cnpg-webhook-service` otherwise. The certificate also carries the historical name, the derived one, the Services of the operator namespace selecting the operator Pods on the webhook port, and the Services the managed webhook configurations point at.

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
## Proxy settings

Behind an egress proxy, the PostgreSQL instances need `HTTP_PROXY`,
`HTTPS_PROXY` and `NO_PROXY` for the traffic that leaves the Kubernetes
cluster, such as WAL archiving and backups to an object store. The operator
usually has these variables already: on OpenShift, Operator Lifecycle Manager
injects the cluster-wide proxy into the operator deployment, and on other
platforms they are set on the operator deployment.

By default (`PROPAGATE_PROXY_ENV` set to `true`), the operator copies its own
proxy settings to the environment of the instance pods and of their jobs
(initialization, join, recovery and major upgrade), and to the `pgbouncer`
container of the poolers. Set it to
`false` to keep the proxy settings to the operator.

- The three variables are read from the environment of the operator, the
  upper case name first and the lower case one as a fallback, as Go does.
  Empty values are not propagated.
- Each setting is written in both spellings (`HTTP_PROXY` and `http_proxy`,
  and so on): Go reads the upper case name first, while curl and some Python
  tooling read the lower case one only.
- When the `env` stanza of a `Cluster`, or the `pgbouncer` container of a
  `Pooler` template, defines any of the six names, in any case, none of the
  operator settings is injected: the resource is in control of its proxy
  settings as a whole, including no proxy at all (an empty value) or a proxy
  URL with credentials read from a Secret through `valueFrom.secretKeyRef`.
  The `envFrom` stanza is not inspected: Kubernetes gives `env` precedence
  over `envFrom` on duplicate names, so an override that comes from a
  ConfigMap or a Secret needs an `env` entry as well.
- The propagated variables are part of the environment of the pods, so a
  change of the operator proxy rolls the clusters and poolers that receive
  them, as a change of `spec.env` or of the pooler template would. The
  `cnpg.io/reconcilePodSpec: disabled` annotation prevents the rollout as
  usual.
- On an operator that already runs behind a proxy, the first start of a
  version with this setting rolls every cluster and pooler once, as the pods
  receive the proxy variables. Set `PROPAGATE_PROXY_ENV` to `false` before
  the upgrade to avoid it.

Keep in mind:

- `NO_PROXY` must cover the Kubernetes API server and the in-cluster traffic
  (service and pod networks, `.svc`, `.cluster.local`), otherwise the instance
  manager reaches the API server through the proxy. The propagated settings
  are the ones the operator itself runs with.
- A proxy URL with credentials is copied in clear text into the pod
  specification, as it already is in the operator deployment. Use the
  `spec.env` override with a Secret for those clusters.
- At startup, the operator logs the settings it propagates, without the
  credentials of the URLs, and warns when a value is not a valid URL.

## Webhook certificates and configurations

The admission webhooks of the operator are served over TLS, through the
`cnpg-webhook-service` Service of the operator namespace, and reached by the API
server through a `MutatingWebhookConfiguration` and a
`ValidatingWebhookConfiguration`, two cluster-scoped objects whose `caBundle`
must trust the serving certificate. The operator supports three ways to run
them.

**The operator manages its PKI and the CA bundle (default).** The operator
generates a CA, stored in the `cnpg-ca-secret` Secret, and a serving
certificate, stored in the Secret mounted at its webhook certificate directory,
`cnpg-webhook-cert` in the manifests, both in its namespace, renews them, and
injects the CA bundle into the two webhook configurations. The serving
certificate is issued for the `cnpg-webhook-service` Service and for every
Service of the namespace that selects the operator Pods on the webhook port,
which the operator lists when it is allowed to. It needs `get` and `patch` on
the webhook configurations, which it looks up under
`cnpg-mutating-webhook-configuration` and `cnpg-validating-webhook-configuration`,
suffixed with the operator namespace when `ENABLE_WEBHOOK_NAMESPACE_SUFFIX` is
set, or under the names given by `MUTATING_WEBHOOK_CONFIGURATION_NAME` and
`VALIDATING_WEBHOOK_CONFIGURATION_NAME` when the configurations follow another
naming convention. All these names, and the one of the webhook Service, follow
the operator Deployment instead when `OPERATOR_DEPLOYMENT_NAME` is set, see
[Several operators in one namespace](#several-operators-in-one-namespace). When a configuration is not found, the operator does not
start: either create it, point the setting at the name in use, or set
`MANAGE_WEBHOOK_CONFIGURATIONS=false` when the CA bundle is injected by
something else.

**The operator manages its PKI, something else injects the CA bundle.** With
`MANAGE_WEBHOOK_CONFIGURATIONS=false`, the operator still generates and renews
its certificates but leaves the webhook configurations untouched: the `caBundle`
is set by the installation, for example by cert-manager's CA injector or by a
GitOps tool. The operator needs no right on the webhook configurations.

**An external certificate (`WEBHOOK_CERT_DIR`).** The serving certificate is
issued outside the operator and mounted in its Pod; the operator manages no
certificate, touches no webhook configuration, and reloads the files when they
change. The directory may hold `apiserver.crt`/`apiserver.key`, the names OLM
writes, or `tls.crt`/`tls.key`, the names of a Kubernetes TLS Secret. The
operator refuses to start, with a message naming the directory and the two
conventions, when neither pair is complete and readable. This is the mode of
an installation where the operator has no cluster-wide right and the webhook
configurations belong to the cluster administration. With cert-manager:

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: cnpg-webhook
  namespace: cnpg-system
spec:
  secretName: cnpg-webhook-cert-manager
  dnsNames:
  - cnpg-webhook-service.cnpg-system.svc
  - cnpg-webhook-service.cnpg-system.svc.cluster.local
  issuerRef:
    name: <issuer>
    kind: Issuer
```

The operator Deployment mounts the Secret and points `WEBHOOK_CERT_DIR` at it:

```yaml
        env:
        - name: WEBHOOK_CERT_DIR
          value: /run/secrets/webhook
        volumeMounts:
        - name: webhook-certificate
          mountPath: /run/secrets/webhook
          readOnly: true
      volumes:
      - name: webhook-certificate
        secret:
          secretName: cnpg-webhook-cert-manager
```

The two webhook configurations carry the CA injector annotation
`cert-manager.io/inject-ca-from: cnpg-system/cnpg-webhook`, so that their
`caBundle` follows the certificate.

## Several operators in one namespace

An operator names the objects it creates for itself, and finds the ones it
needs, with fixed names: the Lease `db9c8771.cnpg.io`, the Secrets
`cnpg-ca-secret` and `cnpg-webhook-cert`, the Service `cnpg-webhook-service`
and the two webhook configurations; it finds its own Deployment, the owner of
its Secrets, by the `app.kubernetes.io/name=cloudnative-pg` label. Two
operators installed in the same namespace would therefore contend for one
Lease, renew each other's CA and find more than one Deployment.

`OPERATOR_DEPLOYMENT_NAME` makes every one of these names follow the
Deployment: with `cnpg-toto`, the operator fetches the Deployment `cnpg-toto`,
takes the Lease `cnpg-toto.cnpg.io`, stores its CA in `cnpg-toto-ca-secret`
and its serving certificate in `cnpg-toto-webhook-cert`, issues it for
`cnpg-toto-webhook-service`, and injects the CA bundle into
`cnpg-toto-mutating-webhook-configuration` and
`cnpg-toto-validating-webhook-configuration`. Each name can still be set on
its own (`LEADER_ELECTION_ID`, `CA_SECRET_NAME`, `WEBHOOK_SERVICE_NAME`,
`MUTATING_WEBHOOK_CONFIGURATION_NAME`, `VALIDATING_WEBHOOK_CONFIGURATION_NAME`);
the serving Secret is the one mounted at the webhook certificate directory of
the Deployment, whatever its name, and the certificate also covers the
Services of the namespace that select the Pods of the Deployment on the
webhook port. A Deployment running one operator per watched namespace then
needs three instance-specific values: the identity, the watched namespace and
the Secret mount.

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: cnpg-toto
  namespace: operators
spec:
  template:
    spec:
      containers:
      - name: manager
        args:
        - controller
        - --leader-elect
        - --config-map-name=cnpg-toto-config
        - --secret-name=cnpg-toto-config
        - --webhook-port=9443
        env:
        - name: OPERATOR_NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
        - name: OPERATOR_DEPLOYMENT_NAME
          value: cnpg-toto
        - name: WATCH_NAMESPACE
          value: toto
        volumeMounts:
        - mountPath: /run/secrets/cnpg.io/webhook
          name: webhook-certificates
      volumes:
      - name: webhook-certificates
        secret:
          secretName: cnpg-toto-webhook-cert
          optional: true
```

The installation provides the Service selecting the Pods of that Deployment on
the webhook port, under the derived name or any other, and the two webhook
configurations pointing at it with a `namespaceSelector` on the watched
namespace. The Secret mount, the identity
and `LEADER_ELECTION_ID` must be Pod environment variables, not entries of the
operator configuration objects, which are read after the Lease is taken.

Setting `OPERATOR_DEPLOYMENT_NAME` on an existing installation renames its
Lease and its two Secrets: scale the Deployment to zero replicas before
changing it, otherwise the Pod being replaced and the new one hold two
different Leases during the rollout. Without the variable, nothing changes:
the lookup is by label and every name keeps its historical value.

A `Cluster` says which operator reconciled it last in `status.operator`, as
far as `OPERATOR_STATUS_IDENTITY` lets the operator tell: the version alone by
default, the Deployment name with `name`, the namespace too with `full`, and
nothing with `off`. A namespace watched by one instance can then read which
one manages its Clusters without any access to the operator namespace, while
a cluster-wide installation keeps its namespace to itself. With `name` and
`full`, the Deployment name is also the field manager of every write of the
operator: `kubectl get --show-managed-fields` on any object it created or
updated names the instance and the time of its last write. The
`cnpg.io/operatorVersion` annotation on the created ConfigMaps, Secrets and
Jobs and the `cloudNativePGCommitHash` status field are written whatever the
mode. `kubectl cnpg status` prints the block as one `Operator` line of its
summary, `1.30.1`, `cnpg-toto (1.30.1)` or `operators/cnpg-toto (1.30.1)`.

```yaml
status:
  operator:
    version: 1.30.1
    name: cnpg-toto
    namespace: operators
```

The warning above still applies: the operators share the CRDs, and an upgrade
across several concurrently running operators is not guaranteed to be
backwards compatible.

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

The rules that compare an update of a `Cluster` with its previous state do not
depend on the webhooks either: the `Cluster` CRD carries them as validation
rules, and the API server enforces them on every update. They refuse a
PostgreSQL major version downgrade and a major upgrade to a version below 16,
a decrease of `.spec.storage.size`, `.spec.walStorage.size` or of a tablespace
size, the removal of `walStorage`, of the `tablespaces` section or of a
tablespace, `wal_level` set to `minimal` on an existing cluster, a change of
`.spec.replicationSlots.highAvailability.slotPrefix` while the feature is
enabled, the removal of `synchronizeReplicas` while enabled, a change of
`postgresUID` or `postgresGID`, a change of the replica role while a switch to
replica cluster is in progress, and a change of `imageName` together with
PostgreSQL parameters when `primaryUpdateMethod` is `switchover`. The
`cnpg.io/validation: disabled` annotation does not bypass these rules.

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
