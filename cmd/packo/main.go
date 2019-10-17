package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/logging"
)

var (
	registry = flag.String("registry", "", "registry")
	path     = flag.String("path", "", "path")
)

func main() {
	flag.Parse()

	err := runGroup(
		Build(*registry+"/controller", *path, "./cmd/controller"),
		Build(*registry+"/build-init", *path, "./cmd/build-init"),
		Build(*registry+"/rebase", *path, "./cmd/rebase"),
		Build(*registry+"/webhook", *path, "./cmd/webhook"),
	)
	if err != nil {
		log.Fatalf(err.Error())
	}
}

func Build(image, path, target string) doneFunc {
	client, err := pack.NewClient(pack.WithLogger(logging.New(logging.NewPrefixWriter(os.Stdout, target))))
	if err != nil {
		log.Fatal(err)
	}

	return func(context context.Context) error {
		fmt.Println("starting up")
		return client.Build(context, pack.BuildOptions{
			Image:   image,
			Builder: "cloudfoundry/cnb:bionic",
			AppPath: path,
			Env: map[string]string{
				"BP_GO_TARGETS": target,
			},
			Publish: true,
			NoPull:  false,
		})
	}

}

type doneFunc func(context context.Context) error

func runGroup(fns ...doneFunc) error {
	var wg sync.WaitGroup
	wg.Add(len(fns))

	ctx, cancel := context.WithCancel(context.TODO())

	result := make(chan error, len(fns))
	for _, fn := range fns {
		go func(fn doneFunc) {
			defer wg.Done()
			result <- fn(ctx)
		}(fn)
	}

	go func() {
		for err := range result {
			if err != nil {
				fmt.Println(err.Error())

				cancel()
			}
		}
	}()

	defer close(result)
	defer wg.Wait()

	return nil
}
