<p align="center">
  <img src="https://raw.githubusercontent.com/fyannk/pgAdmin/main/site/assets/mark.svg" width="140" alt="fyannk elephant mark">
</p>

<h1 align="center">cloudnative-pg, fyannk build</h1>

<p align="center">
  <strong>An unofficial build of the CloudNativePG 1.30.2 operator, with fixes and features proposed upstream.</strong><br>
  It accepts the same manifests and replaces a stock 1.30.2 operator in place; every difference is documented.
</p>

<p align="center">
  <a href="https://github.com/fyannk/cloudnative-pg/actions/workflows/continuous-integration.yml?query=branch%3Av1.30.2-fyannk"><img src="https://github.com/fyannk/cloudnative-pg/actions/workflows/continuous-integration.yml/badge.svg?branch=v1.30.2-fyannk" alt="CI status"></a>
  <a href="https://github.com/fyannk/cloudnative-pg/releases"><img src="https://img.shields.io/github/v/release/fyannk/cloudnative-pg?style=flat-square&color=090c0a&labelColor=1b231d&label=release" alt="Latest release"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-Apache--2.0-090c0a?style=flat-square&labelColor=1b231d" alt="Apache-2.0 license"></a>
  <img src="https://img.shields.io/badge/based%20on-CloudNativePG%201.30.2-090c0a?style=flat-square&logo=postgresql&logoColor=b3bf3a&labelColor=1b231d" alt="Based on CloudNativePG 1.30.2">
  <img src="https://img.shields.io/badge/Go-1.26-090c0a?style=flat-square&logo=go&logoColor=b3bf3a&labelColor=1b231d" alt="Go 1.26">
</p>

This repository is a personal fork of
[CloudNativePG](https://github.com/cloudnative-pg/cloudnative-pg), the
Kubernetes operator for PostgreSQL. The `v1.30.2-fyannk` branch is the stock
`v1.30.2` release plus the changes listed below. Each change is also proposed
upstream as its own pull request; this build lets them run in production
before they are merged, or instead of it.

> [!IMPORTANT]
> This is **not** an official CloudNativePG release, and it is not affiliated
> with or endorsed by the CloudNativePG project or the CNCF. Do not report its
> problems to the CloudNativePG maintainers: open an issue
> [here](https://github.com/fyannk/cloudnative-pg/issues). For the official
> operator, go to
> [cloudnative-pg/cloudnative-pg](https://github.com/cloudnative-pg/cloudnative-pg).

## What differs from stock 1.30.2

Nothing below needs a new permission, and the object names are unchanged.

**Without any new setting**

- Every managed object carries the `cnpg.io/reconcile=true` label, and each
  `Cluster` reports the operator that reconciled it in `status.operator`.
- The proxy settings of the operator reach the instances and the poolers
  (`PROPAGATE_PROXY_ENV=false` disables it).
- `resizeInUseVolumes: false` in the section of the WAL or of a tablespace
  prevents the resize of its volumes.
- Stalls are visible: a refused volume expansion, a resize waiting for a Pod
  restart, a blocked `Pooler`, a failed major upgrade or a failed
  reconciliation each get a condition, a phase or a Warning event.
- The instance manager names the full volumes when PostgreSQL stops, and a
  demoted primary escalates its smart shutdown to a fast one.
- New metrics: `cnpg_pgbouncer_up`, `cnpg_disk_*`, `cnpg_operator_*`.

**Stricter**

- The `Cluster` CRD enforces the update rules of the webhook itself, at most
  64 tablespaces, and a webhook checks the writes of the `scale` subresource.

**Opt-in**

- `autoGrow`: the volumes grow on their own before they fill up.
- A cache restricted to the managed objects, a default `StorageClass` for new
  clusters, labels and annotations per kind of object, monitoring discovery
  annotations, standard five-field cron schedules.
- Restricted installations: running without some APIs or permissions, node
  state without the `Node` API, an external webhook certificate, several
  operators in one namespace.

The full list, with the effect of each change on existing clusters, is in
[Differences from CloudNativePG](docs/src/fork_differences.md).

## Quick start

Prerequisites: a Kubernetes cluster supported by CloudNativePG 1.30, and
`kubectl`.

```sh
kubectl apply --server-side -f \
  https://github.com/fyannk/cloudnative-pg/releases/download/v1.30.2-1/cnpg-1.30.2-1.yaml
```

The operator runs in the `cnpg-system` namespace, from the image
`ghcr.io/fyannk/cloudnative-pg:1.30.2-1`. Clusters are then created as with
stock CloudNativePG: see its
[quickstart](https://cloudnative-pg.io/docs/1.30/quickstart).

To replace a stock 1.30.2 operator, or to go back to it, read
[Replacing an upstream 1.30.2 operator](docs/src/fork_differences.md#replacing-an-upstream-1302-operator)
first: apply the full manifest, not only the image.

## Versions

Releases are named after the stock version they are based on, followed by a
sequence number: `1.30.2-1`, `1.30.2-2`, and so on. In semantic versioning
this suffix sorts below the stock `1.30.2`, so keep this build out of the
channels and update rules of the official one: see
[Unofficial fork builds](docs/src/installation_upgrade.md#unofficial-fork-builds).

## Documentation

- [Differences from CloudNativePG](docs/src/fork_differences.md)
- [Operator configuration](docs/src/operator_conf.md), with the new settings
- Everything else: the
  [CloudNativePG 1.30 documentation](https://cloudnative-pg.io/docs/1.30/)
  applies as is.

## Contributing

Changes are written for upstream first: each one is a pull request on
`main`, then carried into this branch. Open an issue here before writing
code.

## Security

Report a vulnerability of this build privately, through a
[security advisory](https://github.com/fyannk/cloudnative-pg/security/advisories/new)
on this repository. A vulnerability that also affects stock CloudNativePG
belongs to its maintainers, as described in [SECURITY.md](SECURITY.md).

## License

Apache-2.0, as CloudNativePG. See [LICENSE](LICENSE). CloudNativePG is a
Series of LF Projects, LLC; PostgreSQL is a registered trademark of the
PostgreSQL Community Association of Canada.
