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

	"github.com/google/blueprint/bootstrap"
)

func writeDocs(ctx *android.Context, filename string) error {
	moduleTypeList, err := bootstrap.ModuleTypeDocs(ctx.Context)
	if err != nil {
		return err
	}

	buf := &bytes.Buffer{}

	unique := 0

	tmpl, err := template.New("file").Funcs(map[string]interface{}{
		"unique": func() int {
			unique++
			return unique
		}}).Parse(fileTemplate)
	if err != nil {
		return err
	}

	err = tmpl.Execute(buf, moduleTypeList)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(filename, buf.Bytes(), 0666)
	if err != nil {
		return err
	}

	return nil
}

const (
	fileTemplate = `
<html>
<head>
<title>Build Docs</title>
<link rel="stylesheet" href="https://maxcdn.bootstrapcdn.com/bootstrap/3.3.5/css/bootstrap.min.css">
<script src="https://ajax.googleapis.com/ajax/libs/jquery/2.1.4/jquery.min.js"></script>
<script src="https://maxcdn.bootstrapcdn.com/bootstrap/3.3.5/js/bootstrap.min.js"></script>
</head>
<body>
<h1>Build Docs</h1>
<div class="panel-group" id="accordion" role="tablist" aria-multiselectable="true">
  {{range .}}
    {{ $collapseIndex := unique }}
    <div class="panel panel-default">
      <div class="panel-heading" role="tab" id="heading{{$collapseIndex}}">
        <h2 class="panel-title">
          <a class="collapsed" role="button" data-toggle="collapse" data-parent="#accordion" href="#collapse{{$collapseIndex}}" aria-expanded="false" aria-controls="collapse{{$collapseIndex}}">
             {{.Name}}
          </a>
        </h2>
      </div>
    </div>
    <div id="collapse{{$collapseIndex}}" class="panel-collapse collapse" role="tabpanel" aria-labelledby="heading{{$collapseIndex}}">
      <div class="panel-body">
        <p>{{.Text}}</p>
        {{range .PropertyStructs}}
          <p>{{.Text}}</p>
          {{template "properties" .Properties}}
        {{end}}
      </div>
    </div>
  {{end}}
</div>
</body>
</html>

{{define "properties"}}
  <div class="panel-group" id="accordion" role="tablist" aria-multiselectable="true">
    {{range .}}
      {{$collapseIndex := unique}}
      {{if .Properties}}
        <div class="panel panel-default">
          <div class="panel-heading" role="tab" id="heading{{$collapseIndex}}">
            <h4 class="panel-title">
              <a class="collapsed" role="button" data-toggle="collapse" data-parent="#accordion" href="#collapse{{$collapseIndex}}" aria-expanded="false" aria-controls="collapse{{$collapseIndex}}">
                 {{.Name}}{{range .OtherNames}}, {{.}}{{end}}
              </a>
            </h4>
          </div>
        </div>
        <div id="collapse{{$collapseIndex}}" class="panel-collapse collapse" role="tabpanel" aria-labelledby="heading{{$collapseIndex}}">
          <div class="panel-body">
            <p>{{.Text}}</p>
            {{range .OtherTexts}}<p>{{.}}</p>{{end}}
            {{template "properties" .Properties}}
          </div>
        </div>
      {{else}}
        <div>
          <h4>{{.Name}}{{range .OtherNames}}, {{.}}{{end}}</h4>
          <p>{{.Text}}</p>
          {{range .OtherTexts}}<p>{{.}}</p>{{end}}
          <p><i>Type: {{.Type}}</i></p>
          {{if .Default}}<p><i>Default: {{.Default}}</i></p>{{end}}
        </div>
      {{end}}
    {{end}}
  </div>
{{end}}
`
)
