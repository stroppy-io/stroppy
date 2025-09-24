{{/*
Expand the name of the chart.
*/}}
{{- define "stroppy-cloud-pannel.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "stroppy-cloud-pannel.fullname" -}}
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
{{- define "stroppy-cloud-pannel.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "stroppy-cloud-pannel.labels" -}}
app.kubernetes.io/name: stroppy-cloud-pannel
helm.sh/chart: {{ include "stroppy-cloud-pannel.chart" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "stroppy-cloud-pannel.serviceAccountName" }}
{{- if (index .Values "stroppy-cloud-pannel").serviceAccount.create }}
{{- default (include "stroppy-cloud-pannel.fullname" .) (index .Values "stroppy-cloud-pannel").serviceAccount.name }}
{{- else }}
{{- default "default" (index .Values "stroppy-cloud-pannel").serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Extra labels for services
*/}}
{{- define "stroppy-cloud-pannel.extraServiceLabels"  -}}
{{- if (index .Values "stroppy-cloud-pannel").extraServiceLabels  }}
{{- toYaml (index .Values "stroppy-cloud-pannel").extraServiceLabels  }}
{{- end }}
{{- end }}

{{/*
Extra labels for pods
*/}}
{{- define "stroppy-cloud-pannel.extraPodLabels" -}}
{{- if (index .Values "stroppy-cloud-pannel").extraPodLabels  }}
{{- toYaml (index .Values "stroppy-cloud-pannel").extraPodLabels  }}
{{- end }}
{{- end }}

