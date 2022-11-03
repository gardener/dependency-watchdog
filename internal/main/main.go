package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	testutil "github.com/gardener/dependency-watchdog/internal/test"
	"github.com/gardener/dependency-watchdog/internal/util"
	"github.com/gardener/gardener/pkg/utils/flow"
	"github.com/go-logr/logr"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

func main() {
	kubeConfigBuf, err := testutil.ReadFile("/Users/i062009/.kube/config")
	if err != nil {
		panic(err)
	}
	clientConfig, err := clientcmd.NewClientConfigFromBytes(kubeConfigBuf.Bytes())
	if err != nil {
		panic(err)
	}
	config, err := clientConfig.ClientConfig()
	if err != nil {
		panic(err)
	}
	scalesGetter, err := util.CreateScalesGetter(config)
	if err != nil {
		panic(err)
	}

	objRef := &autoscalingv1.CrossVersionObjectReference{
		Kind:       "ConfigMap",
		Name:       "game-demo",
		APIVersion: "v1",
	}
	mapper, err := apiutil.NewDynamicRESTMapper(config)
	if err != nil {
		panic(err)
	}

	cli, err := client.New(config, client.Options{Mapper: mapper})
	if err != nil {
		panic(err)
	}

	cm := &corev1.ConfigMap{}
	cli.Get(context.Background(), types.NamespacedName{Name: objRef.Name, Namespace: "default"}, cm)
	fmt.Printf("found confog-map: %v\n", cm)

	gr, scale, err := util.GetScaleResource(context.Background(), cli, scalesGetter.Scales("default"), logr.Discard(), objRef, 30*time.Second)
	if err != nil {
		panic(err)
	}
	fmt.Printf("groupResource: %v", gr)
	fmt.Printf("scale: %v", scale)
	//createFlow1().Run(context.Background(), flow.Opts{})
	//createFlow2().Run(context.Background(), flow.Opts{})
}

func createFlow1() *flow.Flow {
	g := flow.NewGraph("bingo")

	fn1 := func(ctx context.Context) error {
		fmt.Println("entered step-0")
		time.Sleep(2 * time.Second)
		fmt.Println("done processing step-0")
		return nil
	}
	fn2 := func(ctx context.Context) error {
		fmt.Println("entered step-1")
		time.Sleep(5 * time.Second)
		//fmt.Println("done processing step-1")
		fmt.Println("processing of step-1 failed")
		return errors.New("step-1 failed")
	}
	concurrentTaskFn := flow.Parallel([]flow.TaskFn{fn1, fn2}...)
	level0TaskID := g.Add(flow.Task{
		Name:         "level-0",
		Fn:           concurrentTaskFn,
		Dependencies: nil,
	})
	g.Add(flow.Task{
		Name: "level-1",
		Fn: func(ctx context.Context) error {
			fmt.Println("entered step-2")
			time.Sleep(1 * time.Second)
			fmt.Println("done processing step-2")
			return nil
		},
		Dependencies: flow.NewTaskIDs(level0TaskID),
	})

	return g.Compile()
}

func createFlow2() *flow.Flow {
	g := flow.NewGraph("bingo")
	fn1 := func(ctx context.Context) error {
		fmt.Println("entered step-0")
		time.Sleep(2 * time.Second)
		fmt.Println("done processing step-0")
		return nil
	}
	level0TaskID := g.Add(flow.Task{
		Name:         "level-0",
		Fn:           fn1,
		Dependencies: nil,
	})

	fn2 := func(ctx context.Context) error {
		fmt.Println("entered step-1")
		time.Sleep(5 * time.Second)
		fmt.Println("done processing step-1")
		return errors.New("step-1 failed")
	}
	level1TaskID := g.Add(flow.Task{
		Name:         "level-1",
		Fn:           fn2,
		Dependencies: flow.NewTaskIDs(level0TaskID),
	})

	g.Add(flow.Task{
		Name: "level-2",
		Fn: func(ctx context.Context) error {
			fmt.Println("entered step-2")
			time.Sleep(1 * time.Second)
			fmt.Println("done processing step-2")
			return nil
		},
		Dependencies: flow.NewTaskIDs(level0TaskID, level1TaskID),
	})

	return g.Compile()
}
