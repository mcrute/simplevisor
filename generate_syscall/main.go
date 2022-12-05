package main

import (
	"bytes"
	"go/format"
	"html/template"
	"os"
	"strings"

	"golang.org/x/tools/go/packages"
)

var tpl = template.Must(template.New("").Parse(`package supervise

// GENERATED FILE, DO NOT MODIFY

import "syscall"

var signalMap = map[string]syscall.Signal{
{{- range $name := . }}
	"{{ . }}": syscall.SIG{{ . }},
{{- end }}
}
`))

func main() {
	pkgs, err := packages.Load(&packages.Config{Mode: packages.NeedTypes}, "syscall")
	if err != nil {
		panic(err)
	}

	syms := []string{}
	scope := pkgs[0].Types.Scope()
	for _, n := range scope.Names() {
		o := scope.Lookup(n)
		if strings.HasPrefix(n, "SIG") && o.Type().String() == "syscall.Signal" {
			syms = append(syms, strings.TrimPrefix(n, "SIG"))
		}
	}

	buf := bytes.Buffer{}
	tpl.Execute(&buf, syms)

	src, err := format.Source(buf.Bytes())
	if err != nil {
		panic(err)
	}

	if err := os.WriteFile("zzz_syscall_map.go", src, 0666); err != nil {
		panic(err)
	}
}
