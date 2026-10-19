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
`CA_SECRET_NAME` | The name of the Secret holding the CA the operator generates for its webhooks, used as given. When unset, `<OPERATOR_DEPLOYMENT_NAME>-ca-secret` when the Deployment name is set, `cnpg-ca-secret` otherwise. See [Several operators in one namespace](#several-operators-in-one-namespace).
`CERTIFICATE_DURATION` | Determines the lifetime of the generated certificates in days. Default is 90.
`CLUSTERS_ROLLOUT_DELAY` | The duration (in seconds) to wait between the roll-outs of different clusters during an operator upgrade. This setting controls the timing of upgrades across clusters, spreading them out to reduce system impact. The default value is `0` which means no delay between PostgreSQL cluster upgrades.
`CREATE_ANY_SERVICE` | When set to `true`, will create `-any` service for the cluster. Default is `false`
`DRAIN_TAINTS` | Specifies the taint keys that should be interpreted as indicators of node drain. By default, it includes the taints commonly applied by [kubectl](https://kubernetes.io/docs/concepts/scheduling-eviction/taint-and-toleration/), [Cluster Autoscaler](https://github.com/kubernetes/autoscaler), and [Karpenter](https://github.com/aws/karpenter-provider-aws): `node.kubernetes.io/unschedulable`, `ToBeDeletedByClusterAutoscaler`, `karpenter.sh/disrupted`, `karpenter.sh/disruption`.
`ENABLE_INSTANCE_MANAGER_INPLACE_UPDATES` | When set to `true`, enables in-place updates of the instance manager after an update of the operator, avoiding rolling updates of the cluster (default `false`)
`ENABLE_WEBHOOK_NAMESPACE_SUFFIX` | When set to `true`, the operator looks up its `MutatingWebhookConfiguration` and `ValidatingWebhookConfiguration` under names suffixed with `-<OPERATOR_NAMESPACE>` (e.g. `cnpg-mutating-webhook-configuration-cnpg-team-a` and `cnpg-validating-webhook-configuration-cnpg-team-a`). Use this when running multiple namespaced operator instances on the same cluster to avoid name collisions. The operator does not create the suffixed configurations; you must create them before starting the operator with this flag enabled. If `OPERATOR_NAMESPACE` is not set when this flag is enabled, the operator will refuse to start. An explicit `MUTATING_WEBHOOK_CONFIGURATION_NAME` or `VALIDATING_WEBHOOK_CONFIGURATION_NAME` is used as given, without the suffix. Default is `false`.
`EXPIRING_CHECK_THRESHOLD` | Determines the threshold, in days, for identifying a certificate as expiring. Default is 7.
`INCLUDE_PLUGINS` | A comma-separated list of plugins to be always included in the Cluster's reconciliation.
`INHERITED_ANNOTATIONS` | List of annotation names that, when defined in a `Cluster` metadata, will be inherited by all the generated resources, including pods
`INHERITED_LABELS` | List of label names that, when defined in a `Cluster` metadata, will be inherited by all the generated resources, including pods
`INSTANCES_ROLLOUT_DELAY` | The duration (in seconds) to wait between roll-outs of individual PostgreSQL instances within the same cluster during an operator upgrade. The default value is `0`, meaning no delay between upgrades of instances in the same PostgreSQL cluster.
`KUBERNETES_CLUSTER_DOMAIN` | Defines the domain suffix for service FQDNs within the Kubernetes cluster. If left unset, it defaults to "cluster.local".
`LEADER_ELECTION_ID` | The name of the Lease used for the leader election, used as given. When unset, `<OPERATOR_DEPLOYMENT_NAME>.cnpg.io` when the Deployment name is set, `db9c8771.cnpg.io` otherwise. It must be a Pod environment variable, the Lease is taken before the operator configuration objects are read.
`MANAGE_WEBHOOK_CONFIGURATIONS` | When set to `true` (default), the operator injects its CA bundle into the `MutatingWebhookConfiguration` and `ValidatingWebhookConfiguration`. Set to `false` when the CA bundle is injected externally, for example by cert-manager's CA injector or by GitOps. This is independent of the webhook serving certificate, which the operator always manages when it owns its PKI.
`METRICS_CERT_DIR` | The directory where TLS certificates for the operator metrics server are stored. When set, enables TLS for the metrics endpoint on port 8080. The directory must contain `tls.crt` and `tls.key` files following standard Kubernetes TLS secret conventions. If not set, the metrics server operates without TLS (default behavior).
`MUTATING_WEBHOOK_CONFIGURATION_NAME` | The name of the `MutatingWebhookConfiguration` the operator injects its CA bundle into, used as given. When unset, `<OPERATOR_DEPLOYMENT_NAME>-mutating-webhook-configuration` when the Deployment name is set, `cnpg-mutating-webhook-configuration` otherwise, with the namespace suffix when `ENABLE_WEBHOOK_NAMESPACE_SUFFIX` is set. See [Webhook certificates and configurations](#webhook-certificates-and-configurations).
`MONITORING_QUERIES_CONFIGMAP` | The name of a ConfigMap in the operator's namespace with a set of default queries (to be specified under the key `queries`) to be applied to all created Clusters
`MONITORING_QUERIES_SECRET` | The name of a Secret in the operator's namespace with a set of default queries (to be specified under the key `queries`) to be applied to all created Clusters
`OPERATOR_DEPLOYMENT_NAME` | The name of the Deployment running the operator. When set, the operator fetches its Deployment by this name instead of searching it by the `app.kubernetes.io/name=cloudnative-pg` label, and names the objects it creates for itself after it: `<name>-ca-secret`, `<name>-webhook-cert`, `<name>-webhook-service`, `<name>-mutating-webhook-configuration`, `<name>-validating-webhook-configuration` and the Lease `<name>.cnpg.io`, each one still overridable by its own setting. When unset, the lookup is by label and every name keeps its historical value. It must be a Pod environment variable. See [Several operators in one namespace](#several-operators-in-one-namespace).
`OPERATOR_IMAGE_NAME` | The name of the operator image used to bootstrap Pods. Defaults to the image specified during installation.
`OPERATOR_STATUS_IDENTITY` | How much the operator says about itself in the `status.operator` block of the Clusters it reconciles: `off` nothing, `version` (the default) its version, `name` its version and its Deployment name, `full` its version, its Deployment name and its namespace. With `name` and `full`, the Deployment name is also the field manager of every write of the operator, visible in the `managedFields` of every object it creates or updates. It must be a Pod environment variable, the field manager is fixed before the operator configuration objects are read. See [Several operators in one namespace](#several-operators-in-one-namespace).
`PGBOUNCER_IMAGE_NAME` | The name of the PgBouncer image used by default for new poolers. Defaults to the version specified in the operator.
`POSTGRES_IMAGE_NAME` | The name of the PostgreSQL image used by default for new clusters. Defaults to the version specified in the operator.
`PULL_SECRET_NAME` | Name of an additional pull secret to be defined in the operator's namespace and to be used to download images
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
mode.

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
