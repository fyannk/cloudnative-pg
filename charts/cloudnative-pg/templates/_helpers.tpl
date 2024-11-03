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
{{- if .Values.serviceAccount.create }}
{{- default (include "cloudnative-pg.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
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
RBAC permissions on Generic K8S objects
*/}}
{{- define "cloudnative-pg.genericRbacRules" -}}
- apiGroups:
  - ""
  resources:
  - configmaps
  - secrets
  - services
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - ""
  resources:
  - configmaps/status
  - secrets/status
  verbs:
  - get
  - patch
  - update
- apiGroups:
  - ""
  resources:
  - events
  verbs:
  - create
  - patch
- apiGroups:
  - ""
  resources:
  - persistentvolumeclaims
  - pods
  - pods/exec
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - watch
- apiGroups:
  - ""
  resources:
  - pods/status
  verbs:
  - get
- apiGroups:
  - ""
  resources:
  - serviceaccounts
  verbs:
  - create
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - apps
  resources:
  - deployments
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - batch
  resources:
  - jobs
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - watch
- apiGroups:
  - coordination.k8s.io
  resources:
  - leases
  verbs:
  - create
  - get
  - update
- apiGroups:
  - policy
  resources:
  - poddisruptionbudgets
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - rbac.authorization.k8s.io
  resources:
  - rolebindings
  - roles
  verbs:
  - create
  - get
  - list
  - patch
  - update
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
Determine the name for the mutating webhook 
*/}}
{{- define "cloudnative-pg.mutatingWebhookName" -}}
{{- $name := include "cloudnative-pg.name" . -}}
{{ printf "mutating.%s.%s.postgresql.cnpg.io" $name .Release.Namespace | trunc 63 | trimSuffix "-" }}
{{- end -}}

{{/*
Determine the name for the validation webhook 
*/}}
{{- define "cloudnative-pg.validationWebhookName" -}}
{{- $name := include "cloudnative-pg.name" . -}}
{{ printf "validating.%s.%s.postgresql.cnpg.io" $name .Release.Namespace | trunc 63 | trimSuffix "-" }}
{{- end -}}

{{/*
Determine the name for the webhook CR/CRB 
*/}}
{{- define "cloudnative-pg.webhookCluster" -}}
{{- $name := include "cloudnative-pg.name" . -}}
{{ printf "%s.%s.postgresql.cnpg.io" $name .Release.Namespace | trunc 63 | trimSuffix "-" }}
{{- end -}}