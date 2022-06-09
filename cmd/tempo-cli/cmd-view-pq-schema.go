package main

import (
	"context"
	"fmt"
	"sort"

	"github.com/google/uuid"
	"github.com/segmentio/parquet-go"

	"github.com/grafana/tempo/tempodb/encoding/vparquet"
)

type viewSchemaCmd struct {
	backendOptions

	TenantID string `arg:"" help:"tenant-id within the bucket"`
	BlockID  string `arg:"" help:"block ID to list"`
}

func (cmd *viewSchemaCmd) Run(ctx *globalOptions) error {
	blockID, err := uuid.Parse(cmd.BlockID)
	if err != nil {
		return err
	}

	r, _, _, err := loadBackend(&cmd.backendOptions, ctx)
	if err != nil {
		return err
	}

	meta, err := r.BlockMeta(context.TODO(), blockID, cmd.TenantID)
	if err != nil {
		return err
	}

	fmt.Println("\n***************     block meta    *********************")
	fmt.Printf("%+v\n", meta)

	rr := vparquet.NewBackendReaderAt(context.Background(), r, vparquet.DataFileName, meta.BlockID, meta.TenantID)
	pf, err := parquet.OpenFile(rr, int64(meta.Size))
	if err != nil {
		return err
	}

	fmt.Println("\n***************       schema      ********************")
	fmt.Println(pf.Schema().String())

	columnSizes := map[string]int64{}
	for _, rg := range pf.RowGroups() {
		for _, cc := range rg.ColumnChunks() {
			path, _ := getNodePathByIndex(pf.Root(), "", cc.Column())

			var size int64
			for pg := 0; pg < cc.OffsetIndex().NumPages(); pg++ {
				size += cc.OffsetIndex().CompressedPageSize(pg)
			}

			columnSizes[path] = columnSizes[path] + size
		}
	}
	sizes := []string{}
	for k, v := range columnSizes {
		sizes = append(sizes, fmt.Sprint(k, " size ", v/1024, " KB"))
	}
	sort.Strings(sizes)

	fmt.Println("\n***************   column sizes    *********************")
	for _, s := range sizes {
		fmt.Println(s)
	}

	return nil
}

func getNodePathByIndex(root *parquet.Column, s string, i int) (string, bool) {
	s = s + "." + root.Name()

	if int(root.Index()) == i {
		return s, true
	}
	for _, col := range root.Columns() {
		if path, ok := getNodePathByIndex(col, s, i); ok {
			return path, true
		}
	}
	return "", false
}
