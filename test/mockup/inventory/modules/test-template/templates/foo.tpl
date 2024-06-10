{{- define "foo" }}
This part of the template was defined among the common templates in {{ list .Mango.Metadata.ModuleName "templates/*.tpl" | join "/" | quote }}

{{ template "inspirational_quote" . | toString }}
{{- end }}

{{- define "inspirational_quote" }}
Don't take life too seriously, no one makes it out alive
{{- end }}
