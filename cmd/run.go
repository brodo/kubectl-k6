package cmd

import (
	"context"
	"encoding/csv"
	"fmt"
	"github.com/brodo/kubectl-k6/internal"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

type configuration struct {
	namespace       string
	k6Arguments     string                 `mapstructure:"arguments"`
	k6Env           internal.K6Environment `mapstructure:"env"`
	parallelism     int
	dockerImage     string `mapstructure:"image"`
	imagePullSecret string `mapstructure:"ips"`
	minify          bool
	folder          string
}

var config = configuration{}

// runCmd represents the run command
var runCmd = &cobra.Command{
	Use:   "run [k6 script path]",
	Short: "Run one or more k6 scripts on a k8s cluster",
	Long: `This script can run k6 tests on a remote k8s server if a k6 operator is installed on that cluster.
For example:

k6k8s run myTestScript.js`,
	RunE: func(cmd *cobra.Command, args []string) error {
		loadRunConfig()
		scriptPath := args[0]
		sps := internal.NewScriptProperties(scriptPath)
		err, kc := internal.NewK8sClient(k8sConfig, config.namespace)
		cobra.CheckErr(err)

		templateVars := internal.NewTemplateVars(sps)
		err, k6args := templateVars.ApplyArgTemp(config.k6Arguments)
		cobra.CheckErr(err)
		err = templateVars.ApplyEnvTemp(&config.k6Env)
		cobra.CheckErr(err)

		fmt.Printf("Running k6 with the following arguments: %s\n", k6args)
		fmt.Printf("Running k6 with the following environment variables:\n%s\n", config.k6Env.String())
		fmt.Println("Running pre clean-up...")
		err = kc.DeleteResources(context.Background(), &sps)
		cobra.CheckErr(err)
		k6Config := internal.NewK6Config(config.k6Env, k6args, config.dockerImage, config.parallelism, config.imagePullSecret, config.folder, scriptPath)
		if config.folder == "" {
			fmt.Println("Bundling script...")
			err, jsBundle := internal.Bundle(&sps, config.minify)
			cobra.CheckErr(err)
			if len(jsBundle) > 1048576 {
				return fmt.Errorf("the bundled script is too large: %d MB, max 1 MB - please use `--folder`", len(jsBundle)/1_048_576)
			}
			fmt.Printf("Uploading config map '%s'...\n", sps.ConfigMapName())
			err = kc.CreateConfigMap(context.Background(), &sps, string(jsBundle))
			cobra.CheckErr(err)
		} else {
			err, baseDir := internal.CreateTempFolder(config.folder)
			cobra.CheckErr(err)
			fmt.Printf("Uploading folder '%s' to persistant volume '%s'...\n", config.folder, sps.ConfigMapName())
			err = kc.UploadFolderToPV(context.Background(), baseDir, sps.ConfigMapName(), config.namespace)
			cobra.CheckErr(err)
			err = kc.CreatePVC(context.Background(), sps.ConfigMapName(), sps.ConfigMapName(), config.namespace)
			cobra.CheckErr(err)
		}

		fmt.Printf("Uploading k6 custom resource '%s'...\n", sps.ResourceName())
		err = kc.CreateCustomResource(context.Background(), &k6Config, &templateVars)
		if err != nil {
			fmt.Printf("Error creating custom resource '%s': %v\n", sps.ResourceName(), err)
			return err
		}
		fmt.Println("Waiting for initialization phase...")
		waitCtx, cancel := context.WithTimeout(context.Background(), time.Minute*3)
		err = kc.WaitForStage(waitCtx, sps.ResourceName(), internal.InitializationStage)
		cancel()
		if err != nil {
			logs, logErr := kc.GetOperatorLogsSince(context.Background(), templateVars.Time)
			fmt.Printf("Error in initialization phase for '%s': %v\n", sps.ResourceName(), err)
			if logErr != nil {
				fmt.Printf("Error getting operator logs: %v\n", logErr)
			} else {
				fmt.Printf("Operator logs since %s:\n%s\n", templateVars.Time.Format(time.RFC3339), logs)
			}
			return err
		}
		fmt.Println("Waiting for initialization job to complete...")
		waitCtx, cancel = context.WithTimeout(context.Background(), time.Minute*3)
		err = kc.WaitForInitJobCompletion(waitCtx, &sps, templateVars.Time)
		cancel()
		if err != nil {
			logs, logErr := kc.GetOperatorLogsSince(context.Background(), templateVars.Time)
			fmt.Printf("Init job '%s' did not complete in three minutes! Trying to get logs.\n %v ,\n", sps.InitJobName(), err)
			if logErr != nil {
				fmt.Printf("Error getting operator logs: %v\n", logErr)
			} else {
				if logs == "" {
					logs = "The operator did not log any errors."
				} else {
					fmt.Printf("Operator logs since %s:\n%s\n", templateVars.Time.Format(time.RFC3339), logs)
				}
			}
			logObjs, logErr := kc.GetJobPodLogs(context.Background(), sps.InitJobName())
			if logErr != nil {
				fmt.Printf("Error getting logs for job '%s': %v\n", sps.InitJobName(), logErr)
			} else {
				for _, obj := range logObjs {
					if obj.Logs == "" {
						fmt.Printf("The pod '%s' did not log anything.\n", obj.PodName)
					} else {
						fmt.Printf("Logs for pod '%s':\n%s\n", obj.PodName, obj.Logs)
					}
				}
			}

			return err
		}
		fmt.Println("Waiting for run jobs to be created...")
		waitCtx, cancel = context.WithTimeout(context.Background(), time.Minute*10)
		err = kc.WaitForStage(waitCtx, sps.ResourceName(), internal.CreatedStage)
		cancel()
		if err != nil {
			logs, logErr := kc.GetOperatorLogsSince(context.Background(), templateVars.Time)
			fmt.Printf("Error in creation phase for '%s': %v\n\n", sps.ResourceName(), err)
			if logErr != nil {
				fmt.Printf("Error getting operator logs: %v\n", logErr)
			} else {
				fmt.Printf("Operator logs since %s:\n%s\n", templateVars.Time.Format(time.RFC3339), logs)
			}

			fmt.Println("Getting logs for run jobs...")

			for i := 0; i < config.parallelism; i++ {
				logs, logErr := kc.GetPodLogs(context.Background(), sps.RunnerJobName(i), templateVars.Time)
				if logErr != nil {
					fmt.Printf("Error getting logs for job '%s': %v\n", sps.RunnerJobName(i), logErr)
				} else {
					fmt.Printf("Logs for job '%s':\n%s\n", sps.RunnerJobName(i), logs)
				}
			}
			return err
		}
		fmt.Println("Waiting for run jobs to complete...")
		if config.parallelism == 1 {
			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				defer wg.Done()
				waitCtx, cancel = context.WithTimeout(context.Background(), time.Hour)
				err = kc.WaitForRunJobCompletion(waitCtx, &sps, &k6Config, templateVars.Time)
				cancel()
			}()

			go func() {
				defer wg.Done()
				fmt.Println("BEGIN k6 LOGS:")
				rc, err := kc.GetPodLogStream(context.Background(), sps.RunnerJobName(0), templateVars.Time)
				defer rc.Close()
				if err != nil {
					return
				}
				for {
					buf := make([]byte, 2000)
					numBytes, err := rc.Read(buf)
					if err == io.EOF {
						break
					}
					if err != nil {
						fmt.Printf("Error getting log stream!\n %v", err)
						break
					}
					if numBytes == 0 {
						continue
					}

					message := string(buf[:numBytes])
					fmt.Print(message)
				}
				fmt.Println("END k6 LOGS")
			}()

			wg.Wait()
		} else {
			waitCtx, cancel = context.WithTimeout(context.Background(), time.Hour)
			err = kc.WaitForRunJobCompletion(waitCtx, &sps, &k6Config, templateVars.Time)
			cancel()
			if err != nil {
				fmt.Printf("Error running run jobs!\n %v", err)
				return err
			}
		}

		fmt.Println("All jobs completed successfully!")
		if config.parallelism > 1 {
			for i := 0; i < config.parallelism; i++ {
				logs, logErr := kc.GetPodLogs(context.Background(), sps.RunnerJobName(i), templateVars.Time)
				if logErr != nil {
					fmt.Printf("Error getting logs for job '%s': %v\n", sps.RunnerJobName(i), logErr)
				} else {
					fmt.Printf("Logs for job '%s':\n%s\n", sps.RunnerJobName(i), logs)
				}
			}
		}

		fmt.Println("Cleaning up...")
		delErr := kc.DeleteResources(context.Background(), &sps)
		if delErr != nil {
			fmt.Printf("Error cleaning up resources: %v\n", delErr)
		}
		return nil
	},
	Args: cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
}

