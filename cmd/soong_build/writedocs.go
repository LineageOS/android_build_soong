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
	"reflect"
	"sort"

	"github.com/google/blueprint/bootstrap"
	"github.com/google/blueprint/bootstrap/bpdoc"
)

type moduleTypeTemplateData struct {
	Name       string
	Synopsis   string
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
func moduleTypeDocsToTemplates(ctx *android.Context) ([]moduleTypeTemplateData, error) {
	moduleTypeFactories := android.ModuleTypeFactories()
	bpModuleTypeFactories := make(map[string]reflect.Value)
	for moduleType, factory := range moduleTypeFactories {
		bpModuleTypeFactories[moduleType] = reflect.ValueOf(factory)
	}

	packages, err := bootstrap.ModuleTypeDocs(ctx.Context, bpModuleTypeFactories)
	if err != nil {
		return []moduleTypeTemplateData{}, err
	}
	var moduleTypeList []*bpdoc.ModuleType
	for _, pkg := range packages {
		moduleTypeList = append(moduleTypeList, pkg.ModuleTypes...)
	}

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
	return result, err
}

func writeDocs(ctx *android.Context, filename string) error {
	buf := &bytes.Buffer{}

	// We need a module name getter/setter function because I couldn't
	// find a way to keep it in a variable defined within the template.
	currentModuleName := ""
	data, err := moduleTypeDocsToTemplates(ctx)
	if err != nil {
		return err
	}
	tmpl, err := template.New("file").Funcs(map[string]interface{}{
		"setModule": func(moduleName string) string {
			currentModuleName = moduleName
			return ""
		},
		"getModule": func() string {
			return currentModuleName
		},
	}).Parse(fileTemplate)
	if err == nil {
		err = tmpl.Execute(buf, data)
	}
	if err == nil {
		err = ioutil.WriteFile(filename, buf.Bytes(), 0666)
	}
	return err
}

const (
	fileTemplate = `
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
</head>
<body>
{{- /* Fixed sidebar with module types */ -}}
<ul>
<li><h3>Module Types:</h3></li>
{{range $moduleType := .}}<li><a href="#{{$moduleType.Name}}">{{$moduleType.Name}}</a></li>
{{end -}}
</ul>
{{/* Main panel with H1 section per module type */}}
<div style="margin-left:30ch;padding:1px 16px;">
<H1>Soong Modules Reference</H1>
The latest versions of Android use the Soong build system, which greatly simplifies build
configuration over the previous Make-based system. This site contains the generated reference
files for the Soong build system.
<p>
See the <a href=https://source.android.com/setup/build/build-system>Android Build System</a>
description for an overview of Soong and examples for its use.

{{range $imodule, $moduleType := .}}
	{{setModule $moduleType.Name}}
	<p>
  <h2 id="{{$moduleType.Name}}">{{$moduleType.Name}}</h2>
  {{if $moduleType.Synopsis }}{{$moduleType.Synopsis}}{{else}}<i>Missing synopsis</i>{{end}}
  {{- /* Comma-separated list of module attributes' links module attributes */ -}}
	<div class="breadcrumb">
    {{range $i,$prop := $moduleType.Properties }}
				{{ if gt $i 0 }},&nbsp;{{end -}}
				<a href=#{{getModule}}.{{$prop.Name}}>{{$prop.Name}}</a>
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
        {{- if .Text -}}{{.Text}}{{- end -}}
        {{- with .OtherTexts -}}{{.}}{{- end -}}<i>{{.Type}}</i>
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
)
