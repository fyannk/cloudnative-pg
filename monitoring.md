# Monitoring

## Why ?

Because actual monitoring renders Prometheus Operator almost mandatory
Needed a simple solution to integrate with good old Prometheus scapes by labels/annotations

## Behavior

Current solution is a bit ugly as it involves enforcing labels/annotations everywhere.

Needs to set this env/config/secret for Operator :

MANDATORY_ANNOTATIONS: prometheus.io/port=9187, prometheus.io/scrape=true

Better solution would be to create a flag at the operator level:
- operator (default): Prometheus Operator (makes PodMonitor)
- scrape: Prometheus Scrape (makes scrape annotations on Pods / Services?)

## Risk

