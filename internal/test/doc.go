/*

Package test contains test utilities. Following is the guide on how to use it:

Utilities in kind.go: Tests that require a KIND cluster should use utilities inside this file.

To create a KIND cluster use:
```
	// to create a KIND test cluster
	kindCluster, err := test.CreateKindCluster(test.KindConfig{Name: "<name-of-kind-cluster>"})
	if err != nil {
		panic(err) // you can decide how to handle this error in a different way
	}

	// get the rest.Config for the KIND cluster
	restConfig := kindCluster.GetRestConfig()

	// currently it has only a few methods to easily create/delete some test k8s resources like namespace, deployment. more methods can be added incrementally and when required
	// create a namespace
	err = kindCluster.CreateNamespace("bingo-ns")

	// create a simple NGINX deployment
	err = kindCluster.CreateDeployment("tringo", "bingo-ns", "nginx:1.14.2", 1, map[string]string{"annotation-key":"annotation-value"})

	// delete a previously created deployment
	err = kindCluster.DeleteDeployment("tringo", "bingo-ns")

	// to delete the KIND test cluster
	err = kindCluster.Delete()
```

Utilities in testenv.go: Tests that require a controller-runtime envtest should use utilities inside this file
```
	// to create a controller-runtime test environment
	ctrlTestEnv, err := test.CreateControllerTestEnv()

	// to stop the controller-runtime test environment
	ctrlTestEnv.Delete()

	// to get client.Client for the test environment
	k8sClient := ctrlTestEnv.GetClient()
```
*/
package test
