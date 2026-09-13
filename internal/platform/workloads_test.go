package platform

import (
	"reflect"
	"testing"
)

func TestMagentoQueueArgs(t *testing.T) {
	want := []string{"bin/magento", "queue:consumers:start", "async.operations.all", "--max-messages=10000"}
	if got := MagentoQueueArgs(); !reflect.DeepEqual(got, want) {
		t.Fatalf("MagentoQueueArgs() = %#v, want %#v", got, want)
	}
}

func TestMagentoQueueArgsForNamedConsumers(t *testing.T) {
	want := []string{"bin/magento", "queue:consumers:start", "product_action_attribute.update", "--max-messages=10000"}
	if got := MagentoQueueArgsFor([]string{"product_action_attribute.update"}); !reflect.DeepEqual(got, want) {
		t.Fatalf("MagentoQueueArgsFor() = %#v, want %#v", got, want)
	}
}
