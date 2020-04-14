package setup

import (
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/pivotal/kpack/pkg/apis/build/v1alpha1"
	"github.com/pivotal/kpack/pkg/client/clientset/versioned"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sclient "k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

func SetupEnv(client versioned.Interface, k8sClient k8sclient.Interface, registry, namespace string) error {
	err := setupCustomBuilder(client)
	if err != nil {
		return err
	}

	c, err := loadConfig(registry)
	if err != nil {
		return err
	}

	const secretName = "dockersecret"
	err = k8sClient.CoreV1().Secrets(namespace).Delete(secretName, &metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	_, err = k8sClient.CoreV1().Secrets(namespace).Create(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
			Annotations: map[string]string{
				"build.pivotal.io/docker": func() string {
					if c.registry == "index.docker.io" {
						return "https://index.docker.io/v1/"
					}
					return c.registry
				}(),
			},
		},
		StringData: map[string]string{
			"username": c.username,
			"password": c.password,
		},
		Type: v1.SecretTypeBasicAuth,
	})
	if err != nil {
		return err
	}

	defaultServiceAccount, err := k8sClient.CoreV1().ServiceAccounts(namespace).Get("default", metav1.GetOptions{})

	_, err = k8sClient.CoreV1().ServiceAccounts(namespace).Update(&v1.ServiceAccount{
		ObjectMeta: defaultServiceAccount.ObjectMeta,
		Secrets: []v1.ObjectReference{
			{
				Name: secretName,
			},
		},
	})
	if err != nil {
		return err
	}

	return nil
}

type config struct {
	username string
	password string
	registry string
}

func loadConfig(registry string) (config, error) {

	reg, err := name.ParseReference(registry+"/something", name.WeakValidation)
	if err != nil {
		return config{}, err
	}

	auth, err := authn.DefaultKeychain.Resolve(reg.Context().Registry)
	if err != nil {
		return config{}, err
	}

	basicAuth, err := auth.Authorization()
	if err != nil {
		return config{}, err
	}

	return config{
		username: basicAuth.Username,
		password: basicAuth.Password,
		registry: reg.Context().RegistryStr(),
	}, nil
}

func setupCustomBuilder(client versioned.Interface) error {
	const name = "default"
	_, err := client.ExperimentalV1alpha1().CustomClusterBuilders().Get(name, metav1.GetOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	if errors.IsNotFound(err) {
		_, err = client.BuildV1alpha1().ClusterBuilders().Create(&v1alpha1.ClusterBuilder{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Spec: v1alpha1.BuilderSpec{
				Image:        "cloudfoundry/cnb:bionic",
				UpdatePolicy: v1alpha1.Polling,
			},
		})
		if err != nil {
			return err
		}
	}
	return nil
}
