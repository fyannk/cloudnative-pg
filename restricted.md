# Restricted Installations

## Why ?

Some K8S Cluster Admins do not want to install OLM, hence need to go with Helm.
But ClusterWide install is not possible because their clusters are mutualized and need absolute isolation.

Hence adapting CNPG stock helm chart to try and mitigate this issue. This is a work in progress.

## Behavior

Current problems solved :

- Operator MUST NOT have Nodes(Get,List) rights anymore, it SHOULD only (but there are risks invovled of CNPG Clusters being stuck in reconciliation)
- Operator do not crash anymore if PodMonitor CRD exists and it doesn't have Get,List access to it
- Operator do not need CRD(Get,List,Update) anymore if no OLM/CertManager manages certificates

## Risk

TODO
