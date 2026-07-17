package log_test

import (
	"testing"

	kitlog "github.com/dreamsxin/go-kit/log"
)

func TestNewDevelopment_ReturnsLogger(t *testing.T) {
	logger, err := kitlog.NewDevelopment()
	if err != nil {
		t.Fatalf("NewDevelopment: %v", err)
	}
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
	logger.Sugar().Info("test log message")
}

func TestNew_UsesLevelAndFormat(t *testing.T) {
	logger, err := kitlog.New("debug", "console")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestNew_RejectsInvalidConfig(t *testing.T) {
	if _, err := kitlog.New("info", "xml"); err == nil {
		t.Fatal("expected invalid format error")
	}
	if _, err := kitlog.New("verbose", "json"); err == nil {
		t.Fatal("expected invalid level error")
	}
}

func TestNewNopLogger_DoesNotPanic(t *testing.T) {
	logger := kitlog.NewNopLogger()
	if logger == nil {
		t.Fatal("expected non-nil nop logger")
	}
	// should not panic or produce output
	logger.Sugar().Info("this should be discarded")
	logger.Sugar().Error("this too")
	logger.Sugar().Warn("and this")
}

func TestNewNopLogger_IsNop(t *testing.T) {
	logger := kitlog.NewNopLogger()
	// zap.NewNop() returns a logger that is enabled for no levels
	if logger.Core().Enabled(0) {
		t.Error("nop logger should not be enabled for any level")
	}
}
