# Proxy

## Why ?

Because some clusters have strange and complicated architectures that implies going through proxies for outgoing flows.
One of those flows (main reason here) is the flow between Pods and S3 storage.

And I didn't want End Users having to cope with a Cloud's internal network needs (staying cloud agnostic is better)

## Behavior

Behaviour is :

- OLM on Openshift automatically set proxies if they are defined at the K8S Cluster level
- OLM set proxies on Operator's Pod if they are defined in the subscription
- Helm should support setting those env vars at install

The change was just declairing those env vars to pods, for clusters and poolers

## Risk

If definition of NO_PROXY is wrong, you can break pretty much everything (CNPG Operator and CNPG Clusters)
