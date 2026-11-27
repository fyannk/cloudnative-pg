---
id: fork_differences
sidebar_position: 55
title: Differences from CloudNativePG
---

# Differences from CloudNativePG
<!-- SPDX-License-Identifier: CC-BY-4.0 -->

This build is an [unofficial fork](installation_upgrade.md#unofficial-fork-builds)
of CloudNativePG 1.30.2. It uses the same object names, needs no permission
the upstream release manifest does not grant, and accepts the same manifests
except in the cases of [What is stricter than upstream](#what-is-stricter-than-upstream).
Its release manifest can replace the one of an upstream 1.30.2 operator in
place; the Helm chart and the upstream OLM package cannot be moved to it that
way, see [Unofficial fork builds](installation_upgrade.md#unofficial-fork-builds).
This page lists what behaves differently from upstream, starting with what
changes without any new setting.

## What changes without opting in

### On the objects of existing clusters

| Change | Effect when the operator is replaced | Details |
|---|---|---|
| The objects the operator manages carry the `cnpg.io/reconcile=true` label | Existing objects are patched in place, with no restart, except the primary `Lease`, the `FailoverQuorum`, the pull Secret of a `Pooler` and the certificate Secrets of a `DatabaseRole`, which are left as they are. Each `Pooler` rolls out once, as it does on any operator upgrade, unless it carries `cnpg.io/reconcilePodSpec: disabled`. A managed Service with `updateStrategy: replace` is deleted and created again, as at any change of the operator version | [Operator cache](operator_conf.md#operator-cache) |
| Every `Cluster` reports the operator version in `status.operator.version` | One status write per `Cluster`, and `kubectl cnpg status` prints it. `OPERATOR_STATUS_IDENTITY=off` disables it. With the upstream CRDs, which is not supported, the field is dropped by the API server and written again at every reconciliation | [Several operators in one namespace](operator_conf.md#several-operators-in-one-namespace) |
| The proxy settings of the operator (`HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY`) are set on the instances and the poolers | **When the operator Deployment has such variables, every instance and every pooler receives them, with a restart, in the rollout of the upgrade or on its own when the upgrade keeps the Pods. From then on, the traffic of the instances, WAL archiving and backups included, goes through that proxy unless `NO_PROXY` matches the destination.** A proxy set through `envFrom` is replaced. Nothing changes when the operator Deployment has none. `PROPAGATE_PROXY_ENV=false` disables it | [Proxy settings](operator_conf.md#proxy-settings) |
| `resizeInUseVolumes: false` in the section of the WAL or of a tablespace prevents the resize of its volumes, which upstream ignores. `.spec.storage` still has to allow it, as upstream, except for a section with `autoGrow` | No volume is resized that upstream would not resize. A `false` left in such a section now holds back a resize that upstream performs | [Storage](storage.md#using-the-volume-expansion-kubernetes-feature) |
| The CA, server and replication Secrets created by the operator get the inherited labels and annotations of the `Cluster`, as the Secrets and the pull Secrets of the Poolers do | One patch per Secret, again at each operator upgrade for the `cnpg.io/operatorVersion` annotation | [Labels and annotations](labels_annotations.md) |

### In the behavior of the instances

| Change | Details |
|---|---|
| When it exits, the instance manager names the volumes nothing can be written to anymore in the termination message of the container. The exit code and the failover are unchanged; `autoGrow` uses it | [The full-disk stop of an instance](storage.md#the-full-disk-stop-of-an-instance) |
| Every answer of the status endpoint and every exit of the instance manager reads the usage of the filesystems of the volumes, which `status` reports as `volumeUsages` to the operator. A read that does not answer is abandoned after a timeout | [Storage](storage.md#growing-the-volumes-automatically) |
| A primary being demoted skips its smart shutdown or escalates it to a fast one, also when its Pod is deleted or evicted: the open sessions and a backup running on that primary are cut at once, instead of after `.spec.smartShutdownTimeout` | [Shutdown of the primary during a switchover](instance_manager.md#shutdown-of-the-primary-during-a-switchover) |

### In the behavior of the operator

| Change | Details |
|---|---|
| The optional APIs (`PodMonitor`, `VolumeSnapshot`, `ClusterImageCatalog`, `Node`) are probed once, when the operator starts. Installing one later, or granting a missing permission, requires restarting the operator; upstream notices the `PodMonitor` API at the next reconciliation. One watched namespace without the permission disables the API in all of them. An API found missing or forbidden is logged as a warning, with the namespace; any other error of a probe, or a probe longer than 30 seconds, stops the start | [Restricted installations](operator_conf.md#restricted-installations) |
| The operator ConfigMap and Secret are read before the manager starts, so `WATCH_NAMESPACE` and `METRICS_CERT_DIR` set there take effect, as documented; upstream reads them too early. They override the environment of the operator Deployment, an empty value included. The startup log gives the watched namespaces | [Available options](operator_conf.md#available-options) |
| Without the `VolumeSnapshot` API, an existing `Backup` with the `volumeSnapshot` method is marked as failed and a `ScheduledBackup` no longer reports it in `status.error`; upstream leaves them invalid. Creating one, or a `Cluster` recovering from a snapshot, is refused, with warnings on the other uses of a missing API; an update that does not touch the field is accepted. A `ClusterImageCatalog` refused for the same reason asks to restart the operator | [Restricted installations](operator_conf.md#restricted-installations) |
| When `OPERATOR_NAMESPACE` is unset, the operator namespace is the one of its service account, so the operator ConfigMap and Secret of that namespace are read; upstream ignores them. The release manifest sets it | |
| The operator looks up its Deployment at startup and every hour, to maintain the webhook certificate | [Webhook certificates and configurations](operator_conf.md#webhook-certificates-and-configurations) |
| `metadata.managedFields` is not kept in the cache of the operator: it is missing from the objects given to the plugins and from the `cnpg.io/clusterManifest` annotation of the VolumeSnapshots, which gains `status.operator` | |
| The status of a `Cluster` is compared with a copy taken before the reconciliation, so a condition changed on its own is written | |
| A `Cluster` whose volumes are still being resized two minutes after the request, possibly waiting for a restart of their Pod, gets the `VolumeResizePending` condition and a Warning event naming the instances to restart | [Storage](storage.md#using-the-volume-expansion-kubernetes-feature) |
| A `Cluster` whose volume expansion is refused, by any `403` answer to the patch of a PVC, keeps being reconciled, with the `VolumeExpansionRefused` condition, and the expansion is retried every minute; upstream stops there, so the operations pending on such a `Cluster` resume and its other PVCs grow when the operator is replaced |
| While a primary stopped for a full disk prevents the failover, the PVCs are resized to the size in the `Cluster`, once the phase is recorded; upstream resizes nothing then | [When the expansion is refused](storage.md#when-the-expansion-is-refused) |
| The creation of the objects of a `Cluster` goes on after a failure of the default monitoring queries ConfigMap, reported by a `DefaultMonitoringQueriesError` Warning event | |
| A `Pooler` whose Deployment cannot run its Pods (quota, rollout past its deadline) gets the message of the Deployment as its phase reason. The phase is the upstream one | |
| A `Cluster` whose major upgrade Job failed gets the failure, and how to roll back, as its phase reason. The phase is the upstream one | |
| `kubectl cnpg status` shows, for an instance that does not answer, why its Pod does not run | |
| A `ScheduledBackup` with a five-field schedule gets a `ScheduleFieldCount` Warning event; schedule descriptors such as `@daily` do not raise the admission warning anymore | [Cron schedule](backup.md#cron-schedule) |
| New events, see [Events](#events) | |
| New metrics: `cnpg_pgbouncer_up`, `cnpg_disk_*`, `cnpg_operator_*`. The settings of the operator are published by `cnpg_operator_config_info`, the ones read from the operator Secret being redacted. A metric of a custom query named as a `cnpg_disk_*` one is skipped, with a warning in the log of the instance | [Monitoring](monitoring.md) |
| `kubectl cnpg report operator` finds the operator by its Deployment, and only includes its own webhook configurations | [Plugin](kubectl-plugin.md) |

### Events

The events the upstream operator does not record come from their own
components, named after the upstream ones with the `-diagnostics` suffix, so
they never use the event budget of the others. The states below are reported
when they begin, not at every reconciliation.

| Object | Warning | Normal |
|---|---|---|
| `Cluster` | `ReconciliationFailed`, `QuotaExceeded`, `AdmissionDenied`, `DefaultMonitoringQueriesError`, `InstanceCreationFailed`, `MajorUpgradeFailed`, `NodeStatusUnavailable`, `TopologyUnavailable`, and the reasons of the `VolumeExpansionRefused`, `VolumeResizePending` and `VolumeAutoGrowStopped` conditions | `InstanceCreated`, `VolumeAutoGrow`, and the reasons closing those conditions |
| `Pooler` | `ReconciliationFailed`, `QuotaExceeded`, `AdmissionDenied` | |
| `Backup` | `ReconciliationFailed`, `QuotaExceeded`, `AdmissionDenied`, `VolumeSnapshotError` | `BackupWaiting` |
| `ScheduledBackup` | `ReconciliationFailed`, `QuotaExceeded`, `AdmissionDenied`, `BackupDelayed`, `ScheduleFieldCount` | |

`ReconciliationFailed`, `QuotaExceeded` and `AdmissionDenied` report an error
ending a reconciliation. Errors that a retry settles, the deletion of the
namespace and the deletion of the object are not reported.

Two upstream texts differ: the switchover away from an unschedulable node names
what was read, when the node state does not come from the `Node` API, and the
`InvalidOwnership` event of a `Pooler` names the objects it does not own.

## What is stricter than upstream

- A validating webhook, `vclusterscale.cnpg.io`, checks the writes of the
  `scale` subresource: a `Cluster` holding an invalid field cannot be scaled
  with `kubectl scale` until it is fixed. As with the other webhooks of the
  operator, the scale is refused while the operator does not answer, its own
  rollout included. The webhook reads the `Cluster`: with a namespaced Role, the
  scale is refused in the namespaces the operator has no permission on, unless
  a `namespaceSelector` leaves them out.
- The operator Role does not grant the `status` subresource of ConfigMaps and
  Secrets, which nothing used.

## What must be enabled

These features change nothing until they are configured, except that the
instances always report the usage of their volumes, and that an optional API
found missing at startup is disabled as its setting would:

| Feature | Setting |
|---|---|
| Labels and annotations per kind of object, default and mandatory values | `.spec.metadataPropagation`, `DEFAULT_METADATA`, `MANDATORY_METADATA`: see [Labels and annotations](labels_annotations.md#labels-and-annotations-per-kind-of-object) |
| Monitoring discovery annotations on the Pods | `MONITORING_DISCOVERY_ANNOTATIONS`: see [Monitoring](monitoring.md#collecting-metrics-and-logs-without-the-prometheus-operator) |
| Automatic growth of the volumes | `autoGrow` in a storage section: see [Storage](storage.md#growing-the-volumes-automatically) |
| Default `StorageClass` for new clusters | `DEFAULT_STORAGE_CLASS` |
| Cache restricted to the managed objects | `FILTERED_CACHE_MODE`: see [Operator cache](operator_conf.md#operator-cache) |
| Running without some APIs or permissions | `API_*_ENABLED`: see [Restricted installations](operator_conf.md#restricted-installations) |
| Node state without the `Node` API | `NODE_STATUS_SOURCE` and `NODE_STATUS_UNAVAILABLE_POLICY`: see [Node state from the instance Pods](operator_conf.md#node-state-from-the-instance-pods) |
| Standard five-field cron schedules | `SCHEDULED_BACKUP_CRON_FORMAT`: see [Cron schedule](backup.md#cron-schedule) |
| External webhook certificate, named webhook configurations, several operators in one namespace | `WEBHOOK_CERT_DIR`, `OPERATOR_DEPLOYMENT_NAME` and the related settings: see [Webhook certificates and configurations](operator_conf.md#webhook-certificates-and-configurations) |

## Replacing an upstream 1.30.2 operator

These steps apply to an installation made with the release manifest.

1. Check whether the upstream operator Deployment has proxy variables:

   ```sh
   kubectl get deployment -n cnpg-system cnpg-controller-manager -o json |
     jq -r '.spec.template.spec.containers[].env // [] | .[].name
       | select(test("^(https?|no)_proxy$"; "i"))'
   ```

   If it does, either check that `NO_PROXY` covers what the instances reach
   directly, the object stores first, or set `PROPAGATE_PROXY_ENV=false` in the
   operator configuration. A password in a proxy URL is copied in clear text
   into the Pods. The clusters that take their proxy from `envFrom` need an
   `env` entry to keep it:

   ```sh
   kubectl get clusters.postgresql.cnpg.io -A -o json |
     jq -r '.items[] | select(.spec.envFrom) | "\(.metadata.namespace)/\(.metadata.name)"'
   ```
2. Check `WATCH_NAMESPACE` and `METRICS_CERT_DIR` in the operator ConfigMap
   and Secret. The upstream operator ignores them there, this one applies
   them. Remove the ones that must not take effect; with `METRICS_CERT_DIR`,
   the directory must hold `tls.crt` and `tls.key`:

   ```sh
   kubectl get configmap,secret -n cnpg-system -o json \
     cnpg-controller-manager-config --ignore-not-found |
     jq -r '(.items // [.])[] | .kind as $k | .data // {} | keys[]
       | select(. == "WATCH_NAMESPACE" or . == "METRICS_CERT_DIR") | "\($k): \(.)"'
   ```
3. Find the clusters whose WAL or tablespace sections hold back a resize from
   now on, with `resizeInUseVolumes: false`, which the upstream operator
   ignores:

   ```sh
   kubectl get clusters.postgresql.cnpg.io -A -o json |
     jq -r '.items[] | select([.spec.walStorage, .spec.tablespaces[]?.storage]
         | any(.resizeInUseVolumes? == false))
       | "\(.metadata.namespace)/\(.metadata.name)"'
   ```
4. Find the PVCs that do not request the size of their `Cluster`. A `Cluster`
   stopped by a refused expansion is reconciled again: its pending operations
   resume and its other PVCs grow. To keep the sizes, set
   `resizeInUseVolumes: false` in its `.spec.storage` first:

   ```sh
   kubectl get pvc -A -l cnpg.io/cluster -o custom-columns=\
   NS:.metadata.namespace,NAME:.metadata.name,REQUEST:.spec.resources.requests.storage
   kubectl get clusters.postgresql.cnpg.io -A -o custom-columns=\
   NS:.metadata.namespace,NAME:.metadata.name,SIZE:.spec.storage.size,WAL:.spec.walStorage.size
   ```
5. When the webhook configurations are customised, adapt the customisation to
   the new `vclusterscale.cnpg.io` entry: a CA bundle or a `namespaceSelector`
   set by name must be set on it too, and one set by position must be checked,
   as the entry is inserted between the existing ones.
6. Apply the manifest of the fork release, not only its image: it brings the
   CRDs with the new fields and rules, the webhook of the `scale` subresource
   and two ClusterRoles. `kubectl scale` on a `Cluster` is refused until the
   new operator answers.
7. Label the primary `Lease` of each existing `Cluster`, once the operator is
   replaced. The operator creates the `Lease` of a new `Cluster` with the
   label, but never writes to an existing one, and no switchover adds it.
   This is only needed for a filtered cache, and must be done before enabling
   it (see [Operator cache](operator_conf.md#operator-cache)):

   ```sh
   kubectl label lease <cluster-name> -n <namespace> cnpg.io/reconcile=true
   ```

The instances are upgraded as in any operator upgrade, see
[Upgrades](installation_upgrade.md#upgrades).

## Going back to the upstream operator

Apply the upstream manifest, not only its image: the fork manifest declares
the `vclusterscale.cnpg.io` webhook, which the upstream operator does not
serve, and `kubectl scale` on a `Cluster` fails as long as it is declared.

Before going back:

- set the size of every volume grown by `autoGrow` in its storage section to
  the grown size, then remove `autoGrow`: the upstream operator creates the
  PVCs of new instances at the size in the `Cluster`;
- for every section whose size is above the one its PVCs request, either let
  the expansion succeed or set `resizeInUseVolumes: false` in `.spec.storage`:
  under the upstream operator, a refused expansion stops the reconciliation of
  the `Cluster`, and a resize held back by the flag of the WAL or tablespace
  section is applied. This finds the clusters concerned by the first case:

  ```sh
  kubectl get clusters.postgresql.cnpg.io -A -o json |
    jq -r '.items[] | select(.status.conditions[]?
        | .type == "VolumeExpansionRefused" and .status == "True")
      | "\(.metadata.namespace)/\(.metadata.name)"'
  ```
- with `SCHEDULED_BACKUP_CRON_FORMAT=standard`, rewrite the five-field
  schedules with six fields, see [Cron schedule](backup.md#cron-schedule);
- with `WEBHOOK_CERT_DIR` and certificate files not named as upstream expects,
  or with `OPERATOR_DEPLOYMENT_NAME`, restore the upstream names first.

What stays behind:

- the `cnpg.io/reconcile` label, and the labels and annotations set by the
  rules, which are harmless;
- the conditions of the fork in the status of the clusters, which nothing
  updates anymore: an alert on `VolumeExpansionRefused`, `VolumeResizePending`
  or `VolumeAutoGrowStopped` keeps firing if the condition was `True`;
- the `cnpg-scale-editor-role` and `cnpg-cluster-status-editor-role`
  ClusterRoles, which can be deleted.

When the upstream CRDs are applied, the fields of the fork are dropped from the
specification of the `Cluster`, `Pooler`, `Backup` and `ScheduledBackup`
objects, and the `operator` block from the status of the clusters.
