//go:build floci_gcp

package flocigcp_test

import (
	"context"
	"testing"
	"time"

	monitoring "cloud.google.com/go/monitoring/apiv3/v2"
	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"google.golang.org/genproto/googleapis/api/metric"
	"google.golang.org/genproto/googleapis/api/monitoredres"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestMetricWriteAndListAgainstFlociGCP(t *testing.T) {
	host := requireFlociGCP(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	client, err := monitoring.NewMetricClient(ctx, flociGCPClientOptions(host)...)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()

	const project = "projects/floci-local"
	const metricType = "custom.googleapis.com/magelift/floci_gcp_probe"
	now := timestamppb.Now()
	if err := client.CreateTimeSeries(ctx, &monitoringpb.CreateTimeSeriesRequest{
		Name: project,
		TimeSeries: []*monitoringpb.TimeSeries{{
			Metric:   &metric.Metric{Type: metricType},
			Resource: &monitoredres.MonitoredResource{Type: "global"},
			Points: []*monitoringpb.Point{{
				Interval: &monitoringpb.TimeInterval{EndTime: now},
				Value:    &monitoringpb.TypedValue{Value: &monitoringpb.TypedValue_DoubleValue{DoubleValue: 42}},
			}},
		}},
	}); err != nil {
		t.Fatalf("create time series: %v", err)
	}

	it := client.ListTimeSeries(ctx, &monitoringpb.ListTimeSeriesRequest{
		Name:     project,
		Filter:   `metric.type="` + metricType + `"`,
		Interval: &monitoringpb.TimeInterval{StartTime: timestamppb.New(now.AsTime().Add(-2 * time.Minute)), EndTime: timestamppb.New(now.AsTime().Add(time.Minute))},
		View:     monitoringpb.ListTimeSeriesRequest_FULL,
	})
	series, err := it.Next()
	if err != nil {
		t.Fatalf("list time series: %v", err)
	}
	if len(series.GetPoints()) == 0 || series.GetPoints()[0].GetValue().GetDoubleValue() != 42 {
		t.Fatalf("unexpected series points: %+v", series.GetPoints())
	}
}
