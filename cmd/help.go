// Copyright 2022 SAP SE or an SAP affiliate company
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"bufio"
	"html/template"
	"io"
	"strings"
)

var (
	cliHelpTemplate = `
NAME:
{{printf "%s - %s" .Name .ShortDesc}}

USAGE:
{{printf "\t%s" .UsageLine}}

{{if .LongDesc}}
DESCRIPTION:
{{printf "\t%s" .LongDesc}}
{{end}}
`
	cliUsageTemplate = `dwd is a watch-dog which keeps an eye on kubernetes resources and uses a pre-defined configuration 
to scale up, scale down or stop pods (forcing a restart) based on watches/probes which monitor the health/reachability of defined
kubernetes resources.

Usage:
	<command> [arguments]
Supported commands:
{{range .}}
	{{printf "\t%s: " .Name}} {{.ShortDesc}}
{{end}}
`
)

// PrintHelp prints out the help text for the passed in command
func PrintHelp(cmdName string, w io.Writer) {
	if strings.TrimSpace(cmdName) == "" {
		PrintCliUsage(w)
		return
	}
	for _, cmd := range Commands {
		if cmdName == cmd.Name {
			executeTemplate(w, cliHelpTemplate, cmd)
			return
		}
	}
}

// PrintCliUsage prints the CLI usage text to the passed io.Writer
func PrintCliUsage(w io.Writer) {
	bufW := bufio.NewWriter(w)
	executeTemplate(w, cliUsageTemplate, Commands)
	_ = bufW.Flush()
}

func executeTemplate(w io.Writer, tmplText string, tmplData interface{}) {
	tmpl := template.Must(template.New("usage").Parse(tmplText))
	if err := tmpl.Execute(w, tmplData); err != nil {
		panic(err)
	}
}
