package setup

import (
	"encoding/base64"
	"fmt"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/pivotal/kpack/pkg/apis/build/v1alpha1"
	"github.com/pivotal/kpack/pkg/client/clientset/versioned"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sclient "k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"strings"
)

func SetupEnv(client versioned.Interface, k8sClient k8sclient.Interface, registry string) error {
	err := setupClusterBuilder(client)
	if err != nil {
		return err
	}

	c, err := loadConfig(registry)
	if err != nil {
		return err
	}

	const secretName = "dockersecret"
	err = k8sClient.CoreV1().Secrets(v1.NamespaceDefault).Delete(secretName, &metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	_, err = k8sClient.CoreV1().Secrets(v1.NamespaceDefault).Create(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: secretName,
			Annotations: map[string]string{
				"build.pivotal.io/docker": c.registry,
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

	defaultServiceAccount, err := k8sClient.CoreV1().ServiceAccounts(v1.NamespaceDefault).Get(v1.NamespaceDefault, metav1.GetOptions{})

	_, err = k8sClient.CoreV1().ServiceAccounts(v1.NamespaceDefault).Update(&v1.ServiceAccount{
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

	username, password, ok := parseBasicAuth(basicAuth)
	if !ok {
		return config{}, fmt.Errorf("could not parse auth")
	}

	return config{
		username: username,
		password: password,
		registry: reg.Context().RegistryStr(),
	}, nil
}

// net/http request.go
func parseBasicAuth(auth string) (username, password string, ok bool) {
	const prefix = "Basic "
	// Case insensitive prefix match. See Issue 22736.
	if len(auth) < len(prefix) || !strings.EqualFold(auth[:len(prefix)], prefix) {
		return
	}
	c, err := base64.StdEncoding.DecodeString(auth[len(prefix):])
	if err != nil {
		return
	}
	cs := string(c)
	s := strings.IndexByte(cs, ':')
	if s < 0 {
		return
	}
	return cs[:s], cs[s+1:], true
}

func setupClusterBuilder(client versioned.Interface) error {
	const name = "default-builder"
	clusterBuilder, err := client.BuildV1alpha1().ClusterBuilders().Get(name, metav1.GetOptions{})
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
	} else {
		_, err = client.BuildV1alpha1().ClusterBuilders().Update(&v1alpha1.ClusterBuilder{
			ObjectMeta: clusterBuilder.ObjectMeta,
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
