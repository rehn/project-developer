{{- define "pd.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "pd.fullname" -}}
{{- if .Values.fullnameOverride }}{{ .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}{{ $n := default .Chart.Name .Values.nameOverride }}
{{- if contains $n .Release.Name }}{{ .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}{{ printf "%s-%s" .Release.Name $n | trunc 63 | trimSuffix "-" }}{{ end }}
{{- end }}
{{- end }}

{{- define "pd.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" }}
{{ include "pd.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "pd.selectorLabels" -}}
app.kubernetes.io/name: {{ include "pd.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "pd.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}{{ default (include "pd.fullname" .) .Values.serviceAccount.name }}
{{- else }}{{ default "default" .Values.serviceAccount.name }}{{ end }}
{{- end }}

{{- define "pd.claimName" -}}
{{- default (include "pd.fullname" .) .Values.persistence.existingClaim }}
{{- end }}

{{/* Browser origins allowed to make state-changing requests: ingress hosts + extras. */}}
{{- define "pd.allowedOrigins" -}}
{{- $scheme := ternary "https" "http" (gt (len .Values.ingress.tls) 0) -}}
{{- $o := list -}}
{{- if .Values.ingress.enabled }}{{ range .Values.ingress.hosts }}{{ $o = append $o (printf "%s://%s" $scheme .host) }}{{ end }}{{ end -}}
{{- if .Values.httpRoute.enabled }}{{ range .Values.httpRoute.hostnames }}{{ $o = append $o (printf "%s://%s" $.Values.httpRoute.scheme .) }}{{ end }}{{ end -}}
{{- range .Values.config.allowedOrigins }}{{ $o = append $o . }}{{ end -}}
{{- join "," $o -}}
{{- end }}
