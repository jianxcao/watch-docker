package dockercli

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/moby/moby/client"
)

func TestCleanContainerPathPreservesSpaces(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "trailing space", raw: "/tmp/report ", want: "/tmp/report "},
		{name: "leading space", raw: "/tmp/ report", want: "/tmp/ report"},
		{name: "spaces only filename", raw: "/tmp/  ", want: "/tmp/  "},
		{name: "literal semicolon", raw: "/;", want: "/;"},
		{name: "root", raw: "/", want: "/"},
		{name: "empty", raw: "", wantErr: true},
		{name: "whitespace only", raw: "   ", wantErr: true},
		{name: "null byte", raw: "/tmp\x00a", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := cleanContainerPath(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("cleanContainerPath(%q) expected error, got %q", tc.raw, got)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("cleanContainerPath(%q) = (%q, %v), want (%q, nil)", tc.raw, got, err, tc.want)
			}
		})
	}
}

func TestParseLsOutputPreservesSpacesAndTriggersFallbackOnAmbiguity(t *testing.T) {
	gnuOut := strings.Join([]string{
		"total 16",
		"drwxr-xr-x 2 root root 4096 2026-11-25 10:00:00 .",
		"drwxr-xr-x 3 root root 4096 2026-11-25 10:00:00 ..",
		"-rw-r--r-- 1 root root 1 2026-11-25 10:00:00 report",
		"-rw-r--r-- 1 root root 1 2026-11-25 10:00:00 report ",
		"-rw-r--r-- 1 root root 5 2026-11-25 10:00:00  leading",
		"-rw-r--r-- 1 root root 9 2026-11-25 10:00:00 my report.txt",
		"-rw-r--r-- 1 root root 3 2026-11-25 10:00:00 a -> b",
		"lrwxrwxrwx 1 root root 7 2026-11-25 10:00:00 link  -> target ",
	}, "\n")

	entries, err := parseLsOutput(gnuOut)
	if err != nil {
		t.Fatalf("parseLsOutput GNU failed: %v", err)
	}
	if len(entries) != 6 {
		t.Fatalf("expected 6 entries, got %d: %#v", len(entries), entries)
	}
	wantNames := []string{"report", "report ", " leading", "my report.txt", "a -> b", "link "}
	for i, want := range wantNames {
		if entries[i].Name != want {
			t.Fatalf("entry[%d].Name = %q, want %q", i, entries[i].Name, want)
		}
	}
	if entries[4].Type != "file" || entries[4].LinkTarget != "" {
		t.Fatalf("regular file with ' -> ' misparsed: %#v", entries[4])
	}
	if entries[5].Type != "symlink" || entries[5].LinkTarget != "target " {
		t.Fatalf("symlink spaces misparsed: %#v", entries[5])
	}

	busyboxOut := strings.Join([]string{
		"total 8",
		"-rw-r--r--    1 root     root             1 Nov 25 10:00 report ",
		"-rw-r--r--    1 root     root             1 Apr  1 09:05   ",
	}, "\n")
	bbEntries, err := parseLsOutput(busyboxOut)
	if err != nil {
		t.Fatalf("parseLsOutput BusyBox failed: %v", err)
	}
	if len(bbEntries) != 2 || bbEntries[0].Name != "report " || bbEntries[1].Name != "  " {
		t.Fatalf("unexpected BusyBox entries: %#v", bbEntries)
	}

	// 日期格式未知或符号链接包含多个 " -> " 时必须返回错误以回退到 Archive API
	ambiguousDate := "-rw-r--r-- 1 root root 1 unknown_date file name"
	if _, err := parseLsOutput(ambiguousDate); err == nil {
		t.Fatalf("expected error on unrecognized ls timestamp format")
	}
	ambiguousSymlink := "lrwxrwxrwx 1 root root 10 2026-11-25 10:00:00 a -> b -> c"
	if _, err := parseLsOutput(ambiguousSymlink); err == nil {
		t.Fatalf("expected error on ambiguous symlink delimiter")
	}
}

