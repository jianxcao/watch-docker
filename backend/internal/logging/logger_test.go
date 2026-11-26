package logger

import (
	"testing"

	"go.uber.org/zap/zapcore"
)

func TestLogLevelParsing(t *testing.T) {
	origLogger := Logger
	origAtomicLevel := atomicLevel
	origLevelValue := atomicLevel.Level()
	t.Cleanup(func() {
		origAtomicLevel.SetLevel(origLevelValue)
		atomicLevel = origAtomicLevel
		Logger = origLogger
	})

	cases := []struct {
		input       string
		wantZap     zapcore.Level
		wantString  string
		wantErr     bool
		newFallback string
	}{
		{input: "debug", wantZap: zapcore.DebugLevel, wantString: "debug"},
		{input: "info", wantZap: zapcore.InfoLevel, wantString: "info"},
		{input: "warn", wantZap: zapcore.WarnLevel, wantString: "warn"},
		{input: "warning", wantZap: zapcore.WarnLevel, wantString: "warn"},
		{input: "error", wantZap: zapcore.ErrorLevel, wantString: "error"},
		{input: "dpanic", wantZap: zapcore.DPanicLevel, wantString: "dpanic"},
		{input: "panic", wantZap: zapcore.PanicLevel, wantString: "panic"},
		{input: "fatal", wantZap: zapcore.FatalLevel, wantString: "fatal"},
		{input: "  DPANIC  ", wantZap: zapcore.DPanicLevel, wantString: "dpanic"},
		{input: "  Warning ", wantZap: zapcore.WarnLevel, wantString: "warn"},
		{input: "", wantErr: true, newFallback: "info"},
		{input: "trace", wantErr: true, newFallback: "info"},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			gotZap, err := ParseLevel(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseLevel(%q) expected error, got %v", tc.input, gotZap)
				}
				if err := SetLogLevel(tc.input); err == nil {
					t.Fatalf("SetLogLevel(%q) expected error", tc.input)
				}
				if _, nerr := NewLogger(tc.input, ""); nerr != nil {
					t.Fatalf("NewLogger(%q) failed: %v", tc.input, nerr)
				}
				if GetLogLevel() != tc.newFallback {
					t.Fatalf("NewLogger(%q) threshold = %q, want %q", tc.input, GetLogLevel(), tc.newFallback)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseLevel(%q) unexpected err: %v", tc.input, err)
			}
			if gotZap != tc.wantZap {
				t.Fatalf("ParseLevel(%q) = %v, want %v", tc.input, gotZap, tc.wantZap)
			}

			// 验证 NewLogger 初始阈值（只读取阈值，不调用 Panic/Fatal 日志方法）
			if _, err := NewLogger(tc.input, ""); err != nil {
				t.Fatalf("NewLogger(%q) failed: %v", tc.input, err)
			}
			if got := GetLogLevel(); got != tc.wantString {
				t.Fatalf("NewLogger(%q) GetLogLevel() = %q, want %q", tc.input, got, tc.wantString)
			}

			// 先切到另一级别，再通过 SetLogLevel 验证动态切换阈值
			if tc.wantString != "debug" {
				if err := SetLogLevel("debug"); err != nil {
					t.Fatalf("reset SetLogLevel(debug) failed: %v", err)
				}
			} else {
				if err := SetLogLevel("info"); err != nil {
					t.Fatalf("reset SetLogLevel(info) failed: %v", err)
				}
			}
			if err := SetLogLevel(tc.input); err != nil {
				t.Fatalf("SetLogLevel(%q) failed: %v", tc.input, err)
			}
			if got := GetLogLevel(); got != tc.wantString {
				t.Fatalf("SetLogLevel(%q) GetLogLevel() = %q, want %q", tc.input, got, tc.wantString)
			}
		})
	}
}
