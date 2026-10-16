# Node drain handling in CloudNativePG: current behaviour, constraints and design

Status: design notes, not implemented. Written against the `claude/trusting-curie-c12px8`
branch (based on upstream commit `71b37de`). File and line references point at that tree.

This document records everything discussed about making node drains work
automatically for every CloudNativePG cluster, in every installation mode of the
operator (cluster-wide and restricted/namespaced), without a human having to
switch over clusters one by one.

---

## 1. Problem statement

Cluster administrators upgrade dozens of Kubernetes clusters at a time. A drain
must never require per-PostgreSQL-cluster manual action, and must never stall
indefinitely on a `Cluster` the administrator does not own. At the same time a
drain must not turn planned maintenance into an unplanned failover when a safe
switchover is possible.

The fork supports *restricted* installations of the operator, where the operator
may have:

- no cluster-scoped RBAC (in particular no `get/list/watch` on `nodes`), and
- no admission webhooks (a `ValidatingWebhookConfiguration` is a cluster-scoped
  object the operator is not allowed to create or patch).

Any design therefore has to work in four situations:

| Install mode | Node API | Webhook | Notes |
|---|---|---|---|
| Cluster-wide, stock | yes | yes | today's upstream behaviour |
| Restricted, with a platform helper (NodeEcho and/or a platform eviction webhook) | no | indirectly, via pod annotations | needs a cluster admin to install the helper once |
| Restricted, with cert-manager and per-tenant webhook configs | no | yes, per tenant | see section 7.5 |
| Restricted, nothing else ("Hail Mary") | no | no | only in-namespace signals exist |

---

## 2. What the operator does today (stock behaviour)

### 2.1 Node watch

The cluster controller watches `corev1.Node` objects:

- Registration: `internal/controller/cluster_controller.go` around line 1359,
  `Watches(&corev1.Node{}, handler.EnqueueRequestsFromMapFunc(r.mapNodeToClusters()), builder.WithPredicates(r.nodesPredicate()))`.
- Predicate: `internal/controller/cluster_predicates.go:100` (`nodesPredicate`).
  An update event passes only if `spec.unschedulable` changed, or one of the
  configured *drain taints* appeared, disappeared, or changed value/effect.
  Create and delete events are handled separately in the same predicate.
- Mapper: `internal/controller/cluster_controller.go:1765` (`mapNodeToClusters`).
  It returns nothing unless `isNodeUnschedulableOrBeingDrained` is true for the
  node. Then it lists pods on that node (field index `.spec.nodeName`) that carry
  the labels `cnpg.io/instanceRole=primary` and `cnpg.io/podRole=instance`, and
  enqueues their owning `Cluster`. Only clusters with a **primary** on the node
  are enqueued.

What counts as "being drained" (`internal/controller/replicas.go:258`):

```go
func isNodeUnschedulableOrBeingDrained(node *corev1.Node, drainTaints []string) bool {
    for _, taint := range node.Spec.Taints {
        if slices.Contains(drainTaints, taint.Key) { return true }
    }
    return node.Spec.Unschedulable
}
```

The taint list comes from operator configuration
(`internal/configuration/configuration.go:187`, env `DRAIN_TAINTS`, default
`DefaultDrainTaints` at line 60): the Kubernetes unschedulable taint
`node.kubernetes.io/unschedulable`, the Cluster Autoscaler taint
`ToBeDeletedByClusterAutoscaler`, and the Karpenter disruption taints.

Important nuance: a node **label** triggers nothing. "Tagging" a node only works
if it means cordoning it or applying one of the configured taint keys.

### 2.2 Switchover away from a draining node

`reconcileTargetPrimaryFromPods` (`internal/controller/replicas.go:55`):

1. If the first instance in the sorted status list is the primary, is the
   current primary, and no switchover is already pending
   (`TargetPrimary == CurrentPrimary`), the operator calls
   `isNodeUnschedulableOrBeingDrained(ctx, primary.Node)` which performs a
   `GET nodes/<name>` (`replicas.go:269`).
2. On error (for example a 403 in a restricted install) the error is logged and
   the normal target-primary reconciliation continues. **The drain is silently
   ignored.**
