# Installation - Operator Lifecycle Manager (OLM)

## Catalog

<!-- tabs:start -->

#### **Shell**

Declare Catalog :

```bash
kubectl apply -f yamls/catalog.yaml
```

#### **RAW**

[filename](yamls/catalog.yaml ':include :type=code')

<!-- tabs:end -->

## OperatorGroup

Declare OperatorGroup (needs ClusterAdmin access) :

<!-- tabs:start -->

#### **ClusterWide**

<!-- tabs:start -->

#### **Shell**

```bash
kubectl apply -f https://fyannk.github.io/cloudnative-pg/yamls/operatorgroupcw.yaml
```

#### **RAW**

[filename](yamls/operatorgroupcw.yaml ':include :type=code')

<!-- tabs:end -->

#### **Namespaced**

<!-- tabs:start -->

#### **Shell**

```bash
kubectl apply -f https://fyannk.github.io/cloudnative-pg/yamls/operatorgroupns.yaml
```

#### **RAW**

[filename](yamls/operatorgroupns.yaml ':include :type=code')

<!-- tabs:end -->

<!-- tabs:end -->




#### **RAW**



<!-- tabs:end -->

## Subscription

<!-- tabs:start -->

#### **Shell**

Declare Subscription :

```bash
kubectl apply -f https://fyannk.github.io/cloudnative-pg/yamls/subscription.yaml
```

#### **RAW**

[filename](yamls/subscription.yaml ':include :type=code')

<!-- tabs:end -->