func TestListContainerDirectoryArchiveFallbackPreservesSpaces(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	_ = tw.WriteHeader(&tar.Header{Name: "tmp/", Typeflag: tar.TypeDir, Mode: 0755})
	_ = tw.WriteHeader(&tar.Header{Name: "tmp/report", Typeflag: tar.TypeReg, Mode: 0644, Size: 1})
	_, _ = tw.Write([]byte("A"))
	_ = tw.WriteHeader(&tar.Header{Name: "tmp/report ", Typeflag: tar.TypeReg, Mode: 0644, Size: 1})
	_, _ = tw.Write([]byte("B"))
	_ = tw.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/events"):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/exec"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"Id": "exec-1"})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/start"):
			hj, ok := w.(http.Hijacker)
			if !ok {
				http.Error(w, "no hijack", http.StatusInternalServerError)
				return
			}
			conn, rw, err := hj.Hijack()
			if err != nil {
				return
			}
			defer conn.Close()
			_, _ = rw.WriteString("HTTP/1.1 200 OK\r\nContent-Type: application/vnd.docker.raw-stream\r\n\r\n")
			payload := []byte("-rw-r--r-- 1 root root 1 badfmt report \n")
			var hdr [8]byte
			hdr[0] = 1 // stdcopy.Stdout
			binary.BigEndian.PutUint32(hdr[4:], uint32(len(payload)))
			// 返回无法明确识别日期格式的 ls 输出，触发 Archive 回退
			_, _ = rw.Write(hdr[:])
			_, _ = rw.Write(payload)
			_ = rw.Flush()
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/exec/") && strings.HasSuffix(r.URL.Path, "/json"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"ExitCode": 0})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/archive"):
			w.Header().Set("X-Docker-Container-Path-Stat", base64.StdEncoding.EncodeToString([]byte(`{"name":"tmp","mode":2147484141}`)))
			w.Header().Set("Content-Type", "application/x-tar")
			_, _ = w.Write(tarBuf.Bytes())
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	dockerCli, err := client.New(client.WithHost(srv.URL))
	if err != nil {
		t.Fatalf("create docker client: %v", err)
	}
	defer dockerCli.Close()
	cli := &Client{docker: dockerCli}

	res, err := cli.ListContainerDirectory(ctx, "cid-1", "/tmp")
	if err != nil {
		t.Fatalf("ListContainerDirectory fallback failed: %v", err)
	}
	if len(res.Entries) != 2 || res.Entries[0].Name != "report" || res.Entries[1].Name != "report " {
		t.Fatalf("expected verbatim tar entry names ['report', 'report '], got %#v", res.Entries)
	}
}

func TestPutSingleFileStreamPreservesTrailingSpaceFilename(t *testing.T) {
	var gotTar bytes.Buffer
	var gotDest string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/archive") {
			gotDest = r.URL.Query().Get("path")
			_, _ = io.Copy(&gotTar, r.Body)
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	dockerCli, err := client.New(client.WithHost(srv.URL))
	if err != nil {
		t.Fatalf("create docker client: %v", err)
	}
	defer dockerCli.Close()
	cli := &Client{docker: dockerCli}

	if err := cli.PutSingleFileStream(context.Background(), "cid-1", "/tmp/dir ", "report ", 1, strings.NewReader("B")); err != nil {
		t.Fatalf("PutSingleFileStream failed: %v", err)
	}
	if gotDest != "/tmp/dir " {
		t.Fatalf("destination path = %q, want %q", gotDest, "/tmp/dir ")
	}
	tr := tar.NewReader(&gotTar)
	hdr, err := tr.Next()
	if err != nil {
		t.Fatalf("read uploaded tar header: %v", err)
	}
	if hdr.Name != "report " {
		t.Fatalf("tar header Name = %q, want %q", hdr.Name, "report ")
	}
	data, _ := io.ReadAll(tr)
	if string(data) != "B" {
		t.Fatalf("tar content = %q, want %q", string(data), "B")
	}

	for _, bad := range []string{"", ".", "..", "a/b", "a\\b", "a\x00b"} {
		if err := cli.PutSingleFileStream(context.Background(), "cid-1", "/tmp", bad, 0, strings.NewReader("")); err == nil {
			t.Fatalf("expected error for invalid filename %q", bad)
		}
	}
}
