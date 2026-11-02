{{/*
Expand the name of the chart.
*/}}
{{- define "sportsagentic.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "sportsagentic.fullname" -}}
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
{{- define "sportsagentic.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "sportsagentic.labels" -}}
helm.sh/chart: {{ include "sportsagentic.chart" . }}
{{ include "sportsagentic.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "sportsagentic.selectorLabels" -}}
app.kubernetes.io/name: {{ include "sportsagentic.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "sportsagentic.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "sportsagentic.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Return the Secret name holding CFBD_API_KEY. Fails when no key was provided,
because the server can't call CFBD without one and /health won't show it.
*/}}
{{- define "sportsagentic.secretName" -}}
{{- if .Values.secrets.existingSecret }}
{{- .Values.secrets.existingSecret }}
{{- else if trim (.Values.secrets.cfbdApiKey | default "") }}
{{- include "sportsagentic.fullname" . }}
{{- else }}
{{- fail "CFBD API key is missing: set secrets.existingSecret to a Secret holding it, or set secrets.cfbdApiKey to a non-empty value." }}
{{- end }}
{{- end }}

{{/*
Return the image tag as a string. A number from a values file or --reuse-values
is a float64 here, and toString would turn 8542126 into 8.542126e+06.
*/}}
{{- define "sportsagentic.imageTag" -}}
{{- $tag := .Values.image.tag | default .Chart.AppVersion -}}
{{- if kindIs "float64" $tag -}}
{{- if ne (floor $tag) $tag -}}
{{- fail (printf "image.tag %v is not a whole number, so YAML has already changed it. Quote it, or pass --set-string image.tag=<tag>." $tag) -}}
{{- end -}}
{{- printf "%.0f" $tag -}}
{{- else -}}
{{- $tag -}}
{{- end -}}
{{- end }}
