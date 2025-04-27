/*
Copyright © 2024 ifm julian.dax@ifm.com
*/
package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"path/filepath"
)

var (
	// Used for flags.
	k8sConfigPath string
	cfgFile       string
	// set using ldflags
	version string
)

var k8sConfig *rest.Config

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "kubectl-k6",
	Short: "Run k6 tests on a remote k8s server",
	Long: `This script can run k6 tests on a remote k8s server if a k6 operator is installed on that cluster.
For example:

kubectl-k6 run myTestScript.js
kubectl-k6 run myScriptFolder # Runs all the *.js files in this directory sequentially
`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&k8sConfigPath, "k8scfg", "", "k8s config file path (default is $HOME/.kube/config)")
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "kubectl-k6 config file path (default is $CWD/.kubectl-k6.yml)")
	if version == "" {
		panic("Version was not set when building kubectl-k6 binary!")
	}
	rootCmd.Version = version
	home, err := os.UserHomeDir()
	cobra.CheckErr(err)
	viper.SetDefault("k8scfg", filepath.Join(home, ".kube", "config"))
	viper.SetEnvPrefix("K6K8S")
	viper.AutomaticEnv()
}

func initConfig() {

	if k8sConfigPath == "" {
		k8sConfigPath = viper.GetString("k8scfg")
	}
	if os.Getenv("KUBECONFIG") != "" {
		k8sConfigPath = os.Getenv("KUBECONFIG")
	}

	var err error
	k8sConfig, err = clientcmd.BuildConfigFromFlags("", k8sConfigPath)
	cobra.CheckErr(err)

	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find cwd directory.
		cwd, err := os.Getwd()
		cobra.CheckErr(err)
		viper.AddConfigPath(cwd)
		viper.SetConfigType("yaml")
		viper.SetConfigName(".kubectl-k6.yml")
	}

	if err := viper.ReadInConfig(); err == nil {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}
	cobra.CheckErr(err)
}
