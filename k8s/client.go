package k8s

import (
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

func BuildConfigFromFlags(masterURL, kubeconfigPath string) (*rest.Config, string, error) {

	var clientConfigLoader clientcmd.ClientConfigLoader

	if kubeconfigPath == "" {
		clientConfigLoader = clientcmd.NewDefaultClientConfigLoadingRules()
	} else {
		clientConfigLoader = &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath}
	}

	config := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientConfigLoader,
		&clientcmd.ConfigOverrides{ClusterInfo: api.Cluster{Server: masterURL}})

	namespace, _, _ := config.Namespace()

	clientConfig, err := config.ClientConfig()
	return clientConfig, namespace, err
}
