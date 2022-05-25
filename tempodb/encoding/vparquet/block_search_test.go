package vparquet

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/grafana/tempo/pkg/tempopb"
	v1 "github.com/grafana/tempo/pkg/tempopb/trace/v1"
	"github.com/grafana/tempo/pkg/util"
	"github.com/grafana/tempo/pkg/util/test"
	"github.com/grafana/tempo/tempodb/backend"
	"github.com/grafana/tempo/tempodb/backend/local"
	"github.com/grafana/tempo/tempodb/encoding/common"
	"github.com/stretchr/testify/require"
)

func TestBackendBlockSearch(t *testing.T) {

	// Helper functions to make pointers
	strPtr := func(s string) *string { return &s }
	intPtr := func(i int64) *int64 { return &i }

	// Trace
	// This is a fully-populated trace that we search for every condition
	wantTr := &Trace{
		TraceID:           util.TraceIDToHexString(test.ValidTraceID(nil)),
		StartTimeUnixNano: uint64(time.Now().UnixNano()),
		DurationNanos:     uint64((100 * time.Millisecond).Nanoseconds()),
		RootServiceName:   "RootService",
		RootSpanName:      "RootSpan",
		ResourceSpans: []ResourceSpans{
			{
				Resource: Resource{
					ServiceName:      "myservice",
					Cluster:          strPtr("cluster"),
					Namespace:        strPtr("namespace"),
					Pod:              strPtr("pod"),
					Container:        strPtr("container"),
					K8sClusterName:   strPtr("k8scluster"),
					K8sNamespaceName: strPtr("k8snamespace"),
					K8sPodName:       strPtr("k8spod"),
					K8sContainerName: strPtr("k8scontainer"),
					Attrs: []Attribute{
						{Key: "bat", Value: strPtr("baz")},
					},
				},
				InstrumentationLibrarySpans: []ILS{
					{
						Spans: []Span{
							{
								Name:           "hello",
								HttpMethod:     strPtr("get"),
								HttpUrl:        strPtr("url/hello/world"),
								HttpStatusCode: intPtr(500),
								ID:             []byte{},
								ParentSpanID:   []byte{},
								StatusCode:     int(v1.Status_STATUS_CODE_ERROR),
								Attrs: []Attribute{
									{Key: "foo", Value: strPtr("bar")},
								},
							},
						},
					},
				},
			},
		},
	}

	b := makeBackendBlockWithTrace(t, wantTr)
	ctx := context.TODO()

	// Helper function to make a tag search
	makeReq := func(k, v string) *tempopb.SearchRequest {
		return &tempopb.SearchRequest{
			Tags: map[string]string{
				k: v,
			},
		}
	}

	// Matches
	searchesThatMatch := []*tempopb.SearchRequest{
		{
			// Empty request
		},
		{
			MinDurationMs: 99,
			MaxDurationMs: 101,
		},

		// Well-known resource attributes
		makeReq(LabelServiceName, "service"),
		makeReq(LabelCluster, "cluster"),
		makeReq(LabelNamespace, "namespace"),
		makeReq(LabelPod, "pod"),
		makeReq(LabelContainer, "container"),
		makeReq(LabelK8sClusterName, "k8scluster"),
		makeReq(LabelK8sNamespaceName, "k8snamespace"),
		makeReq(LabelK8sPodName, "k8spod"),
		makeReq(LabelK8sContainerName, "k8scontainer"),

		// Well-known span attributes
		makeReq(LabelHTTPMethod, "get"),
		makeReq(LabelHTTPUrl, "hello"),
		makeReq(LabelHTTPStatusCode, "500"),
		makeReq(LabelStatus, StatusCodeError),

		// Span attributes
		makeReq("foo", "bar"),
		// Resource attributes
		makeReq("bat", "baz"),

		// Mutliple
		{
			Tags: map[string]string{
				"service.name": "service",
				"http.method":  "get",
				"foo":          "bar",
			},
		},
	}
	expected := &tempopb.TraceSearchMetadata{
		TraceID:           wantTr.TraceID,
		StartTimeUnixNano: wantTr.StartTimeUnixNano,
		DurationMs:        uint32(wantTr.DurationNanos / uint64(time.Millisecond)),
		RootServiceName:   wantTr.RootServiceName,
		RootTraceName:     wantTr.RootSpanName,
	}
	for _, req := range searchesThatMatch {
		res, err := b.Search(ctx, req, common.DefaultSearchOptions())
		require.NoError(t, err)
		require.Equal(t, 1, len(res.Traces), req)
		require.Equal(t, expected, res.Traces[0], "search request:", req)
	}

	// Excludes
	searchesThatDontMatch := []*tempopb.SearchRequest{
		{
			MinDurationMs: 101,
		},
		{
			MaxDurationMs: 99,
		},

		// Well-known resource attributes
		makeReq(LabelServiceName, "foo"),
		makeReq(LabelCluster, "foo"),
		makeReq(LabelNamespace, "foo"),
		makeReq(LabelPod, "foo"),
		makeReq(LabelContainer, "foo"),

		// Well-known span attributes
		makeReq(LabelHTTPMethod, "post"),
		makeReq(LabelHTTPUrl, "asdf"),
		makeReq(LabelHTTPStatusCode, "200"),
		makeReq(LabelStatus, StatusCodeOK),

		// Span attributes
		makeReq("foo", "baz"),
	}
	for _, req := range searchesThatDontMatch {
		res, err := b.Search(ctx, req, common.DefaultSearchOptions())
		require.NoError(t, err)
		require.Empty(t, res.Traces, "search request:", req)
	}
}

func makeBackendBlockWithTrace(t *testing.T, tr *Trace) *backendBlock {

	rawR, rawW, _, err := local.New(&local.Config{
		Path: t.TempDir(),
	})
	require.NoError(t, err)

	r := backend.NewReader(rawR)
	w := backend.NewWriter(rawW)
	ctx := context.Background()

	cfg := &common.BlockConfig{
		BloomFP:             0.01,
		BloomShardSizeBytes: 100 * 1024,
	}

	meta := backend.NewBlockMeta("fake", uuid.New(), VersionString, backend.EncNone, "")
	meta.TotalObjects = 1

	s, err := NewStreamingBlock(ctx, cfg, meta, r, w)
	require.NoError(t, err)

	s.Add(tr)

	_, err = s.Complete()
	require.NoError(t, err)

	b, err := NewBackendBlock(s.meta, r)
	require.NoError(t, err)

	return b
}
