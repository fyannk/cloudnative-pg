---
id: fork_differences
sidebar_position: 55
title: Differences from CloudNativePG
---

# Differences from CloudNativePG
<!-- SPDX-License-Identifier: CC-BY-4.0 -->

This build is an [unofficial fork](installation_upgrade.md#unofficial-fork-builds)
of CloudNativePG 1.30.1. It accepts the same manifests, uses the same object
names and needs no permission the upstream operator lacks, so it can replace
an upstream 1.30.1 operator in place. This page lists what behaves differently
from upstream, starting with what changes without any new setting.

## What changes without opting in

### On the objects of existing clusters

| Change | Effect when the operator is replaced | Details |
|---|---|---|
| Every object the operator manages carries the `cnpg.io/reconcile=true` label | Existing objects are patched in place, with no restart. Each `Pooler` rolls out once, as it does on any operator upgrade | [Operator cache](operator_conf.md#operator-cache) |
| Every `Cluster` reports the operator version in `status.operator.version` | One status write per `Cluster`. `OPERATOR_STATUS_IDENTITY=off` disables it | [Several operators in one namespace](operator_conf.md#several-operators-in-one-namespace) |
| The proxy settings of the operator (`HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY`) are set on the instances and the poolers | **When the operator Deployment has such variables, every instance and every pooler restarts once.** Nothing changes otherwise. `PROPAGATE_PROXY_ENV=false` disables it | [Proxy settings](operator_conf.md#proxy-settings) |
| The WAL and tablespace volumes are resized according to their own storage section, not to the `resizeInUseVolumes` of `.spec.storage` | A WAL or tablespace PVC whose size in the `Cluster` is above its current request is expanded at the first reconciliation, which cannot be undone | [Storage](storage.md#using-the-volume-expansion-kubernetes-feature) |
| The CA, server and replication Secrets created by the operator get the inherited labels and annotations of the `Cluster` | One patch per Secret | [Labels and annotations](labels_annotations.md) |

### In the behavior of the instances

| Change | Details |
|---|---|
| PostgreSQL stopping because **any** volume is full (`PGDATA` or a tablespace, not only the WAL volume) is reported as a disk full failure: the `Cluster` goes to the `Not enough disk space` phase and the operator does not fail over, as upstream does for the WAL volume only | [Disk Full Failure](instance_manager.md#disk-full-failure) |
| A primary being demoted skips or escalates its smart shutdown to a fast one | [Shutdown of the primary during a switchover](instance_manager.md#shutdown-of-the-primary-during-a-switchover) |

### In the behavior of the operator

| Change | Details |
|---|---|
| The optional APIs (`PodMonitor`, `VolumeSnapshot`, `ClusterImageCatalog`, `Node`) are probed once, when the operator starts. Installing the Prometheus Operator CRDs later requires restarting the operator to get the `PodMonitor` objects; upstream notices them at the next reconciliation | [Restricted installations](operator_conf.md#restricted-installations) |
| The operator ConfigMap and Secret are read before the manager starts, so `WATCH_NAMESPACE` set there now restricts the cache | |
| A `Cluster` whose volume expansion is refused keeps being reconciled, with the `VolumeExpansionRefused` condition, and the expansion is retried every minute | [When the expansion is refused](storage.md#when-the-expansion-is-refused) |
| The creation of the objects of a `Cluster` goes on after a failure of the default monitoring queries ConfigMap, reported by a `DefaultMonitoringQueriesError` Warning event | |
| A `Pooler` whose Deployment cannot run its Pods (quota, rollout past its deadline) goes to the `inactive` phase, with the message of the Deployment as the reason | |
| A `Cluster` whose major upgrade Job failed goes to the `Unrecoverable` phase instead of staying in the upgrade phase | |
| An error ending a reconciliation of a `Cluster`, `Pooler`, `Backup` or `ScheduledBackup` is reported by a `ReconciliationFailed` Warning event on the object | |
| A `ScheduledBackup` with a five-field schedule gets a `ScheduleFieldCount` Warning event; schedule descriptors such as `@daily` do not raise the admission warning anymore | [Cron schedule](backup.md#cron-schedule) |
| New Warning events and metrics: `BackupDelayed`, `BackupPending`, `cnpg_pgbouncer_up`, `cnpg_disk_*`, `cnpg_operator_*` | [Monitoring](monitoring.md) |
| `kubectl cnpg report operator` finds the operator by its Deployment, and only includes its own webhook configurations | [Plugin](kubectl-plugin.md) |

## What is stricter than upstream

- The `Cluster` CRD carries the update rules of the validating webhook, and
  the API server enforces them even with the `cnpg.io/validation: disabled`
  annotation, except for the decrease of a storage size, which stays with
  the webhook as upstream;
  see [Defaulting and validation without admission webhooks](operator_conf.md#defaulting-and-validation-without-admission-webhooks).
- The `Cluster` CRD limits the length of some fields: 1024 characters for
  `imageName`, 63 for a tablespace name and for the replication slot prefix,
  64 for a storage `size`, and 64 tablespaces per cluster.
- A validating webhook, `vclusterscale.cnpg.io`, checks the writes of the
  `scale` subresource: a `Cluster` holding an invalid field cannot be scaled
  with `kubectl scale` until it is fixed.
- The operator Role does not grant the `status` subresource of ConfigMaps and
  Secrets, which nothing used.

## What must be enabled

These features change nothing until they are configured:

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

## Replacing an upstream 1.30.1 operator

1. Check whether the upstream operator Deployment has proxy variables. If it
   does, either accept one restart of every instance and pooler, or set
   `PROPAGATE_PROXY_ENV=false` in the operator configuration first.
2. Check that no WAL or tablespace storage section holds a size above the
   current request of its PVCs that you do not want applied.
3. Apply the manifest of the fork release, not only its image: it brings the
   CRDs with the new fields and rules, the webhook of the `scale` subresource
   and two ClusterRoles.

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
- with `SCHEDULED_BACKUP_CRON_FORMAT=standard`, rewrite the five-field
  schedules with six fields, see [Cron schedule](backup.md#cron-schedule);
- with `WEBHOOK_CERT_DIR` and certificate files not named as upstream expects,
  or with `OPERATOR_DEPLOYMENT_NAME`, restore the upstream names first.

What stays behind is harmless: the `cnpg.io/reconcile` label, the labels and
annotations set by the rules, and, in the status of the clusters, the
`operator` block and the conditions of the fork, which nothing updates
anymore. The fields of the fork in the `Cluster` specification are dropped
when the upstream CRDs are applied.
