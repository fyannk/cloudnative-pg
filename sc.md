# StorageClass

## Why ?

Because I've seen many clusters having by default the cheapest storage possible.
And most often this storage is not usable even for having decent DB performances

## Behavior

Behaviour is :

1. What End User specified on clusters.postgresql.cnpg.io
2. If empty, what Operator has in DEFAULT_STORAGE_CLASS
3. If empty, what K8S Cluster has as default StorageClass

## Risk

Costs of storage
