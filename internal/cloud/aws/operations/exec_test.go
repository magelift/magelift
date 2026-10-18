package operations

import (
	"context"
	"errors"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
)

type fakeECS struct {
	output *ecs.ListTasksOutput
	input  *ecs.ListTasksInput
	err    error
}

func (f *fakeECS) ListTasks(_ context.Context, input *ecs.ListTasksInput, _ ...func(*ecs.Options)) (*ecs.ListTasksOutput, error) {
	f.input = input
	return f.output, f.err
}

func TestSelectTaskSortsRunningTaskARNs(t *testing.T) {
	client := &fakeECS{output: &ecs.ListTasksOutput{TaskArns: []string{"task-b", "task-a"}}}
	store, err := NewExecFromClient(client)
	if err != nil {
		t.Fatal(err)
	}
	task, err := store.SelectTask(context.Background(), "cluster", "service")
	if err != nil {
		t.Fatal(err)
	}
	if task.ARN != "task-a" || awssdk.ToString(client.input.Cluster) != "cluster" || awssdk.ToString(client.input.ServiceName) != "service" || client.input.DesiredStatus != "RUNNING" {
		t.Fatalf("task=%#v input=%#v", task, client.input)
	}
}

func TestSelectTaskRejectsMissingOrUnavailableTasks(t *testing.T) {
	for name, client := range map[string]*fakeECS{
		"empty": {output: &ecs.ListTasksOutput{}},
		"nil":   {output: nil},
		"error": {err: errors.New("denied")},
	} {
		t.Run(name, func(t *testing.T) {
			store, err := NewExecFromClient(client)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.SelectTask(context.Background(), "cluster", "service"); err == nil {
				t.Fatal("expected task selection error")
			}
		})
	}
	store, err := NewExecFromClient(&fakeECS{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SelectTask(context.Background(), "", "service"); err == nil {
		t.Fatal("expected missing cluster error")
	}
}
