package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/buildpack/pack/logging"
	"github.com/matthewmcnew/packo/k8s"
	"github.com/matthewmcnew/packo/setup"
	"github.com/matthewmcnew/packo/upload"
	"github.com/matthewmcnew/packo/wait"
	"github.com/pivotal/kpack/pkg/apis/build/v1alpha1"
	"github.com/pivotal/kpack/pkg/client/clientset/versioned"
	"github.com/pivotal/kpack/pkg/client/informers/externalversions"
	"github.com/pivotal/kpack/pkg/logs"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	k8sclient "k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	duckv1alpha1 "knative.dev/pkg/apis/duck/v1alpha1"
	"log"
	"os"
	"time"
)

var (
	registry = flag.String("registry", "", "registry")
	path     = flag.String("path", "~/workspace/kpack", "path")
)

func main() {
	flag.Parse()

	if *path == "" {
		log.Fatal("No registry provided. Please provide a registry with --registry")
	}

	clusterConfig, err := k8s.BuildConfigFromFlags("", "")
	if err != nil {
		log.Fatalf("Error building kubeconfig: %v", err)
	}

	client, err := versioned.NewForConfig(clusterConfig)
	if err != nil {
		log.Fatalf("could not get Build client: %s", err)
	}

	k8sClient, err := kubernetes.NewForConfig(clusterConfig)
	if err != nil {
		log.Fatalf("could not get kubernetes client: %s", err.Error())
	}

	err = setup.SetupEnv(client, k8sClient, *registry)
	if err != nil {
		log.Fatalf("could not setup env: %s", err.Error())
	}

	fmt.Printf("Uploading %s ... \n", *path)
	image, err := upload.Upload(*path, *registry)
	if err != nil {
		log.Fatalf("could not upload: %s", err)
	}

	err = wait.RunGroup(
		Build(client, k8sClient, "controller", *registry, image),
		Build(client, k8sClient, "build-init", *registry, image),
		Build(client, k8sClient, "rebase", *registry, image),
		Build(client, k8sClient, "webhook", *registry, image),
		Build(client, k8sClient, "completion", *registry, image),
	)
	if err != nil {
		log.Fatalf(err.Error())
	}
}

var parse = resource.MustParse("2Gi")

func Build(client versioned.Interface, k8sClient k8sclient.Interface, name, registry, sourceImage string) wait.DoneFunc {
	err := createOrUpdateImage(client, &v1alpha1.Image{
		ObjectMeta: v1.ObjectMeta{
			Name: name,
		},
		Spec: v1alpha1.ImageSpec{
			Tag: registry + "/" + name,
			Builder: v1alpha1.ImageBuilder{
				TypeMeta: v1.TypeMeta{
					Kind: "ClusterBuilder",
				},
				Name: "default-builder",
			},
			Source: v1alpha1.SourceConfig{
				Registry: &v1alpha1.Registry{
					Image: sourceImage,
				},
			},
			CacheSize: &parse,
			Build: v1alpha1.ImageBuild{
				Env: []corev1.EnvVar{
					{
						Name:  "BP_GO_TARGETS",
						Value: fmt.Sprintf("./cmd/%s", name),
					},
				},
			},
		},
	})
	if err != nil {
		return done(err)
	}

	return streamLogsUntilFinished(client, k8sClient, name, name)
}

func done(err error) wait.DoneFunc {
	return func(context context.Context) error {
		return err
	}
}

type ImageListener struct {
	doneChan chan<- error
}

func (i ImageListener) OnAdd(obj interface{}) {
	i.checkIfDone(obj)
}

func (i ImageListener) OnUpdate(oldObj, newObj interface{}) {
	i.checkIfDone(newObj)

}

func (i ImageListener) OnDelete(obj interface{}) {
}

func (i ImageListener) checkIfDone(obj interface{}) {
	image := obj.(*v1alpha1.Image)

	if image.Status.GetCondition(duckv1alpha1.ConditionReady).IsTrue() {
		i.doneChan <- nil
	}
}

func streamLogsUntilFinished(client versioned.Interface, k8sClient k8sclient.Interface, name, prefix string) wait.DoneFunc {
	return func(context context.Context) error {
		informerFactory := externalversions.NewSharedInformerFactoryWithOptions(client, 10*time.Hour,
			externalversions.WithTweakListOptions(func(options *v1.ListOptions) {
				options.FieldSelector = fmt.Sprintf("metadata.name=%s", name)
			}),
			externalversions.WithNamespace(v1.NamespaceDefault),
		)

		doneChan := make(chan error)

		imageInformer := informerFactory.Build().V1alpha1().Images()

		imageInformer.Informer().AddEventHandler(ImageListener{doneChan})

		informerFactory.Start(context.Done())

		go func() {
			err := logs.NewBuildLogsClient(k8sClient).Tail(context, logging.NewPrefixWriter(os.Stdout, prefix), name, "", v1.NamespaceDefault)
			if err != nil {
				fmt.Printf("error streaming logs for image %s: %s", name, err)
				//print this out
			}
		}()

		return <-doneChan
	}
}

func createOrUpdateImage(client versioned.Interface, image *v1alpha1.Image) error {
	existingImage, err := client.BuildV1alpha1().Images(v1.NamespaceDefault).Get(image.Name, v1.GetOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	if errors.IsNotFound(err) {
		_, err := client.BuildV1alpha1().Images(v1.NamespaceDefault).Create(image)
		if err != nil {
			return err
		}
	} else {
		_, err := client.BuildV1alpha1().Images(v1.NamespaceDefault).Update(&v1alpha1.Image{
			ObjectMeta: existingImage.ObjectMeta,
			Spec:       image.Spec,
		})
		if err != nil {
			return err
		}
	}
	return nil
}
