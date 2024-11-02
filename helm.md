# Installation - Helm

```bash
helm repo add fyannk-cnpg https://fyannk.github.io/cloudnative-pg
helm repo update fyannk-cnpg
helm search repo fyannk
```

<!-- tabs:start -->

#### **ClusterWide**

```bash
helm install cnpg fyannk-cnpg/operator
```

#### **Namespaced**

TODO

<!-- tabs:end -->
