package scaler

import (
	"testing"

	"github.com/gardener/dependency-watchdog/internal/mock/client-go/scale"
	"github.com/gardener/dependency-watchdog/internal/mock/controller-runtime/client"
	"github.com/gardener/gardener/pkg/utils/flow"
	. "github.com/onsi/gomega"

	papi "github.com/gardener/dependency-watchdog/api/prober"
)

// Tests creation of the flow where there are no parallel tasks at any level.
// There is exactly zero or one task at each level executed sequentially one after the other.
func TestCreateScaleUpSequentialFlow(t *testing.T) {
	g := NewWithT(t)
	var depResInfos []papi.DependentResourceInfo
	depResInfos = append(depResInfos, createTestDeploymentDependentResourceInfo(kcmObjectRef.Name, 0, 2, nil, nil, true))
	depResInfos = append(depResInfos, createTestDeploymentDependentResourceInfo(mcmObjectRef.Name, 1, 1, nil, nil, true))
	depResInfos = append(depResInfos, createTestDeploymentDependentResourceInfo(caObjectRef.Name, 2, 0, nil, nil, true))

	expectedScaleUpResNames := []string{kcmObjectRef.Name, mcmObjectRef.Name, caObjectRef.Name}
	flowName := "testCreateSequentialFlow"
	namespace := "test-sequential"

	fc := newFlowCreator(&client.MockClient{}, &scale.MockScaleInterface{}, &scalerOptions{}, depResInfos)
	f := fc.createFlow(flowName, namespace, scaleUp)
	g.Expect(f.flowStepInfos).To(HaveLen(3))

	previousDepTaskIDs := make([]flow.TaskID, 0, 3)
	for i := 0; i < len(f.flowStepInfos); i++ {
		currentTaskStep := f.flowStepInfos[i]
		// using taskID format (see createTaskName function) extract and assert level and resource ref targeted in the task step
		level, resourceRefNames, err := parseTaskID(string(currentTaskStep.taskID))
		g.Expect(err).To(BeNil())
		g.Expect(level).To(Equal(i))
		g.Expect(resourceRefNames).To(HaveLen(1))
		g.Expect(resourceRefNames[0]).To(Equal(expectedScaleUpResNames[i]))

		// using dependent taskIDs to check if the dependency has been maintained correctly
		depTaskIDs := currentTaskStep.dependentTaskIDs.TaskIDs()
		g.Expect(depTaskIDs).To(HaveLen(i))
		g.Expect(depTaskIDs).To(Equal(previousDepTaskIDs))
		previousDepTaskIDs = append(previousDepTaskIDs, currentTaskStep.taskID)
	}
}

// Tests creation of the flow where there is a combination of sequential and concurrent tasks.
func TestCreateScaleDownSequentialAndConcurrentFlow(t *testing.T) {
	g := NewWithT(t)
	var depResInfos []papi.DependentResourceInfo
	depResInfos = append(depResInfos, createTestDeploymentDependentResourceInfo(kcmObjectRef.Name, 0, 1, nil, nil, true))
	depResInfos = append(depResInfos, createTestDeploymentDependentResourceInfo(mcmObjectRef.Name, 1, 0, nil, nil, true))
	depResInfos = append(depResInfos, createTestDeploymentDependentResourceInfo(caObjectRef.Name, 2, 0, nil, nil, true))

	expectedScaleUpResNames := map[int][]string{0: {mcmObjectRef.Name, caObjectRef.Name}, 1: {kcmObjectRef.Name}}
	flowName := "testCreateSequentialAndConcurrentFlow"
	namespace := "test-sequential-and-concurrent"

	fc := newFlowCreator(&client.MockClient{}, &scale.MockScaleInterface{}, &scalerOptions{}, depResInfos)
	f := fc.createFlow(flowName, namespace, scaleDown)
	g.Expect(f.flowStepInfos).To(HaveLen(2))

	previousDepTaskIDs := make([]flow.TaskID, 0, 3)
	for i := 0; i < len(f.flowStepInfos); i++ {
		currentTaskStep := f.flowStepInfos[i]
		// using taskID format (see createTaskName function) extract and assert level and resource ref targeted in the task step
		level, resourceRefNames, err := parseTaskID(string(currentTaskStep.taskID))
		g.Expect(err).To(BeNil())
		g.Expect(level).To(Equal(i))
		g.Expect(expectedScaleUpResNames[i]).To(Equal(resourceRefNames))

		// using dependent taskIDs to check if the dependency has been maintained correctly
		depTaskIDs := currentTaskStep.dependentTaskIDs.TaskIDs()
		g.Expect(depTaskIDs).To(Equal(previousDepTaskIDs))
		previousDepTaskIDs = append(previousDepTaskIDs, currentTaskStep.taskID)
	}
}