func init() {
	rootCmd.AddCommand(runCmd)
	runCmd.SilenceUsage = true

	const defaultNamespace = "k6-operator-system"
	runCmd.Flags().StringVarP(&config.namespace, "namespace", "n", defaultNamespace, "k8s namespace to run in")

	runCmd.Flags().StringVarP(&config.k6Arguments, "arguments", "a",
		"",
		`runs k6 with the given arguments. 
You can provide a go template string (https://pkg.go.dev/text/template) here. See the documentation for supported variables.`)

	runCmd.Flags().StringToStringVarP((*map[string]string)(&config.k6Env), "env", "e", make(internal.K6Environment),
		`runs k6 with the given environment arguments.
You can provide a go template string (https://pkg.go.dev/text/template) here. See the documentation for supported variables.`)

	runCmd.Flags().IntVarP(&config.parallelism, "parallelism", "p", 1, "How many times a script should be run in parallel. Every parallel execution starts a k8s job.")

	runCmd.Flags().StringVarP(&config.dockerImage, "image", "i", "", "The OCI image to use for running k6")
	runCmd.Flags().StringVarP(&config.dockerImage, "ips", "s", "ifm-jfrog", "The name of the secret to use for pulling the OCI image. This is only used if the image is private.")
	runCmd.Flags().BoolVarP(&config.minify, "minify", "m", false, "Minify Javascript before uploading it to the cluster")
	runCmd.Flags().StringVarP(&config.folder, "folder", "f", "", "Uploads the provided a folder into a persistent volume on k8s.")

	cobra.CheckErr(viper.BindPFlags(runCmd.Flags()))
	viper.SetDefault("namespace", defaultNamespace)
	viper.SetDefault("arguments", "")
	viper.SetDefault("env", make(internal.K6Environment))
	viper.SetDefault("ips", "ifm-jfrog")
	viper.SetDefault("image", "")
	viper.SetDefault("parallelism", 1)
	viper.SetDefault("minify", false)
	viper.SetDefault("folder", "")
}

