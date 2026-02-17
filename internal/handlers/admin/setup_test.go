package admin

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	tmsconfig "github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
)

func TestMain(m *testing.M) {
	logutils.InitLogger("error")

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("runtime.Caller failed")
	}
	projectRoot := filepath.Join(filepath.Dir(file), "..", "..", "..")
	localesPath := filepath.Join(projectRoot, "locales")

	cfg := &tmsconfig.Config{
		Lang:     "en",
		LangPath: localesPath,
	}
	_ = lang.InitLocalizer(cfg)

	os.Exit(m.Run())
}
