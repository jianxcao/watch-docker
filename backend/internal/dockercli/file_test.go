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
	"time"

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

	// GNU --time-style=iso 对半年以上旧文件会在 YYYY-MM-DD 后输出两个空格对齐宽度
	isoOldOut := strings.Join([]string{
		"-rw-r--r-- 1 root root 10 2025-01-15  old.txt",
		"-rw-r--r-- 1 root root 10 2025-01-15   leading",
		"-rw-r--r-- 1 root root 10 2025-01-15 single.txt",
	}, "\n")
	isoEntries, err := parseLsOutput(isoOldOut)
	if err != nil {
		t.Fatalf("parseLsOutput ISO old date failed: %v", err)
	}
	if len(isoEntries) != 3 || isoEntries[0].Name != "old.txt" || isoEntries[1].Name != " leading" || isoEntries[2].Name != "single.txt" {
		t.Fatalf("unexpected ISO old date names: %#v", isoEntries)
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
			_, _ = rw.WriteString("HTTP/1.1 101 UPGRADED\r\nContent-Type: application/vnd.docker.raw-stream\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\n")
			payload := []byte("-rw-r--r-- 1 root root 1 badfmt report \n")
			var hdr [8]byte
			hdr[0] = 1 // stdcopy.Stdout
			binary.BigEndian.PutUint32(hdr[4:], uint32(len(payload)))
			// 返回无法明确识别日期格式的 ls 输出，触发 Archive 回退
			_, _ = rw.Write(hdr[:])
			_, _ = rw.Write(payload)
			_ = rw.Flush()
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/exec"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"Id": "exec-1"})
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

	dockerCli, err := client.New(client.WithHost("tcp://" + srv.Listener.Addr().String()))
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

func TestListContainerDirectorySpecialFilesSkipArchiveFallback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	lsPayload := strings.Join([]string{
		"total 4194308",
		"prw------- 1 root root 0 2026-11-25 10:00:00 events",
		"crw-rw-rw- 1 root root 1,  3 2026-11-25 10:00:00 null",
		"brw-rw---- 1 root disk 8, 0 2026-11-25 10:00:00 sda",
		"srwxr-xr-x 1 root root 0 2026-11-25 10:00:00 app.sock",
		"-rwSr--r-T 1 root root 12 2026-11-25 10:00:00 special ",
		"-rw-r--r-- 1 root root 4294967296 2026-11-25 10:00:00 backup.tar",
	}, "\n") + "\n"

	archiveHits := 0
	var execEnv []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/events"):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
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
			_, _ = rw.WriteString("HTTP/1.1 101 UPGRADED\r\nContent-Type: application/vnd.docker.raw-stream\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\n")
			payload := []byte(lsPayload)
			var hdr [8]byte
			hdr[0] = 1
			binary.BigEndian.PutUint32(hdr[4:], uint32(len(payload)))
			_, _ = rw.Write(hdr[:])
			_, _ = rw.Write(payload)
			_ = rw.Flush()
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/exec"):
			var body struct {
				Env []string `json:"Env"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			execEnv = body.Env
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"Id": "exec-special"})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/exec/") && strings.HasSuffix(r.URL.Path, "/json"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"ExitCode": 0})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/archive"):
			archiveHits++
			http.Error(w, "unexpected archive call", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	dockerCli, err := client.New(client.WithHost("tcp://" + srv.Listener.Addr().String()))
	if err != nil {
		t.Fatalf("create docker client: %v", err)
	}
	defer dockerCli.Close()
	cli := &Client{docker: dockerCli}

	res, err := cli.ListContainerDirectory(ctx, "cid-special", "/tmp")
	if err != nil {
		t.Fatalf("ListContainerDirectory failed: %v", err)
	}
	if archiveHits != 0 {
		t.Fatalf("expected 0 /archive calls, got %d", archiveHits)
	}
	if len(res.Entries) != 6 {
		t.Fatalf("expected 6 entries, got %d: %#v", len(res.Entries), res.Entries)
	}
	if res.Entries[0].Name != "events" || res.Entries[0].Type != "other" || res.Entries[0].Size != 0 {
		t.Fatalf("unexpected fifo entry: %#v", res.Entries[0])
	}
	if res.Entries[1].Name != "null" || res.Entries[1].Type != "other" || res.Entries[1].Size != 0 {
		t.Fatalf("unexpected char device entry: %#v", res.Entries[1])
	}
	if res.Entries[2].Name != "sda" || res.Entries[2].Type != "other" || res.Entries[2].Group != "disk" || res.Entries[2].Size != 0 {
		t.Fatalf("unexpected block device entry: %#v", res.Entries[2])
	}
	if res.Entries[3].Name != "app.sock" || res.Entries[3].Type != "other" || res.Entries[3].Size != 0 {
		t.Fatalf("unexpected socket entry: %#v", res.Entries[3])
	}
	if res.Entries[4].Name != "special " || res.Entries[4].Type != "file" || res.Entries[4].Size != 12 {
		t.Fatalf("unexpected special permission entry: %#v", res.Entries[4])
	}
	if res.Entries[5].Name != "backup.tar" || res.Entries[5].Type != "file" || res.Entries[5].Size != 4294967296 {
		t.Fatalf("unexpected large file entry: %#v", res.Entries[5])
	}
	if len(execEnv) != 2 || execEnv[0] != "TZ=UTC0" || execEnv[1] != "LC_ALL=C" {
		t.Fatalf("expected ls exec env [TZ=UTC0 LC_ALL=C], got %#v", execEnv)
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

	dockerCli, err := client.New(client.WithHost("tcp://" + srv.Listener.Addr().String()))
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

func TestParseFileTimeAtInfersYearAndRejectsInvalidDates(t *testing.T) {
	got := parseFileTime("Apr  1 09:05")
	if strings.HasPrefix(got, "0000-") {
		t.Fatalf("expected non-zero year for BusyBox timestamp, got %q", got)
	}

	cases := []struct {
		name    string
		now     time.Time
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "same year",
			now:   time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC),
			input: "Apr  1 09:05",
			want:  "2026-04-01T09:05:00Z",
		},
		{
			name:  "year rollover Dec in Jan",
			now:   time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC),
			input: "Dec 31 23:58",
			want:  "2025-12-31T23:58:00Z",
		},
		{
			name:  "within 1h tolerance",
			now:   time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC),
			input: "Apr  1 09:05",
			want:  "2026-04-01T09:05:00Z",
		},
		{
			name:  "leap day in leap year",
			now:   time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC),
			input: "Feb 29 08:10",
			want:  "2024-02-29T08:10:00Z",
		},
		{
			name:  "leap day from previous leap year",
			now:   time.Date(2025, 2, 1, 12, 0, 0, 0, time.UTC),
			input: "Feb 29 08:10",
			want:  "2024-02-29T08:10:00Z",
		},
		{
			name:  "explicit year",
			now:   time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC),
			input: "Dec 31 2023",
			want:  "2023-12-31T00:00:00Z",
		},
		{
			name:  "numeric MM-DD HH:MM rollover",
			now:   time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC),
			input: "12-31 23:58",
			want:  "2025-12-31T23:58:00Z",
		},
		{
			name:    "reject Feb 29 when neither year is leap",
			now:     time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC),
			input:   "Feb 29 08:10",
			wantErr: true,
		},
		{
			name:    "reject invalid Feb 30",
			now:     time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC),
			input:   "Feb 30 08:10",
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := parseFileTimeAt(tc.input, tc.now)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for input=%q at now=%v, got %q", tc.input, tc.now, res)
				}
				return
			}
			if err != nil || res != tc.want {
				t.Fatalf("parseFileTimeAt(%q, %v) = (%q, %v), want (%q, nil)", tc.input, tc.now, res, err, tc.want)
			}
		})
	}
}

func TestReadContainerFileAutoFallsBackForSymlink(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	_ = tw.WriteHeader(&tar.Header{
		Name:     "os-release",
		Typeflag: tar.TypeSymlink,
		Linkname: "../usr/lib/os-release",
		Mode:     0777,
	})
	_ = tw.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/events"):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/archive"):
			w.Header().Set("X-Docker-Container-Path-Stat", base64.StdEncoding.EncodeToString([]byte(`{"name":"os-release","mode":134218239,"linkTarget":"../usr/lib/os-release"}`)))
			w.Header().Set("Content-Type", "application/x-tar")
			_, _ = w.Write(tarBuf.Bytes())
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
			_, _ = rw.WriteString("HTTP/1.1 101 UPGRADED\r\nContent-Type: application/vnd.docker.raw-stream\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\n")
			payload := []byte("NAME=\"Alpine Linux\"\n")
			var hdr [8]byte
			hdr[0] = 1
			binary.BigEndian.PutUint32(hdr[4:], uint32(len(payload)))
			_, _ = rw.Write(hdr[:])
			_, _ = rw.Write(payload)
			_ = rw.Flush()
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/exec"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"Id": "exec-cat"})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/exec/") && strings.HasSuffix(r.URL.Path, "/json"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"ExitCode": 0})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	dockerCli, err := client.New(client.WithHost("tcp://" + srv.Listener.Addr().String()))
	if err != nil {
		t.Fatalf("create docker client: %v", err)
	}
	defer dockerCli.Close()
	cli := &Client{docker: dockerCli}

	content, err := cli.ReadContainerFileAuto(ctx, "cid-sym", "/etc/os-release")
	if err != nil {
		t.Fatalf("ReadContainerFileAuto symlink failed: %v", err)
	}
	if content != "NAME=\"Alpine Linux\"\n" {
		t.Fatalf("expected symlink fallback cat content, got %q", content)
	}
}