3. If the node is draining, `setPrimaryOnSchedulableNode`
   (`replicas.go:283`) runs:
   - It computes `podsOnOtherNodes` (instances whose `nodeName` differs from
     the primary's).
   - Unless there are failed pods, it **waits** while
     `Spec.Instances != Status.ReadyInstances` or while fewer than
     `instances-1` pods are on other nodes. The intent is to let replicas that
     were co-located on the draining node be evicted and rescheduled first.
   - If there are failed pods (typically a replica stuck `Pending` because its
     PVC is node-local and the node is cordoned) it proceeds anyway.
   - It then iterates candidates on other nodes and skips any whose node is
     also draining (`replicas.go:323`, again a node `GET`, errors ignored and
     treated as "schedulable"), any that is not Ready, and any whose WAL
     receiver is not active.
   - The first surviving candidate becomes `status.targetPrimary`, the phase is
     set to `PhaseSwitchover`, and an event `SwitchingOver` is recorded.

### 2.3 Pod disruption budgets

Built in `pkg/specs/poddisruptionbudget.go`:

- **Primary PDB** (`BuildPrimaryPodDisruptionBudget`, line 75): name
  `<cluster>-primary`, selector `cnpg.io/cluster=<name>,cnpg.io/instanceRole=primary`,
  `minAvailable: 1`. It exists for every cluster, including single-instance ones.
- **Replica PDB** (`BuildReplicasPodDisruptionBudget`, line 35): only when
  `instances >= 3`. Name `<cluster>`, selector on `instanceRole=replica`,
  `minAvailable: instances-2`, i.e. one replica may be disrupted at a time.
  For `instances == 2` there is no replica PDB and the single replica can be
  evicted freely.

Reconciliation (`internal/controller/cluster_create.go:117`,
`reconcilePodDisruptionBudget`):

- `spec.enablePDB` (default `true`, `api/v1/cluster_funcs.go:836`) is the only
  user knob. `false` deletes both PDBs.
- During a node maintenance window with `reusePVC: true` the replica PDB is
  removed, and for `instances == 1` the primary PDB is removed too, so the node
  can be drained at the cost of downtime.
- During a maintenance window with `reusePVC: false` the primary PDB of a
  single-instance cluster is **kept on purpose**: it is the only thing that
  prevents data loss, because `processUnschedulableInstances`
  (`internal/controller/cluster_controller.go:1072`, PVC deletion at line 1101
  onwards) deletes the PVC group of any unschedulable pod in that mode and has
  **no instance-count guard**. The documentation describes this at
  `docs/src/kubernetes_upgrade.md` ("Single instance clusters with reusePVC set
  to false").

Effect of the PDBs on a drain of a node hosting a primary:

1. `kubectl drain` cordons the node (sets `spec.unschedulable`).
2. The operator notices (node watch) and starts a switchover.
3. Meanwhile the drain tool's eviction of the primary pod is rejected with
   HTTP 429 ("Cannot evict pod as it would violate the pod's disruption
   budget"). `kubectl drain` retries every 5 seconds; autoscalers and node
   controllers retry on their own schedule.
4. The switchover completes, the role label of the old primary changes to
   `replica`, the primary PDB no longer matches it, and the next eviction
   attempt succeeds (subject to the replica PDB).

Documentation stance (`docs/src/kubernetes_upgrade.md`): leave PDBs on in
production; disable them for single-instance development clusters because they
block drains; the node maintenance window is kept for backward compatibility.

### 2.4 The instance manager on SIGTERM

`internal/cmd/manager/instance/run/lifecycle/lifecycle.go:137`: on `SIGTERM`
or `SIGINT` the lifecycle loop immediately calls
`TryShuttingDownSmartFast`, i.e. a smart shutdown bounded by
`smartShutdownTimeout` (default 180 s, `api/v1/cluster_funcs.go:758`) followed
by a fast shutdown, and then the instance manager exits. There is no attempt to
hand the primary role over first.

The pod's `terminationGracePeriodSeconds` is `stopDelay`
(`GetMaxStopDelay`, default 1800 s, `api/v1/cluster_funcs.go:750`, used at
`pkg/specs/pods.go:537`).

### 2.5 The switchover sequence (operator and instance managers)

1. The operator sets `status.targetPrimary` to the candidate and the phase to
   `PhaseSwitchover`.
2. The **old primary's** instance manager, in `reconcileOldPrimary`
   (`internal/management/controller/instance_controller.go:630`), sees it is no
   longer the target primary and, if PostgreSQL is reachable, requests a
   **fast shutdown preceded by a checkpoint** through the lifecycle command
   channel (`RequestFastImmediateShutdown`), then waits for its context to be
   cancelled. The instance manager then exits; in a normal switchover the
   container restarts and comes back as a replica (`Demote` at
   `pkg/management/postgres/instance.go:1056` writes the replica
   configuration). If PostgreSQL is unreachable it requests an immediate
   shutdown instead.
3. The **new primary's** instance manager, in `reconcilePrimary` /
   `handlePromotion` (`instance_controller.go:1283` and `:1355`), acquires the
   primary lease, waits for its WAL receiver to go down
   (`waitForWalReceiverDown`, which only happens once the old primary has
   stopped streaming), then runs `PromoteAndWait` (`pg_ctl promote`, bounded
   by `switchoverDelay`, default 3600 s, `cluster_types.go:1474`).
4. The promotion does **not** need the old primary pod to still exist. It only
   needs the old primary's WAL stream to have stopped.

Guards in the operator that matter for drains:

- `handleSwitchover` (`internal/controller/cluster_controller.go:728`) refuses
  to start a switchover if the current primary is fenced or reports
  `MightBeUnavailable`. If the primary has already shut down when the operator
  looks, the situation is handled as a failover, not a switchover.
- Two instances reporting as primary at the same time makes the operator wait
  (`errOldPrimaryDetected`, `cluster_controller.go:478`). The promotion waiting
  for the WAL receiver to drop prevents this overlap in practice.

### 2.6 Pod metadata reconciliation

`pkg/reconciler/instance/metadata.go` only **adds or updates** the operator's
own labels and annotations on instance pods (`updateClusterAnnotations`,
`updateClusterLabels`, `updateRoleLabels`, `updateOperatorLabels`). It never
deletes foreign keys. Labels or annotations written onto instance pods by a
third party (NodeEcho, a webhook, a human) survive reconciliation.

### 2.7 Behaviour in a restricted install today

- The node watch cannot be registered without `nodes` RBAC. A
  controller-runtime informer without permission never syncs and blocks
  manager start-up, so the restricted build must register that watch
  conditionally (the fork is assumed to do this already).
- `isNodeUnschedulableOrBeingDrained` returns a 403 error, which is logged and
  ignored. A cordon therefore never triggers a switchover.
- PDBs are namespaced and still work. Net effect: **a drain of a node hosting
  any primary stalls until a human promotes another instance**, for example
  with `kubectl cnpg promote`.

---

## 3. The original four-step plan and the assessment

The plan as proposed:

1. Tag node, operator watches nodes, switchover. (Stock.)
2. NodeEcho (https://github.com/fyannk/NodeEcho; the page was not reachable
   from the analysis environment, so it is described from the author's one-line
   summary) transposes node taints and labels onto pods; the operator can,
   configurably, move affected primaries.
3. Evict pods; if the operator has webhooks, intercept `pods/eviction` and
   switch over.
4. If the operator has neither the node API nor webhooks, default PDBs to off.

Assessment:

- Step 1 is complete and works. Only cordon and the configured taint keys
  count; labels do not.
- Step 2, taken literally (operator consumes NodeEcho's labels), only adds
  value where the operator lacks node RBAC. It should be reframed as a generic,
  CNPG-defined pod annotation contract with NodeEcho as one producer (section 6).
- Step 3 is the most valuable addition for cluster-wide installs. It should be
  built as "annotate and allow", not "deny" (section 7). In restricted installs
  the webhook cannot be the operator's own unless certificates and the
  cluster-scoped configuration are provided externally (section 7.5).
- Step 4 was first rejected (the PDB is the only passive guard that works with
  no operator uptime, no RBAC and no webhook, and turning it off by default is a
  behaviour change for every existing cluster), then refined once the restricted
  constraints were made explicit: in a namespaced install there is **no
  pre-eviction signal at all**, so the primary PDB must come off for drains to
  proceed, and the eviction itself must be made graceful (section 8). The
  refined rule: primary PDB off only when no proactive drain signal is
  configured; replica PDB stays; single-instance PDB has its own default.

---

## 4. What is observable from inside a namespace during a drain

This analysis drives the restricted-mode design.

| Event | Visible in the namespace? | How |
|---|---|---|
| Node cordoned | no | `nodes` are cluster-scoped |
| Node tainted | no | same |
| Eviction **rejected** by a PDB (429) | **no** | the eviction REST handler returns the error to the caller and emits no event; the PDB status is not modified; only audit logs see it |
| Eviction **allowed** | yes | the API server (a) records the pod in the PDB's `status.disruptedPods` with a timestamp, (b) adds the pod condition `DisruptionTarget` with reason `EvictionByEvictionAPI`, (c) deletes the pod (sets `deletionTimestamp` and `deletionGracePeriodSeconds`) |
| Plain `kubectl delete pod` | yes, but indistinguishable from the operator's own deletes | `deletionTimestamp` only; no `DisruptionTarget` |
| Node-pressure eviction by kubelet | yes | `DisruptionTarget` with reason `TerminationByKubelet` |
| Preemption | yes | `DisruptionTarget` with reason `PreemptionByScheduler` |
| Taint-manager deletion (NoExecute) | yes | `DisruptionTarget` with reason `DeletionByTaintManager` |
| Graceful node shutdown (kubelet) | partially | pods are killed by the kubelet with the shutdown grace period, which is often 30 s or disabled; this is not a drain |

`DisruptionTarget` comes from the `PodDisruptionConditions` feature gate:
beta and on by default from Kubernetes 1.27, GA in 1.31. Below 1.27 only
`deletionTimestamp` exists.

Conclusion: the first in-namespace signal of a drain is the eviction
**succeeding**. If the primary PDB blocks it, nothing in the namespace ever
learns a drain was attempted. The only in-namespace way to react to a drain is
to let the eviction through and react to the resulting `DisruptionTarget`.

---

## 5. Component A: PDB policy, operator-level and per-cluster

### 5.1 Requirements

- Cluster administrators must be able to set the policy **once per Kubernetes
  cluster** (operator configuration) and have every `Cluster` follow it,
  whatever the installation mode. A per-`Cluster` field alone does not meet the
  "dozens of clusters" requirement.
- Tenants may override per `Cluster`.
- `spec.enablePDB` must keep working as a shorthand.

### 5.2 Proposed settings

Three independent switches, each `on` or `off`, available both as operator
configuration (env / config map, for example `PDB_PRIMARY_DEFAULT`,
`PDB_REPLICAS_DEFAULT`, `PDB_SINGLE_INSTANCE_DEFAULT`) and as per-`Cluster`
overrides:

| Switch | Governs | Suggested default |
|---|---|---|
| primary | the `<cluster>-primary` PDB for clusters with `instances > 1` | `on` when a proactive drain signal exists (node API or annotation producer configured); `off` in the Hail Mary mode (section 8) |
| replicas | the `<cluster>` replica PDB (`instances >= 3`) | `on` always; it costs nothing and keeps replicas going one at a time |
| singleInstance | the primary PDB for `instances == 1` | `off` (a one-node cluster cannot expect HA; a stalled drain is worse than the downtime) |

`spec.enablePDB: false` sets all three to `off`; `true` leaves the defaults.

### 5.3 Mandatory guard when the single-instance default becomes `off`

`processUnschedulableInstances` must refuse to delete the PVC group of the last
instance of a cluster (`instances == 1`) regardless of
`nodeMaintenanceWindow.reusePVC`. Today the primary PDB is the only barrier
between a single-instance cluster with `reusePVC: false` and data loss. With the
PDB off, an eviction succeeds, the pod becomes unschedulable on the cordoned
node, and the maintenance-window path would delete the PVCs. Add the guard
before changing the default.

What single-instance PDB `off` costs otherwise: the pod is evicted, the
instance manager performs a smart-then-fast shutdown, and the pod is recreated.
With network storage it reschedules elsewhere. With node-local storage it stays
`Pending` until the node is uncordoned. Downtime, no data loss.

### 5.4 Interplay with the node maintenance window

Keep the existing logic (`reconcilePodDisruptionBudget`): a maintenance window
with `reusePVC: true` still removes the replica PDB and, for single instance,
the primary PDB. The new switches layer on top: a switch set to `off` removes
the PDB unconditionally; a switch set to `on` keeps today's behaviour.

### 5.5 Visibility

- When the primary PDB is `off` for a multi-instance cluster, record a warning
  event on the `Cluster` at reconcile time (rate-limited) saying that primaries
  may be evicted and describing the resulting behaviour (switchover on eviction
  if section 8 is implemented, failover otherwise).
- When the node watch is unavailable (restricted mode) and no annotation
  producer is configured, surface a `Cluster` condition such as
  `DrainDetectionDegraded` so administrators know drains will stall (PDB on)
  or be handled reactively (PDB off).

---

## 6. Component B: one drain signal, several producers

### 6.1 A single function in the operator

Replace the two direct node lookups (`replicas.go:74` for the current primary,
`replicas.go:323` for candidates) with one function, for example
`isInstanceOnDrainingNode(ctx, pod)`, that ORs the available sources:

1. **Node API** (stock): `GET nodes/<pod.spec.nodeName>` and
   `isNodeUnschedulableOrBeingDrained`. Skipped when the operator has no node
   RBAC (detected once at start-up with a `SelfSubjectAccessReview`, or by
   configuration).
2. **Pod annotation** (section 6.2): present and not stale.
3. **DisruptionTarget condition** on the pod (section 8), for the current
   primary only. Candidates carrying it are being evicted and must be skipped.

Both call sites must use the same function; otherwise a switchover can target a
replica on a node that is also draining.

In restricted mode the candidate selection must not depend on node objects at
all. `spec.nodeName` is on the pod and is enough to prefer candidates on
other nodes. The waiting logic in `setPrimaryOnSchedulableNode` ("all replicas
moved off the drained node first") should be relaxed when no node information
is available, otherwise a co-located replica with node-local storage stalls the
switchover.

### 6.2 The pod annotation contract

Defined by CloudNativePG, consumed by the operator, written by any producer.

- Key: a `cnpg.io/` annotation, for example `cnpg.io/drainRequested`.
- Value: an RFC 3339 timestamp of the latest signal. Producers overwrite it on
  every new signal; drain tools retry evictions every few seconds, so the
  timestamp keeps refreshing while the drain is active.
- Consumer semantics: the operator treats the annotation as valid for a bounded
  time (for example 10 minutes) after its timestamp. A stale annotation is
  ignored, which covers aborted drains and uncordoned nodes without needing a
  "clear" path. The operator may additionally remove the annotation once it has
  acted (after the role label flips).
- Producers must be idempotent and must not write on dry-run requests.
- The annotation survives the instance metadata reconciler (section 2.6).
- Because the operator already owns and watches instance pods
  (`Owns(&corev1.Pod{})`), an annotation change enqueues the `Cluster` with no
  new watch and no new RBAC.
- A human can set it with `kubectl annotate` to force a switchover away from a
  pod, which is a useful debugging and escape hatch on its own.

### 6.3 Producer: NodeEcho

NodeEcho runs as a platform component with its own `nodes` read permission. It
mirrors node state (cordon, configured drain taints) onto the pods scheduled on
that node. For CloudNativePG it should write the annotation above onto instance
pods when the node becomes unschedulable or tainted, and refresh it while the
condition holds.

Properties:

- Installed once per Kubernetes cluster by the platform team, alongside the
  CRDs. Tenants need nothing.
- Proactive: the signal arrives **before** any eviction, exactly like the stock
  node watch, so the primary PDB can stay `on` and the drain behaves as in a
  cluster-wide install.
- It shifts the node-read privilege from the tenant operator to one shared
  component, which is the right place for it in a multi-tenant cluster.
- Natural home for the eviction webhook producer too (section 7.4), giving one
  component, two signal sources, one contract.

### 6.4 Producer: the eviction webhook

See section 7.

### 6.5 Producer: the eviction itself

See section 8. This is not an annotation but the `DisruptionTarget` condition,
consumed through the same function.

---

## 7. Component C: the `pods/eviction` admission webhook

### 7.1 Mechanics

A `ValidatingWebhookConfiguration` rule on API group `""`, resource
`pods/eviction`, operation `CREATE`. The admission request carries a
`policy/v1 Eviction` object whose metadata holds the pod's name and namespace.

Order inside the API server's eviction handler: the pod is fetched, admission
(`createValidation`) runs, **then** PDBs are checked, then the pod is deleted.
Admission therefore sees every attempt, including the ones a PDB will reject.
The Strimzi Drain Cleaner relies on exactly this ordering and is the prior art
for the design below.

### 7.2 Design: annotate and allow, never deny

On each eviction request the webhook:

1. Ignores dry-run requests (`request.dryRun == true`); register the webhook
   with `sideEffects: NoneOnDryRun`.
2. Looks up the pod (the Eviction object does not carry labels).
3. If the pod is a CloudNativePG instance that is the current primary of a
   multi-instance cluster, patches the drain annotation (section 6.2) onto the
   pod with the current timestamp.
4. Returns **allowed**.

The primary PDB then rejects the eviction with 429 as it does today, the drain
tool retries, the operator sees the annotation and switches over, the role label
flips, and the next retry passes the primary PDB.

Why not deny or return 429 from the webhook:

- A webhook denial is a 403 by default, which `kubectl drain` treats as fatal
  for that pod rather than retrying. Returning a 429 status from the webhook is
  possible but makes correctness depend on the webhook being up.
- With "annotate and allow" the PDB remains the only hard gate, so the webhook
  can use `failurePolicy: Ignore`. If the operator or the webhook is down,
  behaviour degrades to today's (PDB blocks, nobody switches over) instead of
  blocking every eviction in the Kubernetes cluster.
- The webhook does not need to decide anything, and never does the switchover
  synchronously (a switchover takes seconds to minutes; admission must answer
  in well under 10 seconds).

### 7.3 Limitations

- `objectSelector` cannot be used to filter by pod labels: on the eviction
  subresource it is evaluated against the Eviction object, which has no labels.
  The webhook is invoked for **every** eviction it is scoped to and must look
  the pod up itself. Keep it cheap: a cached client, a short timeout (2 to 3 s).
- Scoping options: `namespaceSelector` (requires labels on namespaces) or
  `matchConditions` (CEL, GA in Kubernetes 1.30), for example
  `request.namespace.startsWith("tenant-a-")`, which needs no namespace labels.
- Pod **deletion** bypasses the eviction API entirely: `kubectl delete pod`,
  `kubectl drain --disable-eviction`, graceful node shutdown, the operator's
  own rolling-update deletes. Intercepting pod `DELETE` is not an option
  (garbage collection, the operator's own deletes, and kubelet behaviour depend
  on deletes succeeding).
- If the operator serving the webhook is itself on the drained node and has a
  single replica, its own eviction takes the webhook down for the duration of
  its restart. With `failurePolicy: Ignore` evictions proceed normally during
  that window and the PDB still protects primaries; the switchover is delayed
  until the operator is back.

### 7.4 Where the webhook runs

Two deployments of the same handler:

- **Inside the operator's existing webhook server** for cluster-wide installs.
  The operator already manages its own `ValidatingWebhookConfiguration`
  (`internal/cmd/manager/controller/controller.go:490`, `ensurePKI`, and the
  `certs` package inject the CA bundle); adding the `pods/eviction` rule is a
  small change.
- **As a platform component** for restricted installs, ideally inside
  NodeEcho, installed once per Kubernetes cluster. It needs cluster-wide
  `pods` get/patch. It never denies, so it is operator-agnostic and needs no
  per-tenant routing.

### 7.5 Per-tenant webhook configurations in restricted installs (with cert-manager)

A `ValidatingWebhookConfiguration` has exactly one backend (`clientConfig`:
a Service reference or a URL). It cannot be templated to "the operator living in
the evicted pod's namespace". A restricted operator also cannot create or patch
the configuration (cluster-scoped), and cannot inject its CA bundle into it.

It **is** feasible per tenant when certificates and the configuration are
managed externally, which was the author's point:

- cert-manager issues a `Certificate` in the tenant operator's namespace (a
  namespaced object the tenant or the platform can create). The resulting TLS
  secret is mounted into the operator and `WEBHOOK_CERT_DIR` points at it.
  With that variable set the operator uses the external certificate and skips
  its own PKI and CA-bundle injection entirely
  (`internal/cmd/manager/controller/controller.go:187` and `:497`; the file
  names expected are `apiserver.crt` / `apiserver.key`, matching OLM, so the
  secret must be mounted or projected with those names). The configuration
  flag `ManageWebhookConfigurations` also exists to keep the operator away from
  the cluster-scoped objects.
- The cluster administrator creates one `ValidatingWebhookConfiguration` per
  tenant operator, with `clientConfig.service` pointing at that operator's
  webhook Service, scoped with `namespaceSelector` or `matchConditions` to the
  tenant's namespaces, and annotated with
  `cert-manager.io/inject-ca-from: <namespace>/<certificate>`. cert-manager's
  `cainjector` (a cluster-scoped component) fills in `caBundle` and keeps it
  current on renewal.
- The same configuration can carry the existing CNPG validating rules for
  `clusters`, `backups`, `poolers`, etc., scoped to the tenant, which gives
  restricted installs admission validation back as a bonus.

Costs and limits:

- Still one cluster-scoped object per tenant operator, created by someone with
  cluster rights. With GitOps templating this is routine; it is the same
  category of work as installing the CRDs.
- Requires cert-manager (or any equivalent injector) on every cluster.
- Many small webhook configurations each hit the API server on every eviction
  in their scope. Scope them tightly.
- The "annotate and allow" design still applies, so `failurePolicy: Ignore`
  remains safe and a tenant operator being down never blocks a drain.

With this in place, a restricted install with cert-manager gets the same
proactive eviction signal as a cluster-wide install, and the primary PDB can
stay `on`.

---

## 8. Component D: eviction-aware SIGTERM in the instance manager (the Hail Mary that is not uncontrolled)

### 8.1 When it applies

Restricted install, no node API, no annotation producer, no per-tenant webhook.
The primary PDB is `off` for multi-instance clusters (otherwise drains stall),
so evictions of primaries succeed. Goal: turn those evictions into switchovers
instead of failovers.

### 8.2 Mechanism

On `SIGTERM`, before shutting down, the instance manager:

1. Reads its own pod. If the pod carries `DisruptionTarget` (any reason), this
   instance is the primary, and `spec.instances > 1`, continue; otherwise run
   today's shutdown.
2. Computes the hard deadline from `metadata.deletionTimestamp` plus
   `metadata.deletionGracePeriodSeconds` (the **effective** grace period chosen
   by the evictor, which may be shorter than `stopDelay`).
3. Requests a switchover by writing the drain annotation (section 6.2) on its
   own pod, or an equivalent marker on the `Cluster`. The instance manager
   already patches its pod and the cluster status, so no new RBAC.
4. Arms a hold timer instead of calling `TryShuttingDownSmartFast`:
   `hold = min(configured hold, deadline - reserve)`, where `reserve` covers
   the fast shutdown and its checkpoint (at least `smartShutdownTimeout`, or a
   fixed 60 to 120 s). If `deadline - reserve` is already below a few seconds,
   skip the hold entirely.
5. Waits for whichever comes first:
   - the operator moves `status.targetPrimary` away; the existing
     `reconcileOldPrimary` path then sends the fast-shutdown command through
     the lifecycle command channel, PostgreSQL checkpoints and stops, the
     instance manager exits. Because the pod is terminating, the container does
     not restart; the operator recreates the instance elsewhere as a replica.
   - the hold timer fires; fall back to today's smart-then-fast shutdown. The
     operator later handles the missing primary as a failover, exactly as with
     PDB `off` today.

The change is small because the lifecycle loop (`lifecycle.go:137`) already
serves the fast-shutdown command used by demotion; SIGTERM merely arms a timer
instead of shutting down immediately.

Keying on `DisruptionTarget` rather than on `deletionTimestamp` is essential:
the operator's own primary deletes during a rolling update with the `restart`
primary update method are plain deletes, carry no condition, and must keep
today's behaviour (the primary comes back as primary). Rolling updates with the
`switchover` method already switch over before deleting.

### 8.3 Time budget

| Quantity | Value | Source |
|---|---|---|
| Pod grace period | `stopDelay`, default 1800 s | `pkg/specs/pods.go:537` |
| Smart shutdown bound | `smartShutdownTimeout`, default 180 s | `cluster_funcs.go:758` |
| Promotion bound | `switchoverDelay`, default 3600 s | `cluster_types.go:1474` |
| `kubectl drain` | pod's own grace period (`--grace-period` default -1), no overall timeout by default | kubectl |
| Cluster Autoscaler | grace capped at 600 s by default (`--max-graceful-termination-sec`) | CA docs |
| Karpenter | pod's own grace period unless the NodePool sets `terminationGracePeriod`, after which the node is force-terminated | Karpenter docs |
| Managed node pool upgrades (GKE, EKS, AKS) | vendor-specific, typically tens of minutes, then the VM is terminated | vendor docs, verify per provider |
| Kubelet graceful node shutdown | `shutdownGracePeriod`, often 30 s or disabled | kubelet config; not a drain |

What must fit before SIGKILL (the old primary's side only):

1. Read pod, write annotation: well under 1 s.
2. Operator reaction (candidate selection, target primary update): a few
   seconds normally; 10 to 60 s if the operator pod is restarting; longer if
   it is stuck.
3. Fast shutdown with checkpoint: seconds normally; minutes on a large,
   write-heavy instance (identical to every switchover today).

The new primary's promotion (WAL receiver down, `pg_ctl promote`, WAL replay)
runs after the old pod is gone and is bounded by `switchoverDelay`, not by the
grace period.

### 8.4 Catches

- **Endpoints drop at eviction time.** A terminating pod is removed from the
  `-rw` Service endpoints as soon as `deletionTimestamp` is set, before the new
  primary exists. New connections to `-rw` fail for the whole hold plus the
  switchover. Existing connections survive until the fast shutdown. A
  cordon-triggered switchover keeps serving until the actual shutdown. The hold
  therefore **adds** client-visible downtime instead of hiding it. Keep the
  hold timeout short (one to two minutes); the happy path takes seconds and the
  timeout only governs the unhappy path.
- **The operator's switchover guard** (`handleSwitchover`,
  `cluster_controller.go:736`) refuses a switchover when the current primary
  reports `MightBeUnavailable`. The hold is what keeps PostgreSQL reachable so
  the operation stays a switchover. Without the hold the pod dies first and the
  operator performs a failover, which is today's behaviour.
- **SIGKILL mid-checkpoint** is not corruption. PostgreSQL crash-recovers. The
  new primary promotes with the WAL it received. With asynchronous replication
  the loss window is the same as any failover; with synchronous replication
  there is none.
- **VM termination** by a cloud upgrade deadline is a crash. Nothing in the pod
  can help.
- **Kubernetes version**: `DisruptionTarget` is on by default from 1.27, GA in
  1.31. Older clusters keep today's behaviour because the condition is absent.
- **Operator co-evicted**: a single-replica operator on the same node restarts
  elsewhere, usually within tens of seconds. The hold must outlast a normal
  operator restart; it cannot outlast an operator stuck `Pending`.
- **All instances on one node**: no candidate; the operator cannot switch over;
  the hold times out; fall back. Candidate selection must skip pods that carry
  `DisruptionTarget` themselves (they are being evicted too).
- **`instances == 1`**: no switchover possible; shut down as today. The
  single-instance PDB switch (section 5) decides whether the eviction is even
  allowed.
- **Fenced primary**: fencing already shuts PostgreSQL down; no hold.
- **Grace period shortened by the evictor**: read it from the pod; do not
  assume `stopDelay`.
- **`kubectl drain` evicts all pods on the node in parallel**: co-located
  replicas are evicted at the same time (replica PDB permitting). Candidate
  selection via `spec.nodeName` and the `DisruptionTarget` skip handle this.
- **Instance manager RBAC**: it must be able to `get` and `patch` its own pod
  and patch the `Cluster`; it already does both today.

### 8.5 The explicit parameter

The mode must be opted into explicitly, never auto-detected, because it changes
what an eviction does to a primary. Suggested wording: "allow evictions of
primaries" with a description such as "the primary PDB is not created; an
eviction of a primary triggers a switchover when the operator is reachable
within the hold period, and a failover otherwise". A warning event on each
affected `Cluster` while the mode is active, plus the condition from
section 5.5.

---

## 9. Decision matrix

| Install mode | Proactive signal | Primary PDB | Drain of a node with a primary |
|---|---|---|---|
| Cluster-wide | node watch (+ eviction webhook in the operator) | on | switchover before eviction; eviction retries pass once the role flips |
| Restricted + NodeEcho (taints and/or eviction webhook) | pod annotation | on | same as cluster-wide; the signal comes from the platform component |
| Restricted + cert-manager + per-tenant webhook config | pod annotation written by the tenant operator's own webhook | on | same as cluster-wide |
| Restricted, nothing else, mode not enabled | none | on | drain stalls on the primary until a human promotes; condition `DrainDetectionDegraded` |
| Restricted, nothing else, "allow evictions of primaries" | `DisruptionTarget` (reactive) | off | eviction succeeds; instance manager holds; operator switches over; failover if the operator does not answer in time |
| Any, `instances == 1` | n/a | per `singleInstance` switch, default off | eviction succeeds; downtime until rescheduled; PVCs never deleted for the last instance |

Replica PDB: on in every row for `instances >= 3`.

---

## 10. Implementation order

1. PDB policy with operator-level defaults and per-`Cluster` overrides, plus
   the last-instance PVC guard. Smallest change, immediate relief for
   single-instance clusters, prerequisite for everything else.
2. Unified drain-signal function in the operator with the pod annotation
   contract, used at both call sites, with node-free candidate selection.
3. Eviction-aware SIGTERM handling driven by `DisruptionTarget`. This makes the
   "allow evictions of primaries" mode acceptable.
4. Producers for the annotation:
   - `pods/eviction` handler in the operator's own webhook server (cluster-wide
     installs; also usable per tenant with cert-manager, section 7.5);
   - the same handler and the taint mirror in NodeEcho (restricted installs
     without cert-manager).
5. Documentation: the decision matrix, the knobs, and the explicit statement
   that `-rw` connections fail during an eviction-triggered switchover.

Stock node watching stays untouched throughout.

---

## 11. Things still to verify before implementing

- The exact admission-before-PDB ordering in the eviction REST handler for the
  Kubernetes versions in scope (`pkg/registry/core/pod/storage/eviction.go`).
  Strimzi Drain Cleaner depends on it, which is strong evidence, but check.
- That `metadata.deletionGracePeriodSeconds` is populated on the pod object
  read by the instance manager at SIGTERM time (it is set by the API server
  when graceful deletion starts).
- The `DisruptionTarget` reasons emitted by the Kubernetes versions in scope
  (`EvictionByEvictionAPI`, `DeletionByTaintManager`, `PreemptionByScheduler`,
  `DeletionByPodGC`, `TerminationByKubelet`).
- How Karpenter and the Cluster Autoscaler treat a 429 from a PDB over long
  periods (both retry; confirm they do not give up and force-delete).
- Vendor node-pool drain timeouts for the providers in use.
- The restricted build's handling of the node watch registration when `nodes`
  RBAC is absent (conditional registration, or a `SelfSubjectAccessReview` at
  start-up).
- EndpointSlice behaviour for terminating pods on the CNI / kube-proxy in use
  (terminating endpoints are excluded from new connections by default; some
  data planes differ).
- cert-manager `cainjector` presence and the exact secret key names
  (`tls.crt`/`tls.key` from cert-manager versus `apiserver.crt`/`apiserver.key`
  expected when `WEBHOOK_CERT_DIR` is set; a projected volume with `items` can
  rename them).

---

## 12. File reference index

| Topic | Location |
|---|---|
| Node watch registration | `internal/controller/cluster_controller.go` ~1359 |
| Node predicate | `internal/controller/cluster_predicates.go:100` |
| Node to cluster mapper | `internal/controller/cluster_controller.go:1765` |
| Drain taints config | `internal/configuration/configuration.go:58`, `:187` |
| `isNodeUnschedulableOrBeingDrained` | `internal/controller/replicas.go:258`, `:269` |
| Primary-on-draining-node check | `internal/controller/replicas.go:74` |
| `setPrimaryOnSchedulableNode` | `internal/controller/replicas.go:283` |
| Candidate node check | `internal/controller/replicas.go:323` |
| PDB construction | `pkg/specs/poddisruptionbudget.go:35`, `:75` |
| PDB reconciliation | `internal/controller/cluster_create.go:117` |
| `GetEnablePDB` | `api/v1/cluster_funcs.go:836` |
| Unschedulable pod / PVC deletion | `internal/controller/cluster_controller.go:1072`, `:1101` |
| Switchover guard | `internal/controller/cluster_controller.go:728` |
| Two-primaries guard | `internal/controller/cluster_controller.go:478` |
| SIGTERM handling | `internal/cmd/manager/instance/run/lifecycle/lifecycle.go:137` |
| Old primary demotion | `internal/management/controller/instance_controller.go:630` |
| New primary promotion | `internal/management/controller/instance_controller.go:1283`, `:1355`, `:1405` |
| `Demote` | `pkg/management/postgres/instance.go:1056` |
| `stopDelay`, `smartShutdownTimeout`, `switchoverDelay` | `api/v1/cluster_funcs.go:750`, `:758`, `:806` |
| Grace period on the pod | `pkg/specs/pods.go:537` |
| Instance metadata reconciliation | `pkg/reconciler/instance/metadata.go` |
| External webhook certs | `internal/cmd/manager/controller/controller.go:187`, `:490` |
| Node maintenance window type | `api/v1/cluster_types.go:1424` |
| User documentation on drains | `docs/src/kubernetes_upgrade.md` |
