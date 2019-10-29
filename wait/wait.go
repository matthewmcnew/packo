package wait

import (
	"context"
	"fmt"
	"sync"
)

type DoneFunc func(context context.Context) error

func RunGroup(fns ...DoneFunc) error {
	var wg sync.WaitGroup
	wg.Add(len(fns))

	ctx, cancel := context.WithCancel(context.TODO())

	result := make(chan error, len(fns))
	for _, fn := range fns {
		go func(fn DoneFunc) {
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
