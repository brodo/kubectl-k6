package internal

import (
	"errors"
	"fmt"
	"github.com/evanw/esbuild/pkg/api"
)

func Bundle(sps *ScriptProperties, minify bool) (error, []byte) {
	result := api.Build(api.BuildOptions{
		EntryPoints:       []string{sps.ScriptPath},
		Outfile:           "out.js",
		Bundle:            true,
		Write:             false,
		MinifyIdentifiers: minify,
		MinifySyntax:      minify,
		MinifyWhitespace:  minify,
		Platform:          api.PlatformNeutral,
		External:          []string{"k6*"},
		Target:            api.ES2015,
	})
	errs := make([]error, len(result.Errors))
	for i, message := range result.Errors {
		errs[i] = fmt.Errorf("%s", message.Text)
	}
	if len(result.OutputFiles) == 0 {
		return errors.Join(errs...), nil
	}
	return errors.Join(errs...), result.OutputFiles[0].Contents
}
