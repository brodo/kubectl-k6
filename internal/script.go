package internal

import (
	"fmt"
	"github.com/brodo/kubectl-k6/internal/utils"
	"github.com/gobeam/stringy"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
)

type ScriptProperties struct {
	Cwd              string
	Script           string
	ScriptDir        string
	ScriptPath       string
	ScriptWOExt      string
	ScriptWOExtKebab string
	RunId            string
}

func (sp *ScriptProperties) ResourceName() string {
	return "run-" + sp.RunId
}

func (sp *ScriptProperties) ConfigMapName() string {
	return sp.RunId
}

func (sp *ScriptProperties) RunnerJobName(idx int) string {
	return fmt.Sprintf("%s-%d", sp.ResourceName(), idx+1)
}
func (sp *ScriptProperties) InitJobName() string {
	return fmt.Sprintf("%s-initializer", sp.ResourceName())
}

func NewScriptProperties(scriptPath string) ScriptProperties {
	dir, script := filepath.Split(scriptPath)
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			cobra.CheckErr(err)
		}
	}
	cwd, err := os.Getwd()
	cobra.CheckErr(err)
	cwd = filepath.Base(cwd)
	scriptWoExt := script[0 : len(script)-3]
	scriptWoExtKebab := stringy.New(scriptWoExt).KebabCase().Get()
	return ScriptProperties{
		Cwd:              cwd,
		Script:           script,
		ScriptDir:        filepath.Base(dir),
		ScriptPath:       scriptPath,
		ScriptWOExt:      scriptWoExt,
		ScriptWOExtKebab: scriptWoExtKebab,
		RunId:            utils.RandomString(20),
	}
}
