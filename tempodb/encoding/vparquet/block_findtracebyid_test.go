package vparquet

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tempo_io "github.com/grafana/tempo/pkg/io"
	"github.com/grafana/tempo/pkg/util"
	"github.com/grafana/tempo/pkg/util/test"
	"github.com/grafana/tempo/tempodb/backend"
	"github.com/grafana/tempo/tempodb/backend/local"
	"github.com/grafana/tempo/tempodb/encoding/common"
)

func TestBackendBlockFindTraceByID(t *testing.T) {
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

	id := test.ValidTraceID(nil)

	s := newStreamingBlock(ctx, cfg, meta, r, w, tempo_io.NewBufferedWriter)

	bar := "bar"
	require.NoError(t, s.Add(&Trace{
		TraceID: util.TraceIDToHexString(test.ValidTraceID(nil)),
		ResourceSpans: []ResourceSpans{
			{
				Resource: Resource{
					ServiceName: "s",
				},
				InstrumentationLibrarySpans: []ILS{
					{
						Spans: []Span{
							{
								Name: "hello",
								Attrs: []Attribute{
									{Key: "foo", Value: &bar},
								},
								ID:           []byte{},
								ParentSpanID: []byte{},
							},
						},
					},
				},
			},
		},
	}))

	wantTr := &Trace{
		TraceID: util.TraceIDToHexString(id),
		ResourceSpans: []ResourceSpans{
			{
				Resource: Resource{
					ServiceName: "s",
				},
				InstrumentationLibrarySpans: []ILS{
					{
						Spans: []Span{
							{
								Name: "hello",
								Attrs: []Attribute{
									{Key: "foo", Value: &bar},
								},
								ID:           []byte{},
								ParentSpanID: []byte{},
								Events: []Event{
									{
										Attrs: []EventAttribute{
											{
												Key:   "foo",
												Value: "baz",
											},
										},
									}}}}}}}},
	}
	require.NoError(t, s.Add(wantTr))

	_, err = s.Complete()
	require.NoError(t, err)

	b := newBackendBlock(s.meta, r)

	gotTr, err := b.FindTraceByID(ctx, id)
	require.NoError(t, err)

	wantProto, err := parquetTraceToTempopbTrace(wantTr)
	require.NoError(t, err)

	require.Equal(t, wantProto, gotTr)
}

func TestBackendBlockFindTraceByID_TestData(t *testing.T) {
	rawR, _, _, err := local.New(&local.Config{
		Path: "./test-data",
	})
	require.NoError(t, err)

	r := backend.NewReader(rawR)
	ctx := context.Background()

	blocks, err := r.Blocks(ctx, "vulture-tenant")
	require.NoError(t, err)
	assert.Len(t, blocks, 1)

	meta, err := r.BlockMeta(ctx, blocks[0], "vulture-tenant")
	require.NoError(t, err)

	b := newBackendBlock(meta, r)
	bytes, _ := util.HexStringToTraceID("7d80fcd3e4978d6143030ef00d8bccc1")
	tr, err := b.FindTraceByID(ctx, bytes)
	require.NoError(t, err)

	require.NotNil(t, tr)
}
