package vparquet

import (
	"context"
	"fmt"
	"io"

	"github.com/pkg/errors"
	"github.com/segmentio/parquet-go"

	tempo_io "github.com/grafana/tempo/pkg/io"
)

type blockIterator struct {
	blockID string
	r       *parquet.Reader
}

func (b *backendBlock) Iterator(ctx context.Context) (Iterator, error) {
	rr := NewBackendReaderAt(ctx, b.r, DataFileName, b.meta.BlockID, b.meta.TenantID)

	// 16 MB memory buffering
	br := tempo_io.NewBufferedReaderAt(rr, int64(b.meta.Size), 512*1024, 32)

	pf, err := parquet.OpenFile(br, int64(b.meta.Size))
	if err != nil {
		return nil, err
	}

	r := parquet.NewReader(pf, parquet.SchemaOf(&Trace{}))

	return &blockIterator{blockID: b.meta.BlockID.String(), r: r}, nil
}

func (i *blockIterator) Next(context.Context) (*Trace, error) {
	t := &Trace{}
	switch err := i.r.Read(t); err {
	case nil:
		return t, nil
	case io.EOF:
		return nil, nil
	default:
		return nil, errors.Wrap(err, fmt.Sprintf("error iterating through block %s", i.blockID))
	}
}

func (i *blockIterator) Close() {
	// parquet reader is shared, lets not close it here
}
