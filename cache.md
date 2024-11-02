# Cache

## Why ?

Because after deploying CNPG Operator on a somewht big mutualised K8S Cluster in ClusterWide mode, pod went above 8GiB of RSS without event having to manage one CNPG Cluster.

## Behavior

#### If CLUSTER_WIDE_CACHE_FILTER=false

go back to stock CNPG behaviour.

ALL watchers results will be cached by the Operator (hence memory consuption)

#### If CLUSTER_WIDE_CACHE_FILTER unset or true

cache is filtered on ClusterWide installations

Mandatory label is : "cnpg.io/reconcile: true"

Impacted Objects:

- ConfigMap
- Secret
- Job
- PersistentVolumeClaim
- Pod
- Role
- RoleBinding
- Secret
- Service
- ServiceAccount
- PodMonitor

## Risk

If mandatory label is missing, Operator won't see objects. That can lead to infernal loops, like :

- CNPG Cluster is up and running, but without label
- CNPG Operator see the cluster, but no objects (because no label), so it tries to create them
- K8S refuses (because objects already exist)
- ... 
