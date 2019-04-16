// Copyright 2017 Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"android/soong/android"
	"bytes"
	"html/template"
	"io/ioutil"
	"path/filepath"
	"reflect"
	"sort"

	"github.com/google/blueprint/bootstrap"
	"github.com/google/blueprint/bootstrap/bpdoc"
)

type perPackageTemplateData struct {
	Name    string
	Modules []moduleTypeTemplateData
}

type moduleTypeTemplateData struct {
	Name       string
	Synopsis   template.HTML
	Properties []bpdoc.Property
}

// The properties in this map are displayed first, according to their rank.
// TODO(jungjw): consider providing module type-dependent ranking
var propertyRank = map[string]int{
	"name":             0,
	"src":              1,
	"srcs":             2,
	"defautls":         3,
	"host_supported":   4,
	"device_supported": 5,
}

// For each module type, extract its documentation and convert it to the template data.
func moduleTypeDocsToTemplates(moduleTypeList []*bpdoc.ModuleType) []moduleTypeTemplateData {
	result := make([]moduleTypeTemplateData, 0)

	// Combine properties from all PropertyStruct's and reorder them -- first the ones
	// with rank, then the rest of the properties in alphabetic order.
	for _, m := range moduleTypeList {
		item := moduleTypeTemplateData{
			Name:       m.Name,
			Synopsis:   m.Text,
			Properties: make([]bpdoc.Property, 0),
		}
		props := make([]bpdoc.Property, 0)
		for _, propStruct := range m.PropertyStructs {
			props = append(props, propStruct.Properties...)
		}
		sort.Slice(props, func(i, j int) bool {
			if rankI, ok := propertyRank[props[i].Name]; ok {
				if rankJ, ok := propertyRank[props[j].Name]; ok {
					return rankI < rankJ
				} else {
					return true
				}
			}
			if _, ok := propertyRank[props[j].Name]; ok {
				return false
			}
			return props[i].Name < props[j].Name
		})
		// Eliminate top-level duplicates. TODO(jungjw): improve bpdoc to handle this.
		previousPropertyName := ""
		for _, prop := range props {
			if prop.Name == previousPropertyName {
				oldProp := &item.Properties[len(item.Properties)-1].Properties
				bpdoc.CollapseDuplicateProperties(oldProp, &prop.Properties)
			} else {
				item.Properties = append(item.Properties, prop)
			}
			previousPropertyName = prop.Name
		}
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func writeDocs(ctx *android.Context, filename string) error {
	moduleTypeFactories := android.ModuleTypeFactories()
	bpModuleTypeFactories := make(map[string]reflect.Value)
	for moduleType, factory := range moduleTypeFactories {
		bpModuleTypeFactories[moduleType] = reflect.ValueOf(factory)
	}

	packages, err := bootstrap.ModuleTypeDocs(ctx.Context, bpModuleTypeFactories)
	if err != nil {
		return err
	}

	// Produce the top-level, package list page first.
	tmpl := template.Must(template.Must(template.New("file").Parse(packageListTemplate)).Parse(copyBaseUrl))
	buf := &bytes.Buffer{}
	err = tmpl.Execute(buf, packages)
	if err == nil {
		err = ioutil.WriteFile(filename, buf.Bytes(), 0666)
	}

	// Now, produce per-package module lists with detailed information.
	for _, pkg := range packages {
		// We need a module name getter/setter function because I couldn't
		// find a way to keep it in a variable defined within the template.
		currentModuleName := ""
		tmpl := template.Must(
			template.Must(template.New("file").Funcs(map[string]interface{}{
				"setModule": func(moduleName string) string {
					currentModuleName = moduleName
					return ""
				},
				"getModule": func() string {
					return currentModuleName
				},
			}).Parse(perPackageTemplate)).Parse(copyBaseUrl))
		buf := &bytes.Buffer{}
		modules := moduleTypeDocsToTemplates(pkg.ModuleTypes)
		data := perPackageTemplateData{Name: pkg.Name, Modules: modules}
		err = tmpl.Execute(buf, data)
		if err != nil {
			return err
		}
		pkgFileName := filepath.Join(filepath.Dir(filename), pkg.Name+".html")
		err = ioutil.WriteFile(pkgFileName, buf.Bytes(), 0666)
		if err != nil {
			return err
		}
	}
	return err
}

// TODO(jungjw): Consider ordering by name.
const (
	packageListTemplate = `
<html>
<head>
<title>Build Docs</title>
<style>
#main {
  padding: 48px;
}

table{
  table-layout: fixed;
}

td {
  word-wrap:break-word;
}

/* The following entries are copied from source.android.com's css file. */
td,td code {
    color: #202124
}

th,th code {
    color: #fff;
    font: 500 16px/24px Roboto,sans-serif
}

td,table.responsive tr:not(.alt) td td:first-child,table.responsive td tr:not(.alt) td:first-child {
    background: rgba(255,255,255,.95);
    vertical-align: top
}

td,td code {
    padding: 7px 8px 8px
}

tr {
    border: 0;
    background: #78909c;
    border-top: 1px solid #cfd8dc
}

th,td {
    border: 0;
    margin: 0;
    text-align: left
}

th {
    height: 48px;
    padding: 8px;
    vertical-align: middle
}

table {
    border: 0;
    border-collapse: collapse;
    border-spacing: 0;
    font: 14px/20px Roboto,sans-serif;
    margin: 16px 0;
    width: 100%
}

h1 {
    color: #80868b;
    font: 300 34px/40px Roboto,sans-serif;
    letter-spacing: -0.01em;
    margin: 40px 0 20px
}

h1,h2,h3,h4,h5,h6 {
    overflow: hidden;
    padding: 0;
    text-overflow: ellipsis
}

:link,:visited {
    color: #039be5;
    outline: 0;
    text-decoration: none
}

body,html {
    color: #202124;
    font: 400 16px/24px Roboto,sans-serif;
    -moz-osx-font-smoothing: grayscale;
    -webkit-font-smoothing: antialiased;
    height: 100%;
    margin: 0;
    -webkit-text-size-adjust: 100%;
    -moz-text-size-adjust: 100%;
    -ms-text-size-adjust: 100%;
    text-size-adjust: 100%
}

html {
    -webkit-box-sizing: border-box;
    box-sizing: border-box
}

*,*::before,*::after {
    -webkit-box-sizing: inherit;
    box-sizing: inherit
}

body,div,dl,dd,form,img,input,figure,menu {
    margin: 0;
    padding: 0
}
</style>
{{template "copyBaseUrl"}}
</head>
<body>
<div id="main">
<H1>Soong Modules Reference</H1>
The latest versions of Android use the Soong build system, which greatly simplifies build
configuration over the previous Make-based system. This site contains the generated reference
files for the Soong build system.

<table class="module_types" summary="Table of Soong module types sorted by package">
  <thead>
    <tr>
      <th style="width:20%">Package</th>
      <th style="width:80%">Module types</th>
    </tr>
  </thead>
  <tbody>
    {{range $pkg := .}}
      <tr>
        <td>{{.Path}}</td>
        <td>
        {{range $i, $mod := .ModuleTypes}}{{if $i}}, {{end}}<a href="{{$pkg.Name}}.html#{{$mod.Name}}">{{$mod.Name}}</a>{{end}}
        </td>
      </tr>
    {{end}}
  </tbody>
</table>
</div>
</body>
</html>
`

	perPackageTemplate = `
<html>
<head>
<title>Build Docs</title>
<link rel="stylesheet" href="https://maxcdn.bootstrapcdn.com/bootstrap/4.2.1/css/bootstrap.min.css">
<style>
.accordion,.simple{margin-left:1.5em;text-indent:-1.5em;margin-top:.25em}
.collapsible{border-width:0 0 0 1;margin-left:.25em;padding-left:.25em;border-style:solid;
  border-color:grey;display:none;}
span.fixed{display: block; float: left; clear: left; width: 1em;}
ul {
	list-style-type: none;
  margin: 0;
  padding: 0;
  width: 30ch;
  background-color: #f1f1f1;
  position: fixed;
  height: 100%;
  overflow: auto;
}
li a {
  display: block;
  color: #000;
  padding: 8px 16px;
  text-decoration: none;
}

li a.active {
  background-color: #4CAF50;
  color: white;
}

li a:hover:not(.active) {
  background-color: #555;
  color: white;
}
</style>
{{template "copyBaseUrl"}}
</head>
<body>
{{- /* Fixed sidebar with module types */ -}}
<ul>
<li><h3>{{.Name}} package</h3></li>
{{range $moduleType := .Modules}}<li><a href="{{$.Name}}.html#{{$moduleType.Name}}">{{$moduleType.Name}}</a></li>
{{end -}}
</ul>
{{/* Main panel with H1 section per module type */}}
<div style="margin-left:30ch;padding:1px 16px;">
{{range $moduleType := .Modules}}
	{{setModule $moduleType.Name}}
	<p>
  <h2 id="{{$moduleType.Name}}">{{$moduleType.Name}}</h2>
  {{if $moduleType.Synopsis }}{{$moduleType.Synopsis}}{{else}}<i>Missing synopsis</i>{{end}}
  {{- /* Comma-separated list of module attributes' links module attributes */ -}}
	<div class="breadcrumb">
    {{range $i,$prop := $moduleType.Properties }}
				{{ if gt $i 0 }},&nbsp;{{end -}}
				<a href={{$.Name}}.html#{{getModule}}.{{$prop.Name}}>{{$prop.Name}}</a>
		{{- end -}}
  </div>
	{{- /* Property description */ -}}
	{{- template "properties" $moduleType.Properties -}}
{{- end -}}

{{define "properties" -}}
  {{range .}}
    {{if .Properties -}}
      <div class="accordion"  id="{{getModule}}.{{.Name}}">
        <span class="fixed">&#x2295</span><b>{{.Name}}</b>
        {{- range .OtherNames -}}, {{.}}{{- end -}}
      </div>
      <div class="collapsible">
        {{- .Text}} {{range .OtherTexts}}{{.}}{{end}}
        {{template "properties" .Properties -}}
      </div>
    {{- else -}}
      <div class="simple" id="{{getModule}}.{{.Name}}">
        <span class="fixed">&nbsp;</span><b>{{.Name}} {{range .OtherNames}}, {{.}}{{end -}}</b>
        <i>{{.Type}}</i>
        {{- if .Text -}}{{if ne .Text "\n"}}, {{end}}{{.Text}}{{- end -}}
        {{- with .OtherTexts -}}{{.}}{{- end -}}
	{{- if .Default -}}<i>Default: {{.Default}}</i>{{- end -}}
      </div>
    {{- end}}
  {{- end -}}
{{- end -}}
</div>
<script>
  accordions = document.getElementsByClassName('accordion');
  for (i=0; i < accordions.length; ++i) {
    accordions[i].addEventListener("click", function() {
      var panel = this.nextElementSibling;
      var child = this.firstElementChild;
      if (panel.style.display === "block") {
          panel.style.display = "none";
          child.textContent = '\u2295';
      } else {
          panel.style.display = "block";
          child.textContent = '\u2296';
      }
    });
  }
</script>
</body>
`

	copyBaseUrl = `
{{define "copyBaseUrl"}}
<script type="text/javascript">
window.addEventListener('message', (e) => {
  if (e != null && e.data != null && e.data.type === "SET_BASE" && e.data.base != null) {
    const existingBase = document.querySelector('base');
    if (existingBase != null) {
      existingBase.parentElement.removeChild(existingBase);
    }

    const base = document.createElement('base');
    base.setAttribute('href', e.data.base);
    document.head.appendChild(base);
  }
});
</script>
{{end}}
`
)
