{{/* vim: set filetype=mustache: */}}

{{/*
RBAC permissions on Generic K8S objects
*/}}
{{- define "cloudnative-pg.rbac.basic" -}}
{{ $.Files.Get "includes/operator/basic.yaml" }}
{{- end -}}