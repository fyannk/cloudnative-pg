{{/* vim: set filetype=mustache: */}}

{{- define "cloudnative-pg.cluster.shouldWebhooks" -}}
{{- if not .Values.features.cluster.enabled -}}
{{- "false" }}
{{- else if not .Values.features.cluster.webhooks -}}
{{- "false" }}
{{- else if .Values.capabilities.secureNamespace -}} {{/* Namespaced but secure install */}}
{{- "true" }}
{{- else -}}
{{- "false" }}
{{- end -}}
{{- end -}}

{{- define "cloudnative-pg.cluster.shouldManageWebhooks" -}}
{{- $shouldWebhooks := include "cloudnative-pg.cluster.shouldWebhooks" . -}}
{{- if eq $shouldWebhooks "true" -}} {{/* WebHooks are activated*/}}
{{- if .Values.capabilities.secureNamespace -}} {{/* Namespace is secured */}}
{{- if not .Values.cluster.webhooks.certManagerCert -}} {{/* CertManager is not used */}}
{{- if .Values.cluster.webhooks.manageCerts -}} {{/* Operator is asked to manage them */}}
{{- "true" -}}
{{- else -}}
{{- "false" -}}
{{- end -}} {{/* End manageCerts */}}
{{- else -}}
{{- "false" -}}
{{- end -}} {{/* End CertManager */}}
{{- else -}}
{{- "false" -}}
{{- end -}} {{/* End secureNamespace */}}
{{- else -}}
{{- "false" -}}
{{- end -}} {{/* End should */}}
{{- end -}} {{/* End define */}}

{{- define "cloudnative-pg.cluster.shouldRole" -}}
{{- if not .Values.features.cluster.enabled -}}
{{- "false" }}
{{- else if not .Values.features.cluster.role -}}
{{- "false" }}
{{- else if eq (len .Values.managedNamespaces) 0 -}} {{/* ClusterWide install */}}
{{- "true" }}
{{- else if .Values.capabilities.secureNamespace -}} {{/* Namespaced but secure install */}}
{{- "true" }}
{{- else -}}
{{- "false" }}
{{- end -}}
{{- end -}}

{{- define "cloudnative-pg.cluster.shouldRoleBinding" -}}
{{- if not .Values.features.cluster.enabled -}}
{{- "false" }}
{{- else if not .Values.features.cluster.rolebinding -}}
{{- "false" }}
{{- else if eq (len .Values.managedNamespaces) 0 -}} {{/* ClusterWide install */}}
{{- "true" }}
{{- else if .Values.capabilities.secureNamespace -}} {{/* Namespaced but secure install */}}
{{- "true" }}
{{- else -}}
{{- "false" }}
{{- end -}}
{{- end -}}

{{- define "cloudnative-pg.cluster.shouldUserRbac" -}}
{{- if not .Values.features.cluster.enabled -}}
{{- "false" }}
{{- else if not .Values.features.cluster.user -}}
{{- "false" }}
{{- else if eq (len .Values.managedNamespaces) 0 -}} {{/* ClusterWide install */}}
{{- "true" }}
{{- else -}}
{{- "false" }}
{{- end -}}
{{- end -}}

{{- define "cloudnative-pg.operator.shouldConfiguration" -}}
{{- if not .Values.features.operator.enabled -}}
{{- "false" }}
{{- else if not .Values.features.operator.configuration -}}
{{- "false" }}
{{- else -}}
{{- "true" }}
{{- end -}}
{{- end -}}

{{- define "cloudnative-pg.operator.shouldDeployment" -}}
{{- if not .Values.features.operator.enabled -}}
{{- "false" }}
{{- else if not .Values.features.operator.deployment -}}
{{- "false" }}
{{- else -}}
{{- "true" }}
{{- end -}}
{{- end -}}

{{- define "cloudnative-pg.operator.shouldMonitoring" -}}
{{- if not .Values.features.operator.enabled -}}
{{- "false" }}
{{- else if not .Values.features.operator.monitoring -}}
{{- "false" }}
{{- else -}}
{{- "true" }}
{{- end -}}
{{- end -}}

{{- define "cloudnative-pg.operator.shouldNetworkPolicy" -}}
{{- if not .Values.features.operator.enabled -}}
{{- "false" }}
{{- else if not .Values.features.operator.networkpolicy -}}
{{- "false" }}
{{- else -}}
{{- "true" }}
{{- end -}}
{{- end -}}

{{- define "cloudnative-pg.operator.shouldPodMonitor" -}}
{{- if not .Values.features.operator.enabled -}}
{{- "false" }}
{{- else if not .Values.features.operator.podmonitor -}}
{{- "false" }}
{{- else -}}
{{- "true" }}
{{- end -}}
{{- end -}}

{{- define "cloudnative-pg.operator.shouldRole" -}}
{{- if not .Values.features.operator.enabled -}}
{{- "false" }}
{{- else if not .Values.features.operator.role -}}
{{- "false" }}
{{- else if eq (len .Values.managedNamespaces) 0 -}} {{/* ClusterWide install */}}
{{- "false" }}
{{- else -}}
{{- "true" }}
{{- end -}}
{{- end -}}

{{- define "cloudnative-pg.operator.shouldRoleBinding" -}}
{{- if not .Values.features.operator.enabled -}}
{{- "false" }}
{{- else if not .Values.features.operator.rolebinding -}}
{{- "false" }}
{{- else if eq (len .Values.managedNamespaces) 0 -}} {{/* ClusterWide install */}}
{{- "false" }}
{{- else -}}
{{- "true" }}
{{- end -}}
{{- end -}}

{{- define "cloudnative-pg.operator.shouldService" -}}
{{- if not .Values.features.operator.enabled -}}
{{- "false" }}
{{- else if not .Values.features.operator.service -}}
{{- "false" }}
{{- else -}}
{{- "true" }}
{{- end -}}
{{- end -}}

{{- define "cloudnative-pg.operator.shouldServiceAccount" -}}
{{- if not .Values.features.operator.enabled -}}
{{- "false" }}
{{- else if not .Values.features.operator.serviceaccount -}}
{{- "false" }}
{{- else -}}
{{- "true" }}
{{- end -}}
{{- end -}}

{{- define "cloudnative-pg.managed.shouldRole" -}}
{{- if not .Values.features.managed.enabled -}}
{{- "false" }}
{{- else if not .Values.features.managed.role -}}
{{- "false" }}
{{- else if eq (len .Values.managedNamespaces) 0 -}} {{/* ClusterWide install */}}
{{- "false" }}
{{- else }}
{{- "true" }}
{{- end -}}
{{- end -}}

{{- define "cloudnative-pg.managed.shouldRoleBinding" -}}
{{- if not .Values.features.managed.enabled -}}
{{- "false" }}
{{- else if not .Values.features.managed.rolebinding -}}
{{- "false" }}
{{- else if eq (len .Values.managedNamespaces) 0 -}} {{/* ClusterWide install */}}
{{- "false" }}
{{- else }}
{{- "true" }}
{{- end -}}
{{- end -}}
