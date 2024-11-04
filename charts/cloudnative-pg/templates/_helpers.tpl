{{/*
Expand the name of the chart.
*/}}
{{- define "cloudnative-pg.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "cloudnative-pg.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "cloudnative-pg.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "cloudnative-pg.labels" -}}
helm.sh/chart: {{ include "cloudnative-pg.chart" . }}
{{ include "cloudnative-pg.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "cloudnative-pg.selectorLabels" -}}
app.kubernetes.io/name: {{ include "cloudnative-pg.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
cnpg.io/reconcile: "true"
{{- end }}
{{- define "cloudnative-pg.operatorSelector" -}}
app.kubernetes.io/name={{ include "cloudnative-pg.name" . }}
{{- end -}}

{{/*
Create the name of the service account to use
*/}}
{{- define "cloudnative-pg.serviceAccountName" -}}
{{- default (include "cloudnative-pg.fullname" .) .Values.operator.serviceAccount.name }}
{{- end }}

{{/*
Create the name of the service to use
*/}}
{{- define "cloudnative-pg.serviceName" -}}
{{- default (include "cloudnative-pg.fullname" .) .Values.operator.service.name }}
{{- end }}

{{/*
Create the name of the monitoring configmap to use
*/}}
{{- define "cloudnative-pg.monitoringName" -}}
{{- default (include "cloudnative-pg.fullname" .) .Values.operator.monitoring.name }}
{{- end }}

{{/*
Create the name of the configuration to use
*/}}
{{- define "cloudnative-pg.configurationName" -}}
{{- default (include "cloudnative-pg.fullname" .) .Values.operator.configuration.name }}
{{- end }}

{{/*
RBAC permissions to read nodes
*/}}
{{- define "cloudnative-pg.readNodesRbacRule" -}}
- apiGroups:
  - ""
  resources:
  - nodes
  verbs:
  - get
  - list
  - watch
{{- end -}}

{{/*
RBAC permissions on non-namespaced resources
*/}}
{{- define "cloudnative-pg.webhookRbacRules" -}}
- apiGroups:
  - admissionregistration.k8s.io
  resources:
  - mutatingwebhookconfigurations
  - validatingwebhookconfigurations
  verbs:
  - get
  - patch
{{- end -}}

{{/*
RBAC permissions on optional PodMonitor
*/}}
{{- define "cloudnative-pg.optionalPodMonitorRabcRule" -}}
- apiGroups:
  - monitoring.coreos.com
  resources:
  - podmonitors
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - watch
{{- end -}}

{{/*
RBAC permissions on optional VolumeSnapshot
*/}}
{{- define "cloudnative-pg.optionalVolumeSnapshotRabcRule" -}}
- apiGroups:
  - snapshot.storage.k8s.io
  resources:
  - volumesnapshots
  verbs:
  - create
  - get
  - list
  - patch
  - watch
{{- end -}}

{{/*
RBAC View permissions on CNPG objects
*/}}
{{- define "cloudnative-pg.viewRbacRules" -}}
- apiGroups:
  - postgresql.cnpg.io
  resources:
  - backups
  - clusters
  - databases
  - poolers
  - scheduledbackups
  - imagecatalogs
  - clusterimagecatalogs
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - postgresql.cnpg.io
  resources:
  - backups/status
  - clusters/status
  - databases/status
  - scheduledbackups/status
  verbs:
  - get
  - watch
{{- end -}}

{{/*
RBAC Edit permissions on CNPG objects
*/}}
{{- define "cloudnative-pg.editRbacRules" -}}
- apiGroups:
  - postgresql.cnpg.io
  resources:
  - backups
  - clusters
  - databases
  - poolers
  - scheduledbackups
  - imagecatalogs
  - clusterimagecatalogs
  verbs:
  - create
  - delete
  - patch
  - update
{{- end -}}

{{/*
RBAC Admin permissions on CNPG objects
*/}}
{{- define "cloudnative-pg.adminRbacRules" -}}
- apiGroups:
  - postgresql.cnpg.io
  resources:
  - clusters/finalizers
  - databases/finalizers
  - poolers/finalizers
  verbs:
  - update
- apiGroups:
  - postgresql.cnpg.io
  resources:
  - backups/status
  - clusters/status
  - databases/status
  - poolers/status
  verbs:
  - patch
  - update
{{- end -}}

{{/*
  Follow RFC 1035 and 1123
*/}}

{{/*
Determine the name for all external resources 
*/}}
{{- define "cloudnative-pg.namespacedName" -}}
{{- $name := include "cloudnative-pg.name" . -}}
{{ printf "%s-%s-postgresql-cnpg-io" .Release.Name .Release.Namespace | trunc 63 | trimSuffix "-" }}
{{- end -}}

{{/*
Determine the name for the mutating webhook 
*/}}
{{- define "cloudnative-pg.mutatingWebhookName" -}}
{{- $name := include "cloudnative-pg.namespacedName" . -}}
{{ printf "mutating-%s" $name | trunc 63 | trimSuffix "-" }}
{{- end -}}

{{/*
Determine the name for the validation webhook 
*/}}
{{- define "cloudnative-pg.validationWebhookName" -}}
{{- $name := include "cloudnative-pg.namespacedName" . -}}
{{ printf "validating-%s" $name | trunc 63 | trimSuffix "-" }}
{{- end -}}

{{/*
Determine the name for global user rbac viewer 
*/}}
{{- define "cloudnative-pg.globalRbacViewer" -}}
{{- "viewer-postgresql-cnpg-io" }}
{{- end -}}

{{/*
Determine the name for global user rbac editor 
*/}}
{{- define "cloudnative-pg.globalRbacEditor" -}}
{{- "editor-postgresql-cnpg-io" }}
{{- end -}}

{{/*
Determine the name for global user rbac admin 
*/}}
{{- define "cloudnative-pg.globalRbacAdmin" -}}
{{- "admin-postgresql-cnpg-io" }}
{{- end -}}