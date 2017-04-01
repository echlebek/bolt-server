package server

import (
	"html/template"
	"path"
)

var funcs = template.FuncMap{
	"Join": path.Join,
}

var keysTmpl = template.Must(template.New("keys").Funcs(funcs).Parse(keysTemplate))

type KeyPkg struct {
	Path string
	Keys []string
}

const keysTemplate = `<html>
	<head>
		<meta charset="UTF-8">
		<style>
		.body {
			padding: 10px;
			font-family: sans-serif;
		}
		h3 {
			font-weight: normal;
		}
		.item {
			list-style: none;
			padding: 2px;
		}
		</style>
		<title>{{ .Path }}</title>
	</head>
	<body>
		<div class="body">
			<div class="title"><h3>{{ .Path }}</h3></div>
			{{ if .Keys }}
			<ul>
				{{ range $index, $element := .Keys }}
				<div class="item">
					<li><a href="{{ Join $.Path $element }}">{{ $element }}</a></li>
				</div>
				{{ end }}
			</ul>
			{{ else }}
				<div class="info"><h3>Empty bucket.</h3></div>
			{{ end }}
		</div>
	</body>
</html>`
