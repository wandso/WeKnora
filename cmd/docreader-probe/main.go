// Command docreader-probe parses one local file through the docreader gRPC
// service and prints the resulting markdown.
//
// It exists to check the service end-to-end (dial -> Read -> markdown) without
// going through the product UI. Usage:
//
//	go run ./cmd/docreader-probe [addr] [file]
//
// Defaults to 127.0.0.1:50051 and docreader/testdata/test.md.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/docreader/client"
	"github.com/Tencent/WeKnora/docreader/proto"
)

func main() {
	addr := "127.0.0.1:50051"
	file := "docreader/testdata/test.md"
	if len(os.Args) > 1 {
		addr = os.Args[1]
	}
	if len(os.Args) > 2 {
		file = os.Args[2]
	}

	content, err := os.ReadFile(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read %s: %v\n", file, err)
		os.Exit(1)
	}

	c, err := client.NewClient(addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect %s: %v\n", addr, err)
		os.Exit(1)
	}
	defer func() { _ = c.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(file)), ".")
	start := time.Now()
	resp, err := c.Read(ctx, &proto.ReadRequest{
		FileContent: content,
		FileName:    filepath.Base(file),
		FileType:    ext,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Read(%s): %v\n", file, err)
		os.Exit(1)
	}
	if resp.Error != "" {
		fmt.Fprintf(os.Stderr, "docreader reported error: %s\n", resp.Error)
		os.Exit(1)
	}

	fmt.Printf("OK elapsed=%s file=%s type=%s markdown_bytes=%d image_refs=%d metadata=%v\n",
		time.Since(start).Round(time.Millisecond), filepath.Base(file), ext,
		len(resp.MarkdownContent), len(resp.ImageRefs), resp.Metadata)
	fmt.Println("--- markdown ---")
	fmt.Println(resp.MarkdownContent)
}