func loadRunConfig() {
	config.namespace = viper.GetString("namespace")
	config.k6Arguments = viper.GetString("arguments")
	config.parallelism = viper.GetInt("parallelism")
	config.dockerImage = viper.GetString("image")
	config.imagePullSecret = viper.GetString("ips")
	config.minify = viper.GetBool("minify")
	config.folder = viper.GetString("folder")

	// This is clumsy, but it is currently the only way to get a string map from an env variable.
	// I've tested `Unmarshal` and `UnmarshalKey` but they don't work. I've also tested `viper.GetStringMapString`
	// but it doesn't work for env variables either. There are open issues about this on the viper repo.
	if os.Getenv("K6K8S_ENV") == "" {
		config.k6Env = viper.GetStringMapString("env")
	} else {
		env := viper.GetString("env")
		r := csv.NewReader(strings.NewReader(env))
		for {
			record, err := r.Read()
			if err == io.EOF {
				break
			}
			cobra.CheckErr(err)
			for _, kv := range record {
				kvPair := strings.SplitN(kv, "=", 2)
				if len(kvPair) != 2 {
					cobra.CheckErr(fmt.Errorf("error reading k6 env from K6K8S_ENV, key-value pair '%s' is invalid", kv))
				}
				config.k6Env[kvPair[0]] = kvPair[1]
			}
		}
	}
}
