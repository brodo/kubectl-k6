package internal

import (
	"fmt"
	"strings"
)

type K6Environment map[string]string

type K6Config struct {
	Env             K6Environment
	Args            string
	Image           string
	Parallelism     int
	ImagePullSecret string
	Folder          string
	FilePath        string
}

func NewK6Config(env K6Environment, args string, image string, parallelism int, imgPullSecret string, folder, filePath string) K6Config {
	return K6Config{
		Env:             env,
		Args:            args,
		Image:           image,
		Parallelism:     parallelism,
		ImagePullSecret: imgPullSecret,
		Folder:          folder,
		FilePath:        filePath,
	}

}

func (k6Env *K6Environment) ToMapSlice() []map[string]interface{} {
	envNv := make([]map[string]interface{}, len(*k6Env))
	i := 0
	for k, v := range *k6Env {
		envMap := make(map[string]interface{})
		upperName := strings.ToUpper(k)
		envMap["name"] = upperName
		envMap["value"] = v
		envNv[i] = envMap
		i++
	}
	return envNv
}

func (k6Env *K6Environment) String() string {
	infoStr := ""
	for k, v := range *k6Env {
		envMap := make(map[string]interface{})
		upperName := strings.ToUpper(k)
		envMap["name"] = upperName
		envMap["value"] = v
		infoStr += fmt.Sprintf("   %s: %s\n", upperName, v)
	}
	return infoStr
}
