package mail

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPreview(t *testing.T) {
	dir := os.Getenv("MAIL_PREVIEW_DIR")
	if dir == "" {
		t.Skip("set MAIL_PREVIEW_DIR to write sign_in.html")
	}
	m := SignInMessage("preview@example.com", "https://relayforge.site/auth/email?token=rfm_example…")
	if err := os.WriteFile(filepath.Join(dir, "sign_in.html"), []byte(m.HTML), 0o644); err != nil {
		t.Fatal(err)
	}
}
