package internal

import (
	"bytes"
	"text/template"
	"time"
)

type TemplateVars struct {
	FormatANSIC    string
	FormatRFC3339  string
	FormatTimeOnly string
	ScriptProperties
	Time time.Time
}

func NewTemplateVars(properties ScriptProperties) TemplateVars {
	return TemplateVars{
		ScriptProperties: properties,
		Time:             time.Now(),
		FormatRFC3339:    time.RFC3339,
		FormatTimeOnly:   time.TimeOnly,
		FormatANSIC:      time.ANSIC,
	}
}

func (tVars *TemplateVars) ApplyArgTemp(args string) (error, string) {
	argTmpl, err := template.New("argTemplate").Parse(args)
	if err != nil {
		return err, ""
	}
	k6Command := new(bytes.Buffer)
	err = argTmpl.Execute(k6Command, tVars)
	if err != nil {
		return err, ""
	}
	return nil, k6Command.String()
}

func (tVars *TemplateVars) ApplyEnvTemp(k6Env *K6Environment) error {
	for key, value := range *k6Env {
		envTmpl, err := template.New("envTemplate").Parse(value)
		if err != nil {
			return err
		}
		newValue := new(bytes.Buffer)
		err = envTmpl.Execute(newValue, tVars)
		if err != nil {
			return err
		}
		(*k6Env)[key] = newValue.String()
	}
	return nil
}
