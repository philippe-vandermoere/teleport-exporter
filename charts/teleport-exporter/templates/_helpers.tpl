{{/*
Renders a value through tpl, so it may itself contain Helm template
expressions (e.g. "{{ .Release.Name }}") evaluated against .context. Handles
both plain strings and complex values (lists/maps), which must go through
toYaml first since tpl only accepts a string.
Usage: {{ include "teleport-exporter.render" (dict "value" .Values.x "context" $) }}
*/}}
{{- define "teleport-exporter.render" -}}
{{- if typeIs "string" .value }}
{{- tpl .value .context }}
{{- else }}
{{- tpl (.value | toYaml) .context }}
{{- end }}
{{- end -}}

{{/*
Chart name.
*/}}
{{- define "teleport-exporter.name" -}}
{{- $nameOverride := include "teleport-exporter.render" (dict "value" .Values.nameOverride "context" .) }}
{{- default .Chart.Name $nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Fully qualified app name.
*/}}
{{- define "teleport-exporter.fullname" -}}
{{- $fullnameOverride := include "teleport-exporter.render" (dict "value" .Values.fullnameOverride "context" .) }}
{{- if $fullnameOverride }}
{{- $fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $nameOverride := include "teleport-exporter.render" (dict "value" .Values.nameOverride "context" .) }}
{{- $name := default .Chart.Name $nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{- define "teleport-exporter.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "teleport-exporter.labels" -}}
helm.sh/chart: {{ include "teleport-exporter.chart" . }}
{{ include "teleport-exporter.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "teleport-exporter.selectorLabels" -}}
app.kubernetes.io/name: {{ include "teleport-exporter.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "teleport-exporter.serviceAccountName" -}}
{{- $name := include "teleport-exporter.render" (dict "value" .Values.serviceAccount.name "context" .) }}
{{- if .Values.serviceAccount.create }}
{{- default (include "teleport-exporter.fullname" .) $name }}
{{- else }}
{{- default "default" $name }}
{{- end }}
{{- end }}

{{/*
Teleport proxy address (required).
*/}}
{{- define "teleport-exporter.address" -}}
{{- $address := include "teleport-exporter.render" (dict "value" .Values.teleport.address "context" .) }}
{{- required "teleport.address is required (host:port of the Teleport proxy)" $address }}
{{- end }}

{{/*
Teleport cluster name, used as the projected ServiceAccount token's
audience: teleport.clusterName, or else the host part of teleport.address.
*/}}
{{- define "teleport-exporter.clusterName" -}}
{{- $clusterName := include "teleport-exporter.render" (dict "value" .Values.teleport.clusterName "context" .) }}
{{- if $clusterName }}
{{- $clusterName }}
{{- else }}
{{- regexReplaceAll ":[0-9]+$" (include "teleport-exporter.address" .) "" }}
{{- end }}
{{- end }}

{{/*
Directory the identity file lives in, shared by tbot (writer) and the
exporter (reader).
*/}}
{{- define "teleport-exporter.identityDir" -}}
/var/run/teleport-identity
{{- end }}

{{/*
Exporter container args: extraArgs only.
*/}}
{{- define "teleport-exporter.args" -}}
{{- with .Values.extraArgs }}
{{ include "teleport-exporter.render" (dict "value" . "context" $) }}
{{- end }}
{{- end }}

{{/*
Exporter container env vars.
*/}}
{{- define "teleport-exporter.env" -}}
- name: TELEPORT_EXPORTER_ADDRESS
  value: {{ include "teleport-exporter.address" . | quote }}
- name: TELEPORT_EXPORTER_IDENTITY_FILE
  {{- if .Values.tbot.enabled }}
  value: {{ printf "%s/identity" (include "teleport-exporter.identityDir" .) | quote }}
  {{- else }}
  value: {{ printf "%s/%s" (include "teleport-exporter.identityDir" .) (include "teleport-exporter.render" (dict "value" .Values.identity.key "context" $)) | quote }}
  {{- end }}
{{- if .Values.teleport.insecure }}
- name: TELEPORT_EXPORTER_INSECURE
  value: "true"
{{- end }}
{{- if .Values.config.listenAddress }}
- name: TELEPORT_EXPORTER_LISTEN_ADDRESS
  value: {{ .Values.config.listenAddress | quote }}
{{- end }}
{{- if .Values.config.refreshInterval }}
- name: TELEPORT_EXPORTER_REFRESH_INTERVAL
  value: {{ .Values.config.refreshInterval | quote }}
{{- end }}
{{- if .Values.config.collectTimeout }}
- name: TELEPORT_EXPORTER_COLLECT_TIMEOUT
  value: {{ .Values.config.collectTimeout | quote }}
{{- end }}
{{- with .Values.extraEnv }}
{{ include "teleport-exporter.render" (dict "value" . "context" $) }}
{{- end }}
{{- end }}

{{/*
Volumes: the identity volume (an in-memory emptyDir written by tbot, or the
identity Secret), tbot's config and projected ServiceAccount token, plus
extraVolumes.
*/}}
{{- define "teleport-exporter.volumes" -}}
- name: identity
  {{- if .Values.tbot.enabled }}
  emptyDir:
    medium: Memory
  {{- else }}
  secret:
    secretName: {{ required "identity.existingSecret is required when tbot.enabled is false" (include "teleport-exporter.render" (dict "value" .Values.identity.existingSecret "context" $)) }}
  {{- end }}
{{- if .Values.tbot.enabled }}
- name: tbot-config
  configMap:
    name: {{ include "teleport-exporter.fullname" . }}-tbot
- name: join-sa-token
  projected:
    sources:
      - serviceAccountToken:
          path: join-sa-token
          audience: {{ include "teleport-exporter.clusterName" . | quote }}
          expirationSeconds: {{ .Values.tbot.serviceAccountTokenExpirationSeconds }}
{{- end }}
{{- with .Values.extraVolumes }}
{{ include "teleport-exporter.render" (dict "value" . "context" $) }}
{{- end }}
{{- end }}

{{/*
Exporter volume mounts: the identity volume (read-only), plus
extraVolumeMounts.
*/}}
{{- define "teleport-exporter.volumeMounts" -}}
- name: identity
  mountPath: {{ include "teleport-exporter.identityDir" . }}
  readOnly: true
{{- with .Values.extraVolumeMounts }}
{{ include "teleport-exporter.render" (dict "value" . "context" $) }}
{{- end }}
{{- end }}

{{/*
Renders an httpGet probe from a values block shaped like
{enabled, httpGet: {path, port}, initialDelaySeconds?, periodSeconds,
timeoutSeconds, failureThreshold}.
*/}}
{{- define "teleport-exporter.probe" -}}
httpGet:
  path: {{ .httpGet.path }}
  port: {{ .httpGet.port }}
{{- with .initialDelaySeconds }}
initialDelaySeconds: {{ . }}
{{- end }}
periodSeconds: {{ .periodSeconds }}
timeoutSeconds: {{ .timeoutSeconds }}
failureThreshold: {{ .failureThreshold }}
{{- end }}
