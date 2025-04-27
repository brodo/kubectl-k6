package internal_test

import (
	"context"
	"github.com/brodo/kubectl-k6/internal"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/k3s"
)

func TestK8sClient_CreateConfigMap(t *testing.T) {
	ctx := context.Background()
	k3sContainer, err := k3s.Run(ctx, "rancher/k3s:v1.27.1-k3s1")
	testcontainers.CleanupContainer(t, k3sContainer)
	require.NoError(t, err)
	kubeConfigYaml, err := k3sContainer.GetKubeConfig(ctx)
	require.NoError(t, err)

	restcfg, err := clientcmd.RESTConfigFromKubeConfig(kubeConfigYaml)
	require.NoError(t, err)
	err, k8sClient := internal.NewK8sClient(restcfg, "default")
	require.NoError(t, err)
	sps := internal.NewScriptProperties("../examples/liveness/liveness.js")
	content, err := os.ReadFile("../examples/liveness/liveness.js")
	require.NoError(t, err)
	err = k8sClient.CreateConfigMap(ctx, &sps, string(content))
	require.NoError(t, err)
	m, err := k8sClient.GetConfigMap(ctx, sps.ConfigMapName())
	require.NoError(t, err)
	require.Equal(t, sps.ConfigMapName(), m.Name)

}
